// Command wiki-rehearse is the ADR-0018 lane 1 promotion harness: it rehearses
// the v21 page_links relation-vocabulary migration on a THROWAWAY COPY of a real
// dashboard.db and proves, on that copy, the one thing a synthetic fixture cannot:
// that no edge is lost and every legacy relation value normalizes as expected.
//
// It is deliberately incapable of mutating the real database:
//
//   - the source is opened `mode=ro` (read-only), and its only statement is
//     `VACUUM INTO '<copy>'`, which writes to the new file and never the source;
//   - the actual migration runs via db.InitDB on the COPY, i.e. exactly the upgrade
//     path the desktop app would take on next launch, not a hand-rolled subset.
//
// So it is safe to point at a live dashboard.db — but per this repo's rule the
// human runs it themselves; the agent only exercises it against temp databases.
//
// Usage:
//
//	go build -o reponest-wiki-rehearse ./cmd/wiki-rehearse
//	./reponest-wiki-rehearse -db "$HOME/Library/Application Support/…/dashboard.db"
//
// Exit code 0 = the invariant held (no lost/extra edges, all after-relations in
// the vocabulary); nonzero = something would go wrong, with the diff printed.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"repo-nest/internal/db"
)

// edge is one page_links row, read raw (pre-normalization).
type edge struct {
	ID       int64  `json:"id"`
	From     int64  `json:"from"`
	To       int64  `json:"to"`
	Relation string `json:"relation"`
}

// edgeKey is the identity the CHECK/UNIQUE enforce: (from, to, relation).
type edgeKey struct {
	From     int64
	To       int64
	Relation string
}

// Report is the rehearsal verdict.
type Report struct {
	SourceReadOnly  bool           `json:"source_read_only"`
	VersionBefore   int            `json:"schema_version_before"`
	VersionAfter    int            `json:"schema_version_after"`
	BeforeCount     int            `json:"before_edges"`
	AfterCount      int            `json:"after_edges"`
	Collapsed       int            `json:"collapsed_parallel_edges"` // distinct old edges that folded onto one key
	Folded          map[string]int `json:"legacy_relation_folds"`    // old value -> count it was folded from
	Lost            []edgeKey      `json:"lost_edges"`               // in normalize(before) but not in after  (must be empty)
	Extra           []edgeKey      `json:"extra_edges"`              // in after but not from before            (must be empty)
	OffVocabulary   []string       `json:"off_vocabulary_after"`     // after-relations not in the 8-type set   (must be empty)
	BeforeHistogram map[string]int `json:"before_relation_histogram"`
	AfterHistogram  map[string]int `json:"after_relation_histogram"`
	Pass            bool           `json:"pass"`
}

// derivedKey maps a raw edge onto its post-migration identity, i.e. the same
// folding NormalizeRelation + the v21 rebuild apply.
func derivedKey(e edge) edgeKey {
	return edgeKey{From: e.From, To: e.To, Relation: db.NormalizeRelation(e.Relation)}
}

func histogram(edges []edge) map[string]int {
	m := map[string]int{}
	for _, e := range edges {
		m[e.Relation]++
	}
	return m
}

// reconcile is the whole correctness argument, pure so it is unit-testable:
//   - every old edge, after normalization, must exist in the result (no loss);
//   - the result must not contain any edge that did not come from an old one
//     (no fabrication);
//   - every resulting relation must be a vocabulary member.
func reconcile(before, after []edge) Report {
	derived := map[edgeKey]bool{}
	folded := map[string]int{}
	for _, e := range before {
		derived[derivedKey(e)] = true
		if n := db.NormalizeRelation(e.Relation); n != e.Relation {
			folded[e.Relation]++
		}
	}
	afterSet := map[edgeKey]bool{}
	for _, e := range after {
		afterSet[edgeKey{e.From, e.To, e.Relation}] = true
	}

	var lost, extra []edgeKey
	for k := range derived {
		if !afterSet[k] {
			lost = append(lost, k)
		}
	}
	for k := range afterSet {
		if !derived[k] {
			extra = append(extra, k)
		}
	}
	sortEdgeKeys(lost)
	sortEdgeKeys(extra)

	off := map[string]bool{}
	for _, e := range after {
		if !db.ValidRelation(e.Relation) {
			off[e.Relation] = true
		}
	}
	var offVocab []string
	for r := range off {
		offVocab = append(offVocab, r)
	}
	sort.Strings(offVocab)

	return Report{
		BeforeCount:     len(before),
		AfterCount:      len(after),
		Collapsed:       len(before) - len(derived),
		Folded:          folded,
		Lost:            lost,
		Extra:           extra,
		OffVocabulary:   offVocab,
		BeforeHistogram: histogram(before),
		AfterHistogram:  histogram(after),
		Pass:            len(lost) == 0 && len(extra) == 0 && len(offVocab) == 0,
	}
}

func sortEdgeKeys(ks []edgeKey) {
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].From != ks[j].From {
			return ks[i].From < ks[j].From
		}
		if ks[i].To != ks[j].To {
			return ks[i].To < ks[j].To
		}
		return ks[i].Relation < ks[j].Relation
	})
}

// errNoPageLinks lets the caller distinguish "a pre-wiki database with nothing to
// rehearse" (benign) from a real query error.
var errNoPageLinks = fmt.Errorf("no such table: page_links")

func dumpEdges(queryer *sql.DB) ([]edge, error) {
	rows, err := queryer.Query(`SELECT id, from_page_id, to_page_id, relation FROM page_links ORDER BY id`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, errNoPageLinks
		}
		return nil, err
	}
	defer rows.Close()
	var out []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.ID, &e.From, &e.To, &e.Relation); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func readSchemaVersion(queryer *sql.DB) int {
	var raw string
	if err := queryer.QueryRow(`SELECT value FROM app_config WHERE key = 'schema_version'`).Scan(&raw); err != nil {
		return -1
	}
	var v int
	fmt.Sscanf(raw, "%d", &v)
	return v
}

// rehearse runs the whole read-only-copy-migrate-reconcile sequence. copyPath is
// written by the caller (it lives in the out dir). Split out so the copy mechanism
// itself is exercised by tests against temp files, not only the pure reconcile.
func rehearse(absSrc, copyPath string) (Report, bool, error) {
	var rep Report
	src, err := sql.Open("sqlite", "file:"+filepath.ToSlash(absSrc)+"?mode=ro")
	if err != nil {
		return rep, false, fmt.Errorf("open source read-only: %w", err)
	}
	defer src.Close()
	if err := src.Ping(); err != nil { // force the lazy open so a bad path fails here
		return rep, false, fmt.Errorf("open source read-only: %w", err)
	}
	rep.SourceReadOnly = true
	rep.VersionBefore = readSchemaVersion(src)

	before, derr := dumpEdges(src)
	if derr != nil && derr != errNoPageLinks {
		return rep, false, fmt.Errorf("read source edges: %w", derr)
	}
	hadNoTable := derr == errNoPageLinks

	_ = os.Remove(copyPath)
	if _, err := src.Exec("VACUUM INTO '" + strings.ReplaceAll(copyPath, "'", "''") + "'"); err != nil {
		return rep, false, fmt.Errorf("VACUUM INTO copy: %w", err)
	}

	migrated, err := db.InitDB(copyPath)
	if err != nil {
		return rep, false, fmt.Errorf("InitDB on copy: %w", err)
	}
	defer migrated.Close()
	rep.VersionAfter = readSchemaVersion(migrated)
	after, _ := dumpEdges(migrated)

	r := reconcile(before, after)
	r.SourceReadOnly = true
	r.VersionBefore = rep.VersionBefore
	r.VersionAfter = rep.VersionAfter
	return r, hadNoTable, nil
}

func main() {
	dbPath := flag.String("db", "", "path to dashboard.db (opened READ-ONLY; never modified)")
	outDir := flag.String("out", "", "directory for the copy + dumps + report (default: ./v21-rehearse)")
	flag.Parse()

	if strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(os.Stderr, "error: -db <path> is required")
		os.Exit(2)
	}
	abs, err := filepath.Abs(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if _, err := os.Stat(abs); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot read db:", err)
		os.Exit(2)
	}
	if *outDir == "" {
		*outDir = "./v21-rehearse"
	}
	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "error: out dir:", err)
		os.Exit(2)
	}

	copyPath := filepath.Join(*outDir, "rehearse-copy.db")
	rep, hadNoTable, err := rehearse(abs, copyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// The copy's post-migration edges and the source's pre-migration edges are
	// re-dumped for the JSONL artifacts (before from a fresh read-only view).
	if src, e := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro"); e == nil {
		before, _ := dumpEdges(src)
		writeJSONL(filepath.Join(*outDir, "page_links.before.jsonl"), before)
		_ = src.Close()
	}
	if m, e := sql.Open("sqlite", "file:"+filepath.ToSlash(copyPath)+"?mode=ro"); e == nil {
		after, _ := dumpEdges(m)
		writeJSONL(filepath.Join(*outDir, "page_links.after.jsonl"), after)
		_ = m.Close()
	}
	if b, e := json.MarshalIndent(rep, "", "  "); e == nil {
		_ = os.WriteFile(filepath.Join(*outDir, "report.json"), b, 0o600)
	}

	printReport(rep, *outDir, hadNoTable)
	if !rep.Pass {
		os.Exit(1)
	}
}

func writeJSONL(path string, edges []edge) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range edges {
		_ = enc.Encode(e)
	}
}

func printReport(r Report, outDir string, hadNoTable bool) {
	fmt.Println("== ADR-0018 lane 1 · v21 rehearsal (ran on a throwaway copy; source untouched, opened read-only) ==")
	fmt.Printf("  schema_version: %d -> %d\n", r.VersionBefore, r.VersionAfter)
	fmt.Printf("  edges before -> after: %d -> %d\n", r.BeforeCount, r.AfterCount)
	if hadNoTable {
		fmt.Println("  note: source had no page_links table yet (pre-wiki db); v21 will just create it.")
	}
	if r.Collapsed > 0 {
		fmt.Printf("  parallel edges folded into one: %d\n", r.Collapsed)
	}
	if len(r.Folded) > 0 {
		keys := make([]string, 0, len(r.Folded))
		for k := range r.Folded {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Println("  legacy relation values folded onto the vocabulary:")
		for _, k := range keys {
			fmt.Printf("    %-16s x%d -> %q\n", k, r.Folded[k], db.NormalizeRelation(k))
		}
	}
	fmt.Println("  after-relation histogram:")
	akeys := make([]string, 0, len(r.AfterHistogram))
	for k := range r.AfterHistogram {
		akeys = append(akeys, k)
	}
	sort.Strings(akeys)
	for _, k := range akeys {
		fmt.Printf("    %-12s %d\n", k, r.AfterHistogram[k])
	}

	fmt.Println("  verdict:")
	if len(r.Lost) == 0 {
		fmt.Println("    ✓ no edge lost")
	} else {
		fmt.Printf("    ✗ %d edge(s) in normalize(before) missing from result:\n", len(r.Lost))
		for _, k := range r.Lost {
			fmt.Printf("        (%d -> %d : %s)\n", k.From, k.To, k.Relation)
		}
	}
	if len(r.Extra) == 0 {
		fmt.Println("    ✓ no fabricated edge")
	} else {
		fmt.Printf("    ✗ %d edge(s) in result not derived from any input edge:\n", len(r.Extra))
		for _, k := range r.Extra {
			fmt.Printf("        (%d -> %d : %s)\n", k.From, k.To, k.Relation)
		}
	}
	if len(r.OffVocabulary) == 0 {
		fmt.Println("    ✓ every result relation is in the 8-type vocabulary")
	} else {
		fmt.Printf("    ✗ off-vocabulary relations present: %v\n", r.OffVocabulary)
	}
	fmt.Printf("  artifacts: %s/{rehearse-copy.db, page_links.before.jsonl, page_links.after.jsonl, report.json}\n", outDir)
	if r.Pass {
		fmt.Println("\nPASS — lane 1 promotion criterion 1 (rehearse on a real-DB copy + retain old-edge dump) can be marked done.")
	} else {
		fmt.Println("\nFAIL — do NOT promote lane 1; investigate the diff above first.")
	}
}
