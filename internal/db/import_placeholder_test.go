package db

import (
	"testing"

	"repotory/internal/domain"
)

// EnsureImportPlaceholder creates one import_placeholder project keyed on the
// repo path, is idempotent on the path (root_path UNIQUE), derives the name from
// the base of the path when no name is given, and returns 0 for an empty path so
// the caller falls back to the default project (ADR-0023 decision 3).
func TestEnsureImportPlaceholder(t *testing.T) {
	database := setupTestDB(t)

	id1, err := EnsureImportPlaceholder(database, "/code/ghost", "")
	if err != nil {
		t.Fatalf("EnsureImportPlaceholder: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("expected a positive project id, got %d", id1)
	}
	var kind, name string
	if err := database.QueryRow(
		"SELECT project_kind, name FROM projects WHERE id = ?", id1,
	).Scan(&kind, &name); err != nil {
		t.Fatal(err)
	}
	if kind != domain.ProjectKindImportPlaceholder {
		t.Errorf("kind = %q, want %q", kind, domain.ProjectKindImportPlaceholder)
	}
	if name != "ghost" {
		t.Errorf("name = %q, want base name %q", name, "ghost")
	}

	// Same path -> same row (idempotent), name argument ignored on reuse.
	id2, err := EnsureImportPlaceholder(database, "/code/ghost", "ignored")
	if err != nil || id2 != id1 {
		t.Fatalf("re-run must reuse the placeholder: id2=%d (want %d) err=%v", id2, id1, err)
	}

	// Explicit name is honoured for a fresh path (claude-style decoded label).
	id3, err := EnsureImportPlaceholder(database, "import://claude/-Users-me-Foo", "Foo")
	if err != nil {
		t.Fatal(err)
	}
	var name3 string
	database.QueryRow("SELECT name FROM projects WHERE id = ?", id3).Scan(&name3)
	if name3 != "Foo" {
		t.Errorf("name = %q, want explicit %q", name3, "Foo")
	}

	// Empty path -> no placeholder (0, nil), so the caller lands on the default.
	if id, err := EnsureImportPlaceholder(database, "   ", "x"); err != nil || id != 0 {
		t.Fatalf("empty path must return 0, got id=%d err=%v", id, err)
	}
}

// When a scan later discovers the repository backing a placeholder, SyncProjectTx
// — which upserts on root_path — must promote the placeholder to a scanned
// project in place, so the landing project transparently becomes a real one and
// its notes stay put (ADR-0023 decision 4).
func TestImportPlaceholderPromotesOnScan(t *testing.T) {
	database := setupTestDB(t)
	id, err := EnsureImportPlaceholder(database, "/code/real", "")
	if err != nil {
		t.Fatal(err)
	}
	// The placeholder starts un-collected / import kind.
	var before string
	database.QueryRow("SELECT project_kind FROM projects WHERE id = ?", id).Scan(&before)
	if before != domain.ProjectKindImportPlaceholder {
		t.Fatalf("precondition: kind = %q", before)
	}

	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := SyncProjectTx(tx, "real", "/code/real", 0, true)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("SyncProjectTx: %v", err)
	}
	if gotID != id {
		t.Fatalf("SyncProjectTx must adopt the existing placeholder row, got %d want %d", gotID, id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var after string
	database.QueryRow("SELECT project_kind FROM projects WHERE id = ?", id).Scan(&after)
	if after != domain.ProjectKindScanned {
		t.Fatalf("placeholder should promote to %q after a scan adopts it, got %q",
			domain.ProjectKindScanned, after)
	}
}
