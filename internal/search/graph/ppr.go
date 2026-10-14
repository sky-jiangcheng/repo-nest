// Package graph holds the pure graph algorithms ADR-0018 lane 2 needs, kept free
// of SQLite and the service layer so they are unit-testable as pure functions over
// ranked id lists and an adjacency map — no DB, no network, no embedding.
//
// Today it is one algorithm: Personalized PageRank (PPR) with reset, used to rank
// wiki pages for evidence after a seed set is recalled. PPR is a random walk that,
// at each step, either follows an out-edge (distributing the current mass) or
// teleports back to a reset node (with probability 1-damping). With a reset vector
// concentrated on the retrieval hits, the stationary distribution ranks pages by
// how reachable they are FROM the hits along the allowed edges — which is what
// "graph-aware evidence" means: a hit's neighbours, two hops out, surface without
// having matched the query themselves.
package graph

// Adjacency is a directed graph keyed by node id: neighbors[u] = the nodes u has
// an out-edge to. Weights are folded in by the caller (a repeated entry = a
// stronger edge) so this stays a plain multiset adjacency and the algorithm does
// not need to know what an edge "means".
type Adjacency map[int64][]int64

// PPR runs Personalized PageRank with reset over `adj`, seeded by `reset`, for up
// to `iterations` power-iteration steps, and returns the top `topK` node ids by
// stationary score (descending). It is deterministic: ties break by lower id.
//
// Semantics, and why each parameter exists:
//   - damping (d): probability of following an out-edge vs teleporting back to a
//     reset node. 0.85 is the classic PageRank value; lower = the walk stays closer
//     to the seeds, higher = it drifts further into the graph. d<=0 or d>=1 is
//     rejected because it makes the walk degenerate (never teleport / always
//     teleport).
//   - reset: the personalization vector — where the walk teleports to. For
//     evidence this is the retrieval hit set (each seed weighted equally). Nodes
//     not in reset only ever accrue score by being reached FROM a seed.
//   - A dangling node (no out-edges) redistributes its mass uniformly over the
//     whole reset set each step, the standard fix so mass is never lost when the
//     walk hits a sink.
//
// Convergence: for a personal-knowledge-base graph (hundreds to a few thousand
// pages) this converges well within ~15-30 iterations to a stable ordering, far
// inside the 3s EvidenceBudget deadline. iterations<=0 falls back to a small
// default. topK<=0 returns every node with non-zero score, best-first.
func PPR(adj Adjacency, reset map[int64]float64, damping float64, iterations, topK int) []int64 {
	if damping <= 0 || damping >= 1 {
		damping = 0.85
	}
	if iterations <= 0 {
		iterations = 15
	}
	// Normalize the reset vector to a probability distribution.
	total := 0.0
	for _, w := range reset {
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return nil
	}
	tele := make(map[int64]float64, len(reset))
	for id, w := range reset {
		if w > 0 {
			tele[id] = w / total
		}
	}

	// The node universe is every node that appears as a key, a neighbour, or a seed,
	// so an isolated seed still gets ranked and a neighbour of nothing is not lost.
	nodeSet := map[int64]bool{}
	for u, nbrs := range adj {
		nodeSet[u] = true
		for _, v := range nbrs {
			nodeSet[v] = true
		}
	}
	for id := range tele {
		nodeSet[id] = true
	}

	// Out-degree over the (multi)edge set: a node with two edges to the same
	// neighbour hands each edge half its mass, which is how a doubled edge becomes
	// a stronger one without a separate weight table.
	outDeg := make(map[int64]float64, len(adj))
	for u, nbrs := range adj {
		outDeg[u] = float64(len(nbrs))
	}

	rank := make(map[int64]float64, len(nodeSet))
	for id := range nodeSet {
		rank[id] = 0
	}

	for it := 0; it < iterations; it++ {
		next := make(map[int64]float64, len(nodeSet))
		// Teleport mass: every node receives (1-damping) share of the reset vector.
		for id := range nodeSet {
			next[id] = (1 - damping) * tele[id]
		}
		// Dangling mass: nodes with no out-edges spray their whole rank across the
		// reset set, so a sink does not swallow probability.
		dangling := 0.0
		for id := range nodeSet {
			if outDeg[id] == 0 {
				dangling += rank[id]
			}
		}
		if dangling > 0 {
			for id := range nodeSet {
				next[id] += damping * dangling * tele[id]
			}
		}
		// Follow out-edges: split each node's rank across its neighbours.
		for u := range nodeSet {
			ru := rank[u]
			if ru == 0 || outDeg[u] == 0 {
				continue
			}
			share := damping * ru / outDeg[u]
			for _, v := range adj[u] {
				next[v] += share
			}
		}
		rank = next
	}

	return rankTopK(rank, topK)
}

// rankTopK returns node ids ordered by score desc, ties broken by lower id for a
// deterministic result. topK<=0 returns all non-zero-score nodes.
func rankTopK(rank map[int64]float64, topK int) []int64 {
	type scored struct {
		id    int64
		score float64
	}
	all := make([]scored, 0, len(rank))
	for id, s := range rank {
		if s > 0 {
			all = append(all, scored{id, s})
		}
	}
	// Insertion sort: graphs here are small (hundreds to low thousands of nodes),
	// and it avoids importing sort for a list that is already near-sorted by id.
	for i := 1; i < len(all); i++ {
		cur := all[i]
		j := i - 1
		for j >= 0 && lessScored(cur, all[j]) {
			all[j+1] = all[j]
			j--
		}
		all[j+1] = cur
	}
	if topK > 0 && len(all) > topK {
		all = all[:topK]
	}
	out := make([]int64, len(all))
	for i, e := range all {
		out[i] = e.id
	}
	return out
}

// lessScored orders by higher score first, then lower id (deterministic tie-break).
func lessScored(a, b struct {
	id    int64
	score float64
}) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	return a.id < b.id
}
