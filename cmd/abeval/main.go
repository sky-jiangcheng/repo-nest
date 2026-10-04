// Command abeval runs the M3-A A/B evaluation gate (ADR-0012 决策 4) against the
// live knowledge base: it executes every labeled query through lexical search
// (semantic_search off) and hybrid search (on), compares Recall@k / NDCG@k, and
// exits non-zero unless hybrid beats lexical by the configured margin.
//
// Usage:
//
//	abeval -cases queries.jsonl -k 10 -min-recall 0.05
//
// queries.jsonl — one JSON object per line: {"query": "...", "relevant": [id,...]}
// (ids are project_notes.id that actually answer the query). Hybrid needs the
// embedding endpoint configured (semantic_search/embedding_* settings); if it is
// absent, hybrid == lexical and the gate correctly fails (no evidence gained).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"repo-nest/internal/db"
	"repo-nest/internal/domain"
	"repo-nest/internal/platform"
	"repo-nest/internal/search/abeval"
	"repo-nest/internal/service"
)

type labeledCase struct {
	Query    string  `json:"query"`
	Relevant []int64 `json:"relevant"`
}

func main() {
	platform.SetPrivateUmask()

	casesPath := flag.String("cases", "", "path to labeled queries.jsonl (required)")
	k := flag.Int("k", 10, "cutoff for recall@k / ndcg@k")
	minRecall := flag.Float64("min-recall", 0.05, "required recall@k gain (hybrid-lexical) to pass the gate")
	dbPath := flag.String("db", "", "database path (default: the app's DB)")
	flag.Parse()

	if *casesPath == "" {
		log.Fatal("abeval: -cases is required")
	}
	cases, err := loadCases(*casesPath)
	if err != nil {
		log.Fatalf("abeval: load cases: %v", err)
	}
	if len(cases) == 0 {
		log.Fatal("abeval: no cases in file")
	}

	path := *dbPath
	if path == "" {
		path = platform.GetDbPath()
	}
	database, err := db.InitDB(path)
	if err != nil {
		log.Fatalf("abeval: open db: %v", err)
	}
	defer database.Close()
	svc := service.New(database, "abeval")

	// semanticEnabled() reads app_config on EVERY search, and db.SetConfig
	// writes straight through to the user's database. An evaluation tool must
	// not outlive its run: remember the original value and restore it when
	// abeval exits, pass or fail (os.Exit below skips defers, so the restore
	// is registered before any exit path).
	original, _ := db.GetConfig(database, "semantic_search")
	restore := func() {
		if original == "" {
			_ = db.DeleteConfig(database, "semantic_search")
		} else {
			_ = db.SetConfig(database, "semantic_search", original)
		}
	}
	if err := restoreOnSignal(restore); err != nil {
		log.Printf("abeval: signal restore not installed: %v", err)
	}

	// The two arms MUST differ at call time, not at setup time: each closure
	// flips the config it depends on right before searching. Toggling once up
	// front (the pre-fix wiring) left both arms running with semantic_search
	// on — lexical was silently hybrid, the recall delta was always 0, and
	// the gate could never pass.
	lexical := func(q string) []int64 {
		_ = db.SetConfig(database, "semantic_search", "0")
		return ids(svc.SearchNotes(q))
	}

	// Hybrid arm: enable + rebuild the vector index once, then search with
	// semantic on.
	if err := db.SetConfig(database, "semantic_search", "1"); err != nil {
		log.Fatalf("abeval: enable semantic: %v", err)
	}
	if n, err := svc.RebuildEmbeddings(); err != nil {
		fmt.Printf("abeval: semantic rebuild unavailable (%v) — hybrid will equal lexical\n", err)
	} else {
		fmt.Printf("abeval: embedded %d notes for the hybrid arm\n", n)
	}
	hybrid := func(q string) []int64 {
		_ = db.SetConfig(database, "semantic_search", "1")
		return ids(svc.SearchNotes(q))
	}

	lex, hyb, dRecall, dNDCG := abeval.Compare(cases, *k, lexical, hybrid)
	fmt.Println(lex.SummaryLine("lexical"))
	fmt.Println(hyb.SummaryLine("hybrid "))
	fmt.Printf("delta: recall@%d=%+.3f ndcg@%d=%+.3f\n", *k, dRecall, *k, dNDCG)

	restore()

	if dRecall < *minRecall {
		fmt.Printf("GATE FAIL: recall@%d gain %.3f < required %.3f — semantic search not yet worth enabling\n",
			*k, dRecall, *minRecall)
		os.Exit(1)
	}
	fmt.Printf("GATE PASS: recall@%d gain %.3f >= %.3f\n", *k, dRecall, *minRecall)
}

// restoreOnSignal best-effort restores the original semantic_search value on
// SIGINT/SIGTERM too (the gate can run against a live database for minutes).
// SIGKILL is unrecoverable by design; the next RebuildEmbeddings or manual
// toggle repairs the config either way.
func restoreOnSignal(restore func()) error {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		restore()
		os.Exit(130)
	}()
	return nil
}

func loadCases(path string) ([]abeval.Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []abeval.Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var c labeledCase
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("bad line %q: %w", string(line), err)
		}
		out = append(out, abeval.Case{Query: c.Query, Relevant: c.Relevant})
	}
	return out, sc.Err()
}

// ids extracts note ids from search hits, preserving ranked order.
func ids(hits []domain.SearchHit) []int64 {
	out := make([]int64, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}
