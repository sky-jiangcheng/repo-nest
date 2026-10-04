package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"repo-nest/internal/db"
)

// markProjectCollected mirrors what a previous scan did, so the "projects are on
// record" check in the walk-failure test sees the state a real rescan does.
func markProjectCollected(t *testing.T, database *sql.DB, pid int64) {
	t.Helper()
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := db.MarkProjectCollectedTx(tx, pid); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// A scan with no configured roots must never reach the stale-data cleanup.
// ScanRepositories returns zero repos for an empty root list, and the cleanup
// used to read "scanned nothing" as "everything is gone" — deleting every
// repository, stat and orphaned project in the knowledge base. That is
// unrecoverable: the stats were computed from git history and cannot be
// reconstructed without re-reading every repository.
func TestScanNowWithNoScanRootsRefusesAndPreservesData(t *testing.T) {
	svc, _ := setupService(t)
	database := svc.db

	pid := seedProject(t, database, "acme", "/tmp/acme")
	seedRepo(t, database, "/tmp/acme/.git", pid)

	if _, err := svc.ScanNow(context.Background()); err == nil {
		t.Fatal("expected ScanNow to refuse with no scan roots, got success")
	} else if !strings.Contains(err.Error(), "no scan roots configured") {
		t.Fatalf("expected a 'no scan roots configured' error, got: %v", err)
	}

	repos, err := db.GetRepositoriesByProjectID(database, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Path != "/tmp/acme/.git" {
		t.Fatalf("scan destroyed repositories: got %v, want [/tmp/acme/.git]", repos)
	}
}

// TriggerScan shares runCollectedScan, so the no-roots guard covers the async
// path too. This asserts the guard lives in the shared pipeline rather than in
// one caller.
func TestRunCollectedScanWithNoScanRootsRefuses(t *testing.T) {
	svc, _ := setupService(t)

	if _, err := svc.runCollectedScan(context.Background()); err == nil {
		t.Fatal("expected runCollectedScan to refuse with no scan roots")
	}
}

// The no-roots guard alone is not sufficient. With roots configured but the
// filesystem walk coming back empty — an unreadable directory, a symlink whose
// target vanished, an unmounted volume — the scan reaches the cleanup with zero
// scanned paths. That is indistinguishable from "the user deleted every
// repository", and reconciling against it wiped the knowledge base.
//
// This is the variant that actually fired: the server log read
// "scan complete: 0 repos, 0 projects" while projects were on record.
func TestScanWithRootsConfiguredButNothingFoundPreservesData(t *testing.T) {
	svc, _ := setupService(t)
	database := svc.db

	pid := seedProject(t, database, "acme", "/tmp/acme")
	seedRepo(t, database, "/tmp/acme/.git", pid)
	markProjectCollected(t, database, pid)

	// A root that exists but holds no git repository: the walk succeeds and
	// finds nothing, which is exactly the shape of a failed walk.
	if err := db.ReplaceScanRoots(database, []string{t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ScanNow(context.Background())
	if err == nil {
		t.Fatal("expected the scan to refuse, got success")
	}
	if !strings.Contains(err.Error(), "refusing to delete") {
		t.Fatalf("expected a refusal that names the reason, got: %v", err)
	}

	repos, err := db.GetRepositoriesByProjectID(database, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Path != "/tmp/acme/.git" {
		t.Fatalf("an empty scan destroyed repositories: got %v", repos)
	}
}
