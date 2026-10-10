// Command abeval runs the M3-A A/B evaluation gate (ADR-0012 决策 4) against the
// live knowledge base: it executes every labeled query through two retrieval
// arms and exits non-zero unless the second arm beats the first by the
// configured margin. The arms depend on the mode:
//
//	default (note arm): lexical search (semantic_search off) vs hybrid (on)
//	-pages   (page arm): page evidence with wiki_graph_search off (FTS-only)
//	                     vs on (one-hop neighbours ordered by PPR) — the
//	                     ADR-0018 lane 2 measurement
//
// Usage:
//
//	abeval -cases queries.jsonl -k 10 -min-recall 0.05
//	abeval -pages -cases queries.jsonl -k 10 -project 3
//
// queries.jsonl — one JSON object per line:
//
//	{"query": "...", "relevant": [id,...]}         note arm (project_notes.id)
//	{"query": "...", "relevant_pages": [id,...]}   page arm (wiki_page.id)
//
// Hybrid needs the embedding endpoint configured (semantic_search/embedding_*
// settings); if it is absent, hybrid == lexical and the gate correctly fails
// (no evidence gained). Run against a COPY of the database: both arms flip
// config keys (restored on exit, pass or fail), and the note arm rebuilds the
// vector index.
package main

import (
	"bufio"
	"bytes"
	"database/sql"
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
	Query         string  `json:"query"`
	Relevant      []int64 `json:"relevant"`       // note arm: project_notes.id
	RelevantPages []int64 `json:"relevant_pages"` // page arm: wiki_page.id
}

func main() {
	platform.SetPrivateUmask()

	casesPath := flag.String("cases", "", "path to labeled queries.jsonl (required)")
	k := flag.Int("k", 10, "cutoff for recall@k / ndcg@k")
	minRecall := flag.Float64("min-recall", 0.05, "required recall@k gain (arm2-arm1) to pass the gate")
	dbPath := flag.String("db", "", "database path (default: the app's DB)")
	pages := flag.Bool("pages", false, "evaluate the page arm: wiki_graph_search off vs on, ids from relevant_pages")
	project := flag.Int64("project", 0, "page arm project scope (0 = every project's pages plus global ones)")
	flag.Parse()

	if *casesPath == "" {
		log.Fatal("abeval: -cases is required")
	}
	cases, err := loadCases(*casesPath, *pages)
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

	// Both arms read the gate config on EVERY call (EvidencePageRanking and
	// SearchNotes both do), and db.SetConfig writes straight through to the
	// user's database. An evaluation tool must not outlive its run: remember
	// the original value of the key this mode flips and restore it when abeval
	// exits, pass or fail (os.Exit below skips defers, so the restore is
	// registered before any exit path).
	gateKey := "semantic_search"
	if *pages {
		gateKey = "wiki_graph_search"
	}
	original, _ := db.GetConfig(database, gateKey)
	restore := func() {
		if original == "" {
			_ = db.DeleteConfig(database, gateKey)
		} else {
			_ = db.SetConfig(database, gateKey, original)
		}
	}
	if err := restoreOnSignal(restore); err != nil {
		log.Printf("abeval: signal restore not installed: %v", err)
	}

	if *pages {
		runPageGate(cases, *k, *minRecall, svc, database, *project, restore)
		return
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

// runPageGate is the -pages arm (plan Phase 2: quantify the PPR delta over
// page evidence — the first hard evidence of KG value). The two arms differ
// only in wiki_graph_search, flipped per call like the note arm's key: FTS-only
// ranking vs FTS seeds + one-hop neighbours ordered by PPR.
func runPageGate(cases []abeval.Case, k int, minRecall float64, svc *service.Service, database *sql.DB, project int64, restore func()) {
	// Matches the FTS seed cap inside evidencePages, so both arms see the same
	// candidate ceiling and k=10 sits well inside the list.
	const evalLimit = 50
	ftsOnly := func(q string) []int64 {
		_ = db.SetConfig(database, "wiki_graph_search", "0")
		return itemIDs(svc.EvidencePageRanking(q, project, evalLimit))
	}
	graph := func(q string) []int64 {
		_ = db.SetConfig(database, "wiki_graph_search", "1")
		return itemIDs(svc.EvidencePageRanking(q, project, evalLimit))
	}

	fts, ppr, dRecall, dNDCG := abeval.Compare(cases, k, ftsOnly, graph)
	fmt.Println(fts.SummaryLine("fts    "))
	fmt.Println(ppr.SummaryLine("graph  "))
	fmt.Printf("delta: recall@%d=%+.3f ndcg@%d=%+.3f\n", k, dRecall, k, dNDCG)

	restore()

	if dRecall < minRecall {
		fmt.Printf("GATE FAIL: recall@%d gain %.3f < required %.3f — the graph is not yet worth enabling for page evidence\n",
			k, dRecall, minRecall)
		os.Exit(1)
	}
	fmt.Printf("GATE PASS: recall@%d gain %.3f >= %.3f\n", k, dRecall, minRecall)
}

// restoreOnSignal best-effort restores the original gate config value (the
// key the current arm flips) on SIGINT/SIGTERM too (the gate can run against
// a live database for minutes). SIGKILL is unrecoverable by design; the next
// RebuildEmbeddings or manual toggle repairs the config either way.
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

// loadCases parses the labeled file; pages selects which key carries the
// relevant ids (relevant_pages for the page arm, relevant for the note arm).
// Each arm reads ONLY its own key — a page run over note labels would score
// n=0, and the summary line is the only place that confusion would surface.
func loadCases(path string, pages bool) ([]abeval.Case, error) {
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
		relevant := c.Relevant
		if pages {
			relevant = c.RelevantPages
		}
		out = append(out, abeval.Case{Query: c.Query, Relevant: relevant})
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

// itemIDs extracts page ids from an evidence ranking, preserving order.
func itemIDs(items []service.EvidenceItem) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}
