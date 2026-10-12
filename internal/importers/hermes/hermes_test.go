package hermes

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repotory/internal/core/plugin"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportUsesHermesHomeAndAllowlist(t *testing.T) {
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	// A bogus HOME proves $HERMES_HOME wins.
	t.Setenv("HOME", t.TempDir())

	mems := filepath.Join(hermesHome, "memories")
	mustWrite(t, filepath.Join(mems, "MEMORY.md"), "agent notes body")
	mustWrite(t, filepath.Join(mems, "USER.md"), "user profile body")
	// Ignored: sibling secrets of the memories dir (never read) + non-md.
	mustWrite(t, filepath.Join(hermesHome, ".env"), "SECRET=1")
	mustWrite(t, filepath.Join(hermesHome, "leak.md"), "PARENT MUST NOT IMPORT")
	mustWrite(t, filepath.Join(mems, "notmd.txt"), "x")

	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs (MEMORY, USER), got %d: %v", len(docs), titles(docs))
	}
	for _, d := range docs {
		if d.Source != "hermes" {
			t.Errorf("source = %q", d.Source)
		}
		if strings.Contains(d.Title, "leak") {
			t.Fatalf("allowlist violation: imported %q", d.Title)
		}
	}
}

func TestImportDefaultsToHomeWhenNoHermesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERMES_HOME", "") // unset override -> use ~/.hermes
	mustWrite(t, filepath.Join(home, ".hermes", "memories", "MEMORY.md"), "hello from home")

	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Title != "Hermes · MEMORY" {
		t.Fatalf("expected 1 MEMORY doc, got %v", titles(docs))
	}
	if !strings.Contains(docs[0].Content, "hello from home") {
		t.Errorf("content = %q", docs[0].Content)
	}
}

func TestImportMissingIsNoop(t *testing.T) {
	hermesHome := t.TempDir() // exists but has no memories/ dir
	t.Setenv("HERMES_HOME", hermesHome)
	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil || len(docs) != 0 {
		t.Fatalf("expected noop, got err=%v docs=%d", err, len(docs))
	}
}

func TestImplementsInterface(t *testing.T) {
	var _ plugin.KnowledgeImporter = New((*sql.DB)(nil))
}

func titles(docs []plugin.ImportDoc) []string {
	var out []string
	for _, d := range docs {
		out = append(out, d.Title)
	}
	return out
}
