package opencode

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
	"repotory/internal/importers/memsrc"
)

// fakeXDGData points XDG_DATA_HOME at a temp dir so sessionRoot resolves there.
func fakeXDGData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	return dir
}

const sessionFixture = `{"id":"ses_3dd894b12ffeUXjGy50UmJ8UpQ","slug":"mighty-engine","version":"1.1.36","projectID":"9a6f0de","directory":"/Users/u/Workspace/CodeStat","title":"Add caching layer","time":{"created":1770104730861,"updated":1770104758303},"summary":"Introduced an in-memory cache and wired invalidation into the write path."}`

func writeSession(t *testing.T, root, hash, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "opencode", "storage", "session", hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportProducesNote(t *testing.T) {
	root := fakeXDGData(t)
	writeSession(t, root, "projhash", "ses_abc.json", sessionFixture)

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	docs, err := New(database).Import()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	d := docs[0]
	if d.Source != "opencode" || d.Kind != "log" {
		t.Errorf("source/kind = %q/%q", d.Source, d.Kind)
	}
	if !strings.Contains(d.Title, "Add caching layer") || !strings.Contains(d.Title, "mJ8UpQ") {
		t.Errorf("title = %q (want title + id suffix)", d.Title)
	}
	if !strings.Contains(d.Content, "/Users/u/Workspace/CodeStat") {
		t.Errorf("content missing directory: %q", d.Content)
	}
	if !strings.Contains(d.Content, "in-memory cache") {
		t.Errorf("content missing summary: %q", d.Content)
	}
	if d.ProjectID != 0 {
		t.Errorf("empty db should match no project, got %d", d.ProjectID)
	}
}

func TestImportIgnoresGarbageAndEmpty(t *testing.T) {
	root := fakeXDGData(t)
	writeSession(t, root, "h", "good.json", sessionFixture)
	writeSession(t, root, "h", "notjson.json", "{ this is not json")
	writeSession(t, root, "h", "empty.json", `{"id":"ses_x","projectID":"p","directory":"/x/y","title":"","slug":"","summary":""}`)

	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected only the good session, got %d", len(docs))
	}
}

func TestImportMissingDirIsNoop(t *testing.T) {
	fakeXDGData(t)
	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected no docs, got %d", len(docs))
	}
}

func TestNoteTitleStable(t *testing.T) {
	a := noteTitle("Add caching layer", "ses_3dd894b12ffeUXjGy50UmJ8UpQ")
	b := noteTitle("Add caching layer", "ses_3dd894b12ffeUXjGy50UmJ8UpQ")
	if a != b {
		t.Fatalf("noteTitle not deterministic: %q vs %q", a, b)
	}
	if !strings.HasSuffix(a, "mJ8UpQ") {
		t.Errorf("title should end with id tail, got %q", a)
	}
}

func TestMatchProjectFromDirectory(t *testing.T) {
	projects := []db.Project{{ID: 9, Name: "CodeStat"}}
	if got := memsrc.MatchProject(memsrc.LastPathSegment("/Users/u/Workspace/CodeStat"), projects, nil); got != 9 {
		t.Fatalf("match = %d, want 9", got)
	}
}

func TestImporterImplementsInterface(t *testing.T) {
	var _ plugin.KnowledgeImporter = New((*sql.DB)(nil))
}
