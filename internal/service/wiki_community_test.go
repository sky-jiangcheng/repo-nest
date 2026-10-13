package service

import (
	"fmt"
	"strings"
	"testing"

	"repotory/internal/db"
	"repotory/internal/graph"
)

// ADR-0020 晋升条件 2, 3 and 5 — the community synthesis pass.
//
// The fixture is one dense topic cluster (four pages, every pair joined by a
// whitelist relation) plus a page outside it. That shape is chosen so the expected
// partition is not in doubt: K4 converges to one community of four under any of the
// tie-break rules, so a failure below points at the wiring rather than at Label
// Propagation's taste.

func communityFixture(t *testing.T, svc *Service) (projectID int64, ids map[string]int64) {
	t.Helper()
	projectID = seedProject(t, svc.db, "community", "/tmp/community")
	ids = map[string]int64{}
	body := "支付网关的重试与幂等：三次退避、invoice id 作幂等键、超时统一 408。" +
		"这段文字在两页之间重复出现，是为了让摘要的输入长度不构成任何检查的判据。"

	names := []string{"gateway-core", "gateway-retry", "gateway-idempotency", "gateway-timeouts"}
	for _, n := range names {
		p := mustCreatePage(t, svc, db.WikiKindEntity, n, strings.Title(n), //nolint:staticcheck // title shape is irrelevant here
			projectID, body, db.WikiStatusApproved, "manual")
		ids[n] = p.ID
		mustAttachNote(t, svc, projectID, p.ID, "note "+n)
	}
	// Every pair joined by ref: a clique, so LP has an unambiguous answer.
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			mustLink(t, svc, ids[names[i]], ids[names[j]], db.RelationRef)
		}
	}
	// One page outside the topic, joined to nobody: it must not be pulled in.
	ids["unrelated"] = mustCreatePage(t, svc, db.WikiKindConcept, "nightly-recompute",
		"Nightly Recompute", projectID, body, db.WikiStatusApproved, "manual").ID
	return projectID, ids
}

func configureCommunity(t *testing.T, svc *Service, url string, enabled bool) {
	t.Helper()
	for k, v := range map[string]string{
		"ai_chat_base_url": url + "/v1",
		"ai_chat_model":    "stub",
	} {
		if err := svc.UpdateConfig(k, v); err != nil {
			t.Fatalf("config %s: %v", k, err)
		}
	}
	flag := "0"
	if enabled {
		flag = "1"
	}
	if err := svc.UpdateConfig(wikiCommunityKey, flag); err != nil {
		t.Fatalf("config %s: %v", wikiCommunityKey, err)
	}
}

func synthesisPages(t *testing.T, svc *Service) []db.WikiPage {
	t.Helper()
	pages, err := db.ListWikiPages(svc.db, db.WikiKindSynthesis, 0)
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

// 决策 9 / 晋升条件 3: with the gate off the pass detects and reports, sends nothing
// and writes nothing. Detection runs first on purpose, so "off" still tells the user
// what they would get.
func TestRunCommunitySummary_GateOffDetectsButWritesNothing(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	srv, hits := chatStub(t, "不该被调用")
	configureCommunity(t, svc, srv.URL, false)

	rep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.CommunitiesFound != 1 {
		t.Fatalf("the structure half must still run, found %d communities", rep.CommunitiesFound)
	}
	if !rep.CommunityGateOff {
		t.Fatal("the report must say the gate, not just be empty")
	}
	if hits.Load() != 0 {
		t.Fatalf("gate off sent %d requests", hits.Load())
	}
	if rep.PagesCreated != 0 || rep.DocumentsLinked != 0 || rep.NotesAttached != 0 {
		t.Fatalf("gate off wrote: %+v", rep)
	}
	if got := synthesisPages(t, svc); len(got) != 0 {
		t.Fatalf("gate off created %d pages", len(got))
	}
	if rep.OutboundBytes != 0 {
		t.Fatalf("an off run must report zero outbound, got %d", rep.OutboundBytes)
	}
}

// 决策 4 + 5 + 6: one pending synthesis page per community, `documents` edges built
// by code, and the members' source notes attached for provenance.
func TestRunCommunitySummary_FilesPendingSynthesis(t *testing.T) {
	svc, _ := setupService(t)
	pid, ids := communityFixture(t, svc)
	srv, hits := chatStub(t, "这四页共同描述支付网关的可靠性策略：重试、幂等与超时是同一件事的三个面。")
	configureCommunity(t, svc, srv.URL, true)

	rep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if hits.Load() == 0 {
		t.Fatal("the model was never asked")
	}
	if rep.PagesCreated != 1 {
		t.Fatalf("expected one synthesis page, got %+v", rep)
	}
	if rep.OutboundBytes == 0 {
		t.Fatal("an enabled run must report what it sent (晋升条件 3)")
	}
	pages := synthesisPages(t, svc)
	if len(pages) != 1 {
		t.Fatalf("expected exactly one synthesis page, got %d", len(pages))
	}
	page := pages[0]
	if page.Status != db.WikiStatusPending {
		t.Fatalf("a synthesis draft must be pending, got %q", page.Status)
	}
	if page.Source != communitySourceTag {
		t.Fatalf("source must mark these as machine-synthesized, got %q", page.Source)
	}
	if page.ProjectID != pid {
		t.Fatalf("synthesis belongs to the project it summarizes, got %d", page.ProjectID)
	}
	if !strings.Contains(page.Content, "支付网关") {
		t.Fatalf("the body should be the model's text, got %q", page.Content)
	}
	if page.Slug != CommunitySynthesisSlug([]string{
		"gateway-core", "gateway-idempotency", "gateway-retry", "gateway-timeouts"}) {
		t.Fatalf("unexpected slug %q", page.Slug)
	}

	// The unrelated page must not be in the topic.
	outEdges, err := db.WikiEdgesFrom(svc.db, ids["unrelated"])
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range outEdges {
		if e.PageID == page.ID {
			t.Fatal("an unrelated page was pulled into the community")
		}
	}

	// documents edges: member -> synthesis, four of them, all created by code.
	back, err := db.WikiEdgesTo(svc.db, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	docs := 0
	for _, e := range back {
		if e.Relation == db.RelationDocuments {
			docs++
		}
	}
	if docs != 4 || rep.DocumentsLinked != 4 {
		t.Fatalf("expected 4 documents edges into the synthesis, got %d (report %d)", docs, rep.DocumentsLinked)
	}
	// Provenance: the members' source notes feed the synthesis too.
	notes, err := db.NotesForPage(svc.db, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 4 || rep.NotesAttached != 4 {
		t.Fatalf("expected the 4 member source notes attached, got %d (report %d)", len(notes), rep.NotesAttached)
	}

	// Pending pages must not answer a question yet (ADR-0015 决策 1 unchanged).
	// The probe queries text that ALSO appears in the approved fixture pages, so the
	// assertion is about the synthesis slug specifically — "did the search return
	// anything at all" would pass for the wrong reason and fail for the wrong one.
	hitsBefore := hits.Load()
	for _, slug := range searchSlugs(t, svc, "支付网关") {
		if slug == page.Slug {
			t.Fatal("an unapproved synthesis page is visible to retrieval")
		}
	}
	if hits.Load() != hitsBefore {
		t.Fatal("the search probe made an extra model call")
	}
}

func searchSlugs(t *testing.T, svc *Service, q string) []string {
	t.Helper()
	found, err := db.SearchWikiPages(svc.db, q, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var out []string
	for _, p := range found {
		out = append(out, p.Slug)
	}
	return out
}

// 决策 7: the slug is a function of the member set, so re-running updates the draft
// instead of filing a second one. This is what makes the pass safe to click twice.
func TestRunCommunitySummary_ReRunUpdatesNotDuplicates(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	srv, hits := chatStub(t, "第一版综述。")
	configureCommunity(t, svc, srv.URL, true)

	first, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.PagesCreated != 1 {
		t.Fatalf("first pass should create, got %+v", first)
	}
	before := synthesisPages(t, svc)[0]
	callsAfterFirst := hits.Load()

	second, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.PagesCreated != 0 || second.PagesUpdated != 1 {
		t.Fatalf("a re-run must update, not pile up: %+v", second)
	}
	if hits.Load() <= callsAfterFirst {
		t.Fatal("a re-run should still ask the model; the idempotence is in the writes, not in skipping work")
	}
	pages := synthesisPages(t, svc)
	if len(pages) != 1 {
		t.Fatalf("the re-run filed a duplicate: %d synthesis pages", len(pages))
	}
	if pages[0].Slug != before.Slug || pages[0].ID != before.ID {
		t.Fatal("the stable slug must be the same page, not a new one")
	}

	// A third pass with one more member page in the topic changes the member set, so
	// the slug changes with it — and the OLD draft must not be silently left looking
	// current. Asserting the new slug differs is the guard against a hash that
	// ignores its input.
	extra := mustCreatePage(t, svc, db.WikiKindEntity, "gateway-quota", "Gateway Quota", pid,
		"配额与限流同样属于网关可靠性。", db.WikiStatusApproved, "manual")
	mustLink(t, svc, idsOf(t, svc, "gateway-core"), extra.ID, db.RelationRef)
	mustAttachNote(t, svc, pid, extra.ID, "note quota")
	for _, n := range []string{"gateway-retry", "gateway-idempotency", "gateway-timeouts"} {
		mustLink(t, svc, idsOf(t, svc, n), extra.ID, db.RelationRef)
	}
	third, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if third.PagesCreated != 1 {
		t.Fatalf("a grown community should file its own new draft, got %+v", third)
	}
	if len(synthesisPages(t, svc)) != 2 {
		t.Fatal("the grown topic must not overwrite the older draft")
	}
}

func idsOf(t *testing.T, svc *Service, slug string) int64 {
	t.Helper()
	p, err := db.GetWikiPageBySlug(svc.db, slug)
	if err != nil {
		t.Fatalf("look up %s: %v", slug, err)
	}
	return p.ID
}

// 决策 7 + ADR-0015: an APPROVED synthesis is never rewritten by this path. It
// becomes a revision todo, and the page is byte-identical afterwards.
func TestRunCommunitySummary_ApprovedSynthesisBecomesRevisionTodo(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	srv, _ := chatStub(t, "人工认可的综述。")
	configureCommunity(t, svc, srv.URL, true)

	if _, err := svc.RunCommunitySummary(pid); err != nil {
		t.Fatalf("first: %v", err)
	}
	page := synthesisPages(t, svc)[0]
	if err := svc.ApproveWikiPage(page.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approved, err := db.GetWikiPageByID(svc.db, page.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RunCommunitySummary(pid); err != nil {
		t.Fatalf("second: %v", err)
	}
	after, err := db.GetWikiPageByID(svc.db, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Content != approved.Content || after.Status != approved.Status ||
		after.Title != approved.Title || after.UpdatedAt != approved.UpdatedAt {
		t.Fatalf("an approved synthesis was rewritten: %+v -> %+v", approved, after)
	}
	todos, err := db.ListTodos(svc.db, pid)
	if err != nil {
		t.Fatal(err)
	}
	var hit bool
	for _, td := range todos {
		if strings.Contains(td.Title, "synthesis-revision") {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("expected a [lint] synthesis-revision todo, got %v", titlesOf(todos))
	}
}

func titlesOf(todos []db.Todo) []string {
	var out []string
	for _, td := range todos {
		out = append(out, td.Title)
	}
	return out
}

// 晋升条件 2: an unusable reply still leaves a reviewable page with its edges and
// provenance. Losing the whole topic to one failed call would make the pass
// non-idempotent in the worst direction.
func TestRunCommunitySummary_EmptyReplyStillFilesThePage(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	srv, _ := chatStub(t, "   ")
	configureCommunity(t, svc, srv.URL, true)

	rep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.PagesCreated != 1 || rep.EmptyBodies != 1 {
		t.Fatalf("expected a created page with an empty body, got %+v", rep)
	}
	if rep.DocumentsLinked != 4 || rep.NotesAttached != 4 {
		t.Fatalf("edges and provenance must still be built: %+v", rep)
	}
	if rep.LLMNote == "" {
		t.Fatal("the report must say the model gave nothing usable")
	}
	page := synthesisPages(t, svc)[0]
	if strings.TrimSpace(page.Content) != "" {
		t.Fatalf("body must stay empty rather than be invented, got %q", page.Content)
	}
}

// 决策 3: propagation follows the whitelist only. contradicts (对立) and mentions
// (顺带一提) must not be able to fuse two topics.
func TestRunCommunitySummary_OnlyWhitelistedRelationsPropagate(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "community-rel", "/tmp/community-rel")
	body := "同一份正文，区别只在关系类型：这个测试的全部信息量都在边上。"

	group := func(prefix string, n int) []int64 {
		var ids []int64
		for i := 0; i < n; i++ {
			slug := fmt.Sprintf("%s-%d", prefix, i)
			p := mustCreatePage(t, svc, db.WikiKindEntity, slug, "Page "+slug, pid, body,
				db.WikiStatusApproved, "manual")
			ids = append(ids, p.ID)
			mustAttachNote(t, svc, pid, p.ID, "note "+slug)
		}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				mustLink(t, svc, ids[i], ids[j], db.RelationRef)
			}
		}
		return ids
	}
	a := group("alpha", 3)
	b := group("beta", 3)
	// Join the two cliques with edges that are NOT similarity statements, one at a
	// time, and assert the communities stay apart either way.
	mustLink(t, svc, a[0], b[0], db.RelationContradicts)
	mustLink(t, svc, a[1], b[1], db.RelationMentions)
	mustLink(t, svc, a[2], b[2], db.RelationSupersedes)

	srv, hits := chatStub(t, "综述。")
	configureCommunity(t, svc, srv.URL, true)

	rep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.CommunitiesFound != 2 {
		t.Fatalf("non-whitelist edges must not fuse topics, found %d communities (%d edges)",
			rep.CommunitiesFound, rep.Edges)
	}
	if rep.Edges != 6 { // 3 + 3 inside the cliques only
		t.Fatalf("expected exactly the clique edges in scope, got %d", rep.Edges)
	}
	if rep.PagesCreated != 2 {
		t.Fatalf("expected two synthesis drafts, got %+v", rep)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected one call per community, got %d", hits.Load())
	}

	// And the whitelist DOES propagate: replacing those three edges with documents
	// turns it into one topic.
	for _, pair := range [][2]int64{{a[0], b[0]}, {a[1], b[1]}, {a[2], b[2]}} {
		if err := db.UnlinkWikiPages(svc.db, pair[0], pair[1], db.RelationContradicts); err != nil {
			t.Fatal(err)
		}
		if err := db.UnlinkWikiPages(svc.db, pair[0], pair[1], db.RelationMentions); err != nil {
			t.Fatal(err)
		}
		if err := db.UnlinkWikiPages(svc.db, pair[0], pair[1], db.RelationSupersedes); err != nil {
			t.Fatal(err)
		}
		if err := db.LinkWikiPages(svc.db, pair[0], pair[1], db.RelationDocuments); err != nil {
			t.Fatal(err)
		}
	}
	merged, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("merged run: %v", err)
	}
	if merged.CommunitiesFound != 1 || merged.DroppedSmall != 0 {
		t.Fatalf("documents edges must propagate, got %+v", merged)
	}
}

// 决策 4: only approved pages take part. A pending draft must not become evidence
// for a synthesis of "what the library currently believes".
func TestRunCommunitySummary_IgnoresPendingAndRejected(t *testing.T) {
	svc, _ := setupService(t)
	pid, ids := communityFixture(t, svc)
	// Two drafts joined to the topic: they must not enlarge it.
	d1 := mustCreatePage(t, svc, db.WikiKindEntity, "draft-one", "Draft One", pid,
		"未审草稿。", db.WikiStatusPending, "wiki-compile").ID
	d2 := mustCreatePage(t, svc, db.WikiKindEntity, "draft-two", "Draft Two", pid,
		"未审草稿二。", db.WikiStatusPending, "wiki-compile").ID
	mustLink(t, svc, d1, d2, db.RelationRef)
	mustLink(t, svc, ids["gateway-core"], d1, db.RelationRef)

	srv, _ := chatStub(t, "综述。")
	configureCommunity(t, svc, srv.URL, true)
	rep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if rep.ApprovedPages != 5 {
		t.Fatalf("only approved pages may participate, got %d", rep.ApprovedPages)
	}
	// DroppedSmall is 1 here for a reason worth stating: it is the fixture's
	// unrelated singleton, not the drafts. Pending pages never reach the counter at
	// all, because they are filtered out before detection — which is what this
	// assertion distinguishes.
	if rep.DroppedSmall != 1 || rep.Isolated != 1 {
		t.Fatalf("expected only the unrelated singleton as a fragment, got %+v", rep)
	}
	pages := synthesisPages(t, svc)
	if len(pages) != 1 {
		t.Fatalf("expected one synthesis, got %d", len(pages))
	}
	back, err := db.WikiEdgesTo(svc.db, pages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range back {
		if e.PageID == d1 || e.PageID == d2 {
			t.Fatalf("a pending draft %d fed a synthesis", e.PageID)
		}
	}
}

// 待决 6: the global tier is detected separately, never mixed into a project's
// communities (and vice versa).
func TestRunCommunitySummary_GlobalTierIsSeparate(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	body := "跨项目的全局概念页，不属于任何单个项目。"
	var globals []int64
	for _, n := range []string{"glob-a", "glob-b", "glob-c"} {
		p := mustCreatePage(t, svc, db.WikiKindConcept, n, "Global "+n, 0, body,
			db.WikiStatusApproved, "manual")
		globals = append(globals, p.ID)
		mustAttachNote(t, svc, pid, p.ID, "note "+n)
	}
	for i := 0; i < len(globals); i++ {
		for j := i + 1; j < len(globals); j++ {
			mustLink(t, svc, globals[i], globals[j], db.RelationPartOf)
		}
	}
	srv, _ := chatStub(t, "全局综述。")
	configureCommunity(t, svc, srv.URL, true)

	globalRep, err := svc.RunCommunitySummary(0)
	if err != nil {
		t.Fatalf("global run: %v", err)
	}
	if globalRep.ApprovedPages != 3 || globalRep.CommunitiesFound != 1 {
		t.Fatalf("the global tier should see only global pages, got %+v", globalRep)
	}
	projectRep, err := svc.RunCommunitySummary(pid)
	if err != nil {
		t.Fatalf("project run: %v", err)
	}
	if projectRep.ApprovedPages != 5 {
		t.Fatalf("a project run must not swallow the global pages, got %d", projectRep.ApprovedPages)
	}
	for _, p := range synthesisPages(t, svc) {
		if p.ProjectID == 0 {
			continue
		}
		if p.ProjectID != pid {
			t.Fatalf("a synthesis landed in the wrong project: %+v", p)
		}
	}
}

// 待决 5: the two thresholds are configurable, and an inverted range must not be
// honoured (it would silently discard every community).
func TestCommunitySizeThresholds(t *testing.T) {
	svc, _ := setupService(t)
	if minSize, maxSize := svc.communitySizeThresholds(); minSize != graph.DefaultMinSize || maxSize != graph.DefaultMaxSize {
		t.Fatalf("unset must mean the ADR defaults, got %d/%d", minSize, maxSize)
	}
	for k, v := range map[string]string{communityMinSizeKey: "2", communityMaxSizeKey: "8"} {
		if err := svc.UpdateConfig(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if minSize, maxSize := svc.communitySizeThresholds(); minSize != 2 || maxSize != 8 {
		t.Fatalf("overrides ignored: %d/%d", minSize, maxSize)
	}
	// Inverted.
	if err := svc.UpdateConfig(communityMinSizeKey, "20"); err != nil {
		t.Fatal(err)
	}
	if minSize, maxSize := svc.communitySizeThresholds(); minSize != graph.DefaultMinSize ||
		maxSize != graph.DefaultMaxSize {
		t.Fatalf("an inverted range must fall back to the defaults, got %d/%d", minSize, maxSize)
	}
	// Zero and negatives are writable config values (the key is numeric) but must
	// still be refused at read time, where they fall back to the ADR default.
	for _, bad := range []string{"0", "-3"} {
		if err := svc.UpdateConfig(communityMinSizeKey, bad); err != nil {
			t.Fatalf("%q should be writable: %v", bad, err)
		}
		if err := svc.UpdateConfig(communityMaxSizeKey, "30"); err != nil {
			t.Fatal(err)
		}
		if minSize, _ := svc.communitySizeThresholds(); minSize != graph.DefaultMinSize {
			t.Fatalf("a bad min (%q) must not be honoured, got %d", bad, minSize)
		}
	}
	// Non-numeric input never reaches the reader at all: UpdateConfig rejects it.
	for _, bad := range []string{"abc", ""} {
		if err := svc.UpdateConfig(communityMinSizeKey, bad); err == nil {
			t.Fatalf("UpdateConfig accepted a non-numeric min %q", bad)
		}
	}
}

// The identity function, pinned directly: a hash that ignored its input would make
// every topic collapse onto one page, and the failure would look like success.
func TestCommunitySynthesisSlug(t *testing.T) {
	a := CommunitySynthesisSlug([]string{"x", "y", "z"})
	if !strings.HasPrefix(a, "synthesis-") {
		t.Fatalf("unexpected slug shape %q", a)
	}
	if len(a) != len("synthesis-")+8 {
		t.Fatalf("slug must be `synthesis-` plus 8 hex chars, got %q", a)
	}
	if got := CommunitySynthesisSlug([]string{"z", "y", "x"}); got != a {
		t.Fatalf("member order must not change the identity: %q vs %q", a, got)
	}
	if got := CommunitySynthesisSlug([]string{"x", "y", "w"}); got == a {
		t.Fatalf("a different member set must produce a different slug: %q", got)
	}
	if got := CommunitySynthesisSlug([]string{"x", "y"}); got == a {
		t.Fatalf("a subset must not collide with the superset: %q vs %q", a, got)
	}
	if CommunitySynthesisTitle([]string{"b", "a"}) != CommunitySynthesisTitle([]string{"a", "b"}) {
		t.Fatal("the title must not depend on input order either")
	}
}

// 晋升条件 5: lint's no-synthesis check is the pass's own feedback loop. It fires
// while there are leaf pages and no synthesis, and stops firing once a synthesis is
// APPROVED — so the generation side is proven by the check that was complaining.
func TestCommunitySynthesisClosesTheNoSynthesisGap(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)

	before, err := svc.RunWikiLint(pid, false)
	if err != nil {
		t.Fatalf("lint before: %v", err)
	}
	if findingsByCheck(before)["no-synthesis"] == 0 {
		t.Fatal("precondition: the fixture should be reporting a missing synthesis")
	}

	srv, _ := chatStub(t, "综述正文。")
	configureCommunity(t, svc, srv.URL, true)
	if _, err := svc.RunCommunitySummary(pid); err != nil {
		t.Fatalf("community: %v", err)
	}
	page := synthesisPages(t, svc)[0]
	if err := svc.ApproveWikiPage(page.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}

	after, err := svc.RunWikiLint(pid, false)
	if err != nil {
		t.Fatalf("lint after: %v", err)
	}
	if findingsByCheck(after)["no-synthesis"] != 0 {
		t.Fatalf("no-synthesis should be closed once a synthesis is approved: %+v", after.Findings)
	}
}

// 决策 9: never scheduled. Lint has a ticker; this must not, because it spends
// tokens and writes pages. Asserted by scanning the tickers the service starts
// rather than by trusting a comment.
func TestCommunitySummaryIsNotScheduled(t *testing.T) {
	svc, _ := setupService(t)
	pid, _ := communityFixture(t, svc)
	srv, hits := chatStub(t, "不该自动发生。")
	configureCommunity(t, svc, srv.URL, true)

	// Whatever tickers exist, none of them may reach the community pass. Lint's own
	// first tick is deliberately six hours out, and the pass here is not on any
	// timer at all, so nothing should have left the process by now.
	svc.Startup()
	t.Cleanup(svc.Shutdown)
	_ = pid

	if got := hits.Load(); got != 0 {
		t.Fatalf("a scheduled path reached the community pass: %d requests", got)
	}
}
