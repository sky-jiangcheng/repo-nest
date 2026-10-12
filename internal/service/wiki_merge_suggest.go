package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"repo-nest/internal/db"
)

// Merge suggestion — the model's second judgment (ADR-0019 决策 4).
//
// Layer 1 (wiki_merge.go) ranked candidate pairs with structure alone. This is
// layer 2's human-facing half: for each pair that cleared the threshold, ask a
// model once whether the two pages are the same concept and which one should
// survive. The output is a `[lint] merge` todo and nothing else.
//
// The safety model is the lint discipline, unchanged (决策 4):
//
//   - the model cannot merge. It cannot edit a page. It produces a string a
//     human reads; db.MergeWikiPages has exactly one caller, the review UI's
//     adopt button (决策 1);
//   - the reply is untrusted data. Non-JSON means no suggestions at all, and a
//     single fabricated winner/loser slug voids the WHOLE reply — a model that
//     invents a page is a model whose output cannot be bounded, so nothing it
//     said is adopted (晋升条件 4, the lint "must resolve in the store" rule
//     applied at reply granularity);
//   - its own key, wiki_merge_suggest, gates it independently of
//     wiki_lint_llm: authorizing "judge merge candidates" must not imply
//     authorizing "read every page for contradictions";
//   - a per-round pair budget, because the failure mode of this feature is
//     spending money, not being wrong.
//
// It runs inside the lint model half (待决 3): the same job kernel, the same
// budget discipline, the same findings-to-todos path — no new job kind.

const (
	// wikiMergeSuggestKey gates ONLY the merge-candidate second judgment.
	wikiMergeSuggestKey = "wiki_merge_suggest"
	// mergeSuggestMaxPairs bounds one round of second judgment (决策 4's
	// "每轮 pair 预算封顶如 ≤200 对"). Pairs beyond it are simply not asked
	// about this round; the structural candidates stay visible to a human.
	mergeSuggestMaxPairs = 200
	// mergeSuggestExcerptBytes is how much of each page the model sees — the
	// same budget as the lint model pass.
	mergeSuggestExcerptBytes = 500
)

// wikiMergeSuggestEnabled is the explicit opt-in. Off by default: this spends
// money on every run, and its only output is advice.
func (s *Service) wikiMergeSuggestEnabled() bool {
	v, err := db.GetConfig(s.db, wikiMergeSuggestKey)
	return err == nil && v == "1"
}

// runWikiMergeSuggest asks the model about the candidate pairs that cleared
// 决策 3's threshold. It returns findings only — there is no code path from
// here to a page write. The second return value says why the pass did not run;
// "" means it ran (and may still have found nothing).
func (s *Service) runWikiMergeSuggest(projectID int64, pages []db.WikiPage) ([]WikiFinding, string) {
	if len(pages) < 2 {
		return nil, "少于 2 张页面，没有可判定的候选对"
	}
	if !s.wikiMergeSuggestEnabled() {
		return nil, "未开启（wiki_merge_suggest 需显式设为 1）：跳过合并候选二判"
	}
	cfg, err := s.aiChatConfig()
	if err != nil {
		return nil, "AI 端点未配置：跳过合并候选二判"
	}
	scan, err := s.DiscoverMergeCandidates(projectID)
	if err != nil {
		return nil, fmt.Sprintf("候选发现失败：跳过合并候选二判（%v）", err)
	}
	pairs := MergeCandidatesAboveThreshold(scan.Candidates)
	if len(pairs) == 0 {
		return nil, "没有达到判定阈值的候选对"
	}
	if len(pairs) > mergeSuggestMaxPairs {
		pairs = pairs[:mergeSuggestMaxPairs]
	}

	arr := make([]map[string]any, 0, len(pairs))
	for _, c := range pairs {
		arr = append(arr, map[string]any{
			"a": map[string]any{
				"slug": c.A.Slug, "title": c.A.Title, "kind": c.A.Kind,
				"updated": c.A.UpdatedAt, "excerpt": truncateBytes(c.A.Content, mergeSuggestExcerptBytes),
			},
			"b": map[string]any{
				"slug": c.B.Slug, "title": c.B.Title, "kind": c.B.Kind,
				"updated": c.B.UpdatedAt, "excerpt": truncateBytes(c.B.Content, mergeSuggestExcerptBytes),
			},
			"signals": c.Signals,
		})
	}
	encoded, err := json.Marshal(arr)
	if err != nil {
		return nil, "候选对序列化失败：跳过合并候选二判"
	}
	var ctx strings.Builder
	ctx.WriteString("以下是知识库里结构信号判定为「疑似同一概念」的候选页对（JSON 数组）。逐对判断：\n" +
		"1. same：这两页是否在讲同一个概念（标题相近但内容不同的不算）；\n" +
		"2. 若 same 为 true，选出 winner（建议保留的页 slug：来源更多、创建更早、被链接更多的一方通常更适合）与 loser（另一页），并用一句话说明理由。\n" +
		`只输出 JSON 数组，元素形如 {"a":"<slug>","b":"<slug>","same":true或false,"winner":"<same为true时的slug，否则空串>","reason":"<一句话>"}。` +
		"slug 必须原样取自输入，不要发明新 slug；每一对都必须出现在输出里。不要提改写方案，不要输出 JSON 以外的字符。\n\n")
	ctx.Write(encoded)

	reply, err := TestAIChatWithTimeout(cfg.baseURL, cfg.model, cfg.apiKey, []chatMessage{
		{Role: "system", Content: "你只输出 JSON，只做只读分析，不要求改动、合并或删除任何内容。"},
		{Role: "user", Content: ctx.String()},
	}, DefaultBatchChatTimeout)
	if err != nil {
		return nil, fmt.Sprintf("LLM 调用失败，跳过合并候选二判（%v）", err)
	}
	items, ok := parseMergeSuggestReply(reply.Reply)
	if !ok {
		if hint := truncatedHint(reply.FinishReason); hint != "" {
			return nil, "LLM 回复被截断，未能解析为 JSON：本次不采信任何建议" + hint
		}
		return nil, "LLM 回复无法解析为 JSON：本次不采信任何建议"
	}
	bySlug := make(map[string]db.WikiPage, len(pages))
	for _, p := range pages {
		bySlug[p.Slug] = p
	}
	// The pairs actually asked about, so a judgment about a pair we never sent
	// is as untrusted as a slug that does not exist.
	asked := make(map[[2]int64]bool, len(pairs))
	for _, c := range pairs {
		a, b := c.A.ID, c.B.ID
		if a > b {
			a, b = b, a
		}
		asked[[2]int64{a, b}] = true
	}

	var out []WikiFinding
	for _, it := range items {
		if !it.Same {
			continue // the model's "different concept" is agreement with the status quo
		}
		a, okA := bySlug[strings.TrimSpace(it.A)]
		b, okB := bySlug[strings.TrimSpace(it.B)]
		winner, okW := bySlug[strings.TrimSpace(it.Winner)]
		if !okA || !okB || !okW {
			return nil, "LLM 回复里出现了库里查不到的 slug：本次不采信任何建议"
		}
		if winner.ID != a.ID && winner.ID != b.ID {
			return nil, "LLM 回复的 winner 不在它判定的页对里：本次不采信任何建议"
		}
		loser := a
		if winner.ID == a.ID {
			loser = b
		}
		key := [2]int64{a.ID, b.ID}
		if a.ID > b.ID {
			key = [2]int64{b.ID, a.ID}
		}
		if !asked[key] {
			return nil, "LLM 回复判定了未提交给它的页对：本次不采信任何建议"
		}
		out = append(out, WikiFinding{
			Check: "merge", Severity: "info",
			ProjectID: winner.ProjectID, PageID: loser.ID,
			// The suggestion names BOTH directions on purpose (待决 1): the
			// model proposes, the review UI lets a human flip winner/loser,
			// and the click is what decides.
			Detail: fmt.Sprintf("（模型建议，需人工判断）[[%s]] → [[%s]]：%s",
				loser.Slug, winner.Slug, clipRunes(strings.TrimSpace(it.Reason), 200)),
		})
	}
	return out, ""
}

type mergeSuggestItem struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Same   bool   `json:"same"`
	Winner string `json:"winner"`
	Reason string `json:"reason"`
}

// parseMergeSuggestReply tolerates the fenced/prose shapes models emit, but
// refuses to guess at structure — the same contract as parseLintReply, for the
// same reason: a best-effort subset of a malformed reply is how a fabricated
// slug becomes a todo.
func parseMergeSuggestReply(raw string) ([]mergeSuggestItem, bool) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "]"); j >= 0 && j+1 < len(s) {
		s = s[:j+1]
	}
	var items []mergeSuggestItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, false
	}
	return items, true
}
