package db

import (
	"testing"
)

// Expansion follows the whitelist (ref/part-of/documents) and reports hop
// distance. A two-hop chain surfaces the grand-neighbour; a contradicts edge does
// NOT (contradiction is for humans, not for folding into one evidence block).
func TestWikiGraphNeighbors_WhitelistAndHops(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "graph")
	a, _ := CreateWikiPage(db, WikiKindConcept, "a", "A", pid, "a")
	b, _ := CreateWikiPage(db, WikiKindConcept, "b", "B", pid, "b")
	c, _ := CreateWikiPage(db, WikiKindConcept, "c", "C", pid, "c")
	d, _ := CreateWikiPage(db, WikiKindConcept, "d", "D", pid, "d")
	// a -ref-> b -part-of-> c  (c is two hops out)
	if err := LinkWikiPages(db, a.ID, b.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	if err := LinkWikiPages(db, b.ID, c.ID, RelationPartOf); err != nil {
		t.Fatal(err)
	}
	// a -contradicts-> d  (must NOT be expanded)
	if err := LinkWikiPages(db, a.ID, d.ID, RelationContradicts); err != nil {
		t.Fatal(err)
	}

	nbrs, err := WikiGraphNeighbors(db, []int64{a.ID}, 2)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]GraphNeighbor{}
	for _, n := range nbrs {
		byID[n.PageID] = n
	}
	if byID[b.ID].Hops != 1 {
		t.Errorf("b hops = %d, want 1", byID[b.ID].Hops)
	}
	if byID[c.ID].Hops != 2 {
		t.Errorf("c hops = %d, want 2", byID[c.ID].Hops)
	}
	if _, ok := byID[d.ID]; ok {
		t.Errorf("contradicts neighbour d must not be expanded, got %v", byID[d.ID])
	}
}

// A maxHops of 1 must not surface the two-hop node.
func TestWikiGraphNeighbors_HopBound(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "graphhop")
	a, _ := CreateWikiPage(db, WikiKindConcept, "a", "A", pid, "a")
	b, _ := CreateWikiPage(db, WikiKindConcept, "b", "B", pid, "b")
	c, _ := CreateWikiPage(db, WikiKindConcept, "c", "C", pid, "c")
	_ = LinkWikiPages(db, a.ID, b.ID, RelationRef)
	_ = LinkWikiPages(db, b.ID, c.ID, RelationRef)

	nbrs, err := WikiGraphNeighbors(db, []int64{a.ID}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nbrs {
		if n.PageID == c.ID {
			t.Fatalf("two-hop node c surfaced at maxHops=1: %v", nbrs)
		}
	}
}

// A cycle must terminate: a->b->c->a with a generous hop cap still returns a
// finite, bounded result rather than looping. The path guard stops re-entry.
func TestWikiGraphNeighbors_CycleTerminates(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "graphcyc")
	a, _ := CreateWikiPage(db, WikiKindConcept, "a", "A", pid, "a")
	b, _ := CreateWikiPage(db, WikiKindConcept, "b", "B", pid, "b")
	c, _ := CreateWikiPage(db, WikiKindConcept, "c", "C", pid, "c")
	_ = LinkWikiPages(db, a.ID, b.ID, RelationRef)
	_ = LinkWikiPages(db, b.ID, c.ID, RelationRef)
	_ = LinkWikiPages(db, c.ID, a.ID, RelationRef) // cycle

	nbrs, err := WikiGraphNeighbors(db, []int64{a.ID}, 4)
	if err != nil {
		t.Fatal(err)
	}
	// The seed itself is excluded; b and c are each reachable once.
	seen := map[int64]bool{}
	for _, n := range nbrs {
		if n.PageID == a.ID {
			t.Error("the seed must be excluded from its own expansion")
		}
		if seen[n.PageID] {
			t.Errorf("node %d reported more than once", n.PageID)
		}
		seen[n.PageID] = true
	}
	if len(seen) != 2 {
		t.Fatalf("cycle expansion = %v, want exactly b and c", nbrs)
	}
}

// A pending page must never be pulled in by expansion: an edge a->pending is not
// followed, so unreviewed content cannot enter evidence (ADR-0015).
func TestWikiGraphNeighbors_SkipsPendingPages(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "graphpend")
	a, _ := CreateWikiPage(db, WikiKindConcept, "a", "A", pid, "a")
	pend, _ := CreateWikiPageAs(db, WikiKindConcept, "pend", "Pend", pid, "draft", WikiStatusPending, "wiki-compile")
	if err := LinkWikiPages(db, a.ID, pend.ID, RelationRef); err != nil {
		t.Fatal(err)
	}
	nbrs, err := WikiGraphNeighbors(db, []int64{a.ID}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nbrs {
		if n.PageID == pend.ID {
			t.Fatalf("pending page %d pulled into expansion: %v", pend.ID, nbrs)
		}
	}
}

// Empty seeds is a normal no-op, not an error.
func TestWikiGraphNeighbors_EmptySeeds(t *testing.T) {
	db := setupTestDB(t)
	nbrs, err := WikiGraphNeighbors(db, nil, 2)
	if err != nil {
		t.Fatalf("empty seeds errored: %v", err)
	}
	if len(nbrs) != 0 {
		t.Fatalf("empty seeds = %v, want none", nbrs)
	}
}

// WikiAdjacency yields a closed subgraph over the whitelist for PPR: only
// whitelisted, approved edges, and only among reachable nodes.
func TestWikiAdjacency_ClosedSubgraph(t *testing.T) {
	db := setupTestDB(t)
	pid := createTestProject(t, db, "graphadj")
	a, _ := CreateWikiPage(db, WikiKindConcept, "a", "A", pid, "a")
	b, _ := CreateWikiPage(db, WikiKindConcept, "b", "B", pid, "b")
	// Unrelated island, not reachable from a: must not appear in a's subgraph.
	x, _ := CreateWikiPage(db, WikiKindConcept, "x", "X", pid, "x")
	y, _ := CreateWikiPage(db, WikiKindConcept, "y", "Y", pid, "y")
	_ = LinkWikiPages(db, a.ID, b.ID, RelationRef)
	_ = LinkWikiPages(db, x.ID, y.ID, RelationRef)

	adj, err := WikiAdjacency(db, []int64{a.ID}, 2, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(adj[a.ID]) != 1 || adj[a.ID][0] != b.ID {
		t.Errorf("adj[a] = %v, want [b]", adj[a.ID])
	}
	if _, ok := adj[x.ID]; ok {
		t.Errorf("unreachable island node x leaked into the subgraph: %v", adj)
	}
}
