package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// countPageVersions returns the number of snapshots stored for a page.
func countPageVersions(t *testing.T, db *sql.DB, pageID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM page_versions WHERE page_id = ?", pageID).Scan(&n); err != nil {
		t.Fatalf("count page versions: %v", err)
	}
	return n
}

// ADR-0021 晋升条件 2: the snapshot trigger fires when — and only when —
// title/content/kind really change. status transitions (approve/reject) and
// source/wall-clock touches must be silent, the v11 lesson: review state has
// its own queue, mixing it into content history fills the version list with
// no-op entries.
func TestPageVersionsSnap_TriggerPrecision(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	page, err := CreateWikiPage(db, "concept", "trigger-precision", "Title", 0, "body v1")
	if err != nil {
		t.Fatalf("CreateWikiPage: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 0 {
		t.Fatalf("fresh page should have 0 versions, got %d", got)
	}

	// status migration (the approve/reject path): no snapshot.
	if err := SetWikiPageStatus(db, page.ID, "pending"); err != nil {
		t.Fatalf("SetWikiPageStatus pending: %v", err)
	}
	if err := SetWikiPageStatus(db, page.ID, "approved"); err != nil {
		t.Fatalf("SetWikiPageStatus approved: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 0 {
		t.Errorf("status transitions must not snapshot, versions = %d", got)
	}

	// No-op content update (identical title/kind/content; only updated_at
	// moves): still no snapshot.
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "body v1"); err != nil {
		t.Fatalf("UpdateWikiPage no-op: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 0 {
		t.Errorf("no-op update must not snapshot, versions = %d", got)
	}

	// Each of the three snapshotted columns alone must fire the trigger.
	for _, tc := range []struct {
		name                       string
		title, kind, content       string
	}{
		{"title", "Title v2", "concept", "body v1"},
		{"kind", "Title v2", "entity", "body v1"},
		{"content", "Title v2", "entity", "body v2"},
	} {
		if err := UpdateWikiPage(db, page.ID, tc.title, tc.kind, tc.content); err != nil {
			t.Fatalf("UpdateWikiPage (%s): %v", tc.name, err)
		}
	}
	if got := countPageVersions(t, db, page.ID); got != 3 {
		t.Fatalf("title/kind/content updates should snapshot 3 times, versions = %d", got)
	}
}

// The narrowed trigger must survive a fresh init and carry the WHEN clause in
// sqlite_master (the note-side twin of TestNoteVersionsSnap_TriggerHasWhenClause).
func TestPageVersionsSnap_TriggerHasWhenClause(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var sql string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'page_versions_snap'").Scan(&sql)
	if err != nil {
		t.Fatalf("trigger not found: %v", err)
	}
	for _, want := range []string{"WHEN", "new.title IS NOT old.title", "new.content IS NOT old.content", "new.kind IS NOT old.kind"} {
		if !strings.Contains(sql, want) {
			t.Errorf("trigger should contain %q, got:\n%s", want, sql)
		}
	}
}

// ADR-0021 晋升条件 3: rollback snapshots the current content FIRST (history
// only grows), then applies the old version through the same write path; and
// replaying the same version is idempotent (the WHEN clause sees no real
// change the second time).
func TestPageVersions_RestoreKeepsHistoryAndIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	page, err := CreateWikiPage(db, "concept", "restore-flow", "Title", 0, "body v1")
	if err != nil {
		t.Fatalf("CreateWikiPage: %v", err)
	}
	// The trigger snapshots the NEW row (the v8 note precedent), so each
	// update leaves the updated content behind as a version.
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "body v2"); err != nil {
		t.Fatalf("UpdateWikiPage v2: %v", err)
	}
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "body v3"); err != nil {
		t.Fatalf("UpdateWikiPage v3: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 2 {
		t.Fatalf("expected 2 snapshots before restore, got %d", got)
	}

	versions, err := ListPageVersions(db, page.ID)
	if err != nil {
		t.Fatalf("ListPageVersions: %v", err)
	}
	// Newest first: the v3 snapshot, then v2. (v1 predates the first UPDATE,
	// so it was never snapshotted — create is an INSERT, not an UPDATE.)
	if len(versions) != 2 || versions[0].Content != "body v3" || versions[1].Content != "body v2" {
		t.Fatalf("unexpected history: %+v", versions)
	}
	target := versions[1]

	if err := RestorePageVersion(db, page.ID, target.ID); err != nil {
		t.Fatalf("RestorePageVersion: %v", err)
	}
	current, err := GetWikiPageByID(db, page.ID)
	if err != nil {
		t.Fatalf("GetWikiPageByID: %v", err)
	}
	if current.Content != "body v2" {
		t.Fatalf("restore did not apply the target version: %+v", current)
	}
	// ADR-0021 决策 3: the displaced state (body v3) is snapshotted explicitly
	// before the old version is applied, and the applied state is snapshotted
	// by the trigger — 2 + 2 = 4 versions, nothing lost.
	if got := countPageVersions(t, db, page.ID); got != 4 {
		t.Fatalf("rollback must preserve the displaced state and the applied one, versions = %d", got)
	}
	history, err := ListPageVersions(db, page.ID)
	if err != nil {
		t.Fatalf("ListPageVersions after restore: %v", err)
	}
	if history[0].Content != "body v2" {
		t.Fatalf("newest snapshot should be the restored content, got %q", history[0].Content)
	}

	// Replaying the same version changes nothing and snapshots nothing.
	if err := RestorePageVersion(db, page.ID, target.ID); err != nil {
		t.Fatalf("RestorePageVersion replay: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 4 {
		t.Fatalf("replaying the same version must be idempotent, versions = %d", got)
	}

	// A version from another page is refused.
	other, err := CreateWikiPage(db, "concept", "restore-other", "Other", 0, "other body")
	if err != nil {
		t.Fatalf("CreateWikiPage other: %v", err)
	}
	if err := RestorePageVersion(db, other.ID, target.ID); err == nil {
		t.Fatal("restoring a foreign version must fail")
	}
}

// ADR-0021 晋升条件 4: deleting a page takes its versions with it (FK
// cascade); rejecting a pending page is a status transition, so it leaves no
// version residue (reject≠delete, and the tombstone is not history).
func TestPageVersions_DeleteCascadesAndRejectLeavesNothing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	page, err := CreateWikiPageAs(db, "concept", "cascade-page", "Title", 0, "body v1", "pending", "wiki-compile")
	if err != nil {
		t.Fatalf("CreateWikiPageAs: %v", err)
	}
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "body v2"); err != nil {
		t.Fatalf("UpdateWikiPage: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 1 {
		t.Fatalf("expected 1 snapshot, got %d", got)
	}

	// Reject is a status transition: no new snapshot.
	if err := SetWikiPageStatus(db, page.ID, "rejected"); err != nil {
		t.Fatalf("SetWikiPageStatus rejected: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 1 {
		t.Fatalf("reject must not snapshot, versions = %d", got)
	}

	// Delete cascades the history away with the page.
	if err := DeleteWikiPage(db, page.ID); err != nil {
		t.Fatalf("DeleteWikiPage: %v", err)
	}
	if got := countPageVersions(t, db, page.ID); got != 0 {
		t.Fatalf("versions must cascade on delete, got %d", got)
	}
}

// ADR-0021 晋升条件 1: the v23 migration is replay-safe (idempotent) and
// reversible through the layer's own Drop/Ensure pair — the same discipline
// TestWikiMigration_IsReversibleByConstruction pins at the layer level, here
// proven for the page_versions objects specifically.
func TestPageVersions_MigrationReplayAndReverse(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pv.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Replay: re-running InitDB over an up-to-date database is a no-op.
	if _, err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB replay: %v", err)
	}
	var version int
	if err := db.QueryRow("SELECT value FROM app_config WHERE key = 'schema_version'").Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != 23 {
		t.Fatalf("schema_version = %d, want 23", version)
	}

	// Reverse: dropping the wiki layer removes every page_versions object.
	if err := DropWikiSchema(db); err != nil {
		t.Fatalf("DropWikiSchema: %v", err)
	}
	for _, obj := range []struct{ typ, name string }{
		{"table", "page_versions"},
		{"index", "idx_page_versions_page_id"},
		{"trigger", "page_versions_snap"},
		{"trigger", "page_versions_cleanup"},
	} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?", obj.typ, obj.name).Scan(&n); err != nil {
			t.Fatalf("sqlite_master: %v", err)
		}
		if n != 0 {
			t.Errorf("%s %s survived DropWikiSchema", obj.typ, obj.name)
		}
	}
	// Re-apply: the layer (page_versions included) comes back.
	if err := EnsureWikiSchema(db); err != nil {
		t.Fatalf("EnsureWikiSchema: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'page_versions'").Scan(&n); err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	if n != 1 {
		t.Fatal("page_versions did not come back after EnsureWikiSchema")
	}
}

// The diff read-out uses the same LCS algorithm as the note side.
func TestPageVersions_DiffAgainstCurrent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	page, err := CreateWikiPage(db, "concept", "diff-page", "Title", 0, "line one\nline two\n")
	if err != nil {
		t.Fatalf("CreateWikiPage: %v", err)
	}
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "line one\nline 2\n"); err != nil {
		t.Fatalf("UpdateWikiPage: %v", err)
	}
	if err := UpdateWikiPage(db, page.ID, "Title", "concept", "line one\nline 2\nline three\n"); err != nil {
		t.Fatalf("UpdateWikiPage again: %v", err)
	}
	versions, err := ListPageVersions(db, page.ID)
	if err != nil {
		t.Fatalf("ListPageVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(versions))
	}
	// The older snapshot (versions[1], "line one\nline 2\n") against the
	// current content ("line one\nline 2\nline three\n").
	got, err := DiffPageVersions(db, page.ID, versions[1].ID)
	if err != nil {
		t.Fatalf("DiffPageVersions: %v", err)
	}
	if !strings.Contains(got, "+line three") {
		t.Fatalf("diff missing the added line:\n%s", got)
	}
}
