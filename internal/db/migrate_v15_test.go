package db

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// v15 turns startup auto-import OFF for databases created before the change.
//
// The reason a migration is needed at all (rather than just flipping the seeded
// default) is that insertDefaults writes `auto_import='1'` as a ROW, so an
// install the user never configured is byte-for-byte indistinguishable from one
// where they deliberately enabled it. The migration is the explicit act that
// closes that ambiguity in the privacy-safe direction; the paired service change
// makes an absent or junk value mean OFF as well.
func TestInitDB_V15DisablesLegacyAutoImport(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// A fresh database must start out OFF.
	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (fresh): %v", err)
	}
	if v := readConfig(t, db1, "auto_import"); v != "0" {
		t.Fatalf("fresh database auto_import = %q, want %q", v, "0")
	}

	// Simulate a legacy install: auto_import seeded ON by the old default, schema
	// stamped at the version before this migration.
	if err := SetConfig(db1, "auto_import", "1"); err != nil {
		t.Fatal(err)
	}
	if err := SetConfig(db1, migrationVersionKey, "14"); err != nil {
		t.Fatal(err)
	}
	if err := SetConfig(db1, "junk_probe", "keep-me"); err != nil {
		t.Fatal(err) // proves the migration only touches the one key
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (upgrade): %v", err)
	}
	defer db2.Close() //nolint:errcheck // test teardown

	if v := readConfig(t, db2, "auto_import"); v != "0" {
		t.Errorf("after v15 auto_import = %q, want %q — legacy installs must not keep auto-importing", v, "0")
	}
	// Assert the invariant ("v15 has run"), not an exact stamp: a version equality
	// here has to be edited on every later migration, and a stale assertion is how
	// this very test broke when v16 landed.
	if v := readConfig(t, db2, migrationVersionKey); !versionAtLeast(v, 15) {
		t.Errorf("schema_version = %q, want >= 15 so the auto-import normalization has run", v)
	}
	if v := readConfig(t, db2, "junk_probe"); v != "keep-me" {
		t.Errorf("unrelated config row was touched: %q", v)
	}
}

// Re-applying the migration must be harmless (it runs inside a retryable
// migration step, and an interrupted upgrade replays it), and an explicit OFF
// must never be flipped back on.
func TestInitDB_V15IsIdempotentAndNeverEnables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	// A user who explicitly turned it on keeps it: v15 only rewrites values that
	// are not '0', and this is the one case the migration must NOT touch — except
	// that it cannot tell "explicit 1" from "seeded 1", which is exactly why the
	// one-time normalization exists. Re-stamping v14 here therefore models a
	// replay of the migration, and the expected outcome is OFF either way.
	if err := SetConfig(db1, "auto_import", "1"); err != nil {
		t.Fatal(err)
	}
	if err := SetConfig(db1, migrationVersionKey, "14"); err != nil {
		t.Fatal(err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (first replay): %v", err)
	}
	if v := readConfig(t, db2, "auto_import"); v != "0" {
		t.Fatalf("first replay: auto_import = %q, want 0", v)
	}
	if err := db2.Close(); err != nil {
		t.Fatal(err)
	}

	// Second open with nothing to do: an already-normalized OFF stays OFF.
	db3, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB (second replay): %v", err)
	}
	defer db3.Close() //nolint:errcheck // test teardown
	if v := readConfig(t, db3, "auto_import"); v != "0" {
		t.Errorf("second replay: auto_import = %q, want 0", v)
	}
}

// A missing row is the state a fresh install would have if the seed were ever
// removed; the value must never be resurrected to ON by a migration.
func TestInitDB_V15DoesNotCreateAnEnablingRow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	if _, err := db1.Exec("DELETE FROM app_config WHERE key = 'auto_import'"); err != nil {
		t.Fatal(err)
	}
	if err := SetConfig(db1, migrationVersionKey, "14"); err != nil {
		t.Fatal(err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db2.Close() //nolint:errcheck // test teardown
	if v := readConfig(t, db2, "auto_import"); v == "1" {
		t.Fatal("v15 seeded an ON row where none existed")
	}
}

func versionAtLeast(v string, want int) bool {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	return err == nil && n >= want
}

func readConfig(t *testing.T, database *sql.DB, key string) string {
	t.Helper()
	var v string
	err := database.QueryRow("SELECT value FROM app_config WHERE key = ?", key).Scan(&v)
	if err != nil {
		return "<missing:" + err.Error() + ">"
	}
	return v
}
