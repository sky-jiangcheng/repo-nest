package graph

import (
	"math"
	"reflect"
	"testing"
)

// PPR is the ranking half of graph-aware evidence (ADR-0018 lane 2): a
// personalised random walk whose reset vector is the retrieval hits. These
// tests pin the semantics the service layer relies on — damping, the dangling
// policy, determinism — against closed forms rather than golden output.

// With the reset entirely on the hub and the sink dangling, the fixed point is
// hub = (1-d)/(1-d²) and sink = d·hub when dangling mass teleports back into
// the reset vector. Spreading it uniformly instead would put the hub near
// 0.309, so the numbers below decide the dangling policy, not merely "it
// converges". The tolerance is the contraction budget: a bare 2-cycle
// contracts at exactly d per round, so the iteration cap lands within ~1e-7
// of the fixed point.
func TestPPR_MatchesClosedFormOnTwoNodes(t *testing.T) {
	got := PPR(map[int64][]int64{1: {2}}, map[int64]float64{1: 1})
	if diff := got[1] - 0.5405405405; math.Abs(diff) > 1e-6 {
		t.Errorf("hub = %v, want 0.5405405 (closed form)", got[1])
	}
	if diff := got[2] - 0.4594594595; math.Abs(diff) > 1e-6 {
		t.Errorf("sink = %v, want 0.4594595 (closed form)", got[2])
	}
}

// Two seeds endorsing one target: the target must outrank each seed. That is
// the whole point of reading the graph — a page whose body never matched the
// query can still be the page every relevant hit agrees on. The reset is
// given as weights (1, 1), not a distribution, so this also pins the internal
// normalisation.
func TestPPR_EndorsedTargetOutranksEachSeed(t *testing.T) {
	got := PPR(map[int64][]int64{1: {3}, 2: {3}}, map[int64]float64{1: 1, 2: 1})
	if got[3] <= got[1] || got[3] <= got[2] {
		t.Errorf("endorsed target %v did not outrank the seeds (%v / %v)", got[3], got[1], got[2])
	}
	if diff := got[3] - 0.4594594595; math.Abs(diff) > 1e-6 {
		t.Errorf("endorsed target = %v, want 0.4594595 (closed form)", got[3])
	}
	if diff := got[1] - 0.2702702703; math.Abs(diff) > 1e-6 {
		t.Errorf("seed = %v, want 0.2702703 (closed form)", got[1])
	}
	if got[1] != got[2] {
		t.Errorf("symmetric seeds scored differently: %v vs %v", got[1], got[2])
	}
}

// No edges at all: the walk has nothing to say, so the result must be exactly
// the reset distribution. The service layer depends on this — with a silent
// graph every candidate keeps its pre-walk order through the score
// tie-break, so enabling the graph can never scramble a pure FTS ranking.
func TestPPR_NoEdgesKeepsResetDistribution(t *testing.T) {
	got := PPR(nil, map[int64]float64{7: 3, 9: 1})
	if diff := got[7] - 0.75; math.Abs(diff) > 1e-12 {
		t.Errorf("node 7 = %v, want 0.75 (normalised reset)", got[7])
	}
	if diff := got[9] - 0.25; math.Abs(diff) > 1e-12 {
		t.Errorf("node 9 = %v, want 0.25 (normalised reset)", got[9])
	}
}

// Parallel edges are the only edge weight the walk has: two walkable
// relations between the same pair double that pair's share of the outflow
// (multiplicity is weight — there is no separate weight table until abeval
// asks for one).
func TestPPR_ParallelEdgesCarryWeight(t *testing.T) {
	got := PPR(map[int64][]int64{1: {2, 2, 3}}, map[int64]float64{1: 1})
	if diff := got[2] - 2*got[3]; math.Abs(diff) > 1e-9 {
		t.Errorf("doubly-linked target %v, want exactly twice the single one %v", got[2], got[3])
	}
}

// Mass conservation is the invariant that makes scores comparable at all:
// every round redistributes exactly what the previous round held, dangling
// mass included. An empty adjacency list is a dangling node, same as a
// missing key.
func TestPPR_ConservesMass(t *testing.T) {
	adj := map[int64][]int64{1: {2, 3}, 2: {3}, 3: {}, 4: {1}}
	got := PPR(adj, map[int64]float64{1: 1, 4: 1})
	sum := 0.0
	for _, v := range got {
		sum += v
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("total mass = %v, want 1", sum)
	}
}

// Determinism is a requirement, not luck: Go randomises map iteration order,
// so the same graph must score identically run after run, and identically
// again when its adjacency lists and reset arrive permuted. The service
// layer's tie-break assumes it.
func TestPPR_DeterministicRegardlessOfInputOrder(t *testing.T) {
	base := PPR(
		map[int64][]int64{1: {2, 3}, 2: {3, 4}, 3: {4}, 4: {1}},
		map[int64]float64{1: 1, 2: 1},
	)
	permuted := PPR(
		map[int64][]int64{4: {1}, 3: {4}, 2: {4, 3}, 1: {3, 2}},
		map[int64]float64{2: 1, 1: 1},
	)
	for i := 0; i < 5; i++ {
		again := PPR(
			map[int64][]int64{1: {2, 3}, 2: {3, 4}, 3: {4}, 4: {1}},
			map[int64]float64{1: 1, 2: 1},
		)
		if !reflect.DeepEqual(base, again) {
			t.Fatalf("run %d of the same input differs:\n%v\n%v", i, base, again)
		}
	}
	if !reflect.DeepEqual(base, permuted) {
		t.Errorf("permuted input scored differently:\n%v\n%v", base, permuted)
	}
}

// Empty in, empty out: a caller with nothing to rank gets an empty map, not
// a panic and not a fabricated uniform distribution.
func TestPPR_EmptyInput(t *testing.T) {
	if got := PPR(nil, nil); len(got) != 0 {
		t.Errorf("PPR(nil, nil) = %v, want empty", got)
	}
}
