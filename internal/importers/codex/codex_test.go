package codex

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

// fakeHome points os.UserHomeDir at an empty temp dir via $HOME.
func fakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// fixture mirrors the verified real rollout shape (session_meta line 1, then
// response_item message events whose content is an array of {type,text}).
const fixture = `{"timestamp":"2026-03-20T09:38:04.229Z","ordinal":0,"type":"session_meta","payload":{"session_id":"019d0a9b-8c9a-7f03","id":"019d0a9b-8c9a-7f03","timestamp":"2026-03-20T09:37:39.486Z","cwd":"/Users/u/Workspace/Foo"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Fix the flaky test in auth"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done: guarded the timer and added a wait."}]}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_tokens":123}}}
this is not json and must be skipped
{"type":"response_item","payload":{"type":"message","role":"assistant","content":"bare string reply"}}
`

func TestExtractText(t *testing.T) {
	arr := []byte(`[{"type":"output_text","text":"a"},{"type":"refusal","text":""},{"type":"text","text":"b"}]`)
	if got := extractText(arr); got != "a\nb" {
		t.Errorf("array extract = %q, want %q", got, "a\nb")
	}
	if got := extractText([]byte(`"hello"`)); got != "hello" {
		t.Errorf("string extract = %q", got)
	}
	if got := extractText(nil); got != "" {
		t.Errorf("nil extract = %q", got)
	}
}

func TestParseRolloutFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-fixture.jsonl")
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := parseRolloutFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.cwd != "/Users/u/Workspace/Foo" {
		t.Errorf("cwd = %q", p.cwd)
	}
	if p.sessionID != "019d0a9b-8c9a-7f03" {
		t.Errorf("sessionID = %q", p.sessionID)
	}
	if p.date != "2026-03-20" {
		t.Errorf("date = %q", p.date)
	}
	if p.firstUser != "Fix the flaky test in auth" {
		t.Errorf("firstUser = %q", p.firstUser)
	}
	// The LAST assistant message (bare-string form) wins over the earlier one.
	if p.lastAssistant != "bare string reply" {
		t.Errorf("lastAssistant = %q", p.lastAssistant)
	}
}

func TestTitleForStableAndUnique(t *testing.T) {
	p := parsed{sessionID: "019d0a9b-8c9a-7f03", firstUser: "Fix the flaky test in auth", date: "2026-03-20"}
	got := titleFor(p)
	if got != "Fix the flaky test in auth · 019d0a9b" {
		t.Errorf("title = %q", got)
	}
	if titleFor(p) != got {
		t.Error("titleFor must be deterministic for idempotent upsert")
	}
	if titleFor(parsed{}) != "Codex 会话" {
		t.Errorf("empty parsed title = %q", titleFor(parsed{}))
	}
}

func TestImportProducesNote(t *testing.T) {
	home := fakeHome(t)
	sess := filepath.Join(home, ".codex", "sessions", "2026", "03", "20")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sess, "rollout-a.jsonl"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-jsonl file and a message-less file must be ignored.
	if err := os.WriteFile(filepath.Join(sess, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if d.Source != "codex" || d.Kind != "log" {
		t.Errorf("source/kind = %q/%q", d.Source, d.Kind)
	}
	if !strings.Contains(d.Content, "/Users/u/Workspace/Foo") {
		t.Errorf("content missing cwd: %q", d.Content)
	}
	if !strings.Contains(d.Content, "Fix the flaky test in auth") {
		t.Errorf("content missing prompt: %q", d.Content)
	}
	if d.ProjectID != 0 {
		t.Errorf("empty db should match no project, got %d", d.ProjectID)
	}
}

func TestImportMissingDirIsNoop(t *testing.T) {
	fakeHome(t)
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	docs, err := New(database).Import()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected no docs, got %d", len(docs))
	}
}

// The project-matching used by the importer is the shared matcher; verify it
// resolves a session cwd's last path segment to a project.
func TestMatchProjectViaSharedMatcher(t *testing.T) {
	projects := []db.Project{{ID: 7, Name: "Foo"}}
	if got := memsrc.MatchProject(memsrc.LastPathSegment("/Users/u/Workspace/Foo"), projects, nil); got != 7 {
		t.Errorf("match = %d, want 7", got)
	}
}

func TestImporterImplementsInterface(t *testing.T) {
	var _ plugin.KnowledgeImporter = New((*sql.DB)(nil))
}
