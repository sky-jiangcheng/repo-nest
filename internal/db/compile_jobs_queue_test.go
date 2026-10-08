package db

import (
	"database/sql"
	"testing"
)

// Schema v20 widens compile_jobs into the general long-task queue (ADR-0016
// 待决 ①). The risk in a widening migration is not the new databases — it is
// the ones already on disk: a v19 database has real compile history in it, and
// the new code has to keep reading those rows rather than tripping over a
// missing column.

func TestEnsureCompileJobQueue_UpgradesAV19Table(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Build the v19 shape: the table without kind/findings, stamped as v19, and
	// holding a row so the assertion is about real data rather than an empty
	// table.
	_, err = database.Exec(`DROP TABLE compile_jobs`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE compile_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		requested_notes INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued'
			CHECK (status IN ('queued','running','succeeded','failed','canceled')),
		notes_total INTEGER NOT NULL DEFAULT 0,
		notes_done INTEGER NOT NULL DEFAULT 0,
		pages_created INTEGER NOT NULL DEFAULT 0,
		pages_updated INTEGER NOT NULL DEFAULT 0,
		links_created INTEGER NOT NULL DEFAULT 0,
		attachments INTEGER NOT NULL DEFAULT 0,
		revision_todos INTEGER NOT NULL DEFAULT 0,
		rejected_ops INTEGER NOT NULL DEFAULT 0,
		stopped TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		finished_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}
	pid := seedJobTestProject(t, database)
	// Inserted with raw SQL, not CreateCompileJob: that helper already speaks
	// v20 (it names the kind column), which is exactly what this table lacks.
	res, err := database.Exec(
		`INSERT INTO compile_jobs(project_id, requested_notes, status) VALUES (?, 7, 'succeeded')`, pid)
	if err != nil {
		t.Fatalf("a v19 table must still accept the v19 insert shape: %v", err)
	}
	legacyID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	if err := EnsureCompileJobQueue(database); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	// The pre-existing row must still be readable through the v20 column list —
	// this is the query that would fail with "no such column" if the ALTER had
	// not landed, and it is the failure a user with job history would hit.
	j, err := GetCompileJob(database, legacyID)
	if err != nil {
		t.Fatalf("reading a pre-upgrade row: %v", err)
	}
	if j.Kind != JobKindCompile {
		t.Errorf("Kind = %q, want %q for a row that predates the column", j.Kind, JobKindCompile)
	}
	if j.Requested != 7 {
		t.Errorf("Requested = %d, want 7 — the upgrade must not disturb real values", j.Requested)
	}
	if j.Findings != 0 {
		t.Errorf("Findings = %d, want 0", j.Findings)
	}

	// And the queue must be usable afterwards: a lint job lands beside the old
	// compile row, and the kind filter tells them apart.
	lintID, err := CreateLintJob(database, pid, 3)
	if err != nil {
		t.Fatalf("creating a lint job after the upgrade: %v", err)
	}
	lints, err := ListCompileJobs(database, 0, JobKindLint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lints) != 1 || lints[0].ID != lintID {
		t.Errorf("lint list = %+v, want only job %d", lints, lintID)
	}
	compiles, err := ListCompileJobs(database, 0, JobKindCompile, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiles) != 1 || compiles[0].ID != legacyID {
		t.Errorf("compile list = %+v, want only the pre-upgrade job %d", compiles, legacyID)
	}
}

// Running the migration twice must be a no-op, since it is stamped per version
// but Ensure* is also called on every open.
func TestEnsureCompileJobQueue_IsIdempotent(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	for i := 0; i < 3; i++ {
		if err := EnsureCompileJobQueue(database); err != nil {
			t.Fatalf("pass %d: %v", i+1, err)
		}
	}
	if err := EnsureCompileJobs(database); err != nil {
		t.Fatalf("EnsureCompileJobs after the widening: %v", err)
	}
}

func TestEnsureCompileJobQueue_RejectsAnUnknownKind(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := EnsureCompileJobQueue(database); err != nil {
		t.Fatal(err)
	}
	pid := seedJobTestProject(t, database)
	if _, err := database.Exec(
		`INSERT INTO compile_jobs(project_id, kind, requested_notes) VALUES (?, 'nonsense', 1)`, pid); err == nil {
		t.Error("the kind CHECK constraint accepted an unknown kind")
	}
}

func seedJobTestProject(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	res, err := database.Exec(`INSERT INTO projects(name, root_path) VALUES ('jobqueue', '/tmp/jobqueue')`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
