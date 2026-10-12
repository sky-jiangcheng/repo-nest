package graph

import (
	"reflect"
	"testing"
)

// names builds the canonical node list the service would hand over: one entry
// per page, in slug order. The tests use "a", "b", ... so a community's index
// slices read as its members.
func names(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	return out
}

func triangle(lo int) [][2]int {
	return [][2]int{{lo, lo + 1}, {lo + 1, lo + 2}, {lo + 2, lo}}
}

// ADR-0020 晋升条件 1: the five named shapes, with the thresholds in play.

// Two communities joined by a single bridge must STAY two communities. This is
// the case that fails under the asynchronous variant (the bridge's smaller
// label sweeps the whole graph), and the reason DetectCommunities updates
// synchronously.
func TestDetectCommunities_TwoCommunitiesOneBridge(t *testing.T) {
	edges := append(triangle(0), triangle(3)...)
	edges = append(edges, [2]int{2, 3})
	got := DetectCommunities(names(6), edges, DefaultOptions())
	want := [][]int{{0, 1, 2}, {3, 4, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// An isolated triangle converges to itself — the base case every other shape
// is built from.
func TestDetectCommunities_IsolatedTriangle(t *testing.T) {
	got := DetectCommunities(names(3), triangle(0), DefaultOptions())
	if want := [][]int{{0, 1, 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A chain is bipartite, so synchronous LP oscillates with period two and never
// converges (决策 1's accepted weakness, made concrete). 待决 2's "不收敛即取
// 当前" then leaves the parity split, and 决策 2's MinSize drops the smaller
// fragment: {0,2,4} survives, {1,3} is too small to be a topic. Pinned so the
// behaviour is a known, guarded artifact rather than a surprise.
func TestDetectCommunities_ChainOscillatesAndThresholdsGuard(t *testing.T) {
	edges := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}}
	got := DetectCommunities(names(5), edges, DefaultOptions())
	if want := [][]int{{0, 2, 4}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// With MinSize lowered, the parity split is visible: {0,2,4} and {1,3}.
	opts := DefaultOptions()
	opts.MinSize = 1
	got = DetectCommunities(names(5), edges, opts)
	if want := [][]int{{0, 2, 4}, {1, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parity split = %v, want %v", got, want)
	}
}

// The giant LP can collapse into (决策 1): a 40-clique is one community, over
// MaxSize, so it is dropped entirely — a 40-page "summary" would be a
// context-budget failure wearing a topic's clothes. The boundary (exactly
// MaxSize) survives.
func TestDetectCommunities_GiantIsDropped(t *testing.T) {
	clique := func(n int) [][2]int {
		var edges [][2]int
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				edges = append(edges, [2]int{i, j})
			}
		}
		return edges
	}
	if got := DetectCommunities(names(40), clique(40), DefaultOptions()); got != nil {
		t.Fatalf("a 40-clique must be dropped as oversized, got %v", got)
	}
	got := DetectCommunities(names(30), clique(30), DefaultOptions())
	if len(got) != 1 || len(got[0]) != 30 {
		t.Fatalf("a 30-clique is exactly MaxSize and must survive, got %v", got)
	}
}

// 决策 2's MinSize: two pages are a pair, not a topic.
func TestDetectCommunities_FragmentsDropped(t *testing.T) {
	if got := DetectCommunities(names(2), [][2]int{{0, 1}}, DefaultOptions()); got != nil {
		t.Fatalf("a 2-node pair must be dropped as a fragment, got %v", got)
	}
	opts := DefaultOptions()
	opts.MinSize = 5
	edges := append(triangle(0), triangle(3)...)
	if got := DetectCommunities(names(6), edges, opts); got != nil {
		t.Fatalf("with MinSize=5 both triangles must be dropped, got %v", got)
	}
	opts.MaxSize = 2
	if got := DetectCommunities(names(6), edges, DefaultOptionsWithMax(t, 2)); got != nil {
		t.Fatalf("with MaxSize=2 both triangles must be dropped, got %v", got)
	}
}

// DefaultOptionsWithMax is a tiny helper for the boundary cases above.
func DefaultOptionsWithMax(t *testing.T, max int) Options {
	t.Helper()
	o := DefaultOptions()
	o.MaxSize = max
	return o
}

// 晋升条件 1's idempotence half: the same graph must produce byte-identical
// partitions, and the partition must not depend on the order the edges arrive
// in (the db layer's row order is not a contract).
func TestDetectCommunities_Deterministic(t *testing.T) {
	edges := append(triangle(0), triangle(3)...)
	edges = append(edges, [2]int{2, 3})
	first := DetectCommunities(names(6), edges, DefaultOptions())
	second := DetectCommunities(names(6), edges, DefaultOptions())
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("re-running moved the partition: %v vs %v", first, second)
	}
	reversed := make([][2]int, 0, len(edges))
	for i := len(edges) - 1; i >= 0; i-- {
		e := edges[i]
		reversed = append(reversed, [2]int{e[1], e[0]})
	}
	if got := DetectCommunities(names(6), reversed, DefaultOptions()); !reflect.DeepEqual(got, first) {
		t.Fatalf("edge order changed the partition: %v vs %v", got, first)
	}
}

// Documented semantics, pinned: malformed edges are ignored, and parallel
// edges count with multiplicity — two ref edges across a bridge outweigh one
// part-of edge and pull the bridge node into the other community. Callers that
// must not have this behaviour dedupe before calling.
func TestDetectCommunities_EdgeSemantics(t *testing.T) {
	edges := append(triangle(0), triangle(3)...)
	edges = append(edges, [2]int{2, 3}, [2]int{0, 0}, [2]int{1, 99})
	if got := DetectCommunities(names(6), edges, DefaultOptions()); !reflect.DeepEqual(got, [][]int{{0, 1, 2}, {3, 4, 5}}) {
		t.Fatalf("self-loops and out-of-range edges must be ignored, got %v", got)
	}
	doubled := append(triangle(0), triangle(3)...)
	doubled = append(doubled, [2]int{2, 3}, [2]int{2, 3})
	got := DetectCommunities(names(6), doubled, DefaultOptions())
	if want := [][]int{{2, 3, 4, 5}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("a doubled bridge must pull node 2 across, got %v want %v", got, want)
	}
}

func TestDetectCommunities_EmptyAndIsolated(t *testing.T) {
	if got := DetectCommunities(nil, nil, DefaultOptions()); got != nil {
		t.Fatalf("empty graph must yield no communities, got %v", got)
	}
	// Isolated nodes are singletons; MinSize drops them.
	if got := DetectCommunities(names(4), nil, DefaultOptions()); got != nil {
		t.Fatalf("isolated nodes must not form communities, got %v", got)
	}
}
