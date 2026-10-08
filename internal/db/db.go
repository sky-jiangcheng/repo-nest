package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// InitDB initializes the SQLite database, creating tables if they don't exist.
// Returns the database handle and any error encountered.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Embedded SQLite is single-writer: cap the pool to one connection so
	// concurrent Wails/scan/plugin calls serialize instead of hitting
	// "database is locked" (SQLITE_BUSY). busy_timeout retries under lock
	// contention as a backstop.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close() //nolint:errcheck // close on init failure, error irrelevant
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close() //nolint:errcheck // close on init failure, error irrelevant
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	// The database (and its WAL sidecars) holds the user's whole knowledge
	// base. The data directory is created 0750, but the files themselves get
	// the process umask default — 0644 on macOS — which other local users can
	// read. Pin down what exists at init (createTables has already written, so
	// the -wal/-shm sidecars are here). platform.SetPrivateUmask covers files
	// created in later sessions; this also protects embedders that bypass it.
	if dbPath != ":memory:" {
		for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
			_ = os.Chmod(p, 0600)
		}
	}

	if err := upgradeSchema(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to upgrade schema: %w", err)
	}

	// Operational job queue (v19) is re-asserted too: a dropped table must not
	// leave a database stamped with a version whose object is missing.
	if err := EnsureCompileJobs(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to ensure compile jobs: %w", err)
	}

	// Re-assert the derived wiki layer on every open, not just at migration time.
	//
	// Why this is not redundant: migrations are stamped, so v16 never runs twice.
	// DropWikiSchema is a supported operation (the layer is derived cache, and the
	// reversibility test uses it), and without this line a database whose layer was
	// dropped would come back with schema_version=16 and NO tables — every wiki
	// query from then on fails with "no such table" and nothing short of manually
	// re-running the migration repairs it. Same contract as EnsureFTSIndex: safe to
	// call on any database that already has project_notes.
	if err := EnsureWikiSchema(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to ensure wiki schema: %w", err)
	}

	// Same self-healing contract for the page index (v17).
	if err := EnsureWikiFTS(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to ensure wiki FTS: %w", err)
	}

	// ...and for the review columns (v18). A database that predates them must not
	// stay half-migrated just because CREATE TABLE IF NOT EXISTS is a no-op.
	if err := EnsureWikiReviewColumns(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to ensure wiki review columns: %w", err)
	}

	// Triggers cannot backfill: pages that existed before this index was created
	// would stay unsearchable forever. Rebuild is cheap (pages are a derived,
	// bounded artifact) and idempotent, and doing it here is what stops the v7
	// "empty index that never got repaired" failure from repeating.
	if err := RebuildWikiFTS(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, err
	}

	if err := insertDefaults(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to insert defaults: %w", err)
	}

	return db, nil
}

// createTables creates all required tables if they do not exist.
func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS scan_roots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		root_path TEXT NOT NULL,
		level_override INTEGER DEFAULT 0,
		is_auto_grouped BOOLEAN DEFAULT 1,
		collected BOOLEAN DEFAULT FALSE,
		collected_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL UNIQUE,
		project_id INTEGER,
		last_scanned_at DATETIME,
		FOREIGN KEY (project_id) REFERENCES projects(id)
	);

	CREATE TABLE IF NOT EXISTS daily_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repository_id INTEGER NOT NULL,
		stat_date DATE NOT NULL,
		author TEXT NOT NULL,
		files_changed INTEGER DEFAULT 0,
		lines_added INTEGER DEFAULT 0,
		lines_deleted INTEGER DEFAULT 0,
		commits INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (repository_id) REFERENCES repositories(id),
		UNIQUE(repository_id, stat_date, author)
	);

	CREATE TABLE IF NOT EXISTS app_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS project_todos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		completed BOOLEAN DEFAULT 0,
		priority INTEGER DEFAULT 0,
		sort_order INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS project_notes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		title TEXT DEFAULT '',
		content TEXT NOT NULL,
		tags TEXT DEFAULT '',
		kind TEXT DEFAULT 'other',
		pinned INTEGER DEFAULT 0,
		source TEXT DEFAULT 'manual',
		sort_order INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS repo_meta (
		repository_id INTEGER PRIMARY KEY,
		tech_stack TEXT DEFAULT '[]',
		readme_excerpt TEXT DEFAULT '',
		languages TEXT DEFAULT '{}',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
	);
	`

	_, err := db.Exec(schema)
	return err
}
