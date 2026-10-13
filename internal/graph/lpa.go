// Package graph holds the structural algorithms the knowledge layer runs over
// the page graph. It deliberately knows nothing about the database: input is a
// set of node keys and an edge list, output is a partition. Same discipline as
// internal/search/hybrid — the algorithm gets tested on fixtures, and the
// service layer is the only thing that touches SQL.
package graph

import (
	"sort"
	"strings"
)

// Defaults from ADR-0020 决策 2 and 待决 2, exposed so callers can show them in
// the UI rather than hardcoding a second copy of the numbers.
const (
	DefaultMaxRounds = 15
	DefaultMinSize   = 3
	DefaultMaxSize   = 30
)

// Edge is an undirected pair of node keys. Keys are opaque to this package and
// only ever compared and sorted, so callers should pass something stable and
// human-meaningful — a page slug, not a database row id — because the
// determinism guarantee below is stated in terms of that key.
type Edge struct {
	A string
	B string
}

// Community is one label group. Members is always sorted lexicographically, so
// two runs that produce the same partition produce the same bytes.
type Community struct {
	Label   string
	Members []string
}

// Size is len(Members); a method so the sort below reads as intent.
func (c Community) Size() int { return len(c.Members) }

// Options bounds the run. Zero values mean "use the ADR default", which keeps
// the common call site free of magic numbers while still allowing the two size
// thresholds to be configured (ADR-0020 待决 5 exposes them as config keys).
type Options struct {
	MaxRounds int
	MinSize   int
	MaxSize   int
}

func (o Options) withDefaults() Options {
	if o.MaxRounds <= 0 {
		o.MaxRounds = DefaultMaxRounds
	}
	if o.MinSize <= 0 {
		o.MinSize = DefaultMinSize
	}
	if o.MaxSize <= 0 {
		o.MaxSize = DefaultMaxSize
	}
	return o
}

// RunStats records what the iteration actually did, because "the community
// structure looks thin today" has at least three different causes and a report
// that cannot tell them apart is not worth showing:
//
//   - Converged=false means the round cap was hit, so the partition may still be
//     moving (ADR-0020 待决 2: take what you have and say so);
//   - DroppedSmall counts GROUPS (not nodes) under MinSize — the graph really is
//     sparse;
//   - DroppedLarge counts oversized blocks, which is LP collapsing everything
//     into one blob rather than a shortage of topics;
//   - Isolated counts nodes with no whitelisted edge at all.
//
// Those are not mutually exclusive and deliberately are not reconciled: every
// isolated node also forms a singleton group, so it is counted in Isolated AND in
// DroppedSmall. Reporting the overlap is more useful than picking one bucket per
// node, because Isolated answers "why" while DroppedSmall answers "how much".
//
// Three of them are otherwise mutually exclusive explanations of "zero
// communities".
type RunStats struct {
	Rounds        int  `json:"rounds"`
	Converged     bool `json:"converged"`
	DroppedSmall  int  `json:"dropped_small"`
	DroppedLarge  int  `json:"dropped_large"`
	Isolated      int  `json:"isolated"`
	Nodes         int  `json:"nodes"`
	Edges         int  `json:"edges"`
	CommunitySize int  `json:"communities"`
}

// LabelPropagation runs synchronous label propagation (Raghavan et al., 2007)
// and returns the surviving communities.
//
// Synchronous (Jacobi) rather than the usual in-place asynchronous sweep, and
// that choice is the determinism guarantee, not a performance preference: every
// node's next label is computed from the PREVIOUS round's labels, so the update
// order within a round cannot affect the result. Combined with (a) initial
// labels assigned from the node key itself, (b) one vote for the node's own
// current label in every tally, (c) ties broken by lexicographically smallest
// label, and (d) sorted output, the same input always yields the same partition —
// byte for byte. That is what makes "re-running the community pass must not churn
// the graph" testable rather than aspirational. Asynchronous LP reaches a
// different partition depending on which node it touches first, and there is no
// tie-break strong enough to rescue it.
//
// Known weaknesses this package does NOT try to hide (ADR-0020 决策 1 accepts
// them explicitly): LP boundaries are fuzzy, and on a weakly-connected graph it
// can collapse many topics into one giant community. The size thresholds are the
// containment, not the algorithm.
func LabelPropagation(nodes []string, edges []Edge, opt Options) ([]Community, RunStats) {
	opt = opt.withDefaults()

	adj := map[string]map[string]bool{}
	for _, n := range nodes {
		if _, ok := adj[n]; !ok {
			adj[n] = map[string]bool{}
		}
	}
	directed := 0
	for _, e := range edges {
		if e.A == "" || e.B == "" || e.A == e.B {
			continue // self-loops carry no community information
		}
		if _, okA := adj[e.A]; !okA {
			continue // an edge to an unknown node is a caller bug; ignore rather
		}
		if _, okB := adj[e.B]; !okB {
			continue // than invent a node the caller never declared
		}
		if adj[e.A][e.B] {
			continue // dedupe: a parallel edge would double-count in the vote
		}
		adj[e.A][e.B] = true
		adj[e.B][e.A] = true
		directed++
	}

	stats := RunStats{Nodes: len(adj), Edges: directed}
	if len(adj) == 0 {
		return nil, stats
	}

	// Sorted node order everywhere: map iteration in Go is randomised, and one
	// unsorted loop is all it takes to lose the determinism property.
	names := make([]string, 0, len(adj))
	for n := range adj {
		names = append(names, n)
	}
	sort.Strings(names)

	label := make(map[string]string, len(names))
	for _, n := range names {
		label[n] = n
		if len(adj[n]) == 0 {
			stats.Isolated++
		}
	}

	for round := 1; round <= opt.MaxRounds; round++ {
		next := make(map[string]string, len(label))
		changed := false
		for _, n := range names {
			if len(adj[n]) == 0 {
				// An isolated node has no one to agree with: it keeps its own label
				// forever, which is what makes it a single-node group and gets it
				// dropped by MinSize below.
				next[n] = label[n]
				continue
			}
			next[n] = majorityLabel(adj[n], label, label[n])
			if next[n] != label[n] {
				changed = true
			}
		}
		label = next
		stats.Rounds = round
		if !changed {
			stats.Converged = true
			break
		}
	}

	groups := map[string][]string{}
	for _, n := range names {
		groups[label[n]] = append(groups[label[n]], n)
	}

	var out []Community
	for lbl, members := range groups {
		switch {
		case len(members) < opt.MinSize:
			stats.DroppedSmall++
			continue
		case len(members) > opt.MaxSize:
			// "截断 > maxCommunitySize 的巨无霸" is implemented as discard-and-report
			// rather than keep-the-first-N. Truncating a community to an arbitrary
			// subset of its members would produce a synthesis page about a slice of a
			// topic that claims to summarise the topic — the worse of the two options,
			// and one the reader cannot see. The count lands in the report instead.
			stats.DroppedLarge++
			continue
		}
		sort.Strings(members)
		out = append(out, Community{Label: lbl, Members: members})
	}
	stats.CommunitySize = len(out)

	// Largest first so a caller that can only afford N communities takes the
	// substantive ones; label breaks the tie so the ordering is still total.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size() != out[j].Size() {
			return out[i].Size() > out[j].Size()
		}
		return strings.Compare(out[i].Label, out[j].Label) < 0
	})
	return out, stats
}

// majorityLabel picks the most common label among a node's neighbours PLUS its own
// current label, breaking ties by lexicographically smallest label (ADR-0020
// 决策 3).
//
// The self-vote is not a nudge, it is the fix for a failure mode of textbook LP:
// with neighbour-only voting, a two-node component swaps labels every round
// forever (each adopts what the other just adopted), so a connected pair is never
// recognised as a community and a whole graph of small components never
// converges. Giving the node one vote for its own label stabilises exactly that
// case, and it is still a majority rule — a neighbourhood with a clear preference
// outvotes the incumbent. Determinism is unaffected: the tally does not depend on
// iteration order any more than before.
//
// The empty return is unreachable from LabelPropagation, which routes
// neighbourless nodes elsewhere; it exists so this function cannot invent a label
// for a node it has no evidence about.
func majorityLabel(neighbours map[string]bool, label map[string]string, self string) string {
	if len(neighbours) == 0 {
		return ""
	}
	// The incumbent gets one vote up front; see above for why that is load-bearing.
	counts := map[string]int{self: 1}
	for nb := range neighbours {
		counts[label[nb]]++
	}
	best := ""
	bestCount := -1
	for lbl, c := range counts {
		if c > bestCount || (c == bestCount && lbl < best) {
			best, bestCount = lbl, c
		}
	}
	return best
}

// ClustersOf is a small helper for callers that need to answer "which community
// is this node in" without re-running the algorithm.
func ClustersOf(communities []Community) map[string]Community {
	out := make(map[string]Community, len(communities))
	for _, c := range communities {
		for _, m := range c.Members {
			out[m] = c
		}
	}
	return out
}
