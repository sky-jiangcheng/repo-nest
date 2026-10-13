package graph

import (
	"reflect"
	"strings"
	"testing"
)

// ADR-0020 晋升条件 1: the algorithm's own contract.
//
// The interesting assertion in this file is not "does LP find communities" — it is
// TestLabelPropagation_RerunIsByteIdentical, which permutes the INPUT order. LP
// has a well-known source of non-determinism (which node is updated first), and
// the whole reason this implementation is synchronous with a lexicographic
// tie-break is so that idempotence in the service layer is a property of the
// algorithm rather than a lucky artifact of how Go happened to iterate a map.

func clique(prefix string, n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = prefix + string(rune('1'+i))
	}
	return out
}

// complete adds every pair inside the group.
func complete(nodes []string) []Edge {
	var e []Edge
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			e = append(e, Edge{A: nodes[i], B: nodes[j]})
		}
	}
	return e
}

func communityStrings(cs []Community) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Label+"="+strings.Join(c.Members, ","))
	}
	return out
}

func TestLabelPropagation_TwoCliquesOneBridge(t *testing.T) {
	a := clique("a", 5)
	b := clique("b", 5)
	nodes := append(append([]string{}, a...), b...)
	edges := append(complete(a), complete(b)...)
	edges = append(edges, Edge{A: "a1", B: "b1"})

	got, stats := LabelPropagation(nodes, edges, Options{})
	if len(got) != 2 {
		t.Fatalf("two dense groups joined by one edge should give two communities, got %v", communityStrings(got))
	}
	if !stats.Converged {
		t.Fatalf("expected convergence well inside 15 rounds, stats %+v", stats)
	}
	want := map[string][]string{
		"a1": a,
		"b1": b,
	}
	for _, c := range got {
		w, ok := want[c.Label]
		if !ok {
			t.Fatalf("unexpected community label %q (all: %v)", c.Label, communityStrings(got))
		}
		if !reflect.DeepEqual(c.Members, w) {
			t.Fatalf("community %q members = %v, want %v", c.Label, c.Members, w)
		}
		delete(want, c.Label)
	}
	if len(want) != 0 {
		t.Fatalf("missing communities %v", want)
	}
	if stats.Nodes != 10 || stats.Edges != 21 {
		t.Fatalf("stats should describe the input graph, got %+v", stats)
	}
}

// LP's documented weakness, asserted rather than hoped away. Two triangles joined
// by one bridge do NOT come out as "two topics": the bridge node defects to the
// other side, which leaves one community of four and a stranded fragment of two.
// ADR-0020 决策 1 accepts "边界模糊" in the abstract; this is what it costs in a
// concrete case worth knowing before the pass is sold to anyone — with MinSize=3
// the stranded 2-fragment is DISCARDED, so topic B loses its synthesis page
// entirely even though all three of its pages exist and are approved.
//
// The mitigation the service layer therefore has to carry is reporting, not
// silence: RunStats.DroppedSmall is surfaced in the community report precisely so
// "nothing to summarize" can be read as "the graph is fragmented" instead of
// "you have no topics".
func TestLabelPropagation_WeakBridgeBlursTheBoundary(t *testing.T) {
	a := clique("a", 3)
	b := clique("b", 3)
	nodes := append(append([]string{}, a...), b...)
	edges := append(complete(a), complete(b)...)
	edges = append(edges, Edge{A: "a1", B: "b1"})

	got, stats := LabelPropagation(nodes, edges, Options{})
	if len(got) != 1 {
		t.Fatalf("expected the bridge to blur the split into one kept community, got %v", communityStrings(got))
	}
	if !reflect.DeepEqual(got[0].Members, []string{"a1", "a2", "a3", "b1"}) {
		t.Fatalf("the bridge node should have defected into group a, got %v", got[0].Members)
	}
	if stats.DroppedSmall != 1 {
		t.Fatalf("the stranded remainder must be counted, not swallowed: %+v", stats)
	}
	if stats.Converged {
		t.Log("this input converged; the stranding finding stands regardless of convergence")
	}
	// Lowering the floor to 2 makes the lost topic visible again — the exact knob
	// ADR-0020 待决 5 exposes as a config key, and the reason its default matters.
	kept, _ := LabelPropagation(nodes, edges, Options{MinSize: 2})
	if len(kept) != 2 {
		t.Fatalf("MinSize=2 should recover the stranded fragment, got %v", communityStrings(kept))
	}
}

func TestLabelPropagation_MinSizeDropsFragments(t *testing.T) {
	// a-b is a pair, c is alone: neither is a topic.
	nodes := []string{"a", "b", "c"}
	edges := []Edge{{A: "a", B: "b"}}

	got, stats := LabelPropagation(nodes, edges, Options{})
	if len(got) != 0 {
		t.Fatalf("a 2-node group and an isolated node must both be dropped, got %v", communityStrings(got))
	}
	if stats.DroppedSmall != 2 {
		t.Fatalf("expected 2 dropped groups, got %+v", stats)
	}
	if stats.Isolated != 1 {
		t.Fatalf("expected exactly one neighbourless node, got %+v", stats)
	}
	// Lowering the floor to 2 keeps the pair and still drops the singleton.
	got2, stats2 := LabelPropagation(nodes, edges, Options{MinSize: 2})
	if len(got2) != 1 || got2[0].Size() != 2 || stats2.DroppedSmall != 1 {
		t.Fatalf("MinSize=2 should keep the pair only, got %v stats %+v", communityStrings(got2), stats2)
	}
}

// Every node must be accounted for exactly once across kept groups and dropped
// groups — otherwise a threshold bug could quietly swallow pages.
func TestLabelPropagation_MaxSizeDiscardsGiantBlob(t *testing.T) {
	// A single dense group is one community; the ceiling exists for the case where
	// LP has swallowed everything, and the ADR's answer is discard-and-report
	// (决策 2) rather than summarising an arbitrary slice of the topic.
	g := clique("g", 5)
	edges := complete(g)

	kept, stats := LabelPropagation(g, edges, Options{MaxSize: 4})
	if len(kept) != 0 {
		t.Fatalf("an oversized community must be discarded, got %v", communityStrings(kept))
	}
	if stats.DroppedLarge != 1 || stats.CommunitySize != 0 {
		t.Fatalf("the discard must be counted in the report, got %+v", stats)
	}
	whole, stats2 := LabelPropagation(g, edges, Options{MaxSize: 5})
	if len(whole) != 1 || whole[0].Size() != 5 || stats2.DroppedLarge != 0 {
		t.Fatalf("exactly at the ceiling the community is kept, got %v %+v", communityStrings(whole), stats2)
	}
}

func TestLabelPropagation_ConservesNodes(t *testing.T) {
	a := clique("a", 4)
	b := clique("b", 4)
	nodes := append(append(append([]string{}, a...), b...), "lonely")
	edges := append(complete(a), complete(b)...)
	edges = append(edges, Edge{A: "a1", B: "b1"})

	got, _ := LabelPropagation(nodes, edges, Options{MinSize: 1, MaxSize: 100})
	seen := map[string]int{}
	for _, c := range got {
		for _, m := range c.Members {
			seen[m]++
		}
	}
	for _, n := range nodes {
		if seen[n] != 1 {
			t.Fatalf("node %q appears %d times; the partition must cover every node exactly once", n, seen[n])
		}
	}
	if len(seen) != len(nodes) {
		t.Fatalf("partition covers %d of %d nodes", len(seen), len(nodes))
	}
}

// THE determinism contract (晋升条件 1's "同图重跑划分逐字节一致"). Same graph, four
// different input orders, plus duplicated edges and a self-loop thrown in.
func TestLabelPropagation_RerunIsByteIdentical(t *testing.T) {
	a := clique("a", 6)
	b := clique("b", 6)
	c := clique("c", 6)
	nodes := append(append(append([]string{}, a...), b...), c...)
	edges := append(complete(a), complete(b)...)
	edges = append(edges, complete(c)...)
	edges = append(edges, Edge{A: "a1", B: "b1"}, Edge{A: "b2", B: "c1"})
	// Noise that must not change anything.
	noisy := append(append([]Edge{}, edges...), edges[0], edges[1], Edge{A: "a1", B: "a1"})

	base, baseStats := LabelPropagation(nodes, edges, Options{})
	if len(base) == 0 {
		t.Fatal("fixture produced no communities; the test would pass vacuously")
	}

	perm := func(in []string) []string {
		out := make([]string, len(in))
		for i := range in {
			// A fixed but non-trivial permutation.
			out[i] = in[(i*7)%len(in)]
		}
		return out
	}
	revEdges := func(in []Edge) []Edge {
		out := make([]Edge, len(in))
		for i := range in {
			out[i] = in[len(in)-1-i]
			if i%3 == 0 {
				out[i].A, out[i].B = out[i].B, out[i].A // undirected: orientation must not matter
			}
		}
		return out
	}

	for _, trial := range []struct {
		name  string
		nodes []string
		edges []Edge
	}{
		{"reversed nodes", reversed(nodes), edges},
		{"permuted nodes", perm(nodes), edges},
		{"reversed+flipped edges", nodes, revEdges(edges)},
		{"with duplicate edges and a self-loop", perm(nodes), noisy},
		{"with duplicate nodes", append(append([]string{}, nodes...), nodes[0], nodes[len(nodes)/2]), edges},
	} {
		got, stats := LabelPropagation(trial.nodes, trial.edges, Options{})
		if !reflect.DeepEqual(communityStrings(base), communityStrings(got)) {
			t.Fatalf("%s changed the partition:\n base = %v\n trial = %v", trial.name, communityStrings(base), communityStrings(got))
		}
		if stats != baseStats {
			t.Fatalf("%s changed the run stats: %+v vs %+v", trial.name, stats, baseStats)
		}
	}
}

func reversed(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = in[len(in)-1-i]
	}
	return out
}

// 待决 2: the round cap is a real stop, and it is reported rather than hidden.
func TestLabelPropagation_RoundCapIsReported(t *testing.T) {
	a := clique("a", 5)
	b := clique("b", 5)
	nodes := append(append([]string{}, a...), b...)
	edges := append(complete(a), complete(b)...)
	edges = append(edges, Edge{A: "a1", B: "b1"})

	got, stats := LabelPropagation(nodes, edges, Options{MaxRounds: 1})
	if stats.Rounds != 1 || stats.Converged {
		t.Fatalf("one round must report one round and no convergence, got %+v", stats)
	}
	// The result is still a valid partition — stopping early must not corrupt it.
	seen := map[string]bool{}
	for _, c := range got {
		for _, m := range c.Members {
			if seen[m] {
				t.Fatalf("node %q landed in two communities after an early stop", m)
			}
			seen[m] = true
		}
	}
	full, fullStats := LabelPropagation(nodes, edges, Options{})
	if !fullStats.Converged || len(full) != 2 {
		t.Fatalf("the same graph must converge with enough rounds, got %v stats %+v", communityStrings(full), fullStats)
	}
}

func TestLabelPropagation_EdgeCases(t *testing.T) {
	if got, stats := LabelPropagation(nil, nil, Options{}); len(got) != 0 || stats.Nodes != 0 {
		t.Fatalf("empty input must return nothing, got %v %+v", communityStrings(got), stats)
	}
	// A node with no edges at all is a singleton, which MinSize removes.
	got, stats := LabelPropagation([]string{"solo"}, nil, Options{})
	if len(got) != 0 || stats.Isolated != 1 {
		t.Fatalf("an isolated node must not become a community, got %v %+v", communityStrings(got), stats)
	}
	// Edges naming unknown nodes are ignored rather than conjuring nodes into the
	// graph: the caller decides what exists.
	got, stats = LabelPropagation([]string{"x", "y", "z"},
		[]Edge{{A: "x", B: "y"}, {A: "y", B: "z"}, {A: "z", B: "ghost"}, {A: "", B: "x"}},
		Options{MinSize: 2})
	if stats.Edges != 2 {
		t.Fatalf("the ghost and empty endpoints should not be counted, got %+v", stats)
	}
	if len(got) != 1 || got[0].Size() != 3 {
		t.Fatalf("x/y/z form one group, got %v", communityStrings(got))
	}
}

func TestOptionsDefaults(t *testing.T) {
	got := Options{}.withDefaults()
	if got.MaxRounds != DefaultMaxRounds || got.MinSize != DefaultMinSize || got.MaxSize != DefaultMaxSize {
		t.Fatalf("zero options must fall back to the ADR defaults, got %+v", got)
	}
	// A negative is a mistake, not "unlimited": it must not disable a threshold.
	got = Options{MinSize: -5, MaxSize: -1, MaxRounds: 0}.withDefaults()
	if got.MinSize != DefaultMinSize || got.MaxSize != DefaultMaxSize || got.MaxRounds != DefaultMaxRounds {
		t.Fatalf("negative thresholds must fall back too, got %+v", got)
	}
}

func TestClustersOf(t *testing.T) {
	in := []Community{
		{Label: "a1", Members: []string{"a1", "a2"}},
		{Label: "b1", Members: []string{"b1"}},
	}
	m := ClustersOf(in)
	if len(m) != 3 {
		t.Fatalf("expected one entry per member, got %d", len(m))
	}
	if m["a2"].Label != "a1" || m["b1"].Size() != 1 {
		t.Fatalf("lookup returned the wrong community: %+v", m)
	}
}
