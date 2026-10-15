package abeval

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s: got %.6f, want %.6f", msg, got, want)
	}
}

func TestRecallAtK(t *testing.T) {
	approx(t, RecallAtK([]int64{1, 2, 3}, []int64{2, 5}, 2), 0.5, "half relevant in top-2")
	approx(t, RecallAtK([]int64{1, 1, 2}, []int64{2}, 2), 0, "dup at pos0; relevant at pos2 outside window")
	approx(t, RecallAtK([]int64{5, 6, 7}, []int64{6, 7}, 3), 1.0, "all relevant within k")
	approx(t, RecallAtK([]int64{1}, nil, 3), 0, "no relevant -> 0")
	approx(t, RecallAtK([]int64{1}, []int64{1}, 0), 0, "k<=0 -> 0")
}

func TestNDCGAtK(t *testing.T) {
	approx(t, NDCGAtK([]int64{2, 3}, []int64{2, 3}, 3), 1.0, "perfect ranking")
	// 2@pos0 (gain 1.0) + 3@pos2 (gain 1/log2(4)=0.5) over IDCG (1 + 1/log2(3)).
	approx(t, NDCGAtK([]int64{2, 1, 3}, []int64{2, 3}, 3), 1.5/(1+1/math.Log2(3)), "relevant at 0 and 2")
	approx(t, NDCGAtK([]int64{1, 2}, nil, 2), 0, "no relevant -> 0")
}

func TestScoreSkipsUnlabeled(t *testing.T) {
	m := Score([]Retrieved[int64]{
		{Case: Case[int64]{"", nil}, Ranks: []int64{1}},
		{Case: Case[int64]{"", []int64{2, 5}}, Ranks: []int64{2, 3}},
	}, 3)
	if m.N != 1 {
		t.Fatalf("N=%d, want 1 (unlabeled skipped)", m.N)
	}
	approx(t, m.MeanRecall, 0.5, "1 of 2 relevant in top-3")
}

func TestCompareHybridBeatsLexical(t *testing.T) {
	cases := []Case[int64]{
		{Query: "q1", Relevant: []int64{10}},
		{Query: "q2", Relevant: []int64{20, 21}},
	}
	lexical := func(q string) []int64 {
		if q == "q1" {
			return []int64{99} // misses 10
		}
		return []int64{20} // gets one of two
	}
	hybrid := func(q string) []int64 {
		if q == "q1" {
			return []int64{10} // finds it
		}
		return []int64{21, 20} // finds both
	}
	lex, hyb, dRecall, dNDCG := Compare(cases, 3, lexical, hybrid)
	if !(hyb.MeanRecall > lex.MeanRecall) {
		t.Fatalf("expected hybrid recall > lexical; lex=%.3f hyb=%.3f", lex.MeanRecall, hyb.MeanRecall)
	}
	if dRecall <= 0 || dNDCG <= 0 {
		t.Fatalf("expected positive deltas; dRecall=%.3f dNDCG=%.3f", dRecall, dNDCG)
	}
}

func TestCompareNilRetrieverSafe(t *testing.T) {
	lex, hyb, _, _ := Compare([]Case[int64]{{"x", []int64{1}}}, 3, nil, func(string) []int64 { return []int64{1} })
	approx(t, lex.MeanRecall, 0, "nil lexical -> 0")
	approx(t, hyb.MeanRecall, 1, "hybrid finds it")
}

func TestSummaryLine(t *testing.T) {
	line := Metrics{K: 5, N: 2, MeanRecall: 0.5, MeanNDCG: 0.25}.SummaryLine("hybrid")
	if line == "" {
		t.Fatal("empty line")
	}
}

// TestDocIDSpacesDoNotCollide pins the reason the evidence gate needs DocID at
// all. wiki_pages.id and project_notes.id are independent sequences, so a bare
// int64 scorer would read page #7 as a hit for a labeled note #7 and report a
// perfect score for a retriever that returned the wrong document entirely.
func TestDocIDSpacesDoNotCollide(t *testing.T) {
	relevant := []DocID{{Type: "note", ID: 7}}
	if got := RecallAtK([]DocID{{Type: "page", ID: 7}}, relevant, 5); got != 0 {
		t.Fatalf("page 7 must not satisfy a labeled note 7: recall=%.3f", got)
	}
	if got := RecallAtK([]DocID{{Type: "note", ID: 7}}, relevant, 5); got != 1 {
		t.Fatalf("note 7 must satisfy a labeled note 7: recall=%.3f", got)
	}
	// The same page surfaced twice still counts once.
	if got := RecallAtK(
		[]DocID{{Type: "note", ID: 7}, {Type: "note", ID: 7}}, relevant, 5); got != 1 {
		t.Fatalf("duplicate hit must count once: recall=%.3f", got)
	}
}

// TestCompareTypedArms shows the evidence gate's shape: an interleaved page+note
// ranking scored against page labels, where graph expansion is what adds the
// page the FTS pass never returned.
func TestCompareTypedArms(t *testing.T) {
	cases := []Case[DocID]{
		{Query: "q1", Relevant: []DocID{{Type: "page", ID: 3}}},
		{Query: "q2", Relevant: []DocID{{Type: "page", ID: 5}}},
	}
	baseline := func(q string) []DocID { return nil } // pure FTS: nothing found
	candidate := func(q string) []DocID {
		if q == "q1" {
			return []DocID{{Type: "page", ID: 1}, {Type: "page", ID: 3}}
		}
		return []DocID{{Type: "page", ID: 5}}
	}
	base, cand, dRecall, _ := Compare(cases, 5, baseline, candidate)
	approx(t, base.MeanRecall, 0, "baseline found nothing")
	approx(t, cand.MeanRecall, 1, "graph arm found both")
	approx(t, dRecall, 1, "delta is the whole gain")
}

// TestCompareIgnoresForeignTypes is the belt-and-braces version: a retriever
// that only ever returns notes scores zero against page labels, no matter how
// many notes it returns. A gate that let this pass would be measuring nothing.
func TestCompareIgnoresForeignTypes(t *testing.T) {
	cases := []Case[DocID]{{Query: "q", Relevant: []DocID{{Type: "page", ID: 1}}}}
	notesOnly := func(string) []DocID {
		return []DocID{{Type: "note", ID: 1}, {Type: "note", ID: 2}, {Type: "note", ID: 3}}
	}
	_, cand, dRecall, _ := Compare(cases, 5, notesOnly, notesOnly)
	approx(t, cand.MeanRecall, 0, "note hits must not satisfy a page label")
	approx(t, dRecall, 0, "no phantom gain")
}

