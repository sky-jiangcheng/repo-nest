package openclaw

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
)

func fakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportAllowlistsWorkspaceMarkdown(t *testing.T) {
	home := fakeHome(t)
	ws := filepath.Join(home, ".openclaw-autoclaw", "workspace")
	mustWrite(t, filepath.Join(ws, "SOUL.md"), "---\nfm: yes\n---\nsoul body text")
	mustWrite(t, filepath.Join(ws, "USER.md"), "user profile body")
	// Must be ignored: non-md, a subdir file (no recursion), and a sibling of
	// workspace under the secret-bearing parent.
	mustWrite(t, filepath.Join(ws, "notes.txt"), "x")
	mustWrite(t, filepath.Join(ws, "sub", "nested.md"), "nested")
	mustWrite(t, filepath.Join(home, ".openclaw-autoclaw", "vault-roots.md"), "PARENT MUST NOT IMPORT")

	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs (SOUL, USER), got %d: %+v", len(docs), titles(docs))
	}
	for _, d := range docs {
		if strings.Contains(d.Title, "vault-roots") || strings.Contains(d.Title, "nested") || strings.Contains(d.Title, "notes") {
			t.Fatalf("allowlist violation: imported %q", d.Title)
		}
		if d.Source != "openclaw" || d.Tags != "openclaw" {
			t.Errorf("source/tags = %q/%q", d.Source, d.Tags)
		}
	}
	soul := docs[0]
	for _, d := range docs {
		if d.Title == "OpenClaw · SOUL" {
			soul = d
		}
	}
	if !strings.Contains(soul.Content, "soul body text") {
		t.Errorf("frontmatter not stripped / body missing: %q", soul.Content)
	}
	if strings.Contains(soul.Content, "fm: yes") {
		t.Errorf("frontmatter leaked: %q", soul.Content)
	}
	if soul.ProjectID != 0 {
		t.Errorf("no config target -> ProjectID should be 0, got %d", soul.ProjectID)
	}
}

func TestImportMissingIsNoop(t *testing.T) {
	fakeHome(t)
	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil || len(docs) != 0 {
		t.Fatalf("expected noop, got err=%v docs=%d", err, len(docs))
	}
}

func TestImportClipsOversizedToValidUTF8(t *testing.T) {
	home := fakeHome(t)
	ws := filepath.Join(home, ".openclaw-autoclaw", "workspace")
	// A body far larger than the note cap (CJK, 3 bytes each).
	huge := strings.Repeat("漢", db.MaxNoteContentLen)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "SOUL.md"), []byte(huge), 0o644); err != nil {
		t.Fatal(err)
	}
	database, _ := sql.Open("sqlite", ":memory:")
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil || len(docs) != 1 {
		t.Fatalf("import: err=%v docs=%d", err, len(docs))
	}
	c := docs[0].Content
	if len(c) > db.MaxNoteContentLen {
		t.Errorf("content %d bytes exceeds cap %d (header added after clip?)", len(c), db.MaxNoteContentLen)
	}
	if !utf8.ValidString(c) {
		t.Error("content clipped mid-rune (invalid UTF-8)")
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
