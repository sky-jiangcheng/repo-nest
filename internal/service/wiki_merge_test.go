package service

import (
	"fmt"
	"testing"

	"repo-nest/internal/db"
)

// ADR-0019 晋升条件 3: each of the four blocking signals gets a positive and a
// negative case, so "the signal fires" and "the signal stays quiet" are both
// pinned rather than inferred from one happy path.

func TestMergeSlugShape_Signal(t *testing.T) {
	page := func(slug string) db.WikiPage { return db.WikiPage{Slug: slug} }
	cases := []struct {
		name, a, b string
		want       bool
	}{
		{"word-order split", "payment-gateway", "gateway-payment", true},
		{"prefix", "payment", "payment-gateway", true},
		{"typo within edit distance", "paymnet", "payment", true},
		{"normalizes to the same form", "Payment Gateway", "payment-gateway", true},
		{"unrelated slugs", "payment-gateway", "retry-policy", false},
		{"short prefix is noise", "au", "auth-service", false},
		{"beyond the edit budget", "payment", "payroll", false},
	}
	for _, tc := range cases {
		if got := mergeSlugShape(page(tc.a), page(tc.b)); got != tc.want {
			t.Errorf("%s: mergeSlugShape(%q, %q) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMergeTitleTokens_Overlap(t *testing.T) {
	// CJK bigrams: one character apart still overlaps.
	if jaccard(mergeTitleTokens("支付网关"), mergeTitleTokens("支付网关卡")) < mergeJaccardMin {
		t.Error("one-character-apart Chinese titles must overlap")
	}
	// Word order does not matter for English titles.
	if jaccard(mergeTitleTokens("Payment Gateway"), mergeTitleTokens("Gateway Payment")) < mergeJaccardMin {
		t.Error("word-order-swapped English titles must overlap")
	}
	// Unrelated titles share nothing.
	if jaccard(mergeTitleTokens("支付网关"), mergeTitleTokens("重试策略")) != 0 {
		t.Error("unrelated Chinese titles must not overlap")
	}
	// An empty title must not make every pair look similar.
	if jaccard(mergeTitleTokens(""), mergeTitleTokens("Payment Gateway")) != 0 {
		t.Error("empty title must not overlap anything")
	}
}

func TestMergeSharedSource_Signal(t *testing.T) {
	notes := map[int64][]int64{1: {10, 11}, 2: {11, 12}, 3: {12}}
	if !mergeSharedSource(notes, 1, 2) {
		t.Error("pages sharing note 11 must fire the signal")
	}
	if mergeSharedSource(notes, 1, 3) {
		t.Error("pages with disjoint notes must not fire the signal")
	}
	if mergeSharedSource(notes, 1, 99) {
		t.Error("an unknown page id must not fire the signal (and must not panic)")
	}
}

func TestMergeLinkedPairs_OnlyRefAndMentions(t *testing.T) {
	edges := []db.PageLinkRow{
		{FromID: 1, ToID: 2, Relation: db.RelationRef},
		{FromID: 2, ToID: 3, Relation: db.RelationMentions},
		{FromID: 3, ToID: 4, Relation: db.RelationDepends},
		{FromID: 4, ToID: 5, Relation: db.RelationImplements},
	}
	linked := mergeLinkedPairs(edges)
	for _, pair := range [][2]int64{{1, 2}, {2, 3}} {
		if !linked[pair] {
			t.Errorf("ref/mentions pair %v must be indexed", pair)
		}
	}
	for _, pair := range [][2]int64{{3, 4}, {4, 5}} {
		if linked[pair] {
			t.Errorf("non-sameness relation pair %v must not be indexed", pair)
		}
	}
}

// 晋升条件 3's structural half: blocking must NOT degenerate into the all-pairs
// scan it exists to avoid. A store of mutually dissimilar pages costs zero pair
// comparisons (no shared bucket), and planting one real split costs a handful —
// numbers a full n² traversal cannot produce.
func TestMergeBlocking_NoFullScan(t *testing.T) {
	const n = 40
	var pages []db.WikiPage
	notes := map[int64][]int64{}
	for i := 0; i < n; i++ {
		id := int64(i + 1)
		// Distinct in every indexable dimension: slug prefix, slug tokens,
		// title tokens, source note.
		pages = append(pages, db.WikiPage{
			ID: id, ProjectID: 1,
			Slug:  fmt.Sprintf("s%02dx-%02dy", i, i),
			Title: fmt.Sprintf("T%d", i),
		})
		notes[id] = []int64{id}
	}
	pairs, compared := mergeBlocking(pages, notes, nil)
	if len(pairs) != 0 || compared != 0 {
		t.Fatalf("dissimilar store must cost no comparisons, got pairs=%d compared=%d", len(pairs), compared)
	}

	// Plant the flagship split: same slug tokens in the opposite order, titles
	// overlapping, everything else distinct.
	pages = append(pages,
		db.WikiPage{ID: 100, ProjectID: 1, Slug: "split-a-b", Title: "Split A B"},
		db.WikiPage{ID: 101, ProjectID: 1, Slug: "split-b-a", Title: "Split B A"},
	)
	notes[100] = []int64{100}
	notes[101] = []int64{101}
	pairs, compared = mergeBlocking(pages, notes, nil)
	if len(pairs) != 1 {
		t.Fatalf("the planted split must surface exactly one pair, got %d", len(pairs))
	}
	// A full scan of 42 pages compares C(42,2)=861 pairs; blocking compares
	// only inside the handful of buckets the split shares.
	if compared > 20 {
		t.Fatalf("blocking compared %d pairs — that is the n² scan it must avoid", compared)
	}
}

// mergeFixture seeds the flagship split plus controls: an unrelated page (must
// never pair), a rejected tombstone (must never surface), and a second project
// holding the same slug shape (a merge the db layer would refuse, 决策 4).
func mergeFixture(t *testing.T, svc *Service) (winner, loser, other, tomb int64) {
	t.Helper()
	pid := seedProject(t, svc.db, "merge", "/tmp/merge")
	var err error
	w, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "payment-gateway", "Payment Gateway",
		pid, "canonical body", db.WikiStatusApproved, "manual")
	if err != nil {
		t.Fatalf("create winner: %v", err)
	}
	l, err := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "gateway-payment", "Gateway Payment",
		pid, "duplicate body", db.WikiStatusPending, "wiki-compile")
	if err != nil {
		t.Fatalf("create loser: %v", err)
	}
	o, err := db.CreateWikiPage(svc.db, db.WikiKindEntity, "retry-policy", "Retry Policy", pid, "retries")
	if err != nil {
		t.Fatalf("create control: %v", err)
	}
	tb, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "old-gateway", "Old Gateway", pid, "merged away")
	if err != nil {
		t.Fatalf("create tombstone: %v", err)
	}
	if err := db.SetWikiPageStatus(svc.db, tb.ID, db.WikiStatusRejected); err != nil {
		t.Fatalf("reject tombstone: %v", err)
	}
	// The split pair shares a source note (the second signal) and is
	// cross-linked with ref (the fourth).
	n, err := db.CreateNoteEx(svc.db, pid, "payment behaviour", "body", "", "knowledge", "manual")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := db.AttachNoteToPage(svc.db, n.ID, w.ID); err != nil {
		t.Fatalf("attach note to winner: %v", err)
	}
	if err := db.AttachNoteToPage(svc.db, n.ID, l.ID); err != nil {
		t.Fatalf("attach note to loser: %v", err)
	}
	if err := db.LinkWikiPages(svc.db, l.ID, w.ID, db.RelationRef); err != nil {
		t.Fatalf("link loser→winner: %v", err)
	}
	return w.ID, l.ID, o.ID, tb.ID
}

func TestDiscoverMergeCandidates_EndToEnd(t *testing.T) {
	svc, _ := setupService(t)
	winner, loser, other, tomb := mergeFixture(t, svc)

	scan, err := svc.DiscoverMergeCandidates(0)
	if err != nil {
		t.Fatalf("DiscoverMergeCandidates: %v", err)
	}
	if scan.Pages != 4 {
		t.Fatalf("Pages = %d, want 4 (the whole store, tombstone included)", scan.Pages)
	}
	if scan.Layer2Note == "" {
		t.Error("the absent lane-2 layer must be reported, not silent")
	}

	var found bool
	for _, c := range scan.Candidates {
		if c.A.ID == tomb || c.B.ID == tomb {
			t.Fatalf("a rejected page must never surface as a candidate: %+v", c)
		}
		if c.A.ID == other || c.B.ID == other {
			t.Fatalf("the unrelated control must never pair: %+v", c)
		}
		pair := [2]int64{c.A.ID, c.B.ID}
		if pair == [2]int64{winner, loser} {
			found = true
			// slug-shape (0.5) + shared-source (0.25) + title-overlap (0.25)
			// + cross-linked (0.2) = 1.2
			if c.Score < mergeScoreThreshold {
				t.Errorf("the flagship split scored %v, below the threshold", c.Score)
			}
			if len(c.Signals) != 4 {
				t.Errorf("the flagship split should fire all four signals, got %v", c.Signals)
			}
		}
	}
	if !found {
		t.Fatal("the flagship split pair was not surfaced")
	}
	if above := MergeCandidatesAboveThreshold(scan.Candidates); len(above) == 0 {
		t.Error("at least one candidate must clear the layer-1 threshold")
	}
}

func TestDiscoverMergeCandidates_SkipsCrossProject(t *testing.T) {
	svc, _ := setupService(t)
	pidA := seedProject(t, svc.db, "merge-a", "/tmp/merge-a")
	pidB := seedProject(t, svc.db, "merge-b", "/tmp/merge-b")
	// The same slug shape in two projects: db.MergeWikiPages would refuse the
	// pair (决策 4), so discovery must not surface it as actionable.
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "shared-alpha-beta", "Shared Alpha Beta", pidA, "a"); err != nil {
		t.Fatalf("create page a: %v", err)
	}
	if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, "shared-beta-alpha", "Shared Beta Alpha", pidB, "b"); err != nil {
		t.Fatalf("create page b: %v", err)
	}
	scan, err := svc.DiscoverMergeCandidates(0)
	if err != nil {
		t.Fatalf("DiscoverMergeCandidates: %v", err)
	}
	if len(scan.Candidates) != 0 {
		t.Fatalf("cross-project pairs must not surface, got %+v", scan.Candidates)
	}
}

// 晋升条件 3: top-K truncation. More qualifying pairs than mergeTopK must
// return exactly mergeTopK candidates (and report the full pair count, so the
// truncation is visible rather than silent).
func TestDiscoverMergeCandidates_TopK(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "topk", "/tmp/topk")
	total := mergeTopK + 5
	for i := 0; i < total; i++ {
		// Each pair shares a slug token set (so slug-shape fires and clears
		// the threshold alone) while the shared "alpha"/"beta" tokens push the
		// title buckets past mergeMaxBucket — exercising the cap too.
		a := fmt.Sprintf("pair%02d-alpha-beta", i)
		b := fmt.Sprintf("pair%02d-beta-alpha", i)
		if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, a, a, pid, "body"); err != nil {
			t.Fatalf("create %s: %v", a, err)
		}
		if _, err := db.CreateWikiPage(svc.db, db.WikiKindConcept, b, b, pid, "body"); err != nil {
			t.Fatalf("create %s: %v", b, err)
		}
	}
	scan, err := svc.DiscoverMergeCandidates(pid)
	if err != nil {
		t.Fatalf("DiscoverMergeCandidates: %v", err)
	}
	if scan.Pairs != total {
		t.Fatalf("Pairs = %d, want %d (blocking found them all)", scan.Pairs, total)
	}
	if len(scan.Candidates) != mergeTopK {
		t.Fatalf("Candidates = %d, want %d (top-K truncation)", len(scan.Candidates), mergeTopK)
	}
}

// The human action itself: the service wrapper reaches the merge transaction,
// and the merge is the only writer on this path.
func TestServiceMergeWikiPages(t *testing.T) {
	svc, _ := setupService(t)
	winner, loser, _, _ := mergeFixture(t, svc)

	before, err := db.GetWikiPageByID(svc.db, winner)
	if err != nil {
		t.Fatalf("GetWikiPageByID before: %v", err)
	}
	if err := svc.MergeWikiPages(winner, loser); err != nil {
		t.Fatalf("MergeWikiPages: %v", err)
	}
	after, err := db.GetWikiPageByID(svc.db, winner)
	if err != nil {
		t.Fatalf("GetWikiPageByID after: %v", err)
	}
	if *before != *after {
		t.Fatalf("winner changed by the merge:\n before=%+v\n after =%+v", *before, *after)
	}
	tomb, err := db.GetWikiPageByID(svc.db, loser)
	if err != nil {
		t.Fatalf("GetWikiPageByID loser: %v", err)
	}
	if tomb.Status != db.WikiStatusRejected {
		t.Fatalf("loser status = %q, want rejected", tomb.Status)
	}
}
