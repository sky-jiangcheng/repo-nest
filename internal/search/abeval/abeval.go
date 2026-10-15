// Package abeval is the offline retrieval quality gate (ADR-0012 决策 4,
// ADR-0018 lane 2 条件 6). It measures whether a candidate retriever actually
// beats the incumbent on a labeled query set — before the candidate is exposed to
// users. It is pure computation over ranked id lists, so it has no DB, network,
// or embedding dependencies: the caller runs the two retrievers (e.g.
// SearchNotes with semantic_search on vs off) and feeds the results in.
//
// Two id spaces, one scorer. The notes gate ranks notes only, so its ids are
// bare int64. The evidence gate (ADR-0018 lane 2) ranks wiki pages AND notes in
// one interleaved list, and wiki_pages.id / project_notes.id are INDEPENDENT
// sequences — page #7 is not note #7. Scoring both as bare int64 would silently
// award a page hit for a labeled note carrying the same number, which is the
// exact trap wiki_evidence.go documents at its RRF call site. Hence the type
// parameter: DocID carries the type tag, so the two spaces cannot be confused.
//
// Metrics are binary-relevance (a doc is relevant or not), the honest bar for
// "does this retrieval surface the docs it should" on a personal corpus.
package abeval

import (
	"fmt"
	"math"
)

// DocID identifies one retrievable document across both id spaces. The Type tag
// is what keeps the spaces apart: {page,7} and {note,7} are different documents
// and score as such.
type DocID struct {
	Type string // "page" | "note"
	ID   int64
}

// Case is one labeled query: what the user typed and the set of ids that
// actually answer it. A case with no relevant ids is skipped by the scorers.
type Case[T comparable] struct {
	Query    string
	Relevant []T
}

// Retrieved is a ranked result (best first) for one case.
type Retrieved[T comparable] struct {
	Case  Case[T]
	Ranks []T
}

// Metrics holds aggregate quality numbers over retrievals at cutoff k.
type Metrics struct {
	K          int
	N          int     // cases scored (non-empty Relevant)
	MeanRecall float64 // mean |topK ∩ relevant| / |relevant|
	MeanNDCG   float64 // mean normalized DCG@K (binary relevance)
}

// RecallAtK = |ranks[:k] ∩ relevant| / |relevant|; 0 when relevant is empty.
// Repeated ids in ranks are counted once.
func RecallAtK[T comparable](ranks, relevant []T, k int) float64 {
	if len(relevant) == 0 || k <= 0 {
		return 0
	}
	set := idSet(relevant)
	seen := make(map[T]struct{}, k)
	hits := 0
	for i, id := range ranks {
		if i >= k {
			break
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := set[id]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(set))
}

// NDCGAtK is binary-relevance normalized DCG@K. Repeated ids count once (they
// occupy their first position only), matching how a user sees a ranked list.
func NDCGAtK[T comparable](ranks, relevant []T, k int) float64 {
	if len(relevant) == 0 || k <= 0 {
		return 0
	}
	set := idSet(relevant)
	seen := make(map[T]struct{}, k)
	dcg := 0.0
	for i, id := range ranks {
		if i >= k {
			break
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := set[id]; ok {
			dcg += 1.0 / math.Log2(float64(i)+2.0) // position i (0-based) -> log2(i+2)
		}
	}
	idcg := 0.0
	for i := 0; i < k && i < len(set); i++ {
		idcg += 1.0 / math.Log2(float64(i)+2.0)
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

// Score aggregates Recall@k and NDCG@k over retrievals, skipping unlabeled cases.
func Score[T comparable](retrievals []Retrieved[T], k int) Metrics {
	m := Metrics{K: k}
	if k <= 0 {
		return m
	}
	var sumR, sumN float64
	for _, r := range retrievals {
		if len(r.Case.Relevant) == 0 {
			continue
		}
		sumR += RecallAtK(r.Ranks, r.Case.Relevant, k)
		sumN += NDCGAtK(r.Ranks, r.Case.Relevant, k)
		m.N++
	}
	if m.N > 0 {
		m.MeanRecall = sumR / float64(m.N)
		m.MeanNDCG = sumN / float64(m.N)
	}
	return m
}

// Compare runs the same labeled cases through two retrievers (baseline and
// candidate) and returns both metric sets plus the deltas (candidate - baseline).
// A positive delta over a real labeled set is the evidence the ADRs require
// before flipping a default. A nil retriever yields empty ranks, which scores 0
// rather than panicking — a broken arm must read as "no gain", never as a pass.
func Compare[T comparable](cases []Case[T], k int, baseline, candidate func(query string) []T) (base, cand Metrics, dRecall, dNDCG float64) {
	baseRet := make([]Retrieved[T], 0, len(cases))
	candRet := make([]Retrieved[T], 0, len(cases))
	for _, c := range cases {
		baseRet = append(baseRet, Retrieved[T]{Case: c, Ranks: call(baseline, c.Query)})
		candRet = append(candRet, Retrieved[T]{Case: c, Ranks: call(candidate, c.Query)})
	}
	base = Score(baseRet, k)
	cand = Score(candRet, k)
	return base, cand, cand.MeanRecall - base.MeanRecall, cand.MeanNDCG - base.MeanNDCG
}

// SummaryLine is a compact one-line report for logging / eval output.
func (m Metrics) SummaryLine(label string) string {
	return fmt.Sprintf("%s: n=%d recall@%d=%.3f ndcg@%d=%.3f", label, m.N, m.K, m.MeanRecall, m.K, m.MeanNDCG)
}

func call[T comparable](f func(string) []T, q string) []T {
	if f == nil {
		return nil
	}
	return f(q)
}

func idSet[T comparable](ids []T) map[T]struct{} {
	m := make(map[T]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}
