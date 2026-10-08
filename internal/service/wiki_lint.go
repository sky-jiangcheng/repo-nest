package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"repo-nest/internal/db"
)

// Wiki lint (ADR-0014 决策 5 / TODO M6-W4) — the anti-corruption layer.
//
// The ADR calls lint mandatory rather than optional for one reason: an LLM can
// compile a store into confident-looking prose, and nothing about reading it later
// reveals that it drifted. Lint is what keeps a derived layer honest.
//
// It is split by what actually needs a model. Three of the five checks are
// decidable in SQL — orphan pages, missing cross-references, and data gaps (thin
// pages, pages with no source note, dangling body links, nothing ever
// synthesized). Two are not — contradictions and stale claims — and those cost
// money and can be wrong, so they sit behind their own explicit gate while the
// structural half runs on demand and on a timer.
//
// The rule that makes this safe to leave on: **a lint finding never mutates the
// wiki.** Every finding, mechanical or LLM-suggested, becomes a todo on the
// project that owns the page. An LLM judging other LLM output will sometimes be
// wrong, and a wrong suggestion costs the user one glance at a checkbox; a wrong
// "fix" that rewrites a page corrupts the store one layer deeper and hides the
// corruption. TestWikiLint_NeverMutatesPages is the contract test for that
// sentence.

const (
	// wikiLintLLMKey gates ONLY the model-backed checks.
	wikiLintLLMKey = "wiki_lint_llm"
	// wikiLintInterval: lint is maintenance, not a live feature, so the scheduled
	// pass is deliberately slow.
	wikiLintInterval = 6 * time.Hour
	// lintTodoPrefix marks a todo as machine-generated, for the dedupe query and
	// for a human scanning the list.
	lintTodoPrefix = "[lint]"
	// lintMaxLLMPages bounds the model pass by cost and context, not correctness.
	lintMaxLLMPages = 24
	// lintExcerptBytes is how much of each page the model sees.
	lintExcerptBytes = 500
	// lintMinRefTitleLen keeps the missing-cross-reference heuristic from
	// suggesting links for two-word titles that appear everywhere.
	lintMinRefTitleLen = 6
	// lintMaxPairScan bounds the O(n^2) scan on a large store.
	lintMaxPairScan = 200
	// lintThinPageBytes: below this a page is a stub, not knowledge.
	lintThinPageBytes = 80
)

// WikiFinding is one lint result. ProjectID may be 0 for a global page, in which
// case the finding is reported but cannot become a todo (todos require a project).
type WikiFinding struct {
	Check     string `json:"check"`
	Severity  string `json:"severity"` // info | warn
	ProjectID int64  `json:"project_id"`
	PageID    int64  `json:"page_id"`
	Detail    string `json:"detail"`
}

// WikiLintReport is one pass, and is what the binding returns.
type WikiLintReport struct {
	ProjectID int64         `json:"project_id"`
	Pages     int           `json:"pages"`
	Findings  []WikiFinding `json:"findings"`
	// TodosCreated / TodosExisting separate "first run" from "idempotent re-run",
	// which is the difference between a maintenance tool and a todo spammer.
	TodosCreated  int  `json:"todos_created"`
	TodosExisting int  `json:"todos_existing"`
	TodosUnfiled  int  `json:"todos_unfiled"` // no owning project, or a write failure
	LLMRan        bool `json:"llm_ran"`
	// LLMNote says why the model half did not run. "Nothing to report" and
	// "the feature is off" must not look identical in the output.
	LLMNote   string `json:"llm_note,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
	CheckedAt string `json:"checked_at"`
}

// startWikiLintTicker wires the scheduled half of ADR-0014 决策 5 ("定时与手动触发
// 各一条路径"). Background-tracked, so Shutdown waits for it.
//
// The first tick lands a full interval later rather than at startup: filing todos
// into the user's project list as a side effect of opening the app would be an
// unasked-for write, and nothing about lint is urgent.
func (s *Service) startWikiLintTicker() {
	s.bgGo("wiki-lint", func(ctx context.Context) {
		t := time.NewTicker(wikiLintInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := s.RunWikiLint(0, false); err != nil {
					log.Printf("wiki lint (scheduled): %v", err)
				}
			}
		}
	})
}

// RunWikiLint lints projectID (0 = every page) and files findings as todos.
// wantLLM asks for the model-backed checks too; they still need their own config
// gate, so this is permission to consider, not an override.
func (s *Service) RunWikiLint(projectID int64, wantLLM bool) (*WikiLintReport, error) {
	start := time.Now()
	pages, err := db.ListWikiPages(s.db, "", projectID)
	if err != nil {
		return nil, err
	}
	report := &WikiLintReport{
		ProjectID: projectID,
		Pages:     len(pages),
		Findings:  []WikiFinding{},
		CheckedAt: start.UTC().Format(time.RFC3339),
	}
	// Structural half: pure SQL, no model, no cost, no page writes.
	report.Findings = append(report.Findings, s.lintLinkShape(pages)...)
	report.Findings = append(report.Findings, s.lintContent(pages)...)
	report.Findings = append(report.Findings, s.lintCoverage(pages)...)

	if wantLLM {
		findings, note := s.runWikiLintLLM(pages)
		report.LLMRan = note == ""
		report.LLMNote = note
		report.Findings = append(report.Findings, findings...)
	}

	// Findings arrive from several passes; a stable order keeps the todo dedupe
	// and any later diffing from depending on map iteration order.
	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		if a.PageID != b.PageID {
			return a.PageID < b.PageID
		}
		return a.Detail < b.Detail
	})

	s.fileFindings(report)
	report.ElapsedMS = time.Since(start).Milliseconds()
	return report, nil
}

// lintLinkShape: orphan pages, dangling body links, and titles mentioned without
// a link.
func (s *Service) lintLinkShape(pages []db.WikiPage) []WikiFinding {
	var out []WikiFinding
	for _, p := range pages {
		back, err := db.WikiEdgesTo(s.db, p.ID)
		if err != nil {
			continue
		}
		if len(back) == 0 {
			out = append(out, WikiFinding{
				Check: "orphan", Severity: "info", ProjectID: p.ProjectID, PageID: p.ID,
				Detail: fmt.Sprintf("页面 [[%s]] 没有任何反向链接，图里它是座孤岛", p.Slug),
			})
		}
		for _, m := range wikiLinkRe.FindAllStringSubmatch(p.Content, -1) {
			target := strings.TrimSpace(m[1])
			if _, ok := findPageBySlug(pages, target); ok {
				continue
			}
			out = append(out, WikiFinding{
				Check: "dangling-link", Severity: "warn", ProjectID: p.ProjectID, PageID: p.ID,
				Detail: fmt.Sprintf("[[%s]] 指向不存在的页（出现在 [[%s]] 正文里）", target, p.Slug),
			})
		}
	}
	return append(out, s.lintMissingRefs(pages)...)
}

// lintMissingRefs is a labelled heuristic: another page's title appears in a body
// with no link to it. It emits a suggestion for a human and nothing more — an
// automated "add the obvious link" can invent a relation that was never intended.
func (s *Service) lintMissingRefs(pages []db.WikiPage) []WikiFinding {
	scan := pages
	if len(scan) > lintMaxPairScan {
		scan = scan[:lintMaxPairScan]
	}
	// Edges are pre-fetched per page: asking inside the inner loop would turn this
	// O(n^2) scan into O(n^2) queries.
	linked := make(map[int64]map[string]bool, len(scan))
	for _, a := range scan {
		m := map[string]bool{}
		if edges, err := db.WikiEdgesFrom(s.db, a.ID); err == nil {
			for _, e := range edges {
				m[e.Slug] = true
			}
		}
		linked[a.ID] = m
	}
	var out []WikiFinding
	for _, a := range scan {
		hay := strings.ToLower(a.Content)
		for _, b := range pages {
			if b.ID == a.ID {
				continue
			}
			title := strings.TrimSpace(b.Title)
			if len([]rune(title)) < lintMinRefTitleLen {
				continue
			}
			if !strings.Contains(hay, strings.ToLower(title)) {
				continue
			}
			if linked[a.ID][b.Slug] {
				continue // already linked; the body merely repeats the title
			}
			out = append(out, WikiFinding{
				Check: "missing-ref", Severity: "info", ProjectID: a.ProjectID, PageID: a.ID,
				Detail: fmt.Sprintf("[[%s]] 正文提到了「%s」却没有指向 [[%s]]，考虑补一条链接", a.Slug, title, b.Slug),
			})
		}
	}
	return out
}

// lintContent reports data gaps inside a single page.
func (s *Service) lintContent(pages []db.WikiPage) []WikiFinding {
	var out []WikiFinding
	for _, p := range pages {
		body := strings.TrimSpace(p.Content)
		if len(body) < lintThinPageBytes {
			out = append(out, WikiFinding{
				Check: "thin-page", Severity: "info", ProjectID: p.ProjectID, PageID: p.ID,
				Detail: fmt.Sprintf("页面 [[%s]] 只有 %d 字节，还不构成可依赖的知识", p.Slug, len(body)),
			})
		}
		notes, err := db.NotesForPage(s.db, p.ID)
		if err == nil && len(notes) == 0 {
			// Provenance is the one thing a derived layer cannot afford to lack: a
			// page with no source note cannot be checked against anything once it
			// turns out wrong.
			out = append(out, WikiFinding{
				Check: "no-source", Severity: "warn", ProjectID: p.ProjectID, PageID: p.ID,
				Detail: fmt.Sprintf("页面 [[%s]] 没有挂任何来源笔记，无法追溯它凭什么这么写", p.Slug),
			})
		}
	}
	return out
}

// lintCoverage reports a gap in the shape of the store rather than in one page:
// raw pages with nothing ever synthesized is the "compiled knowledge" claim
// failing to materialize.
func (s *Service) lintCoverage(pages []db.WikiPage) []WikiFinding {
	if len(pages) == 0 {
		return nil
	}
	counts, err := db.WikiPageCount(s.db)
	if err != nil {
		return nil
	}
	leafish := counts[db.WikiKindEntity] + counts[db.WikiKindConcept] + counts[db.WikiKindSource]
	if leafish >= 5 && counts[db.WikiKindSynthesis] == 0 {
		return []WikiFinding{{
			Check: "no-synthesis", Severity: "info", ProjectID: pages[0].ProjectID,
			Detail: fmt.Sprintf("已有 %d 张实体/概念/来源页，却没有任何综述页：知识仍是散片，未合成观点", leafish),
		}}
	}
	return nil
}

// runWikiLintLLM is the model-backed half: contradictions and stale claims. Gated
// on its own key plus a configured chat endpoint. It returns findings only and has
// no code path to page writes.
func (s *Service) runWikiLintLLM(pages []db.WikiPage) ([]WikiFinding, string) {
	if len(pages) < 2 {
		return nil, "少于 2 张页面，矛盾/过时检查没有意义"
	}
	if !s.wikiLintLLMEnabled() {
		return nil, "未开启（wiki_lint_llm 需显式设为 1）：跳过矛盾与过时检查"
	}
	cfg, err := s.aiChatConfig()
	if err != nil {
		return nil, "AI 端点未配置：跳过矛盾与过时检查"
	}
	list := pages
	if len(list) > lintMaxLLMPages {
		list = list[:lintMaxLLMPages]
	}

	arr := make([]map[string]any, 0, len(list))
	for _, p := range list {
		arr = append(arr, map[string]any{
			"page_id": p.ID, "slug": p.Slug, "kind": p.Kind, "title": p.Title,
			"updated": p.UpdatedAt, "excerpt": truncateBytes(p.Content, lintExcerptBytes),
		})
	}
	encoded, err := json.Marshal(arr)
	if err != nil {
		return nil, "页面序列化失败：跳过 LLM 检查"
	}
	var ctx strings.Builder
	ctx.WriteString("以下是若干互相链接的知识页（JSON 数组，含各自更新时间）。找出两类问题：\n" +
		"1. contradiction：两页对同一事实陈述冲突；\n" +
		"2. stale：某页断言被更新时间更晚的页覆盖或已过时。\n" +
		`只输出 JSON 数组，元素形如 {"type":"contradiction|stale","page_id":<id>,"other_id":<id或null>,"detail":"<一句话>"}。` +
		"没有发现就输出 []。不要提改写方案，不要输出 JSON 以外的字符。\n\n")
	ctx.Write(encoded)

	// Page content is untrusted data and the model reply is untrusted data too: an
	// instruction buried in a page cannot become an action here, because this path
	// can only produce findings.
	reply, err := TestAIChatWithTimeout(cfg.baseURL, cfg.model, cfg.apiKey, []chatMessage{
		{Role: "system", Content: "你只输出 JSON，只做只读分析，不要求改动任何内容。"},
		{Role: "user", Content: ctx.String()},
	}, DefaultBatchChatTimeout)
	if err != nil {
		return nil, fmt.Sprintf("LLM 调用失败，跳过矛盾与过时检查（%v）", err)
	}
	items, ok := parseLintReply(reply.Reply)
	if !ok {
		// An unparseable reply yields no findings rather than a best-effort guess:
		// a hallucinated "contradiction" todo is noise the user clears by hand.
		//
		// Truncation is reported separately because the remedy differs: raise the
		// output budget or switch model, rather than re-running the same call.
		if hint := truncatedHint(reply.FinishReason); hint != "" {
			return nil, "LLM 回复被截断，未能解析为 JSON：本次不采信任何建议" + hint
		}
		return nil, "LLM 回复无法解析为 JSON：本次不采信任何建议"
	}
	byID := make(map[int64]db.WikiPage, len(pages))
	for _, p := range pages {
		byID[p.ID] = p
	}
	var out []WikiFinding
	for _, it := range items {
		p, ok := byID[it.PageID]
		if !ok {
			continue // the model named a page that does not exist; never address it
		}
		check := "stale"
		if it.Type == "contradiction" {
			check = "contradiction"
		}
		other := ""
		if o, ok := byID[it.OtherID]; ok && it.OtherID != it.PageID {
			other = fmt.Sprintf("，涉及 [[%s]]", o.Slug)
		}
		out = append(out, WikiFinding{
			Check: check, Severity: "warn", ProjectID: p.ProjectID, PageID: p.ID,
			Detail: fmt.Sprintf("（模型建议，需人工判断）[[%s]] %s%s", p.Slug,
				clipRunes(strings.TrimSpace(it.Detail), 200), other),
		})
	}
	return out, ""
}

type lintReplyItem struct {
	Type    string `json:"type"`
	PageID  int64  `json:"page_id"`
	OtherID int64  `json:"other_id"`
	Detail  string `json:"detail"`
}

// parseLintReply tolerates the fenced/prose shapes models emit, but refuses to
// guess at structure: an unparseable reply means no findings at all.
func parseLintReply(raw string) ([]lintReplyItem, bool) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "]"); j >= 0 && j+1 < len(s) {
		s = s[:j+1]
	}
	var items []lintReplyItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, false
	}
	return items, true
}

// fileFindings turns findings into todos on the owning project, deduplicated
// against that project's OPEN todos. A closed todo may come back: if a page
// drifts again after being ticked off, that is new information, not spam.
func (s *Service) fileFindings(report *WikiLintReport) {
	seen := map[string]bool{}
	for _, f := range report.Findings {
		title := lintTodoTitle(f)
		key := fmt.Sprintf("%d|%s", f.ProjectID, title)
		if seen[key] {
			report.TodosExisting++
			continue
		}
		seen[key] = true
		if f.ProjectID == 0 {
			// A global page has no project to own the todo. Surfacing it in the
			// report is honest; inventing a home for it is not.
			report.TodosUnfiled++
			continue
		}
		open, err := db.HasTodoWithTitle(s.db, f.ProjectID, title)
		if err != nil {
			log.Printf("wiki lint: dedupe check failed for project %d: %v", f.ProjectID, err)
		}
		if open {
			report.TodosExisting++
			continue
		}
		if _, err := db.CreateTodo(s.db, f.ProjectID, title); err != nil {
			log.Printf("wiki lint: could not file todo %q: %v", title, err)
			report.TodosUnfiled++
			continue
		}
		report.TodosCreated++
	}
}

func lintTodoTitle(f WikiFinding) string {
	return clipRunes(fmt.Sprintf("%s %s: %s", lintTodoPrefix, f.Check, f.Detail), 180)
}

func findPageBySlug(pages []db.WikiPage, slug string) (db.WikiPage, bool) {
	for _, p := range pages {
		if p.Slug == slug {
			return p, true
		}
	}
	return db.WikiPage{}, false
}
