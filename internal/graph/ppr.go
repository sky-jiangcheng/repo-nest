// Package graph holds the pure graph algorithms the wiki layer needs
// (ADR-0018 lane 2). Everything here is deterministic and allocation-only: no
// SQL, no I/O, no clocks — the db layer hands over adjacency, the service
// layer hands over the reset vector, and scores come back.
package graph

import (
	"math"
	"sort"
)

const (
	// pprDamping is the random-surfer restart probability: the share of each
	// round spent walking edges instead of teleporting to the reset vector.
	pprDamping = 0.85
	// pprMaxIter caps the walk. Degenerate graphs (a bare 2-cycle) contract
	// at exactly pprDamping per round and reach the cap, landing within
	// ~1e-7 of the fixed point — ranking does not care about the tail, and
	// the cost is maxIter × edges float operations either way.
	pprMaxIter = 100
	// pprEpsilon is the per-round change below which the walk is declared
	// converged.
	pprEpsilon = 1e-9
)

// PPR runs personalised PageRank on the directed multigraph adj with teleport
// (reset) vector reset, and returns every node's stationary share.
//
// The semantics the callers depend on:
//   - reset is a weight vector, normalised internally. A node absent from it
//     can still score (the walk reaches it) but never teleports.
//   - a node with no outgoing edge is dangling: its mass teleports back into
//     the reset vector. Losing it would make scores depend on how much of the
//     graph happens to be inside the candidate set; spreading it uniformly
//     would let an unrelated page outrank the seed that endorsed it.
//   - parallel edges (two walkable relations between the same pair) carry
//     their multiplicity as weight.
//   - an all-zero or empty reset leaves every node at zero rather than
//     inventing a uniform distribution nobody asked for.
//   - the result is deterministic: nodes and adjacency lists are processed in
//     sorted id order, so map iteration order cannot move a score.
func PPR(adj map[int64][]int64, reset map[int64]float64) map[int64]float64 {
	// The node set is the union of everything mentioned anywhere: an
	// adjacency key with no edges still has a score, and so does a pure
	// target or a reset-only node.
	seen := map[int64]bool{}
	for u, targets := range adj {
		seen[u] = true
		for _, v := range targets {
			seen[v] = true
		}
	}
	for u := range reset {
		seen[u] = true
	}
	if len(seen) == 0 {
		return map[int64]float64{}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// Copy the adjacency into sorted lists so neither the out-degree nor the
	// distribution order depends on how the caller happened to build them.
	walk := make(map[int64][]int64, len(adj))
	for u, targets := range adj {
		if len(targets) == 0 {
			continue
		}
		list := append([]int64(nil), targets...)
		sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
		walk[u] = list
	}

	p := make(map[int64]float64, len(ids))
	total := 0.0
	for _, w := range reset {
		total += w
	}
	if total > 0 {
		for _, id := range ids {
			p[id] = reset[id] / total
		}
	}
	r := p

	for round := 0; round < pprMaxIter; round++ {
		dangling := 0.0
		for _, id := range ids {
			if len(walk[id]) == 0 {
				dangling += r[id]
			}
		}
		teleport := 1 - pprDamping + pprDamping*dangling
		next := make(map[int64]float64, len(ids))
		for _, id := range ids {
			next[id] = teleport * p[id]
		}
		for _, u := range ids {
			targets := walk[u]
			if len(targets) == 0 {
				continue
			}
			share := pprDamping * r[u] / float64(len(targets))
			for _, v := range targets {
				next[v] += share
			}
		}
		delta := 0.0
		for _, id := range ids {
			if d := math.Abs(next[id] - r[id]); d > delta {
				delta = d
			}
		}
		r = next
		if delta < pprEpsilon {
			break
		}
	}
	return r
}
