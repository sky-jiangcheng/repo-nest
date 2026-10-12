package runtime

import (
	"database/sql"
	"os"
	"path/filepath"
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

// writePlugin writes a plugin.go into dir.
func writePlugin(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
}

func TestLoadHelloPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "hello"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "hello" }

func Init(ctx *plugin.Context) error { return nil }
`)

	rt := New(setupDB(t))
	rt.Load(dir)

	statuses := rt.PluginStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %+v", len(statuses), statuses)
	}
	s := statuses[0]
	if !s.Loaded {
		t.Fatalf("plugin failed to load: %+v", s)
	}
	if s.Name != "hello" {
		t.Fatalf("expected name hello, got %s", s.Name)
	}
}

func TestLoadMissingDir(t *testing.T) {
	rt := New(setupDB(t))
	rt.Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if got := rt.PluginStatuses(); len(got) != 0 {
		t.Fatalf("expected no plugins for missing dir, got %+v", got)
	}
}

func TestLoadEmptyDir(t *testing.T) {
	dir := t.TempDir()
	rt := New(setupDB(t))
	rt.Load(dir)
	if got := rt.PluginStatuses(); len(got) != 0 {
		t.Fatalf("expected no plugins for empty dir, got %+v", got)
	}
}

func TestPluginPanicDoesNotCrashHost(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "panic-on-init"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "panic-on-init" }

func Init(ctx *plugin.Context) error {
	panic("boom")
}
`)

	rt := New(setupDB(t))
	rt.Load(dir) // must not panic

	statuses := rt.PluginStatuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 plugin status, got %d", len(statuses))
	}
	if statuses[0].Loaded {
		t.Fatalf("panicking plugin should not be marked loaded: %+v", statuses[0])
	}
	if statuses[0].Err == "" {
		t.Fatalf("expected error message for panicking plugin")
	}
}

func TestPluginMissingRequiredSymbols(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "broken"), `package main
func Name() string { return "no-import-ok" }
`)
	rt := New(setupDB(t))
	rt.Load(dir)
	statuses := rt.PluginStatuses()
	if len(statuses) != 1 || statuses[0].Loaded {
		t.Fatalf("plugin missing Init should fail load: %+v", statuses)
	}
}

func TestCompileErrorRecorded(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "bad-syntax"), `package main
func Name( { return "x" }
`)
	rt := New(setupDB(t))
	rt.Load(dir)
	statuses := rt.PluginStatuses()
	if len(statuses) != 1 || statuses[0].Loaded {
		t.Fatalf("bad syntax should fail load: %+v", statuses)
	}
	if statuses[0].Err == "" {
		t.Fatalf("expected compile error message")
	}
}

func TestEventHandling(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "listener"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "listener" }

func Init(ctx *plugin.Context) error {
	ctx.On("note.created", func(e plugin.Event) error {
		return nil
	})
	return nil
}
`)

	rt := New(setupDB(t))
	rt.Load(dir)

	// Emit must not panic even with no handlers or with handlers.
	rt.Emit("note.created", "hello")
	rt.Emit("unknown.event", nil)
}

func TestEventHandlerPanicRecovered(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "bad-handler"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "bad-handler" }

func Init(ctx *plugin.Context) error {
	ctx.On("note.created", func(e plugin.Event) error {
		panic("handler blew up")
	})
	return nil
}
`)

	rt := New(setupDB(t))
	rt.Load(dir)
	rt.Emit("note.created", "x") // must not crash host
}

func TestKnowledgeSourceRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "importer"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "importer" }

func Source() string { return "claude" }

func Init(ctx *plugin.Context) error { return nil }

func Import(ctx *plugin.Context) ([]plugin.ImportDoc, error) {
	return []plugin.ImportDoc{{ProjectID: 1, Title: "t", Content: "c", Kind: "knowledge"}}, nil
}
`)

	rt := New(setupDB(t))
	rt.Load(dir)

	sources := rt.SourceStatuses()
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d: %+v", len(sources), sources)
	}
	if sources[0].Name != "claude" {
		t.Fatalf("expected source claude, got %s", sources[0].Name)
	}
}

func TestTriggerImportWritesNotes(t *testing.T) {
	db := setupDB(t)
	// Insert a project row so the FK-free import can reference it.
	_, err := db.Exec("INSERT INTO project_notes (project_id, title, content, source) VALUES (?, ?, ?, ?)",
		42, "seed", "seed-content", "seed")
	if err != nil {
		t.Fatalf("seed note: %v", err)
	}

	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "importer"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "importer" }
func Source() string { return "claude" }
func Init(ctx *plugin.Context) error { return nil }

func Import(ctx *plugin.Context) ([]plugin.ImportDoc, error) {
	return []plugin.ImportDoc{
		{ProjectID: 42, Title: "first", Content: "body-1", Tags: "a,b", Kind: "knowledge"},
		{ProjectID: 42, Title: "second", Content: "body-2", Kind: "log"},
	}, nil
}
`)

	rt := New(db)
	rt.Load(dir)

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

	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "imp"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "imp" }
func Source() string { return "claude" }
func Init(ctx *plugin.Context) error { return nil }
func Import(ctx *plugin.Context) ([]plugin.ImportDoc, error) {
	return []plugin.ImportDoc{{ProjectID: 7, Title: "note", Content: "v2"}}, nil
}
`)

	rt := New(db)
	rt.Load(dir)

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

	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "bad-import"), `package main
import "repotory/internal/core/plugin"

func Name() string { return "bad-import" }
func Source() string { return "boom" }
func Init(ctx *plugin.Context) error { return nil }
func Import(ctx *plugin.Context) ([]plugin.ImportDoc, error) {
	panic("import exploded")
}
`)

	rt := New(db)
	rt.Load(dir)
	_, err = rt.TriggerImport("boom")
	if err == nil {
		t.Fatalf("expected error from panicking importer")
	}
}

// The interpreter runs with an allowlisted symbol table (safeSymbols), not
// stdlib wholesale: os/os/exec/syscall/unsafe would let a dropped-in
// plugin.go escape to the process table and filesystem. Allowed stdlib
// imports keep working; a dangerous one fails to compile into a normal
// per-plugin load failure instead of granting the capability.
func TestPluginSafeSymbolsAllowlist(t *testing.T) {
	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "uses-strings"), `package main
import (
	"fmt"
	"strings"

	"repotory/internal/core/plugin"
)

func Name() string { return strings.ToUpper(fmt.Sprintf("%s", "safe")) }

func Init(ctx *plugin.Context) error { return nil }
`)
	writePlugin(t, filepath.Join(dir, "uses-exec"), `package main
import (
	"os/exec"

	"repotory/internal/core/plugin"
)

func Name() string { return "evil" }

func Init(ctx *plugin.Context) error {
	_ = exec.Command
	return nil
}
`)

	rt := New(setupDB(t))
	rt.Load(dir)

	byName := map[string]bool{}
	for _, s := range rt.PluginStatuses() {
		byName[filepath.Base(s.Path)] = s.Loaded
	}
	if !byName["uses-strings"] {
		t.Errorf("plugin using allowed stdlib imports must load: %+v", rt.PluginStatuses())
	}
	if byName["uses-exec"] {
		t.Errorf("plugin importing os/exec must NOT load: %+v", rt.PluginStatuses())
	}
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
