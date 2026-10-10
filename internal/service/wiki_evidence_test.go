package service

import (
	"fmt"
	"strings"
	"testing"

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
