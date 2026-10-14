package service

import (
	"testing"
	"time"

	"repotory/internal/db"
)

// The acceptance bar ADR-0018 lane 2 sets for the OFF state: with wiki_graph_search
// off (the default), evidencePages returns the exact FTS ordering it always has.
// This is the zero-regression guarantee — enabling the feature later must not
// change today's answers, so the off path is proven identical here.
func TestEvidencePages_GraphOffIsFTSPure(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "goff", "/tmp/goff")
	// A hit and a neighbour linked to it; with the switch off the neighbour must
	// NOT appear (no expansion), only the FTS hit.
	hit, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "hit", "alpha token", pid, "alpha token body")
	nbr, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "nbr", "neighbour", pid, "unrelated body")
	if err := db.LinkWikiPages(svc.db, hit.ID, nbr.ID, db.RelationRef); err != nil {
		t.Fatal(err)
	}

	pages := svc.evidencePages("alpha token", pid, 20, 3*time.Second)
	if len(pages) != 1 || pages[0].ID != hit.ID {
		t.Fatalf("graph-off evidence = %v, want exactly the FTS hit %d (no expansion)", pageItemIDs(pages), hit.ID)
	}
}

// With the switch ON, a structurally adjacent page that never matched the query can
// surface — and crucially the original FTS hit is still present (a superset, never
// a subset: enabling graph search cannot drop a hit).
func TestEvidencePages_GraphOnExpands(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "gon", "/tmp/gon")
	if err := svc.UpdateConfig("wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	hit, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "hit", "alpha token", pid, "alpha token body")
	nbr, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "nbr", "neighbour", pid, "completely different words")
	if err := db.LinkWikiPages(svc.db, hit.ID, nbr.ID, db.RelationRef); err != nil {
		t.Fatal(err)
	}

	pages := svc.evidencePages("alpha token", pid, 20, 3*time.Second)
	ids := pageItemIDs(pages)
	if !containsInt64(ids, hit.ID) {
		t.Fatalf("graph-on evidence dropped the FTS hit %d: %v (must be a superset)", hit.ID, ids)
	}
	if !containsInt64(ids, nbr.ID) {
		t.Fatalf("graph-on evidence did not surface the linked neighbour %d: %v", nbr.ID, ids)
	}
}

// A contradicts edge must NOT expand: contradiction is shown to a human, not folded
// into the same evidence block as the thing it disputes.
func TestEvidencePages_GraphOnIgnoresContradicts(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "gcontra", "/tmp/gcontra")
	if err := svc.UpdateConfig("wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	hit, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "hit", "alpha token", pid, "alpha token body")
	dispute, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "dispute", "counter", pid, "opposite claim")
	if err := db.LinkWikiPages(svc.db, hit.ID, dispute.ID, db.RelationContradicts); err != nil {
		t.Fatal(err)
	}

	pages := svc.evidencePages("alpha token", pid, 20, 3*time.Second)
	if containsInt64(pageItemIDs(pages), dispute.ID) {
		t.Fatalf("a contradicts neighbour was pulled into evidence: %v", pageItemIDs(pages))
	}
}

// A pending neighbour must never be expanded into evidence, even with the switch on
// (ADR-0015: unreviewed content never answers a question).
func TestEvidencePages_GraphOnSkipsPendingNeighbour(t *testing.T) {
	svc, _ := setupService(t)
	pid := seedProject(t, svc.db, "gpend", "/tmp/gpend")
	if err := svc.UpdateConfig("wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	hit, _ := db.CreateWikiPage(svc.db, db.WikiKindConcept, "hit", "alpha token", pid, "alpha token body")
	pend, _ := db.CreateWikiPageAs(svc.db, db.WikiKindConcept, "pend", "draft", pid, "draft", db.WikiStatusPending, "wiki-compile")
	if err := db.LinkWikiPages(svc.db, hit.ID, pend.ID, db.RelationRef); err != nil {
		t.Fatal(err)
	}
	pages := svc.evidencePages("alpha token", pid, 20, 3*time.Second)
	if containsInt64(pageItemIDs(pages), pend.ID) {
		t.Fatalf("pending neighbour pulled into evidence: %v", pageItemIDs(pages))
	}
}

// graphSearchEnabled must be OFF unless explicitly "1" — the safe state is default.
func TestGraphSearchEnabled_DefaultOff(t *testing.T) {
	svc, _ := setupService(t)
	if svc.graphSearchEnabled() {
		t.Fatal("graph search enabled on a fresh database; it must default OFF")
	}
	if err := svc.UpdateConfig("wiki_graph_search", "1"); err != nil {
		t.Fatal(err)
	}
	if !svc.graphSearchEnabled() {
		t.Fatal("graph search not enabled after setting the key to 1")
	}
}

func pageItemIDs(items []EvidenceItem) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func containsInt64(xs []int64, want int64) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
