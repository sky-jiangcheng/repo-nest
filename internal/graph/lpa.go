package graph

import "sort"

// Label Propagation community detection (ADR-0020 决策 1/3).
//
// The wiki layer has five page kinds but no systematic generator for the
// synthesis kind (ADR-0020 背景): the compiler sees one note at a time and
// cannot write an overview, so no-synthesis lint stays red forever. This is the
// algorithm that closes the loop — find the topic communities, let a model
// draft one overview per community, and a human approves it.
//
// LP is chosen over Louvain/Leiden deliberately (决策 1): dozens of lines, zero
// dependencies, zero CGO, auditable. Its two known weaknesses (fuzzy borders,
// collapsing into a giant) are accepted and guarded by size thresholds
// (决策 2) — this file does not pretend otherwise.
//
// Determinism is the hard requirement, stronger than community quality: the
// same graph must produce byte-identical partitions, or the idempotence test
// for the synthesis pages (决策 7) is meaningless. So:
//
//   - nodes must be passed in canonical order (the service sorts by slug);
//     initial labels are the nodes' own positions in that order;
//   - ties take the smallest label — which, given canonical input, means the
//     slug-lexicographically smallest member wins;
//   - updates are SYNCHRONOUS: every round computes from the previous round's
//     labels. The classic Raghavan variant is asynchronous, but an in-order
//     asynchronous sweep merges anything connected into one giant community
//     (hand-simulated on the two-triangles-one-bridge case: the bridge's
//     smaller label sweeps across), which would make this algorithm useless on
//     a real wiki graph where everything is linked. The price of synchronous
//     updates is that bipartite structures (every chain, every tree) oscillate
//     with period two and never converge — ADR-0020 待决 2's "不收敛即取当前"
//     is exactly this case, and the size thresholds are what keep the
//     resulting parity split from becoming synthesis pages.

// lpaDefaultRounds is ADR-0020 待决 2's ≤15-round cap.
const lpaDefaultRounds = 15

// Options tunes one detection run. The zero value is not usable — use
// DefaultOptions.
type Options struct {
	// MaxRounds caps propagation. Non-convergent graphs (bipartite
	// structures) stop here and keep whatever labels they have.
	MaxRounds int
	// MinSize drops fragments: fewer than this many pages is not a topic, and
	// a summary of them would be forced (ADR-0020 决策 2).
	MinSize int
	// MaxSize drops LP's degenerate giants — the collapse 决策 1 accepts —
	// which would also blow the model's context budget.
	MaxSize int
}

// DefaultOptions is ADR-0020's 决策 2/待决 2: ≤15 rounds, drop <3, drop >30.
func DefaultOptions() Options {
	return Options{MaxRounds: lpaDefaultRounds, MinSize: 3, MaxSize: 30}
}

// DetectCommunities runs synchronous Label Propagation over the undirected
// graph described by nodes and edges, and returns the surviving communities as
// sorted index slices (each community sorted ascending, the list sorted by
// first member).
//
// The contract callers depend on:
//
//   - nodes MUST be in canonical order (slug-sorted by the caller). The
//     partition's tie-breaks are "smallest label", i.e. smallest position —
//     any other order silently changes the outcome;
//   - edges are index pairs into nodes; the graph is treated as undirected
//     (a link says "these two pages are about the same thing", direction is
//     the relation vocabulary's business, not the partition's);
//   - malformed edges (out of range, self-loops) are ignored rather than
//     trusted, but PARALLEL EDGES COUNT MULTIPLY — dedupe before calling, or
//     two ref edges between a pair will outweigh one part-of edge;
//   - the result is a pure function of (nodes, edges, opts): no clocks, no
//     maps iterated for a decision, no randomness.
func DetectCommunities(nodes []string, edges [][2]int, opts Options) [][]int {
	if opts.MaxRounds <= 0 {
		opts.MaxRounds = lpaDefaultRounds
	}
	n := len(nodes)
	if n == 0 {
		return nil
	}
	adj := make([][]int, n)
	for _, e := range edges {
		a, b := e[0], e[1]
		if a == b || a < 0 || b < 0 || a >= n || b >= n {
			continue
		}
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}

	labels := make([]int, n)
	for i := range labels {
		labels[i] = i
	}
	for round := 0; round < opts.MaxRounds; round++ {
		next := make([]int, n)
		changed := false
		for i := 0; i < n; i++ {
			if len(adj[i]) == 0 {
				// Isolated node: its own label, a singleton community the
				// size thresholds will drop.
				next[i] = labels[i]
				continue
			}
			counts := make(map[int]int, len(adj[i]))
			for _, j := range adj[i] {
				counts[labels[j]]++
			}
			// Most frequent neighbour label; ties take the smallest. The map
			// iteration order cannot affect this: the comparison is a total
			// order over (count, label).
			best, bestCount := 0, -1
			for label, c := range counts {
				if c > bestCount || (c == bestCount && label < best) {
					best, bestCount = label, c
				}
			}
			next[i] = best
			if best != labels[i] {
				changed = true
			}
		}
		labels = next
		if !changed {
			break
		}
	}

	groups := map[int][]int{}
	for i, l := range labels {
		groups[l] = append(groups[l], i)
	}
	out := make([][]int, 0, len(groups))
	for _, g := range groups {
		sort.Ints(g)
		if len(g) < opts.MinSize || len(g) > opts.MaxSize {
			continue
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
