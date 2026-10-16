package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The point of CopyForTooling is a property about a file the tool must not touch,
// so the central test asserts on the SOURCE database after the copy has been
// migrated — not merely that the copy works.

// stampBehind simulates a database that predates the current binary: it rewinds the
// version stamp and removes the newest migration's objects, so a migration really
// has work to do when something opens it.
func stampBehind(t *testing.T, path string, to string) {
	t.Helper()
	d, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open for stamping: %v", err)
	}
	defer func() { _ = d.Close() }()
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS page_versions_no_update`,
		`DROP TRIGGER IF EXISTS page_versions_no_delete`,
		`DROP TRIGGER IF EXISTS page_versions_snap`,
		`DROP INDEX IF EXISTS idx_page_versions_page`,
		`DROP TABLE IF EXISTS page_versions`,
		`UPDATE app_config SET value = '` + to + `' WHERE key = 'schema_version'`,
	} {
		if _, err := d.Exec(stmt); err != nil {
			t.Fatalf("stamp %q: %v", stmt, err)
		}
	}
}

func readVersion(t *testing.T, path string) string {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatalf("open for read: %v", err)
	}
	defer func() { _ = d.Close() }()
	var v string
	if err := d.QueryRow(`SELECT value FROM app_config WHERE key='schema_version'`).Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	return v
}

func hasTable(t *testing.T, path, table string) bool {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = d.Close() }()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
		t.Fatalf("inspect %s: %v", table, err)
	}
	return n > 0
}

// The regression this whole file exists for: an analysis tool opening a live
// database through InitDB MIGRATED it. The copy must absorb the upgrade and the
// source must be byte-identical in the ways that matter.
func TestCopyForToolingMigratesTheCopyNotTheSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.db")

	first, err := InitDB(src)
	if err != nil {
		t.Fatalf("init source: %v", err)
	}
	proj := createTestProject(t, first, "toolcopy")
	if _, err := CreateWikiPage(first, WikiKindConcept, "cp-1", "Page", proj, "body"); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if err := EnsurePageVersions(first); err != nil {
		t.Fatalf("ensure versions: %v", err)
	}
	_ = first.Close()

	stampBehind(t, src, "20")
	if readVersion(t, src) != "20" || hasTable(t, src, "page_versions") {
		t.Fatal("precondition failed: the source should look like a pre-v22 database")
	}

	cp, err := CopyForTooling(src, OpenCopyOptions{})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	defer func() { _ = cp.Close() }()

	// The copy is current and usable.
	var got int
	if err := cp.DB.QueryRow(`SELECT value = '24' FROM app_config WHERE key='schema_version'`).Scan(&got); err != nil {
		t.Fatalf("read copy version: %v", err)
	}
	if got != 1 {
		t.Fatal("the copy should have been migrated to v24")
	}
	var snaps int
	if err := cp.DB.QueryRow(`SELECT COUNT(*) FROM page_versions`).Scan(&snaps); err != nil {
		t.Fatalf("copy page_versions: %v", err)
	}
	if snaps != 1 {
		t.Fatalf("the v22 backfill should have run on the copy, got %d snapshots", snaps)
	}

	// And the source is exactly as untouched as it was.
	if v := readVersion(t, src); v != "20" {
		t.Fatalf("the copy operation MIGRATED the source: schema_version=%s", v)
	}
	if hasTable(t, src, "page_versions") {
		t.Fatal("the copy operation created objects in the source database")
	}
}

// A read-only handle must refuse writes — this is what makes "the source is safe"
// an enforced property rather than a promise about code discipline.
func TestCopyForToolingSourceIsPhysicallyReadOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ro.db")
	d, err := InitDB(src)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	_ = d.Close()

	cp, err := CopyForTooling(src, OpenCopyOptions{})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	defer func() { _ = cp.Close() }()

	srcRO, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer func() { _ = srcRO.Close() }()
	if _, err := srcRO.Exec(`CREATE TABLE should_not_exist (x INTEGER)`); err == nil {
		t.Fatal("a write to the source succeeded; mode=ro is not doing its job")
	}
	if hasTable(t, src, "should_not_exist") {
		t.Fatal("the table exists anyway")
	}
	// Writes through the copy are expected to work (that is where tools may write).
	if _, err := cp.DB.Exec(`CREATE TABLE may_exist (x INTEGER)`); err != nil {
		t.Fatalf("writing to the copy should succeed: %v", err)
	}
}

func TestCopyForToolingCleanupAndKeep(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "cleanup.db")
	d, err := InitDB(src)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	_ = d.Close()

	cp, err := CopyForTooling(src, OpenCopyOptions{})
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	copyPath := cp.Path
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatalf("the copy should exist while open: %v", err)
	}
	if err := cp.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Default: the temp tree is gone. Leaving multi-megabyte database copies behind
	// on every tool run would be its own leak.
	if _, err := os.Stat(filepath.Dir(copyPath)); !os.IsNotExist(err) {
		t.Fatalf("the copy directory should have been removed, stat err = %v", err)
	}

	kept, err := CopyForTooling(src, OpenCopyOptions{KeepCopy: true})
	if err != nil {
		t.Fatalf("keep copy: %v", err)
	}
	keptPath := kept.Path
	if err := kept.Close(); err != nil {
		t.Fatalf("close kept: %v", err)
	}
	if _, err := os.Stat(keptPath); err != nil {
		t.Fatalf("KeepCopy must leave the artifact (it is the tool's output): %v", err)
	}
	_ = os.RemoveAll(filepath.Dir(keptPath))
}

func TestCopyForToolingRefusesBadSources(t *testing.T) {
	dir := t.TempDir()

	if _, err := CopyForTooling(filepath.Join(dir, "missing.db"), OpenCopyOptions{}); err == nil {
		t.Fatal("a nonexistent source must be refused")
	} else if !strings.Contains(err.Error(), "missing.db") {
		t.Fatalf("the error should name the file, got %v", err)
	}

	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, []byte("not a database, just a file with a .db name"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyForTooling(junk, OpenCopyOptions{}); err == nil {
		t.Fatal("a non-SQLite file must be refused")
	} else if !strings.Contains(err.Error(), "not a readable SQLite database") {
		t.Fatalf("expected the explicit non-database message, got %v", err)
	}
}

// A read-only caller that cannot migrate must refuse a behind database loudly.
// Querying a half-migrated store produces confident nonsense, which is worse than
// an error: nothing downstream looks wrong.
func TestRequireSchemaVersion(t *testing.T) {
	database := setupTestDB(t)
	if err := RequireSchemaVersion(database, 24); err != nil {
		t.Fatalf("a current database should pass, got %v", err)
	}
	if err := RequireSchemaVersion(database, 99); !errors.Is(err, ErrSchemaBehind) {
		t.Fatalf("a behind database must return ErrSchemaBehind, got %v", err)
	} else {
		// The remedy has to be in the message, or the error is just a shrug.
		for _, want := range []string{"v24", "v99", "desktop app", "copy"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q should mention %q", err.Error(), want)
			}
		}
	}
	// Newer-than-expected is not "behind": a tool built from an older tree should
	// still be able to read, and refusing would make every downgrade fatal.
	if err := RequireSchemaVersion(database, 1); err != nil {
		t.Fatalf("expected > current should not be refused: %v", err)
	}
}
