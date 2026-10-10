package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"repo-nest/internal/db"
	"repo-nest/internal/search/abeval"
)

// The W2 acceptance gate, run on a fixture corpus.
//
// ADR-0014 sets W2's bar as "过 cmd/abeval 门（真实标注 query 集 + Recall@k /
// NDCG@k delta），不接受『感觉变好了』". The honest version of that under the
// chosen sequencing (pages cannot exist in volume before W3 compiles them) is
// what this file does: the MECHANISM is measured against labelled cases on a
// fixture, while any claim about the user's real corpus stays unproven. t.Logf
// prints the numbers so a regression is visible, and the assertions are what stop
// "evidence retrieval" from shipping as a slower way of doing nothing.

// id-space encoding shared by both retrievers under test.
//
// This is load-bearing, not tidiness: page ids and note ids come from separate
// sequences, so note #7 and page #7 are both real objects. Comparing them as one
// id would make Recall@k count the wrong hit — the same cross-type collision that
// keeps SearchAll out of the RRF fuse today.
const pageIDEpoch = int64(1_000_000)

func encodedRef(item EvidenceItem) int64 {
	if item.Type == "page" {
		return pageIDEpoch + item.ID
	}
	return item.ID
}

// legacyNoteIDs reproduces the pre-W2 behaviour exactly: the notes ListNotes
// returns, first 10, no relevance anywhere in the choice.
func legacyNoteIDs(t *testing.T, svc *Service, projectID int64) []int64 {
	t.Helper()
	notes, err := db.ListNotes(svc.db, projectID)
	if err != nil {
		t.Fatal(err)
	}
	var out []int64
	for i, n := range notes {
		if i >= 10 {
			break
		}
		out = append(out, n.ID)
	}
	return out
}

func evidenceIDs(t *testing.T, svc *Service, projectID int64, query string) []int64 {
	t.Helper()
	ev := svc.GatherEvidence(projectID, query, EvidenceBudget{})
	out := make([]int64, 0, len(ev.Items))
	for _, it := range ev.Items {
		out = append(out, encodedRef(it))
	}
	return out
}

// buildGateCorpus creates a project whose knowledge is deliberately buried: the
// six answer-bearing notes are written FIRST, then 24 filler notes, so the
// recency-ordered legacy block cannot see them. Six pages carry the same topics in
// title/content so retrieval can find them.
func buildGateCorpus(t *testing.T, svc *Service) (projectID int64, noteIDs, pageIDs []int64) {
	t.Helper()
	projectID = seedProject(t, svc.db, "gate", "/tmp/gate")

	// 24 filler notes are written FIRST on purpose: db.ListNotes orders
	// `pinned DESC, sort_order ASC, created_at ASC, id ASC`, i.e. oldest first, so
	// the legacy "10 notes" window is the ten oldest ever written. Answers created
	// afterwards therefore fall outside it, which is what makes the gate measure
	// relevance rather than ordering.
	for i := 0; i < 24; i++ {
		if _, err := db.CreateNoteEx(svc.db, projectID,
			fmt.Sprintf("chore note %d", i), "routine scratch content with no bearing on any question",
			"", "log", "manual"); err != nil {
			t.Fatal(err)
		}
	}

	topics := []struct {
		slug, title, body string
	}{
		{"payment-gateway-retry", "Payment Gateway retry policy",
			"Payments retry three times with exponential backoff; the idempotency key is the invoice id."},
		{"session-timeout", "Session timeout behaviour",
			"Sessions expire after 30 minutes idle; refresh tokens rotate on every use."},
		{"release-checklist", "Release checklist",
			"Releases need a migration dry-run, a sha256 manifest fill and a rollback note."},
		{"sqlite-vec-choice", "Why sqlite-vec",
			"Vector storage stays local because the pure-Go port keeps the zero-CGO build promise."},
		{"handoff-protocol", "Handoff note protocol",
			"Handoff notes carry the handoff tag and refuse to be overwritten by notes_update."},
		{"scan-depth", "Scan depth semantics",
			"scan_depth bounds directory descent; repositories already known are always re-scanned."},
	}
	for _, tp := range topics {
		n, err := db.CreateNoteEx(svc.db, projectID, tp.title, tp.body, "gate", "knowledge", "manual")
		if err != nil {
			t.Fatal(err)
		}
		noteIDs = append(noteIDs, n.ID)
		p, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, tp.slug, tp.title, projectID, tp.body)
		if err != nil {
			t.Fatal(err)
		}
		pageIDs = append(pageIDs, p.ID)
	}
	return projectID, noteIDs, pageIDs
}

func gateCases(noteIDs, pageIDs []int64) []abeval.Case {
	at := func(ids []int64, i int) int64 { return ids[i] }
	return []abeval.Case{
		{Query: "payment gateway retry idempotency backoff",
			Relevant: []int64{at(noteIDs, 0), pageIDEpoch + at(pageIDs, 0)}},
		{Query: "when does the session timeout rotate refresh tokens",
			Relevant: []int64{at(noteIDs, 1), pageIDEpoch + at(pageIDs, 1)}},
		{Query: "release checklist migration dry run rollback",
			Relevant: []int64{at(noteIDs, 2), pageIDEpoch + at(pageIDs, 2)}},
		{Query: "why sqlite-vec zero CGO vector storage",
			Relevant: []int64{at(noteIDs, 3), pageIDEpoch + at(pageIDs, 3)}},
		{Query: "handoff notes refuse notes_update overwrite",
			Relevant: []int64{at(noteIDs, 4), pageIDEpoch + at(pageIDs, 4)}},
		{Query: "scan depth directory descent repositories",
			Relevant: []int64{at(noteIDs, 5), pageIDEpoch + at(pageIDs, 5)}},
	}
}

// The gate itself: retrieval must beat "the ten oldest notes" on labelled cases.
//
// Read the numbers with the fixture in mind: the corpus is built so that every
// answer-bearing note sits OUTSIDE the legacy window, which is why the baseline
// measures 0.000 rather than merely low. That proves the mechanism routes to the
// answer instead of to recency; it does NOT say how much real users gain, because
// real note orders are not this adversarial. The honest version of the W2 gate on
// real data still needs a labelled set drawn from an actual library.
func TestEvidenceRetrievalGate_BeatsLegacyStuffing(t *testing.T) {
	svc, _ := setupService(t)
	projectID, noteIDs, pageIDs := buildGateCorpus(t, svc)

	legacy := legacyNoteIDs(t, svc, projectID)
	if len(legacy) != 10 {
		t.Fatalf("legacy window = %d notes, want the documented 10", len(legacy))
	}
	// Guard the fixture itself: if an answer-bearing note happened to land inside
	// the legacy window, the comparison would be measuring noise, not the claim.
	inWindow := 0
	for _, want := range noteIDs {
		for _, got := range legacy {
			if got == want {
				inWindow++
			}
		}
	}
	if inWindow > 2 {
		t.Fatalf("%d/6 answer notes are already inside the legacy 10-note window; the corpus no longer separates relevance from recency", inWindow)
	}

	cases := gateCases(noteIDs, pageIDs)
	const k = 8
	lex, hyb, dRecall, dNDCG := abeval.Compare(cases, k,
		func(q string) []int64 { return legacy },
		func(q string) []int64 { return evidenceIDs(t, svc, projectID, q) })

	t.Logf("legacy (the 10 OLDEST notes, per ListNotes): %s", lex.SummaryLine("legacy"))
	t.Logf("evidence retrieval      : %s", hyb.SummaryLine("evidence"))
	t.Logf("delta Recall@%d = %+.3f, NDCG@%d = %+.3f", k, dRecall, k, dNDCG)

	if hyb.MeanRecall <= lex.MeanRecall {
		t.Errorf("evidence retrieval Recall@%d = %.3f did not beat legacy %.3f — the whole W2 claim is that ranking beats recency", k, hyb.MeanRecall, lex.MeanRecall)
	}
	if hyb.MeanNDCG <= lex.MeanNDCG {
		t.Errorf("evidence NDCG@%d = %.3f did not beat legacy %.3f", k, hyb.MeanNDCG, lex.MeanNDCG)
	}
	// A gate that only just clears zero is indistinguishable from noise; demand a
	// real margin on a corpus built to be winnable.
	if dRecall < 0.5 {
		t.Errorf("Recall delta %+.3f is below the 0.5 bar this fixture should clear", dRecall)
	}
	// Pages must be what carries the win: if evidence only ever returns notes, the
	// wiki layer is decorative.
	all := svc.GatherEvidence(projectID, "release checklist migration dry run rollback", EvidenceBudget{})
	var sawPage, sawNote bool
	for _, it := range all.Items {
		if it.Type == "page" {
			sawPage = true
		}
		if it.Type == "note" {
			sawNote = true
		}
	}
	if !sawPage {
		t.Error("evidence returned no pages; the wiki layer is not being used")
	}
	if !sawNote {
		t.Error("evidence returned no notes; existing knowledge is being ignored")
	}
}

// Budget: the three caps ADR-0014 asks for, each enforced on its own terms.
func TestEvidenceBudgetCaps(t *testing.T) {
	svc, _ := setupService(t)
	_, _, _ = buildGateCorpus(t, svc)
	query := "payment gateway retry idempotency backoff"

	capped := svc.GatherEvidence(0, query, EvidenceBudget{MaxItems: 2})
	uncapped := svc.GatherEvidence(0, query, EvidenceBudget{MaxItems: 20})
	if len(capped.Items) > 2 {
		t.Errorf("MaxItems 2 produced %d items", len(capped.Items))
	}
	// Truncation is only meaningful when there WAS more to give; asserting it
	// against a 2-candidate corpus would be asserting the fixture, not the cap.
	if len(uncapped.Items) > 2 && (!capped.Truncated || capped.Dropped == 0) {
		t.Errorf("%d candidates available but a 2-item cap reported no truncation (truncated=%v dropped=%d)",
			len(uncapped.Items), capped.Truncated, capped.Dropped)
	}
	// A character quota smaller than one long snippet must still return something
	// rather than looping or returning nothing: it is a ceiling, not a filter.
	tight := svc.GatherEvidence(0, query, EvidenceBudget{MaxItems: 12, MaxChars: 40})
	if len(tight.Items) == 0 {
		t.Error("a 40-char budget returned nothing; the first item must always fit or the cap is a mute button")
	}
	chars := 0
	for _, it := range tight.Items {
		chars += len([]rune(it.Title)) + len([]rune(it.Snippet))
	}
	// A 40-char quota admits exactly one item (the first always fits); two would
	// mean the ceiling is not applied per item.
	if len(tight.Items) != 1 {
		t.Errorf("a 40-char budget produced %d items, want exactly 1 (chars=%d)", len(tight.Items), chars)
	}
	// An empty or whitespace query must not flood the prompt with "everything".
	if ev := svc.GatherEvidence(0, "   ", EvidenceBudget{}); len(ev.Items) != 0 {
		t.Errorf("blank query returned %d items, want 0", len(ev.Items))
	}
	// Rendered refs must be unique and sequential per type, else citations collide.
	ev := svc.GatherEvidence(0, query, EvidenceBudget{})
	seen := map[string]bool{}
	for _, it := range ev.Items {
		if it.Ref == "" {
			t.Fatalf("item %s %d has no citation ref", it.Type, it.ID)
		}
		if seen[it.Ref] {
			t.Errorf("duplicate citation ref %s", it.Ref)
		}
		seen[it.Ref] = true
	}
	if block := ev.Render(); block == "" {
		t.Error("non-empty evidence rendered to nothing")
	} else if !strings.Contains(block, "[P") && !strings.Contains(block, "[N") {
		t.Error("rendered block carries no citation markers")
	}
}

// Cross-project bleed: a page owned by another project must not answer here.
func TestEvidenceRespectsProjectScoping(t *testing.T) {
	svc, _ := setupService(t)
	other := seedProject(t, svc.db, "other", "/tmp/other")
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "secret-of-other", "Secret Of Other",
		other, "invoice idempotency and retry policy live here"); err != nil {
		t.Fatal(err)
	}
	home := seedProject(t, svc.db, "home", "/tmp/home")
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "home-page", "Home Page",
		home, "nothing to do with payments at all"); err != nil {
		t.Fatal(err)
	}
	ev := svc.GatherEvidence(home, "invoice idempotency retry policy", EvidenceBudget{})
	for _, it := range ev.Items {
		if it.Type == "page" && strings.Contains(it.Title, "Secret Of Other") {
			t.Fatalf("another project's page leaked into evidence: %+v", it)
		}
	}
	// Global pages (project_id 0) are allowed everywhere — that is the point of them.
	glob, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "global-convention", "Global Convention",
		0, "invoice idempotency conventions across all projects")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range svc.GatherEvidence(home, "invoice idempotency conventions", EvidenceBudget{}).Items {
		if it.Type == "page" && it.ID == glob.ID {
			found = true
		}
	}
	if !found {
		t.Error("a global page was excluded from a project-scoped query")
	}
}

// The query loop: an answer becomes a `query` page linked to what it cited, so the
// store grows from use and not only from compiles.
func TestFileAnswerAsPageClosesTheLoop(t *testing.T) {
	svc, _ := setupService(t)
	projectID, _, pageIDs := buildGateCorpus(t, svc)

	refs := []string{"P1", "P2"}
	ev := svc.GatherEvidence(projectID, "release checklist migration dry run rollback", EvidenceBudget{})
	for _, it := range ev.Items {
		if it.Type == "page" {
			refs = append(refs, it.Ref)
		}
	}
	evForFiling := svc.GatherEvidence(projectID, "release checklist migration dry run rollback", EvidenceBudget{})
	id, err := svc.FileAnswerAsPage(projectID, "release checklist migration dry run rollback",
		"Always dry-run the migration, then fill the sha256 manifest, then write the rollback note.",
		evForFiling, refs)
	if err != nil {
		t.Fatalf("file answer: %v", err)
	}
	page, err := db.GetWikiPageByID(svc.db, id)
	if err != nil {
		t.Fatal(err)
	}
	if page.Kind != db.WikiKindQuery {
		t.Errorf("filed page kind = %q, want %q", page.Kind, db.WikiKindQuery)
	}
	if !strings.Contains(page.Content, "rollback note") {
		t.Error("filed page lost the answer body")
	}
	out, err := db.WikiEdgesFrom(svc.db, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("a filed answer has no citation edges; the loop did not close")
	}
	var sawRef bool
	for _, e := range out {
		if e.Relation == db.RelationRef {
			sawRef = true
		}
	}
	if !sawRef {
		t.Errorf("filed answer edges = %+v, want at least one ref edge to a cited page", out)
	}

	// Filing the same question twice is a re-run: it updates rather than erroring
	// or spawning a shadow page.
	second, err := svc.FileAnswerAsPage(projectID, "release checklist migration dry run rollback",
		"Updated answer.", evForFiling, []string{"P1"})
	if err != nil {
		t.Fatalf("refiling the same question errored: %v", err)
	}
	if second != id {
		t.Errorf("refiling produced page %d, want the same page %d", second, id)
	}
	pages, _ := db.ListWikiPages(svc.db, db.WikiKindQuery, 0)
	if len(pages) != 1 {
		t.Errorf("%d query pages after two filings, want 1", len(pages))
	}
	// Cited notes attach as source material, not as peer pages. The note must be
	// reachable by ref, so it is written on-topic and its ref is read back out of
	// the same evidence set — which also proves ref -> object mapping resolves to
	// the right note rather than to the note with that numeric id.
	const loopQuery = "sqlite-vec note keeps vector storage local"
	if _, err := db.CreateNoteEx(svc.db, projectID, "sqlite-vec note",
		"sqlite-vec note keeps vector storage local and zero CGO.", "gate", "knowledge", "manual"); err != nil {
		t.Fatal(err)
	}
	noteEv := svc.GatherEvidence(projectID, loopQuery, EvidenceBudget{})
	var noteRef string
	var noteID int64
	for _, it := range noteEv.Items {
		if it.Type == "note" && strings.Contains(it.Title, "sqlite-vec note") {
			noteRef, noteID = it.Ref, it.ID
		}
	}
	if noteRef == "" {
		t.Fatal("fixture note was not retrieved; the loop test cannot assert attachment")
	}
	filedAgain, err := svc.FileAnswerAsPage(projectID, loopQuery, "Because.",
		noteEv, []string{noteRef, "P1"})
	if err != nil {
		t.Fatal(err)
	}
	// A different question is a different page: assert on the page that was just
	// created, not on the first one (an earlier version of this test did exactly
	// that and "passed" nothing).
	attached, err := db.NotesForPage(svc.db, filedAgain)
	if err != nil {
		t.Fatal(err)
	}
	var got bool
	for _, a := range attached {
		if a == noteID {
			got = true
		}
	}
	if !got {
		t.Errorf("cited note %d was not attached to page %d (attached=%v)", noteID, id, attached)
	}
	// A ref that was never in the evidence set must not resolve to anything.
	if items := resolveAgainstEvidence(noteEv, []string{"P999", "N0", "N1", ""}); len(items) != 0 {
		t.Errorf("invented refs resolved to %d items, want 0", len(items))
	}
	_ = pageIDs
}

// ADR-0018 lane 2: evidence reads the page graph behind its gate.
//
// The zero-regression contract is the point of the first half: with
// wiki_graph_search explicitly "0" the result must be exactly what FTS
// ranking produced before the graph existed — a page reachable only through a
// link stays invisible. ("0" is now the only OFF value; the gate defaulted to
// ON on 2026-10-10, so the off half has to say so out loud — see
// TestWikiGraphSearch_DefaultOnExplicitZeroOff.) The second half proves the
// walk adds one-hop neighbours (their bodies share no word with the query, so
// text search cannot reach them) while the relation whitelist and the
// approved-only gate still hold, and the budget still caps the combined list.
func TestEvidencePages_GraphSearchGate(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "graph-gate", "/tmp/graph-gate")

	hub, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "alpha-hub", "Alpha Hub", pid,
		"alpha routing rules live here and nowhere else in this fixture")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "omega-detail", "Omega Detail", pid,
		"omega downstream specifics written in a disjoint vocabulary")
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "omega-conflict", "Omega Conflict", pid,
		"omega disputes the hub claim outright")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := db.CreateWikiPageAs(svc.db, db.WikiKindEntity, "omega-draft", "Omega Draft", pid,
		"omega draft nobody has reviewed yet", db.WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		to       int64
		relation string
	}{
		{detail.ID, db.RelationRef},
		{conflict.ID, db.RelationContradicts},
		{draft.ID, db.RelationRef},
	} {
		if err := db.LinkWikiPages(svc.db, hub.ID, e.to, e.relation); err != nil {
			t.Fatal(err)
		}
	}

	query := "alpha routing rules"
	pagesOf := func(items []EvidenceItem) []EvidenceItem {
		var out []EvidenceItem
		for _, it := range items {
			if it.Type == "page" {
				out = append(out, it)
			}
		}
		return out
	}

	// OFF (explicit "0" — the only value that disables the walk since the
	// 2026-10-10 default flip): the pre-graph result, exactly — one page,
	// the FTS hit.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "0"); err != nil {
		t.Fatal(err)
	}
	off := pagesOf(svc.evidencePages(query, pid, 10, time.Second))
	if len(off) != 1 || off[0].ID != hub.ID {
		t.Fatalf("gate off: pages = %+v, want just the FTS hit %d", off, hub.ID)
	}

	// ON: the neighbour enters after the seed; the contradicts target and the
	// pending draft stay out (whitelist and approved-only hold at the service
	// layer too), and the budget still caps the combined list.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	on := pagesOf(svc.evidencePages(query, pid, 10, time.Second))
	if len(on) != 2 || on[0].ID != hub.ID || on[1].ID != detail.ID {
		t.Fatalf("gate on: pages = %+v, want [hub %d, detail %d]", on, hub.ID, detail.ID)
	}
	for _, it := range on {
		if it.ID == conflict.ID || it.ID == draft.ID {
			t.Errorf("gate on: %q entered evidence; contradicts targets and pending pages must stay out", it.Title)
		}
	}
	if capped := pagesOf(svc.evidencePages(query, pid, 1, time.Second)); len(capped) != 1 || capped[0].ID != hub.ID {
		t.Errorf("gate on with limit 1: pages = %+v, want just the FTS hit", capped)
	}
}

// The gate's default semantics after the 2026-10-10 flip: ON unless the row
// says "0". This is the inverse of auto_import's predicate on purpose (an
// absent row there means OFF), and the difference must stay deliberate — the
// walk gates no egress, only the order of pages the FTS path already found.
// The deleted-row case is the one an old database hits at upgrade: no row at
// all must mean ON, not silently revert to the pre-graph ranking.
func TestWikiGraphSearch_DefaultOnExplicitZeroOff(t *testing.T) {
	svc, _ := setupService(t)

	if !svc.wikiGraphSearchEnabled() {
		t.Error("fresh database: wiki_graph_search must default ON")
	}
	if err := db.SetConfig(svc.db, "wiki_graph_search", "0"); err != nil {
		t.Fatal(err)
	}
	if svc.wikiGraphSearchEnabled() {
		t.Error(`wiki_graph_search="0" must disable the walk`)
	}
	if err := db.SetConfig(svc.db, "wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	if !svc.wikiGraphSearchEnabled() {
		t.Error(`wiki_graph_search="1" must enable the walk`)
	}
	if err := db.DeleteConfig(svc.db, "wiki_graph_search"); err != nil {
		t.Fatal(err)
	}
	if !svc.wikiGraphSearchEnabled() {
		t.Error("a deleted row (the pre-flip database state) must read as ON, not fall back to the pre-graph ranking")
	}
}

// ADR-0018 lane 2, second slice: with the gate on, the graph decides the
// ORDER, not just the membership — bm25 degrades to seed recall (ADR-0018
// 待决 3). The fixture makes the three orderings distinguishable:
//   - canonical is endorsed by BOTH FTS hits while its body shares no term
//     with the query — it must lead, which is the entire point of reading
//     the graph;
//   - solo is endorsed by only one seed — it must trail the seeds;
//   - the two seeds tie, so they keep their FTS order — a graph with nothing
//     to say between two candidates must not scramble the text ranking.
func TestEvidencePages_GraphRankingEndorsesConsensus(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "graph-rank", "/tmp/graph-rank")

	seedA, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "alpha-seed-a", "Alpha Seed A", pid,
		"alpha routing rules as the first seed page knows them")
	if err != nil {
		t.Fatal(err)
	}
	seedB, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "alpha-seed-b", "Alpha Seed B", pid,
		"alpha routing rules restated by a second seed page")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "omega-canonical", "Omega Canonical", pid,
		"omega canonical wording shares no term with the query at all")
	if err != nil {
		t.Fatal(err)
	}
	solo, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "omega-solo", "Omega Solo", pid,
		"omega solo wording is likewise disjoint from the query")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct{ from, to int64 }{
		{seedA.ID, canonical.ID},
		{seedB.ID, canonical.ID},
		{seedA.ID, solo.ID},
	} {
		if err := db.LinkWikiPages(svc.db, e.from, e.to, db.RelationRef); err != nil {
			t.Fatal(err)
		}
	}

	query := "alpha routing rules"
	pagesOf := func(items []EvidenceItem) []EvidenceItem {
		var out []EvidenceItem
		for _, it := range items {
			if it.Type == "page" {
				out = append(out, it)
			}
		}
		return out
	}

	// OFF (explicit "0"): exactly the two FTS hits — the graph-only pages
	// stay invisible.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "0"); err != nil {
		t.Fatal(err)
	}
	off := pagesOf(svc.evidencePages(query, pid, 10, time.Second))
	if len(off) != 2 {
		t.Fatalf("gate off: pages = %+v, want just the two FTS hits", off)
	}
	ftsOrder := []int64{off[0].ID, off[1].ID}

	// ON: PPR orders all four candidates.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	on := pagesOf(svc.evidencePages(query, pid, 10, time.Second))
	if len(on) != 4 {
		t.Fatalf("gate on: pages = %+v, want the 2 seeds plus 2 neighbours", on)
	}
	if on[0].ID != canonical.ID {
		t.Errorf("first page = %d (%s), want the doubly-endorsed page %d", on[0].ID, on[0].Title, canonical.ID)
	}
	if on[3].ID != solo.ID {
		t.Errorf("last page = %d (%s), want the singly-endorsed page %d", on[3].ID, on[3].Title, solo.ID)
	}
	if on[1].ID != ftsOrder[0] || on[2].ID != ftsOrder[1] {
		t.Errorf("tied seeds reordered: got [%d %d], want FTS order %v", on[1].ID, on[2].ID, ftsOrder)
	}
	for i, it := range on {
		if it.Rank != i+1 {
			t.Errorf("item %d carries Rank %d, want the position it was ranked into", it.ID, it.Rank)
		}
	}

	// The budget still caps the ranked list, and the cap keeps the ranking
	// rather than reverting to the FTS prefix.
	capped := pagesOf(svc.evidencePages(query, pid, 2, time.Second))
	if len(capped) != 2 || capped[0].ID != canonical.ID || capped[1].ID != ftsOrder[0] {
		t.Errorf("gate on with limit 2: pages = %+v, want [canonical, first FTS hit]", capped)
	}
}

// EvidencePageRanking is the exported page half of the evidence path — the
// surface the abeval page arm measures (plan Phase 2: quantify the PPR delta).
// The contract under test is that it IS the evidence ranking, not a lookalike:
// the gate is read per call (so an eval harness can flip it between arms), and
// the ordering matches evidencePages in both gate states.
func TestEvidencePageRanking_MatchesEvidencePath(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "page-ranking", "/tmp/page-ranking")

	seedA, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "alpha-seed-a", "Alpha Seed A", pid,
		"alpha routing rules as the first seed page knows them")
	if err != nil {
		t.Fatal(err)
	}
	seedB, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "alpha-seed-b", "Alpha Seed B", pid,
		"alpha routing rules restated by a second seed page")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "omega-canonical", "Omega Canonical", pid,
		"omega canonical wording shares no term with the query at all")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct{ from, to int64 }{
		{seedA.ID, canonical.ID},
		{seedB.ID, canonical.ID},
	} {
		if err := db.LinkWikiPages(svc.db, e.from, e.to, db.RelationRef); err != nil {
			t.Fatal(err)
		}
	}

	query := "alpha routing rules"

	// Gate off: the FTS-only ranking — both seeds, no graph-only pages. The
	// bm25 order between two near-identical seeds is not the contract here,
	// so it is captured and reused as the reference below.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "0"); err != nil {
		t.Fatal(err)
	}
	off := svc.EvidencePageRanking(query, pid, 10)
	if len(off) != 2 {
		t.Fatalf("gate off: ranking = %+v, want exactly the two FTS hits", off)
	}
	ftsOrder := []int64{off[0].ID, off[1].ID}
	seen := map[int64]bool{seedA.ID: true, seedB.ID: true}
	for _, id := range ftsOrder {
		if !seen[id] {
			t.Fatalf("gate off: ranking = %+v, want only the two seeds", off)
		}
	}

	// Gate on: the doubly-endorsed page leads, the seeds keep their FTS
	// order — the same PPR ranking evidencePages produces, with Ranks
	// re-stamped to the exported list's positions.
	if err := db.SetConfig(svc.db, "wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	on := svc.EvidencePageRanking(query, pid, 10)
	if len(on) != 3 || on[0].ID != canonical.ID {
		t.Fatalf("gate on: ranking = %+v, want the endorsed page first of three", on)
	}
	if on[1].ID != ftsOrder[0] || on[2].ID != ftsOrder[1] {
		t.Errorf("tied seeds reordered: got [%d %d], want FTS order %v", on[1].ID, on[2].ID, ftsOrder)
	}
	for i, it := range on {
		if it.Rank != i+1 {
			t.Errorf("item %d carries Rank %d, want %d", it.ID, it.Rank, i+1)
		}
	}
}
