package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Async batch jobs for compilation (ADR-0016).
//
// The table exists because a real rehearsal measured one note at 173 seconds on a
// local 27B model: a synchronous call at that scale is wrong even when timeouts
// no longer cut it off (the UI must hang, there is no progress, and the result is
// lost if you navigate away). Submission therefore returns a job id immediately
// and progress is polled.
//
// Like the rest of this layer, the table is derived operational state, not
// user knowledge: dropping it loses no note or page, only job history.

// Compile job statuses. The set is deliberately small and terminal states are
// terminal: a job that died with the process is marked failed, never silently
// resumed, because nothing records which note it stopped on.
const (
	CompileJobQueued   = "queued"
	CompileJobRunning  = "running"
	CompileJobDone     = "succeeded"
	CompileJobFailed   = "failed"
	CompileJobCanceled = "canceled"
)

// Job kinds. The table is a general long-task queue, not a compile-log: the lint
// model pass is just as long (same 10-minute ceiling, same local-model latency)
// and ADR-0016 待决 ① resolved it as "one table, one polling surface".
//
// The discriminator exists because the counters are not interchangeable — a
// compile job counts pages, a lint job counts findings — so the worker has to
// know which kernel to call before it reads a single row. Rows written before
// this column existed are all compile jobs, which is why the default is
// 'compile' and not ”.
const (
	JobKindCompile = "compile"
	JobKindLint    = "lint"
)

var compileJobStatements = []string{
	`CREATE TABLE IF NOT EXISTS compile_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		kind TEXT NOT NULL DEFAULT 'compile'
			CHECK (kind IN ('compile','lint')),
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
		findings INTEGER NOT NULL DEFAULT 0,
		stopped TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		finished_at DATETIME,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_compile_jobs_status ON compile_jobs(status, id)`,
	`CREATE INDEX IF NOT EXISTS idx_compile_jobs_project ON compile_jobs(project_id, id DESC)`,
	// The worker claims the oldest queued row of ANY kind, so the claim query
	// filters on status alone. This index serves the review UI's "lint jobs
	// only" listing, which is the one query that filters on kind.
	`CREATE INDEX IF NOT EXISTS idx_compile_jobs_kind ON compile_jobs(kind, id DESC)`,
}

// EnsureCompileJobQueue turns the v19 compile-job table into the general
// long-task queue ADR-0016 待决 ① asks for: a kind discriminator plus the lint
// pass's own counter. ALTER rather than a new table — job history is derived
// operational state, but there is no reason to throw away a user's compile
// history to add a column.
//
// Idempotent, and every pre-existing row becomes kind='compile' / findings=0 via
// the DEFAULTs, which is the truth: lint jobs could not be written before this.
func EnsureCompileJobQueue(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE compile_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'compile'
			CHECK (kind IN ('compile','lint'))`,
		`ALTER TABLE compile_jobs ADD COLUMN findings INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_compile_jobs_kind ON compile_jobs(kind, id DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			msg := err.Error()
			if strings.Contains(msg, "duplicate column name") || isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure compile job queue columns: %w", err)
		}
	}
	return nil
}

// EnsureCompileJobs creates the job table. Idempotent and re-run at every open,
// for the same reason the wiki layer is: a dropped table must not leave a
// database stamped with a version whose objects are gone.
func EnsureCompileJobs(db *sql.DB) error {
	for _, stmt := range compileJobStatements {
		if _, err := db.Exec(stmt); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure compile jobs: %w", err)
		}
	}
	return nil
}

// DropCompileJobs removes the operational history (never user knowledge).
func DropCompileJobs(db *sql.DB) error {
	for _, o := range []struct{ kind, name string }{
		{"INDEX", "idx_compile_jobs_project"},
		{"INDEX", "idx_compile_jobs_status"},
		{"INDEX", "idx_compile_jobs_kind"},
		{"TABLE", "compile_jobs"},
	} {
		if _, err := db.Exec("DROP " + o.kind + " IF EXISTS " + o.name); err != nil {
			return fmt.Errorf("db: drop compile jobs %s: %w", o.name, err)
		}
	}
	return nil
}

// CompileJob is one row of the queue.
type CompileJob struct {
	ID            int64  `json:"id"`
	ProjectID     int64  `json:"project_id"`
	Kind          string `json:"kind"`
	Requested     int    `json:"requested_notes"`
	Status        string `json:"status"`
	NotesTotal    int    `json:"notes_total"`
	NotesDone     int    `json:"notes_done"`
	PagesCreated  int    `json:"pages_created"`
	PagesUpdated  int    `json:"pages_updated"`
	LinksCreated  int    `json:"links_created"`
	Attachments   int    `json:"attachments"`
	RevisionTodos int    `json:"revision_todos"`
	RejectedOps   int    `json:"rejected_ops"`
	// Findings is the lint job's counter — the analogue of PagesCreated for a
	// compile job. Kept as its own column rather than folded into an existing
	// one because "rejected_ops" means "ops the model emitted that we refused",
	// which is a different fact from "findings filed as todos".
	Findings   int    `json:"findings"`
	Stopped    string `json:"stopped,omitempty"`
	Note       string `json:"note,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

const compileJobCols = `id, project_id, kind, requested_notes, status, notes_total, notes_done,
	pages_created, pages_updated, links_created, attachments, revision_todos, rejected_ops,
	findings, stopped, note, error, COALESCE(created_at, ''), COALESCE(started_at, ''), COALESCE(finished_at, '')`

// CreateCompileJob enqueues a compile job and returns its id, so the caller can leave.
func CreateCompileJob(db *sql.DB, projectID int64, requested int) (int64, error) {
	if projectID <= 0 {
		return 0, fmt.Errorf("db: a compile job needs a project")
	}
	if requested <= 0 {
		return 0, fmt.Errorf("db: a compile job needs a note count")
	}
	res, err := db.Exec(
		"INSERT INTO compile_jobs(project_id, kind, requested_notes) VALUES (?, ?, ?)",
		projectID, JobKindCompile, requested)
	if err != nil {
		return 0, fmt.Errorf("db: create compile job: %w", err)
	}
	return res.LastInsertId()
}

// CreateLintJob enqueues the model-backed half of one project's lint pass.
//
// A job is always scoped to a project, exactly like a compile job: the table's
// project_id is NOT NULL under a foreign key, and inventing a sentinel row to
// mean "every project" would put a fake project in the user's project list. The
// all-pages pass therefore stays on the synchronous path that already runs it
// (the scheduled lint), and what gets queued is the per-project model run the
// panel triggers.
//
// requestedPages is recorded in requested_notes for the progress display; that
// column is the queue's item counter, not a claim that notes are involved.
func CreateLintJob(db *sql.DB, projectID int64, requestedPages int) (int64, error) {
	if projectID <= 0 {
		return 0, fmt.Errorf("db: a lint job needs a project")
	}
	if requestedPages <= 0 {
		return 0, fmt.Errorf("db: a lint job needs a page count")
	}
	res, err := db.Exec(
		"INSERT INTO compile_jobs(project_id, kind, requested_notes) VALUES (?, ?, ?)",
		projectID, JobKindLint, requestedPages)
	if err != nil {
		return 0, fmt.Errorf("db: create lint job: %w", err)
	}
	return res.LastInsertId()
}

// ClaimCompileJob atomically takes the oldest queued job. The single UPDATE ...
// WHERE status='queued' statement is the mutual exclusion: two workers cannot
// claim the same row, and there is exactly one writer connection anyway.
func ClaimCompileJob(db *sql.DB) (*CompileJob, error) {
	res, err := db.Exec(
		`UPDATE compile_jobs SET status = 'running', started_at = CURRENT_TIMESTAMP
		  WHERE id = (SELECT id FROM compile_jobs WHERE status = 'queued' ORDER BY id LIMIT 1)`)
	if err != nil {
		return nil, fmt.Errorf("db: claim compile job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return nil, nil // nothing queued
	}
	rows, err := db.Query("SELECT " + compileJobCols + " FROM compile_jobs WHERE status = 'running' ORDER BY started_at DESC, id DESC LIMIT 1")
	if err != nil {
		return nil, fmt.Errorf("db: read claimed job: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanCompileJob(rows)
}

func scanCompileJob(rows *sql.Rows) (*CompileJob, error) {
	var j CompileJob
	err := rows.Scan(&j.ID, &j.ProjectID, &j.Kind, &j.Requested, &j.Status, &j.NotesTotal, &j.NotesDone,
		&j.PagesCreated, &j.PagesUpdated, &j.LinksCreated, &j.Attachments, &j.RevisionTodos,
		&j.RejectedOps, &j.Findings, &j.Stopped, &j.Note, &j.Error,
		&j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	if err != nil {
		return nil, err
	}
	if j.Kind == "" {
		// A row predating the kind column: it is a compile job, because lint
		// jobs could not be written before it existed.
		j.Kind = JobKindCompile
	}
	return &j, nil
}

// UpdateCompileJobProgress writes the counters a poller reads. A row already
// canceled is NOT overwritten back to running: cancellation is checked between
// notes, and a late progress write must not resurrect a job a human stopped.
func UpdateCompileJobProgress(db *sql.DB, id int64, j *CompileJob) error {
	_, err := db.Exec(
		`UPDATE compile_jobs SET status='running', notes_total=?, notes_done=?, pages_created=?,
		        pages_updated=?, links_created=?, attachments=?, revision_todos=?, rejected_ops=?,
		        findings=?, stopped=?, note=?
		  WHERE id = ? AND status <> 'canceled'`,
		j.NotesTotal, j.NotesDone, j.PagesCreated, j.PagesUpdated, j.LinksCreated,
		j.Attachments, j.RevisionTodos, j.RejectedOps, j.Findings, j.Stopped, j.Note, id)
	if err != nil {
		return fmt.Errorf("db: update compile job %d: %w", id, err)
	}
	return nil
}

// FinishCompileJob moves a job to a terminal state.
func FinishCompileJob(db *sql.DB, id int64, status, note, errMsg string) error {
	switch status {
	case CompileJobDone, CompileJobFailed, CompileJobCanceled:
	default:
		return fmt.Errorf("db: not a terminal compile job status %q", status)
	}
	if _, err := db.Exec(
		`UPDATE compile_jobs SET status = ?, note = ?, error = ?, finished_at = CURRENT_TIMESTAMP
		  WHERE id = ?`, status, note, errMsg, id); err != nil {
		return fmt.Errorf("db: finish compile job %d: %w", id, err)
	}
	return nil
}

// GetCompileJob loads one job for polling.
func GetCompileJob(db *sql.DB, id int64) (*CompileJob, error) {
	rows, err := db.Query("SELECT "+compileJobCols+" FROM compile_jobs WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanCompileJob(rows)
}

// ListCompileJobs returns recent jobs for a project (0 = all projects), newest
// first, bounded so a long-lived desktop session cannot grow an unbounded panel.
// kind == "" means both kinds; the review panel asks for one kind at a time
// because the counters it renders mean different things per kind.
func ListCompileJobs(db *sql.DB, projectID int64, kind string, limit int) ([]CompileJob, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := "SELECT " + compileJobCols + " FROM compile_jobs"
	var args []any
	if projectID > 0 {
		q += " WHERE project_id = ?"
		args = append(args, projectID)
		if kind != "" {
			q += " AND kind = ?"
			args = append(args, kind)
		}
	} else if kind != "" {
		q += " WHERE kind = ?"
		args = append(args, kind)
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("db: list compile jobs: %w", err)
	}
	defer rows.Close()
	var out []CompileJob
	for rows.Next() {
		j, err := scanCompileJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// RequestCancelCompileJob marks a queued or running job canceled. The worker
// notices between notes; it does not abort an HTTP request already in flight,
// which would only save a request whose cost has already been paid.
func RequestCancelCompileJob(db *sql.DB, id int64) (*CompileJob, error) {
	if _, err := db.Exec(
		`UPDATE compile_jobs SET status='canceled', finished_at = CURRENT_TIMESTAMP
		  WHERE id = ? AND status IN ('queued','running')`, id); err != nil {
		return nil, fmt.Errorf("db: cancel compile job %d: %w", id, err)
	}
	return GetCompileJob(db, id)
}

// FailInterruptedCompileJobs runs at startup: anything left 'running' or 'queued'
// died with the previous process. Marking it failed is honest; silently resuming
// would pretend we know where it stopped, and we do not.
func FailInterruptedCompileJobs(db *sql.DB) (int, error) {
	res, err := db.Exec(
		`UPDATE compile_jobs SET status = 'failed', error = ?, finished_at = CURRENT_TIMESTAMP
		  WHERE status IN ('running','queued')`,
		"interrupted by application restart; work is not resumed because the stopping point is unknown")
	if err != nil {
		return 0, fmt.Errorf("db: fail interrupted compile jobs: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CompileJobIsCanceled reports the flag the per-note loop checks. Cheap enough to
// call once per note (a job is seconds-to-minutes per step, not microseconds).
func CompileJobIsCanceled(db *sql.DB, id int64) (bool, error) {
	var s string
	err := db.QueryRow("SELECT status FROM compile_jobs WHERE id = ?", id).Scan(&s)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(s, CompileJobCanceled), nil
}
