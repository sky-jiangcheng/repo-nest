package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"repotory/internal/domain"
)

// columnShape reads projects.project_kind's definition from pragma_table_info.
func columnShape(t *testing.T, database *sql.DB) (name, typ string, notNull int, def sql.NullString) {
	t.Helper()
	err := database.QueryRow(
		`SELECT name, type, "notnull", dflt_value FROM pragma_table_info('projects') WHERE name = 'project_kind'`,
	).Scan(&name, &typ, &notNull, &def)
	if err == sql.ErrNoRows {
		t.Fatal("project_kind column is missing from projects")
	}
	if err != nil {
		t.Fatalf("read project_kind column: %v", err)
	}
	return
}

// TestProjectKindColumnExists asserts migration v24 added the column with the
// shape ADR-0023 decision 1 specifies: TEXT NOT NULL, constant default 'scanned'
// (the constant default is what lets pre-v24 rows read back as scanned).
func TestProjectKindColumnExists(t *testing.T) {
	database := setupTestDB(t)
	name, typ, notNull, def := columnShape(t, database)
	if name != "project_kind" {
		t.Fatalf("column name = %q", name)
	}
	if typ != "TEXT" {
		t.Fatalf("column type = %q, want TEXT", typ)
	}
	if notNull != 1 {
		t.Fatalf("column notnull = %d, want 1", notNull)
	}
	if !def.Valid || def.String != "'"+domain.ProjectKindScanned+"'" {
		t.Fatalf("column default = %q (valid=%v), want '%s'", def.String, def.Valid, domain.ProjectKindScanned)
	}
}

// TestDefaultProjectSeeded asserts the single default project exists with the
// sentinel root_path and internal name from ADR-0023 decision 2.
func TestDefaultProjectSeeded(t *testing.T) {
	database := setupTestDB(t)
	var (
		id            int64
		gotName, root string
	)
	err := database.QueryRow(
		`SELECT id, name, root_path FROM projects WHERE project_kind = 'default'`,
	).Scan(&id, &gotName, &root)
	if err != nil {
		t.Fatalf("default project row missing: %v", err)
	}
	if gotName != domain.DefaultProjectName {
		t.Fatalf("default name = %q, want %q", gotName, domain.DefaultProjectName)
	}
	if root != domain.DefaultProjectRootPath {
		t.Fatalf("default root_path = %q, want %q", root, domain.DefaultProjectRootPath)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected exactly one default project, got %d", n)
	}
}

// TestEnsureProjectKindIdempotent calls EnsureProjectKind again after a full init
// and asserts it neither errors (the ALTER hits a duplicate column) nor duplicates
// the default row — the property that makes it safe as an InitDB self-heal.
func TestEnsureProjectKindIdempotent(t *testing.T) {
	database := setupTestDB(t)
	if err := EnsureProjectKind(database); err != nil {
		t.Fatalf("second EnsureProjectKind must be a tolerated no-op: %v", err)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("re-running EnsureProjectKind must not duplicate the default row, got %d", n)
	}
}

// TestProjectKindReopenKeepsOneDefault opens a file-backed database twice; the
// second open stamps v24 already-applied but still runs the EnsureProjectKind
// self-heal, so exactly one default row must survive — proving migration +
// self-heal converge rather than double-seed.
func TestProjectKindReopenKeepsOneDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kind.db")

	first, err := InitDB(path)
	if err != nil {
		t.Fatalf("first InitDB: %v", err)
	}
	var pre int
	if err := first.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='default'`).Scan(&pre); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	second, err := InitDB(path)
	if err != nil {
		t.Fatalf("reopen InitDB: %v", err)
	}
	defer second.Close() //nolint:errcheck

	var post int
	if err := second.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='default'`).Scan(&post); err != nil {
		t.Fatal(err)
	}
	if pre != 1 || post != 1 {
		t.Fatalf("default row count pre=%d post=%d, want 1 and 1", pre, post)
	}
	var version int
	if err := second.QueryRow(`SELECT value FROM app_config WHERE key='schema_version'`).Scan(&version); err != nil {
		// schema_version is stored as TEXT; Atoi-free Scan into int works via
		// modernc coercion, but be defensive and report the raw if it fails.
		var raw string
		_ = second.QueryRow(`SELECT value FROM app_config WHERE key='schema_version'`).Scan(&raw)
		t.Fatalf("read schema_version (raw=%q): %v", raw, err)
	}
	if version != 24 {
		t.Fatalf("schema_version = %d, want 24", version)
	}
}

// TestGetDefaultProjectID pins the helper the importer landing path uses.
func TestGetDefaultProjectID(t *testing.T) {
	database := setupTestDB(t)
	id, err := GetDefaultProjectID(database)
	if err != nil {
		t.Fatalf("GetDefaultProjectID: %v", err)
	}
	if id <= 0 {
		t.Fatalf("GetDefaultProjectID = %d, want a positive id", id)
	}
	// The sentinel must never match a scanned repo path, so a repo-style insert
	// cannot collide with it via the UNIQUE index.
	if _, err := database.Exec(
		"INSERT INTO projects (name, root_path, project_kind) VALUES ('real', '/code/real', 'scanned')",
	); err != nil {
		t.Fatalf("inserting a scanned project near the sentinel: %v", err)
	}
}

// TestCleanupKeepsEmptyDefaultProject is the ADR-0023 decision 6 exemption: the
// default project has no repos/notes/todos yet must survive an empty cleanup.
func TestCleanupKeepsEmptyDefaultProject(t *testing.T) {
	database := setupTestDB(t)
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// scannedPaths is non-empty (an empty slice is a hard error) but matches
	// nothing, so every repo-less project is a cleanup candidate except the
	// exempt default row.
	if err := CleanupStaleDataTx(tx, []string{"/tmp/some-live-repo"}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("cleanup: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='default'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("default project must survive empty cleanup, got %d", n)
	}
}

// TestCleanupStillDeletesEmptyPlaceholder proves the exemption is scoped to
// 'default' only: an empty placeholder (import notes organized away) is still
// collectable, so the default project is the sole non-deletable row.
func TestCleanupStillDeletesEmptyPlaceholder(t *testing.T) {
	database := setupTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO projects (name, root_path, project_kind) VALUES ('ph', 'repotory://placeholder/gone', 'import_placeholder')",
	); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := CleanupStaleDataTx(tx, []string{"/tmp/some-live-repo"}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("cleanup: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM projects WHERE project_kind='import_placeholder'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty import_placeholder project must be cleaned up, got %d", n)
	}
}
