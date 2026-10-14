package graph

import (
	"reflect"
	"testing"
)

// A seed with a chain of neighbours: the seed ranks first, its direct neighbour
// next, and a node two hops out still accrues score (that two-hop reachability is
// the whole reason graph-aware evidence exists).
func TestPPR_RanksNeighboursByReachability(t *testing.T) {
	adj := Adjacency{
		1: {2},
		2: {3},
		3: {},
	}
	ranked := PPR(adj, map[int64]float64{1: 1}, 0.85, 30, 0)
	if len(ranked) != 3 {
		t.Fatalf("ranked = %v, want all three nodes", ranked)
	}
	if ranked[0] != 1 {
		t.Errorf("seed 1 should rank first, got %v", ranked)
	}
	if ranked[1] != 2 {
		t.Errorf("direct neighbour 2 should rank second, got %v", ranked)
	}
	if ranked[2] != 3 {
		t.Errorf("two-hop node 3 should still rank, got %v", ranked)
	}
}

// topK truncates best-first.
func TestPPR_TopK(t *testing.T) {
	adj := Adjacency{1: {2}, 2: {3}, 3: {}}
	ranked := PPR(adj, map[int64]float64{1: 1}, 0.85, 30, 1)
	if !reflect.DeepEqual(ranked, []int64{1}) {
		t.Fatalf("topK=1 = %v, want [1]", ranked)
	}
}

// A dangling node (no out-edges) must not swallow probability: with the uniform
// dangling fix, mass that reaches the sink is sprayed back over the reset set, so
// the seed keeps a large share and the walk does not collapse onto the sink.
func TestPPR_DanglingDoesNotSwallowMass(t *testing.T) {
	adj := Adjacency{
		1: {2},
		2: {}, // sink
	}
	ranked := PPR(adj, map[int64]float64{1: 1}, 0.85, 40, 0)
	if len(ranked) != 2 {
		t.Fatalf("ranked = %v, want both nodes", ranked)
	}
	if ranked[0] != 1 {
		t.Errorf("seed should stay on top despite the sink, got %v", ranked)
	}
}

// A cycle must terminate (the iteration cap bounds it) and must not panic or loop
// forever. The result just needs to be a stable, finite ranking.
func TestPPR_CycleTerminates(t *testing.T) {
	adj := Adjacency{
		1: {2},
		2: {3},
		3: {1}, // cycle back to the seed
	}
	ranked := PPR(adj, map[int64]float64{1: 1}, 0.85, 20, 0)
	if len(ranked) != 3 {
		t.Fatalf("ranked = %v, want the three cycle nodes", ranked)
	}
}

// Determinism: two runs over the same inputs return the identical ordering, so a
// tie never reorders evidence between calls (map iteration order must not leak).
func TestPPR_Deterministic(t *testing.T) {
	adj := Adjacency{
		1: {2, 3, 4},
		2: {5},
		3: {5},
		4: {5},
		5: {},
	}
	a := PPR(adj, map[int64]float64{1: 1}, 0.85, 25, 0)
	b := PPR(adj, map[int64]float64{1: 1}, 0.85, 25, 0)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("two runs differ: %v vs %v", a, b)
	}
}

// An empty reset means there is nothing to personalise toward, so there is no
// meaningful ranking — return nothing rather than a uniform ranking of everything.
func TestPPR_EmptyResetReturnsNothing(t *testing.T) {
	adj := Adjacency{1: {2}, 2: {}}
	if ranked := PPR(adj, nil, 0.85, 10, 0); ranked != nil {
		t.Fatalf("empty reset = %v, want nil", ranked)
	}
	if ranked := PPR(adj, map[int64]float64{1: 0}, 0.85, 10, 0); ranked != nil {
		t.Fatalf("all-zero reset = %v, want nil", ranked)
	}
}

// A doubled edge is a stronger edge: node 1 has two edges to node 2 and one to
// node 3, so 2 should out-rank 3 for equal damping.
func TestPPR_MultiEdgeIsStronger(t *testing.T) {
	adj := Adjacency{
		1: {2, 2, 3},
		2: {},
		3: {},
	}
	ranked := PPR(adj, map[int64]float64{1: 1}, 0.85, 40, 0)
	// Find positions of 2 and 3.
	pos := map[int64]int{}
	for i, id := range ranked {
		pos[id] = i
	}
	if pos[2] > pos[3] {
		t.Errorf("doubled edge to 2 should out-rank single edge to 3: %v", ranked)
	}
}
