package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"repotory/internal/db"
	"repotory/internal/graph"
)

// Community synthesis (ADR-0020) — filling the fifth page kind.
//
// ADR-0014 决策 1 defined five kinds, and generation only ever covered four: the
// compiler works one note at a time, and a synthesis is by definition a claim
// across sources, which a single-note view cannot produce. The consequence was a
// dead loop — W4's `no-synthesis` check (five or more leaf pages, zero syntheses)
// reported a gap that nothing in the system could close, forever. This is the step
// that closes it: detect topics by walking the graph, ask the model for one
// paragraph per topic, and file the result as a pending page for a human.
//
// This is the second place the page graph is read for something other than linting
// itself, and it is only expressible because ADR-0018 lane 1 made `relation` a
// controlled vocabulary: "propagate along the edges that mean 'same topic'" is not
// a question you can ask of free text.
//
// Deliberate smallness, inherited from ADR-0015 and made stricter here:
//
//   - the model has LESS power than the compiler. It outputs a paragraph of plain
//     text and nothing else — no kind, no slug, no links. Pages, edges and note
//     attachments are all built by code (决策 6). A model that cannot name a page
//     cannot invent one.
//   - `contradicts` is excluded from propagation (对立不是同主题), and so is
//     `mentions` (顺带提一句不代表同一社区). Including either would merge topics that
//     are in deliberate disagreement.
//   - never scheduled (决策 9). Lint may run on a timer because it is read-free-of
//     -cost and writes only todos; this spends tokens and writes pages, so it runs
//     when a person clicks. No MCP tool either — an agent that could pour summaries
//     into the library would be the human gate, bypassed.
//   - synchronous with hard budgets rather than job-ified (决策 8, 待决 4 taken as
//     recommended). Job-ifying needs a new value in compile_jobs' kind CHECK, which
//     in SQLite means rebuilding the table; paying that for a manual, bounded action
//     is backwards. If a real library ever needs ten minutes, that rebuild happens
//     once, alongside whatever else requires it.

const (
	// wikiCommunityKey is this pass's own consent, off by default like every other
	// path that sends page content to an endpoint.
	wikiCommunityKey = "wiki_community_summary"
	// communityMinSizeKey / communityMaxSizeKey are 待决 5's two thresholds,
	// overridable as numeric config keys.
	communityMinSizeKey = "community_min_size"
	communityMaxSizeKey = "community_max_size"
	// communitySourceTag separates these pages from the compiler's output in the
	// review UI, in lint, and in the "may an automated path touch this?" question
	// (待决 3).
	communitySourceTag = "wiki-community"

	// communityMaxPerRun: 决策 8's "单次上限 ≤10 个社区".
	communityMaxPerRun = 10
	// communityMaxMembers: 决策 8's "每社区 ≤24 页".
	communityMaxMembers = 24
	// communityExcerptBytes is how much of each member page the model sees. At the
	// 24-page ceiling that is ~21 KB of context, which is the real limit on raising
	// this number — not correctness.
	communityExcerptBytes = 900
	// communityRevisionCheck is the lintTodoTitle Check name reused when the slug is
	// already held by a page this pass may not touch (决策 7).
	communityRevisionCheck = "synthesis-revision"
)

// communityRelations is 决策 3's propagation whitelist. ref / part-of / depends /
// implements / documents all say "these pages concern one thing". contradicts and
// mentions say something else entirely, and supersedes is a statement about time
// rather than about topic — all three are excluded on purpose.
var communityRelations = map[string]bool{
	db.RelationRef:        true,
	db.RelationPartOf:     true,
	db.RelationDepends:    true,
	db.RelationImplements: true,
	db.RelationDocuments:  true,
}

// WikiCommunityReport is one run. The structural stats are not decoration: "no
// communities found" has three different causes (the graph is genuinely sparse,
// everything collapsed into one oversized blob, or nothing is approved yet), and a
// report that cannot tell them apart would send a user to re-approve pages for
// nothing.
type WikiCommunityReport struct {
	ProjectID     int `json:"project_id"`
	ApprovedPages int `json:"approved_pages"`
	Edges         int `json:"edges"`

	// Structural half: free, no model, no writes.
	CommunitiesFound int  `json:"communities_found"`
	DroppedSmall     int  `json:"dropped_small"`
	DroppedLarge     int  `json:"dropped_large"`
	Isolated         int  `json:"isolated"`
	Rounds           int  `json:"rounds"`
	Converged        bool `json:"converged"`

	// Generation half.
	PagesCreated     int      `json:"pages_created"`
	PagesUpdated     int      `json:"pages_updated"`
	DocumentsLinked  int      `json:"documents_linked"`
	NotesAttached    int      `json:"notes_attached"`
	RevisionTodos    int      `json:"revision_todos"`
	EmptyBodies      int      `json:"empty_bodies"`
	SummarizedSlugs  []string `json:"summarized_slugs"`
	SkippedByBudget  []string `json:"skipped_by_budget"`
	OutboundBytes    int      `json:"outbound_bytes"` // 晋升条件 3: an enabled run says what it sent
	Calls            int      `json:"calls"`
	CommunityGateOff bool     `json:"community_gate_off"`
	LLMNote          string   `json:"llm_note,omitempty"`
	ElapsedMS        int64    `json:"elapsed_ms"`
	RunAt            string   `json:"run_at"`
}

// RunCommunitySummary detects topics among projectID's APPROVED pages and files one
// pending synthesis page per topic.
//
// projectID 0 means the GLOBAL tier (pages belonging to no project), NOT "every
// project": a community spanning two projects would summarise something neither
// user asked about, and "all projects" has no representation in the schema because
// project_id is a real foreign key. The review surface therefore requires an
// explicit project to run against — the same rule StartCompile enforces.
func (s *Service) RunCommunitySummary(projectID int64) (*WikiCommunityReport, error) {
	start := time.Now()
	report := &WikiCommunityReport{
		ProjectID:       int(projectID),
		SummarizedSlugs: []string{},
		SkippedByBudget: []string{},
		RunAt:           start.UTC().Format(time.RFC3339),
	}

	minSize, maxSize := s.communitySizeThresholds()

	pages, err := s.communityInputPages(projectID)
	if err != nil {
		return nil, err
	}
	report.ApprovedPages = len(pages)
	if len(pages) < minSize {
		report.LLMNote = fmt.Sprintf("可参与检测的已批准页面不足 %d 张（community_min_size），没有主题可归纳", minSize)
		report.ElapsedMS = time.Since(start).Milliseconds()
		return report, nil
	}

	links, err := db.ListPageLinks(s.db)
	if err != nil {
		return nil, err
	}
	inScope := make(map[string]bool, len(pages))
	bySlug := make(map[string]db.WikiPage, len(pages))
	nodes := make([]string, 0, len(pages))
	for _, p := range pages {
		inScope[p.Slug] = true
		bySlug[p.Slug] = p
		nodes = append(nodes, p.Slug)
	}
	// Sorted so input order cannot leak into the result. LPA is order-independent by
	// construction; this keeps that claim checkable rather than accidental.
	sort.Strings(nodes)

	edges, edgesInScope := communityEdges(links, inScope)
	report.Edges = edgesInScope

	communities, stats := graph.LabelPropagation(nodes, edges, graph.Options{
		MinSize: minSize,
		MaxSize: maxSize,
	})
	report.CommunitiesFound = len(communities)
	report.DroppedSmall = stats.DroppedSmall
	report.DroppedLarge = stats.DroppedLarge
	report.Isolated = stats.Isolated
	report.Rounds = stats.Rounds
	report.Converged = stats.Converged
	if len(communities) == 0 {
		report.LLMNote = communityEmptyNote(stats)
		report.ElapsedMS = time.Since(start).Milliseconds()
		return report, nil
	}

	// ---- the consent gate sits AFTER the free half -------------------------
	//
	// Detection is pure structure: it costs nothing and writes nothing, so a report
	// can tell the user "there are 4 topics worth summarising" before they enable
	// anything. Everything below needs permission, and the whole of it is skipped
	// when the gate is off — so "off means zero outbound requests and zero page
	// writes" is a property of the control flow rather than of a careful prompt.
	if !s.wikiCommunityEnabled() {
		report.CommunityGateOff = true
		report.LLMNote = fmt.Sprintf("未开启（%s 需显式设为 1）：已完成结构检测（%d 个社区），未生成任何页面",
			wikiCommunityKey, len(communities))
		report.ElapsedMS = time.Since(start).Milliseconds()
		return report, nil
	}
	cfg, err := s.aiChatConfig()
	if err != nil {
		report.LLMNote = "AI 端点未配置：无法生成综述正文，本次未生成任何页面"
		report.ElapsedMS = time.Since(start).Milliseconds()
		return report, nil
	}

	kept := communities
	if len(kept) > communityMaxPerRun {
		report.SkippedByBudget = append(report.SkippedByBudget,
			fmt.Sprintf("%d 个社区超出单次上限 %d，按规模顺序留到下次",
				len(kept)-communityMaxPerRun, communityMaxPerRun))
		kept = kept[:communityMaxPerRun]
	}

	for _, c := range kept {
		if time.Since(start) > communityTimeBudget() {
			report.SkippedByBudget = append(report.SkippedByBudget,
				fmt.Sprintf("墙钟预算（%s）耗尽，剩余社区留到下次", communityTimeBudget()))
			break
		}
		if err := s.summarizeCommunity(cfg, c, bySlug, report); err != nil {
			// One community failing must not abandon the rest: the failure is
			// recorded and the loop continues, because the user pays per call and a
			// partial result beats a rolled-back run.
			report.LLMNote = strings.TrimSpace(report.LLMNote + " " + err.Error())
			continue
		}
	}

	report.ElapsedMS = time.Since(start).Milliseconds()
	return report, nil
}

// communityEdges filters the page graph down to the whitelist, keeps only edges
// with BOTH ends in scope, and deduplicates directions (LP is undirected, and a
// ref edge recorded both ways must not count twice in a vote).
func communityEdges(links []db.PageLinkRow, inScope map[string]bool) ([]graph.Edge, int) {
	var out []graph.Edge
	seen := map[[2]string]bool{}
	for _, l := range links {
		if !communityRelations[l.Relation] {
			continue
		}
		if !inScope[l.FromSlug] || !inScope[l.ToSlug] || l.FromSlug == l.ToSlug {
			continue
		}
		key := [2]string{l.FromSlug, l.ToSlug}
		if key[0] > key[1] {
			key = [2]string{l.ToSlug, l.FromSlug}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, graph.Edge{A: key[0], B: key[1]})
	}
	return out, len(out)
}

// communityTimeBudget is the wall-clock ceiling for one manual run: the batch
// timeout, because cmd/server derives its write timeout from that value and asserts
// on the relationship (see ai.go). Passing it would trade a slow request for a
// truncated one and lose the report either way.
func communityTimeBudget() time.Duration {
	return DefaultBatchChatTimeout
}

// communityInputPages loads the approved pages for one scope.
//
// projectID 0 is handled explicitly rather than by falling through: zero is also
// ListWikiPagesByStatus's "no filter" sentinel, so an unfiltered read here would
// silently make the global tier mean "every page in every project".
//
// Synthesis pages are EXCLUDED from the input, and that is not a nitpick: a
// synthesis is joined to its members by `documents` edges, which are on the
// propagation whitelist. Letting an approved synthesis back into detection means
// run N's output enlarges run N+1's input — the member set grows by one each time,
// so the stable slug (决策 7) stops being stable, 决策 7's revision todo never
// fires, and a machine-written summary starts summarising itself. Detection must
// run over what the library believes, not over what this pass has drafted.
func (s *Service) communityInputPages(projectID int64) ([]db.WikiPage, error) {
	approved := func(pid int64) ([]db.WikiPage, error) {
		return db.ListWikiPagesByStatus(s.db, db.WikiStatusApproved, pid)
	}
	var (
		all []db.WikiPage
		err error
	)
	if projectID > 0 {
		all, err = approved(projectID)
	} else {
		if all, err = approved(0); err == nil {
			filtered := make([]db.WikiPage, 0, len(all))
			for _, p := range all {
				if p.ProjectID == 0 {
					filtered = append(filtered, p)
				}
			}
			all = filtered
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]db.WikiPage, 0, len(all))
	for _, p := range all {
		if p.Kind == db.WikiKindSynthesis {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// communityEmptyNote turns three different causes into three different sentences,
// because "0 communities" otherwise reads as "you have nothing to learn".
func communityEmptyNote(stats graph.RunStats) string {
	switch {
	case stats.DroppedLarge > 0:
		return fmt.Sprintf("图塌成了 %d 个超出上限的块（Label Propagation 的已知退化）："+
			"提高 community_max_size，或先补交叉引用把主题拆开再跑", stats.DroppedLarge)
	case stats.DroppedSmall > 0:
		return fmt.Sprintf("只有碎片：community_min_size 之下有 %d 组，尚不足以构成主题，可降低门槛或补充关系边",
			stats.DroppedSmall)
	case stats.Isolated == stats.Nodes && stats.Nodes > 0:
		return "页面之间没有任何可传播的关系边，检测不到主题"
	default:
		return "没有达到规模门槛的社区"
	}
}

// summarizeCommunity turns one detected topic into a pending synthesis page.
//
// Ordering is contractual: the page, its documents edges and its note attachments
// are created even when the model gave nothing usable (晋升条件 2's "非纯文本/空回复
// → 不写正文但页与 documents 边与来源挂接照常建"). A failed call therefore leaves a
// real, reviewable, empty page that says what it covers, rather than nothing —
// which is honest, and keeps the run idempotent whether or not the endpoint
// answered.
func (s *Service) summarizeCommunity(cfg aiChatConfig, c graph.Community,
	bySlug map[string]db.WikiPage, report *WikiCommunityReport) error {

	members := c.Members
	truncated := false
	if len(members) > communityMaxMembers {
		members = members[:communityMaxMembers]
		truncated = true
	}
	slug := CommunitySynthesisSlug(members)

	existing, err := db.GetWikiPageBySlug(s.db, slug)
	switch {
	case err == nil && existing.Status == db.WikiStatusApproved:
		// 决策 7: an approved synthesis is never rewritten by an automated path. It
		// becomes a revision todo — the same disposition W3's compiler uses when a
		// note wants to touch a page it does not own.
		s.fileCommunityRevisionTodo(existing, members, report)
		return nil
	case err == nil && existing.Status == db.WikiStatusPending && existing.Source != communitySourceTag:
		// Another generator's draft holds this slug. Claiming it would let two
		// writers overwrite each other, so: report and leave.
		s.fileCommunityRevisionTodo(existing, members, report)
		return nil
	case err == nil:
		// Our own pending page from a previous run: updated in place below.
	case errors.Is(err, sql.ErrNoRows):
		existing = nil
	default:
		return fmt.Errorf("community: look up synthesis page: %w", err)
	}

	body, askErr := s.askCommunitySummary(cfg, members, bySlug, report)
	if askErr != nil {
		body = ""
		report.LLMNote = strings.TrimSpace(report.LLMNote + " " + askErr.Error())
	}
	if body == "" {
		report.EmptyBodies++
	}

	page := existing
	if page != nil {
		if err := db.UpdateWikiPage(s.db, page.ID, page.Title, db.WikiKindSynthesis, body); err != nil {
			return fmt.Errorf("community: update synthesis page: %w", err)
		}
		report.PagesUpdated++
	} else {
		created, cerr := db.CreateWikiPageAs(s.db, db.WikiKindSynthesis, slug,
			CommunitySynthesisTitle(members), memberProjectID(members, bySlug), body,
			db.WikiStatusPending, communitySourceTag)
		if cerr != nil {
			if !errors.Is(cerr, db.ErrWikiSlugTaken) {
				return fmt.Errorf("community: create synthesis page: %w", cerr)
			}
			// Lost the race against an identical concurrent run: adopt its page and
			// keep going rather than failing the whole topic.
			page, err = db.GetWikiPageBySlug(s.db, slug)
			if err != nil {
				return fmt.Errorf("community: reread synthesis page: %w", err)
			}
		} else {
			page = created
		}
		report.PagesCreated++
	}
	report.SummarizedSlugs = append(report.SummarizedSlugs, page.Slug)
	if truncated {
		report.SkippedByBudget = append(report.SkippedByBudget,
			fmt.Sprintf("[[%s]] 成员超出 %d 页上限，正文只覆盖前 %d 页", page.Slug, communityMaxMembers, communityMaxMembers))
	}

	// 决策 5: the member -> synthesis edges are built by CODE. The model never got an
	// option to link anything, so it cannot invent a relationship through this path.
	for _, m := range members {
		mem, ok := bySlug[m]
		if !ok {
			continue
		}
		if err := db.LinkWikiPages(s.db, mem.ID, page.ID, db.RelationDocuments); err != nil {
			return fmt.Errorf("community: link [[%s]] -> [[%s]]: %w", m, page.Slug, err)
		}
		report.DocumentsLinked++
		// Provenance: the notes the members were built from feed the synthesis too,
		// so a reviewer can open the sources instead of trusting the paragraph.
		noteIDs, err := db.NotesForPage(s.db, mem.ID)
		if err != nil {
			return fmt.Errorf("community: read member sources: %w", err)
		}
		for _, nid := range noteIDs {
			if err := db.AttachNoteToPage(s.db, nid, page.ID); err != nil {
				return fmt.Errorf("community: attach note %d: %w", nid, err)
			}
			report.NotesAttached++
		}
	}
	return nil
}

// askCommunitySummary makes one model call per community and returns plain text.
// There is no JSON to parse: the reply IS the body, so the model has no vocabulary
// in which to express an action (决策 6).
func (s *Service) askCommunitySummary(cfg aiChatConfig, members []string,
	bySlug map[string]db.WikiPage, report *WikiCommunityReport) (string, error) {

	var ctx strings.Builder
	ctx.WriteString("以下是同一主题下的若干知识页，请写一段中文综述，说明这些页共同在讲什么、" +
		"它们之间的关系，以及值得单独立页的结论。\n")
	ctx.WriteString("只写正文，不要输出 JSON、标题层级或对读者的指令；不要编造页面里没有的结论。\n\n")
	for _, m := range members {
		p, ok := bySlug[m]
		if !ok {
			continue
		}
		fmt.Fprintf(&ctx, "### [[%s]]（%s：%s）\n%s\n\n",
			p.Slug, p.Kind, p.Title, truncateBytes(p.Content, communityExcerptBytes))
	}
	report.OutboundBytes += ctx.Len()
	report.Calls++

	reply, err := TestAIChatWithTimeout(cfg.baseURL, cfg.model, cfg.apiKey, []chatMessage{
		{Role: "system", Content: "你只输出综述正文，不要求改动任何内容。"},
		{Role: "user", Content: ctx.String()},
	}, DefaultBatchChatTimeout)
	if err != nil {
		return "", fmt.Errorf("community: LLM 调用失败（%v）", err)
	}
	body := strings.TrimSpace(reply.Reply)
	if body == "" {
		if hint := truncatedHint(reply.FinishReason); hint != "" {
			return "", fmt.Errorf("community: 模型回复为空且已被截断，页面建档但正文留断%s", hint)
		}
		return "", fmt.Errorf("community: 模型返回空正文，页面已建档但正文留空")
	}
	// The ceiling is the compiler's per-page budget, so a synthesis cannot quietly
	// outgrow the pages it summarizes.
	if len(body) > compileMaxPageBytes {
		body = truncateBytes(body, compileMaxPageBytes)
		report.SkippedByBudget = append(report.SkippedByBudget,
			fmt.Sprintf("某社区正文超出 %d 字节上限，已裁剪", compileMaxPageBytes))
	}
	return body, nil
}

// fileCommunityRevisionTodo is 决策 7's disposition. A global synthesis page has no
// project to own a todo, and todos require a project — so it is counted and left
// alone rather than given an invented home (the same rule lint's TodosUnfiled uses).
func (s *Service) fileCommunityRevisionTodo(existing *db.WikiPage, members []string,
	report *WikiCommunityReport) {

	report.RevisionTodos++
	if existing.ProjectID == 0 {
		return
	}
	title := lintTodoTitle(WikiFinding{
		Check: communityRevisionCheck,
		Detail: fmt.Sprintf("[[%s]] 的社区成员有变化（%d 页），自动路径无权修改已批准页面：%s",
			existing.Slug, len(members), clipRunes(strings.Join(members, ", "), 120)),
	})
	if open, err := db.HasTodoWithTitle(s.db, existing.ProjectID, title); err == nil && open {
		return
	}
	if _, err := db.CreateTodo(s.db, existing.ProjectID, title); err != nil {
		report.LLMNote = strings.TrimSpace(fmt.Sprintf("%s 待办写入失败：%v", report.LLMNote, err))
	}
}

// CommunitySynthesisSlug is 决策 7's stable identity: `synthesis-` plus the first
// eight hex characters of sha256 over the sorted member slug list. Same community,
// same slug forever — which is the only reason a re-run updates a page instead of
// filing a second one.
func CommunitySynthesisSlug(members []string) string {
	sorted := append([]string(nil), members...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return "synthesis-" + hex.EncodeToString(sum[:])[:8]
}

// CommunitySynthesisTitle names the page for a human, leading with the alphabetically
// first member so the title cannot drift between runs either.
func CommunitySynthesisTitle(members []string) string {
	sorted := append([]string(nil), members...)
	sort.Strings(sorted)
	switch len(sorted) {
	case 0:
		return "社区综述"
	case 1:
		return fmt.Sprintf("综述：%s", sorted[0])
	default:
		return fmt.Sprintf("综述：%s 等 %d 页", sorted[0], len(sorted))
	}
}

// memberProjectID is a synthesis page's owning project. Members are all in one
// scope by construction (the input query filters to a single project, and the global
// tier runs separately), so this is the first non-zero project, else global.
func memberProjectID(members []string, bySlug map[string]db.WikiPage) int64 {
	for _, m := range members {
		if p, ok := bySlug[m]; ok && p.ProjectID > 0 {
			return p.ProjectID
		}
	}
	return 0
}

// wikiCommunityEnabled is the explicit opt-in. A missing row, an empty value and
// garbage all mean OFF, for the same reason autoImportEnabled does: db.GetConfig
// maps "no row" to "", so comparing against "1" makes the safe state the default.
func (s *Service) wikiCommunityEnabled() bool {
	v, err := db.GetConfig(s.db, wikiCommunityKey)
	return err == nil && v == "1"
}

// communitySizeThresholds reads 待决 5's two overrides, falling back to the ADR
// defaults on a missing row or a nonsense value. Zero and negatives are refused
// rather than honoured: MinSize 0 would make every page its own community, and
// MaxSize 0 would discard all of them.
func (s *Service) communitySizeThresholds() (int, int) {
	minSize, maxSize := graph.DefaultMinSize, graph.DefaultMaxSize
	if v, err := db.GetConfig(s.db, communityMinSizeKey); err == nil {
		if n, cerr := strconv.Atoi(strings.TrimSpace(v)); cerr == nil && n > 0 {
			minSize = n
		}
	}
	if v, err := db.GetConfig(s.db, communityMaxSizeKey); err == nil {
		if n, cerr := strconv.Atoi(strings.TrimSpace(v)); cerr == nil && n > 0 {
			maxSize = n
		}
	}
	if minSize > maxSize {
		// An inverted range would silently discard every community, so neither
		// number the user typed is safer than the documented default.
		return graph.DefaultMinSize, graph.DefaultMaxSize
	}
	return minSize, maxSize
}
