package runtime

import (
	"database/sql"
	"testing"

	"repotory/internal/core/plugin"
	"repotory/internal/db"
	"repotory/internal/domain"
)

// newFullDB builds a complete in-memory database via the production InitDB so the
// landing path can exercise the real projects table, the seeded default project,
// and the FTS/embedding triggers that note writes depend on.
func newFullDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func noteProjectID(t *testing.T, database *sql.DB, title string) int64 {
	t.Helper()
	var pid int64
	if err := database.QueryRow("SELECT project_id FROM project_notes WHERE title = ?", title).Scan(&pid); err != nil {
		t.Fatalf("note %q not written: %v", title, err)
	}
	return pid
}

// An unmatched note that carries a repo path must be LANDED on a placeholder
// project, never skipped (ADR-0023 decision 3).
func TestImportLandsUnmatchedWithRepoPathOnPlaceholder(t *testing.T) {
	database := newFullDB(t)
	rt := New(database)
	rt.RegisterSourceManual("fake", staticImporter{source: "fake", docs: []plugin.ImportDoc{
		{RepoPath: "/code/ghost-repo", Title: "orphan", Content: "x", Kind: "log", Source: "fake"},
	}})

	run, err := rt.TriggerImport("fake")
	if err != nil {
		t.Fatalf("TriggerImport: %v", err)
	}
	if run.Skipped != 0 {
		t.Fatalf("a repo-path doc must be landed, not skipped (skipped=%d)", run.Skipped)
	}
	if run.Created != 1 {
		t.Fatalf("want 1 created note, got %+v", run)
	}

	var (
		pid        int64
		kind, name string
	)
	if err := database.QueryRow(
		"SELECT id, project_kind, name FROM projects WHERE root_path = '/code/ghost-repo'",
	).Scan(&pid, &kind, &name); err != nil {
		t.Fatalf("placeholder project not created: %v", err)
	}
	if kind != domain.ProjectKindImportPlaceholder {
		t.Errorf("placeholder kind = %q, want %q", kind, domain.ProjectKindImportPlaceholder)
	}
	if name != "ghost-repo" {
		t.Errorf("placeholder name = %q, want repo base name %q", name, "ghost-repo")
	}
	if got := noteProjectID(t, database, "orphan"); got != pid {
		t.Errorf("note landed on project %d, want the placeholder %d", got, pid)
	}
}

// An unmatched note with no repo path (agent-global source) must land on the
// seeded default ("Uncategorized") project, not be skipped.
func TestImportLandsUnmatchedWithoutRepoPathOnDefault(t *testing.T) {
	database := newFullDB(t)
	defID, err := db.GetDefaultProjectID(database)
	if err != nil || defID <= 0 {
		t.Fatalf("default project must exist: id=%d err=%v", defID, err)
	}
	rt := New(database)
	rt.RegisterSourceManual("glob", staticImporter{source: "glob", docs: []plugin.ImportDoc{
		{Title: "global", Content: "y", Source: "glob"}, // no repo path -> default
	}})

	run, err := rt.TriggerImport("glob")
	if err != nil {
		t.Fatalf("TriggerImport: %v", err)
	}
	if run.Skipped != 0 {
		t.Fatalf("a path-less doc must land on the default, not be skipped (skipped=%d)", run.Skipped)
	}
	if got := noteProjectID(t, database, "global"); got != defID {
		t.Errorf("note landed on project %d, want the default project %d", got, defID)
	}
}

// A note that already matched a project keeps its explicit ProjectID; the landing
// path must not touch resolved docs.
func TestImportKeepsResolvedProjectID(t *testing.T) {
	database := newFullDB(t)
	var realID int64
	if err := database.QueryRow(
		"INSERT INTO projects (name, root_path, project_kind) VALUES ('real', '/code/real', 'scanned') RETURNING id",
	).Scan(&realID); err != nil {
		t.Fatal(err)
	}
	rt := New(database)
	rt.RegisterSourceManual("mixed", staticImporter{source: "mixed", docs: []plugin.ImportDoc{
		{ProjectID: realID, Title: "resolved", Content: "z", Source: "mixed"},
		{RepoPath: "/code/land", Title: "landed", Content: "w", Source: "mixed"},
	}})

	run, err := rt.TriggerImport("mixed")
	if err != nil {
		t.Fatalf("TriggerImport: %v", err)
	}
	if run.Created != 2 || run.Skipped != 0 {
		t.Fatalf("want both docs written, got %+v", run)
	}
	if got := noteProjectID(t, database, "resolved"); got != realID {
		t.Errorf("resolved note moved off its project: %d, want %d", got, realID)
	}
	// The landed note must NOT be on the resolved project, nor on the default.
	defID, _ := db.GetDefaultProjectID(database)
	if got := noteProjectID(t, database, "landed"); got == realID || got == defID {
		t.Errorf("repo-path note should land on a fresh placeholder, got project %d (resolved=%d default=%d)", got, realID, defID)
	}
}
