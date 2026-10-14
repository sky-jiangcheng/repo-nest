package service

import (
	"log"

	"repotory/internal/db"
	"repotory/internal/search/graph"
)

// Graph-aware page evidence (ADR-0018 lane 2, schema v23). The consumer half: when
// wiki_graph_search is on, AskAI's page evidence is expanded from the FTS/vector
// hits along ref/part-of/documents edges and re-ranked with PageRank, so a page
// that is structurally adjacent to a hit — but never matched the query itself —
// can still be cited. When the switch is OFF (the default), evidencePages returns
// the exact FTS ordering it always has: zero behavior regression, which is the
// acceptance bar ADR-0018 lane 2 sets for the off state.
//
// The design keeps the two index spaces apart: page ids are ranked with page-vector
// recall and PPR inside the page id space only, and the result is handed back as an
// ordered []EvidenceItem that GatherEvidence interleaves with notes by position —
// never fused into one RRF with note ids (the collision trap). See wiki_evidence.go.

// wikiGraphSearchKey gates graph-aware evidence. Default OFF.
const wikiGraphSearchKey = "wiki_graph_search"

// graphSearchEnabled reports whether graph-aware page evidence is on. Anything
// other than "1" (including unset) keeps it OFF, so the safe state is the default.
func (s *Service) graphSearchEnabled() bool {
	v, err := db.GetConfig(s.db, wikiGraphSearchKey)
	return err == nil && v == "1"
}

// graphExpandHops is how far expansion walks. 1 hop by default (ADR-0018 待决 5:
// "default 1 hop, 2 only after abeval shows a gain"), with 2 available as a tuned
// value. Kept a const so the abeval gate can widen it in one place.
const graphExpandHops = 1

// graphMaxNeighbors bounds the expansion so a hub page with hundreds of approved
// neighbours cannot flood the evidence budget. The triple budget in GatherEvidence
// is the real backstop; this is the per-query input cap.
const graphMaxNeighbors = 50

// graphAdjacencyEdgeLimit bounds the subgraph loaded for PPR, so a pathological hub
// cannot pull an unbounded graph into memory inside the 3s deadline.
const graphAdjacencyEdgeLimit = 2000

// expandPageEvidence takes the FTS-ordered page hits (the seed set) and, when graph
// search is on, returns a re-ranked list that includes graph neighbours. It is a
// pure reordering over page ids; the caller maps ids back to EvidenceItem.
//
// When the switch is off, or expansion/PPR yields nothing useful, it returns the
// input order unchanged — the caller cannot tell the difference, which is exactly
// the zero-regression guarantee. Errors are logged and degrade to the input order:
// graph evidence is an enrichment, and a failure must never turn AskAI into an
// error (the same "never errors" contract GatherEvidence holds).
func (s *Service) expandPageEvidence(seedIDs []int64) []int64 {
	if !s.graphSearchEnabled() || len(seedIDs) == 0 {
		return seedIDs
	}
	nbrs, err := db.WikiGraphNeighbors(s.db, seedIDs, graphExpandHops)
	if err != nil {
		log.Printf("wiki graph evidence: expansion failed, using FTS order: %v", err)
		return seedIDs
	}
	if len(nbrs) == 0 {
		return seedIDs // an isolated hit has no neighbours; FTS order stands
	}
	if len(nbrs) > graphMaxNeighbors {
		nbrs = nbrs[:graphMaxNeighbors]
	}

	// Build the reset vector: the retrieval hits are the personalization seeds,
	// each weighted equally. Neighbours are NOT in the reset — they only accrue
	// score by being reached from a seed, which is what makes "adjacent to a hit"
	// rank below "is a hit".
	reset := make(map[int64]float64, len(seedIDs))
	for _, id := range seedIDs {
		reset[id] = 1
	}
	adj, err := db.WikiAdjacency(s.db, seedIDs, graphExpandHops, graphAdjacencyEdgeLimit)
	if err != nil {
		log.Printf("wiki graph evidence: adjacency load failed, using FTS order: %v", err)
		return seedIDs
	}
	ranked := graph.PPR(adj, reset, 0.85, 20, len(seedIDs)+len(nbrs))
	if len(ranked) == 0 {
		return seedIDs
	}

	// The PPR order is the new page order, but we must not DROP a seed that PPR
	// ranked poorly (a hit is a hit regardless of graph position). Merge: PPR order
	// first, then any seed PPR did not surface, then any neighbour PPR did not
	// surface — so the result is a superset of the FTS hits, re-ordered, never a
	// subset. This keeps "enabling graph search cannot return fewer page hits".
	seen := make(map[int64]bool, len(ranked))
	out := make([]int64, 0, len(ranked)+len(seedIDs))
	for _, id := range ranked {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range seedIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, n := range nbrs {
		if !seen[n.PageID] {
			seen[n.PageID] = true
			out = append(out, n.PageID)
		}
	}
	return out
}
