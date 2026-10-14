package runtime

import (
	"database/sql"
	"strings"
	"testing"

	"repotory/internal/core/plugin"
)

// setupDB creates an in-memory database for tests.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Reuse the app's table creation by pointing InitDB at a temp file is
	// heavyweight; create the minimal notes table for import tests.
	_, err = db.Exec(`
	CREATE TABLE project_notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		title TEXT DEFAULT '',
		content TEXT NOT NULL,
		tags TEXT DEFAULT '',
		kind TEXT DEFAULT 'other',
		pinned INTEGER DEFAULT 0,
		source TEXT DEFAULT 'manual',
		sort_order INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

// staticImporter returns a fixed document set, standing in for a real
// KnowledgeImporter implementation.
type staticImporter struct {
	source string
	docs   []plugin.ImportDoc
}

func (s staticImporter) Source() string { return s.source }
func (s staticImporter) Import() ([]plugin.ImportDoc, error) {
	return s.docs, nil
}

// panicImporter simulates an importer that explodes mid-import.
type panicImporter struct {
	source string
}

func (p panicImporter) Source() string { return p.source }
func (p panicImporter) Import() ([]plugin.ImportDoc, error) {
	panic("import exploded")
}

// countingImporter records how many times Import ran, for gating assertions.
type countingImporter struct {
	name   string
	called *int
}

func (c countingImporter) Source() string { return c.name }
func (c countingImporter) Import() ([]plugin.ImportDoc, error) {
	*c.called++
	return nil, nil
}

// probeImporter is a staticImporter that also implements plugin.Prober.
type probeImporter struct {
	staticImporter
	detected bool
	detail   string
}

func (p probeImporter) Probe() plugin.ProbeResult {
	return plugin.ProbeResult{Detected: p.detected, Detail: p.detail}
}

// panicProbeImporter implements plugin.Prober and explodes when probed, to
// lock the safeProbe recovery.
type panicProbeImporter struct {
	staticImporter
}

func (p panicProbeImporter) Probe() plugin.ProbeResult { panic("probe exploded") }

func TestTriggerImportWritesNotes(t *testing.T) {
	db := setupDB(t)
	// Insert a project row so the FK-free import can reference it.
	_, err := db.Exec("INSERT INTO project_notes (project_id, title, content, source) VALUES (?, ?, ?, ?)",
		42, "seed", "seed-content", "seed")
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}

	rt := New(db)
	rt.RegisterSource("claude", staticImporter{source: "claude", docs: []plugin.ImportDoc{
		{ProjectID: 42, Title: "first", Content: "body-1", Tags: "a,b", Kind: "knowledge"},
		{ProjectID: 42, Title: "second", Content: "body-2", Kind: "log"},
	}})

	n, err := rt.TriggerImport("claude")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n.Created != 2 {
		t.Fatalf("expected 2 docs created, got %+v", n)
	}

	// Verify the note was written with correct metadata.
	var title, content, source, kind string
	err = db.QueryRow("SELECT title, content, source, kind FROM project_notes WHERE title = ?", "first").
		Scan(&title, &content, &source, &kind)
	if err != nil {
		t.Fatalf("query imported note: %v", err)
	}
	if title != "first" || content != "body-1" || source != "claude" || kind != "knowledge" {
		t.Fatalf("unexpected imported note row: %q %q %q %q", title, content, source, kind)
	}
}

func TestTriggerImportIdempotent(t *testing.T) {
	db := setupDB(t)
	_, err := db.Exec("INSERT INTO project_notes (project_id, title, content, source) VALUES (?, ?, ?, ?)",
		7, "seed", "s", "seed")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	rt := New(db)
	rt.RegisterSource("claude", staticImporter{source: "claude", docs: []plugin.ImportDoc{
		{ProjectID: 7, Title: "note", Content: "v2"},
	}})

	if _, err := rt.TriggerImport("claude"); err != nil {
		t.Fatalf("import 1: %v", err)
	}
	if _, err := rt.TriggerImport("claude"); err != nil {
		t.Fatalf("import 2: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM project_notes WHERE title = ?", "note").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("idempotent import should not duplicate notes, got %d", count)
	}
}

func TestTriggerImportUnknownSource(t *testing.T) {
	rt := New(setupDB(t))
	_, err := rt.TriggerImport("nope")
	if err == nil {
		t.Fatalf("expected error for unknown source")
	}
}

func TestImportPanicRecovered(t *testing.T) {
	db := setupDB(t)
	_, err := db.Exec("INSERT INTO project_notes (project_id, title, content, source) VALUES (?, ?, ?, ?)",
		5, "seed", "s", "seed")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	rt := New(db)
	rt.RegisterSource("boom", panicImporter{source: "boom"})

	_, err = rt.TriggerImport("boom")
	if err == nil {
		t.Fatalf("expected error from panicking importer")
	}
}

// TestImportAllSkipsManualSource locks the ADR-0011 决策 4 privacy gate:
// RegisterSource (auto) sources run in the startup ImportAll pass, while
// RegisterSourceManual (e.g. codex transcripts) sources are excluded there but
// still run when triggered explicitly by name.
func TestImportAllSkipsManualSource(t *testing.T) {
	rt := New(setupDB(t))
	autoRuns, manualRuns := 0, 0
	rt.RegisterSource("auto", countingImporter{name: "auto", called: &autoRuns})
	rt.RegisterSourceManual("manual", countingImporter{name: "manual", called: &manualRuns})

	// Manual source must still be discoverable in the list.
	if len(rt.SourceStatuses()) != 2 {
		t.Fatalf("expected 2 registered sources, got %+v", rt.SourceStatuses())
	}

	// ImportAll runs only the auto source.
	ran := map[string]bool{}
	for _, r := range rt.ImportAll() {
		ran[r.Name] = true
	}
	if !ran["auto"] {
		t.Error("ImportAll must run the auto source")
	}
	if ran["manual"] {
		t.Error("ImportAll must NOT run a manual (opt-in) source")
	}
	if manualRuns != 0 {
		t.Errorf("manual importer ran %d times in ImportAll, want 0", manualRuns)
	}

	// Explicit trigger DOES run the manual source.
	if _, err := rt.TriggerImport("manual"); err != nil {
		t.Fatalf("explicit trigger of manual source: %v", err)
	}
	if manualRuns != 1 {
		t.Errorf("manual importer runs = %d, want 1 after explicit trigger", manualRuns)
	}
}

// TestSourceStatusesProbe locks the ADR-0022 决策 1 presence contract: a
// source implementing plugin.Prober contributes detected/detail, and a source
// without a probe is still reported as present (so the settings page never
// hides an importable source).
func TestSourceStatusesProbe(t *testing.T) {
	rt := New(setupDB(t))
	rt.RegisterSource("detected", probeImporter{staticImporter: staticImporter{source: "detected"}, detected: true, detail: "found: 3 files"})
	rt.RegisterSourceManual("absent", probeImporter{staticImporter: staticImporter{source: "absent"}, detected: false, detail: "not installed"})
	rt.RegisterSource("plain", staticImporter{source: "plain"})

	byName := map[string]SourceStatus{}
	for _, st := range rt.SourceStatuses() {
		byName[st.Name] = st
	}

	d, ok := byName["detected"]
	if !ok || !d.Detected || d.Detail != "found: 3 files" {
		t.Fatalf("detected source status wrong: %+v (present=%v)", byName["detected"], ok)
	}
	a, ok := byName["absent"]
	if !ok || a.Detected || a.Detail != "not installed" {
		t.Fatalf("absent source status wrong: %+v (present=%v)", byName["absent"], ok)
	}
	if p := byName["plain"]; !p.Detected || p.Detail != "" {
		t.Fatalf("non-Prober source should be reported as present with empty detail, got %+v", p)
	}
}

// TestSourceStatusesProbePanicRecovered locks safeProbe: one broken probe can
// never take down the whole settings source list.
func TestSourceStatusesProbePanicRecovered(t *testing.T) {
	rt := New(setupDB(t))
	rt.RegisterSource("boom", panicProbeImporter{staticImporter: staticImporter{source: "boom"}})

	st := rt.SourceStatuses()
	if len(st) != 1 {
		t.Fatalf("expected 1 source, got %+v", st)
	}
	if st[0].Detected {
		t.Errorf("panicking probe must not report detected, got %+v", st[0])
	}
	if !strings.Contains(st[0].Detail, "探活失败") {
		t.Errorf("panicking probe should surface a failure detail, got %q", st[0].Detail)
	}
}
