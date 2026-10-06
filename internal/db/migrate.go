package db

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// migrationVersionKey is the app_config key tracking the applied schema version.
const migrationVersionKey = "schema_version"

// ftsSchemaStatements creates the FTS5 full-text indexes (issue #18) and the
// triggers that keep them in sync with their source tables. The indexes are
// external-content tables backed by project_notes / project_todos, using the
// trigram tokenizer which gives case-insensitive substring matching for both
// ASCII and CJK text (queries of 3+ characters per term).
//
// All statements are idempotent (IF NOT EXISTS) so they are safe to run from
// both the schema migration and the test harness.
//
// IMPORTANT: The insert trigger (_ai) must appear before the delete trigger
// (_ad) in this list. FTS5 requires the virtual table to be created and the
// insert trigger to be defined before delete/update triggers can reference
// the fts table by name in their SQL body.
var ftsSchemaStatements = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS project_notes_fts USING fts5(title, content, content='project_notes', content_rowid='id', tokenize='trigram')`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS project_todos_fts USING fts5(title, content='project_todos', content_rowid='id', tokenize='trigram')`,
	// notes: insert / delete / update
	`CREATE TRIGGER IF NOT EXISTS project_notes_fts_ai AFTER INSERT ON project_notes BEGIN INSERT INTO project_notes_fts(rowid, title, content) VALUES (new.id, COALESCE(new.title, ''), new.content); END`,
	`CREATE TRIGGER IF NOT EXISTS project_notes_fts_ad AFTER DELETE ON project_notes BEGIN INSERT INTO project_notes_fts(project_notes_fts, rowid, title, content) VALUES ('delete', old.id, COALESCE(old.title, ''), old.content); END`,
	`CREATE TRIGGER IF NOT EXISTS project_notes_fts_au AFTER UPDATE ON project_notes BEGIN INSERT INTO project_notes_fts(project_notes_fts, rowid, title, content) VALUES ('delete', old.id, COALESCE(old.title, ''), old.content); INSERT INTO project_notes_fts(rowid, title, content) VALUES (new.id, COALESCE(new.title, ''), new.content); END`,
	// todos: insert / delete / update
	`CREATE TRIGGER IF NOT EXISTS project_todos_fts_ai AFTER INSERT ON project_todos BEGIN INSERT INTO project_todos_fts(rowid, title) VALUES (new.id, new.title); END`,
	`CREATE TRIGGER IF NOT EXISTS project_todos_fts_ad AFTER DELETE ON project_todos BEGIN INSERT INTO project_todos_fts(project_todos_fts, rowid, title) VALUES ('delete', old.id, old.title); END`,
	`CREATE TRIGGER IF NOT EXISTS project_todos_fts_au AFTER UPDATE ON project_todos BEGIN INSERT INTO project_todos_fts(project_todos_fts, rowid, title) VALUES ('delete', old.id, old.title); INSERT INTO project_todos_fts(rowid, title) VALUES (new.id, new.title); END`,
}

// EnsureFTSIndex creates the FTS5 full-text indexes and sync triggers. It is
// safe to call on any database that already has the project_notes and
// project_todos tables. Exported so tests (and future callers) can set up the
// index without running the full migration machinery.
func EnsureFTSIndex(db *sql.DB) error {
	for _, stmt := range ftsSchemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("create FTS index: %w", err)
		}
	}
	return nil
}

// upgradeSchema applies incremental schema changes for existing databases.
// Migrations are tracked by a monotonically increasing version number stored in
// app_config, so each migration runs exactly once even if ALTER errors leave a
// column already present.
func upgradeSchema(db *sql.DB) error {
	from, err := readSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("failed to read schema version: %w", err)
	}

	migrations := []migration{
		// v1: add is_starred to projects (introduced v0.11.0)
		{id: 1, sql: "ALTER TABLE projects ADD COLUMN is_starred INTEGER DEFAULT 0"},
		// v2: structured notes metadata
		{id: 2, sql: []string{
			"ALTER TABLE project_notes ADD COLUMN title TEXT DEFAULT ''",
			"ALTER TABLE project_notes ADD COLUMN tags TEXT DEFAULT ''",
			"ALTER TABLE project_notes ADD COLUMN kind TEXT DEFAULT 'other'",
			"ALTER TABLE project_notes ADD COLUMN pinned INTEGER DEFAULT 0",
			"ALTER TABLE project_notes ADD COLUMN source TEXT DEFAULT 'manual'",
		}},
		// v3: knowledge mining cache + config key for git user
		{id: 3, sql: []string{
			"CREATE TABLE IF NOT EXISTS repo_meta (" +
				"repository_id INTEGER PRIMARY KEY," +
				"tech_stack TEXT DEFAULT '[]'," +
				"readme_excerpt TEXT DEFAULT ''," +
				"languages TEXT DEFAULT '{}'," +
				"dependencies TEXT DEFAULT '[]'," +
				"top_contributors TEXT DEFAULT '[]'," +
				"activity TEXT DEFAULT '{}'," +
				"updated_at DATETIME DEFAULT CURRENT_TIMESTAMP," +
				"FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE)",
		}},
		// v4: add UNIQUE constraint on projects.root_path to prevent duplicates
		{id: 4, sql: []string{
			// Temporarily disable FKs since we drop and recreate the projects table
			"PRAGMA foreign_keys = OFF",
			// 1. create new table with UNIQUE(root_path)
			"CREATE TABLE IF NOT EXISTS projects_new (" +
				"id INTEGER PRIMARY KEY AUTOINCREMENT," +
				"name TEXT NOT NULL," +
				"root_path TEXT NOT NULL UNIQUE," +
				"level_override INTEGER DEFAULT 0," +
				"is_auto_grouped BOOLEAN DEFAULT 1," +
				"is_starred INTEGER DEFAULT 0," +
				"created_at DATETIME DEFAULT CURRENT_TIMESTAMP)",
			// 2. copy data, keeping the lowest id when duplicates exist
			"INSERT OR IGNORE INTO projects_new (id, name, root_path, level_override, is_auto_grouped, is_starred, created_at)" +
				" SELECT id, name, root_path, level_override, is_auto_grouped, is_starred, created_at FROM projects ORDER BY id ASC",
			// 3. reassign repos/notes/todos to the surviving project id for each root_path
			"UPDATE repositories SET project_id = COALESCE((SELECT MIN(p2.id) FROM projects p2 WHERE p2.root_path = " +
				"(SELECT p1.root_path FROM projects p1 WHERE p1.id = repositories.project_id)), project_id)" +
				" WHERE project_id IS NOT NULL",
			"UPDATE project_notes SET project_id = (SELECT MIN(p2.id) FROM projects p2 WHERE p2.root_path = " +
				"(SELECT p1.root_path FROM projects p1 WHERE p1.id = project_notes.project_id))",
			"UPDATE project_todos SET project_id = (SELECT MIN(p2.id) FROM projects p2 WHERE p2.root_path = " +
				"(SELECT p1.root_path FROM projects p1 WHERE p1.id = project_todos.project_id))",
			// 4. replace old table
			"DROP TABLE projects",
			"ALTER TABLE projects_new RENAME TO projects",
			// 5. recreate indexes
			"CREATE INDEX IF NOT EXISTS idx_projects_starred ON projects(is_starred)",
			// Restore FKs
			"PRAGMA foreign_keys = ON",
		}},
		// v5: add collected flag to projects for controlled scan flow
		{id: 5, sql: []string{
			"ALTER TABLE projects ADD COLUMN collected BOOLEAN NOT NULL DEFAULT FALSE",
			"ALTER TABLE projects ADD COLUMN collected_at DATETIME",
			"CREATE INDEX IF NOT EXISTS idx_projects_collected ON projects(collected)",
		}},
		// v6: add indexes for performance
		{id: 6, sql: []string{
			"CREATE INDEX IF NOT EXISTS idx_projects_collected ON projects(collected)",
			"CREATE INDEX IF NOT EXISTS idx_projects_collected_at ON projects(collected_at)",
		}},
		// v7: FTS5 full-text search index (issue #18). External-content tables
		// with the trigram tokenizer power substring + relevance search over
		// notes and todos; triggers keep the index in sync. Existing rows are
		// backfilled idempotently.
		//
		// KNOWN DEFECT in the backfill below, fixed retroactively by v12: the
		// `NOT IN (SELECT rowid FROM project_notes_fts)` predicate reads the
		// content table, not the index, so it matches nothing and the backfill
		// never runs. Do not copy this pattern to a new migration — use the
		// FTS5 'rebuild' command as v12 does. Left as shipped so that databases
		// which already recorded v7 keep a truthful migration history.
		{id: 7, sql: append(append([]string{}, ftsSchemaStatements...),
			`INSERT INTO project_notes_fts(rowid, title, content) `+
				`SELECT id, COALESCE(title, ''), content FROM project_notes `+
				`WHERE id NOT IN (SELECT rowid FROM project_notes_fts)`,
			`INSERT INTO project_todos_fts(rowid, title) `+
				`SELECT id, title FROM project_todos `+
				`WHERE id NOT IN (SELECT rowid FROM project_todos_fts)`,
		)},
		// v8: note version history (issue #16). Creates note_versions table with
		// trigger to snapshot on content changes, plus cleanup to keep recent
		// versions.
		{id: 8, sql: []string{
			`CREATE TABLE IF NOT EXISTS note_versions (` +
				`id INTEGER PRIMARY KEY AUTOINCREMENT,` +
				`note_id INTEGER NOT NULL,` +
				`title TEXT DEFAULT '',` +
				`content TEXT NOT NULL,` +
				`tags TEXT DEFAULT '',` +
				`kind TEXT DEFAULT 'other',` +
				`created_at DATETIME DEFAULT CURRENT_TIMESTAMP,` +
				`FOREIGN KEY (note_id) REFERENCES project_notes(id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS idx_note_versions_note_id ON note_versions(note_id, created_at DESC)`,
			// Snapshot current content when note is updated.
			`CREATE TRIGGER IF NOT EXISTS note_versions_snap AFTER UPDATE ON project_notes` +
				` BEGIN` +
				`  INSERT INTO note_versions(note_id, title, content, tags, kind)` +
				`  VALUES (new.id, COALESCE(new.title, ''), new.content, new.tags, new.kind);` +
				` END`,
			// Cleanup: keep at most 50 versions per note.
			`CREATE TRIGGER IF NOT EXISTS note_versions_cleanup AFTER INSERT ON note_versions` +
				` BEGIN` +
				`  DELETE FROM note_versions WHERE id IN (` +
				`   SELECT id FROM note_versions` +
				`   WHERE note_id = new.note_id` +
				`   ORDER BY created_at DESC` +
				`   LIMIT -1 OFFSET 50` +
				`  );` +
				` END`,
		}},
		// v9: add real commit counts to daily_stats (previously the heatmap
		// reported COUNT(DISTINCT author) as "commits").
		{id: 9, sql: "ALTER TABLE daily_stats ADD COLUMN commits INTEGER NOT NULL DEFAULT 0"},
		// v10: repair repo_meta schema drift. Older databases created repo_meta
		// with only 5 columns (createTables now defines all), so the knowledge
		// cache columns were missing and UpsertRepoMeta/GetRepoMeta failed. On
		// up-to-date databases these ALTERs are no-ops (duplicate column ->
		// isAlreadyExistsErr, safe).
		{id: 10, sql: []string{
			"ALTER TABLE repo_meta ADD COLUMN dependencies TEXT DEFAULT '[]'",
			"ALTER TABLE repo_meta ADD COLUMN top_contributors TEXT DEFAULT '[]'",
			"ALTER TABLE repo_meta ADD COLUMN activity TEXT DEFAULT '{}'",
		}},
		// v11: narrow the note version snapshot trigger. It fired on every
		// UPDATE, so pinning a note or moving it between projects created a
		// duplicate version with identical content (updated_at always changes,
		// so the row-level UPDATE cannot be filtered by comparing the whole
		// row). Only the four columns actually stored in note_versions matter.
		{id: 11, sql: []string{
			`DROP TRIGGER IF EXISTS note_versions_snap`,
			`CREATE TRIGGER IF NOT EXISTS note_versions_snap AFTER UPDATE ON project_notes` +
				` FOR EACH ROW WHEN` +
				`  new.content IS NOT old.content` +
				`  OR new.title IS NOT old.title` +
				`  OR new.tags IS NOT old.tags` +
				`  OR new.kind IS NOT old.kind` +
				` BEGIN` +
				`  INSERT INTO note_versions(note_id, title, content, tags, kind)` +
				`  VALUES (new.id, COALESCE(new.title, ''), new.content, new.tags, new.kind);` +
				` END`,
		}},
		// v12: repair the FTS5 indexes that migration v7 left empty.
		//
		// v7 backfilled with
		//
		//	... FROM project_notes
		//	WHERE id NOT IN (SELECT rowid FROM project_notes_fts)
		//
		// which is a permanent no-op: project_notes_fts is an external-content
		// table, so `SELECT rowid FROM project_notes_fts` is answered by the
		// CONTENT table (project_notes), not by the index. The predicate is
		// therefore "id not in project_notes", which is never true, and the
		// index stays empty on every database that had rows when v7 ran.
		//
		// The visible symptom is silent: SearchNotes falls back to LIKE only
		// when the FTS query *errors*, not when it matches nothing, so search
		// keeps returning a subset of the notes with no error anywhere. v7 is
		// left untouched on purpose — migrations are immutable and already
		// shipped — so this rebuild is what repairs the damage.
		//
		// 'rebuild' is FTS5's own command for this: it drops and repopulates
		// the index from the content table. It is idempotent, and cheap enough
		// to run once on open (a personal knowledge base is thousands of
		// notes, not millions).
		{id: 12, sql: []string{
			`INSERT INTO project_notes_fts(project_notes_fts) VALUES('rebuild')`,
			`INSERT INTO project_todos_fts(project_todos_fts) VALUES('rebuild')`,
		}},
		// v13: normalize note tags + invalidate multi-repo mining caches.
		//
		// Note writers used to store the tags input verbatim, so "cbipay, 样例
		// 测试, 支付" was ONE tag row: the tag list showed a single joined chip
		// that the frontend filter (which splits tags on commas) could never
		// match, hiding the note behind its own tags. Rewritten via Go because
		// the split/dedupe/rejoin logic must mirror db.NormalizeTags exactly.
		//
		// repo_meta rows for MULTI-repo projects are dropped: mining ran git in
		// the project root, which for a container of sibling repos is not a git
		// repository, so activity and top contributors were silently zero and
		// that broken result was cached. A true zero-commit multi-repo project
		// cannot be distinguished from the broken cache, so invalidation is the
		// only reliable repair; the next detail-page load remines (cheap on a
		// personal knowledge base). Single-repo projects mined their actual
		// repository and keep their cache.
		{id: 13, fn: migrateV13NormalizeTagsAndDropMultiRepoMeta},
	}

	for _, m := range migrations {
		if m.id <= from {
			continue
		}
		if err := m.apply(db); err != nil {
			// Distinguish "already exists" (idempotent, safe to stamp) from
			// real migration failures (must not stamp or the migration is
			// silently skipped on future runs).
			if !isAlreadyExistsErr(err) {
				return fmt.Errorf("schema migration %d failed: %w", m.id, err)
			}
			logMigrationError(m.id, err)
		}
		if err := writeSchemaVersion(db, m.id); err != nil {
			return fmt.Errorf("failed to stamp schema version %d: %w", m.id, err)
		}
	}
	return nil
}

// migration represents one schema change. Either sql (a statement list run
// inside one transaction) or fn (a Go-side data migration whose logic cannot
// be expressed in SQL) must be set; fn takes precedence.
type migration struct {
	id  int
	sql any
	fn  func(db *sql.DB) error
}

func (m migration) apply(db *sql.DB) error {
	if m.fn != nil {
		return m.fn(db)
	}
	stmts := stmtList(m.sql)
	// Non-PRAGMA statements of one migration run inside a single transaction,
	// so a multi-step migration cannot leave the schema half-applied if the
	// process dies between statements — v4 drops and renames the projects
	// table, and a crash between those two steps used to require manual
	// repair (the version stamp would not be written, so the next start would
	// re-run the migration against a database whose projects table was gone).
	// PRAGMA foreign_keys cannot change inside a transaction, so PRAGMAs run
	// outside one (committing whatever batch is open first). With the
	// single-connection pool the tx and the bare Exec never contend.
	var tx *sql.Tx
	commit := func() error {
		if tx == nil {
			return nil
		}
		err := tx.Commit()
		tx = nil
		return err
	}
	for _, s := range stmts {
		upper := strings.ToUpper(strings.TrimSpace(s))
		if strings.HasPrefix(upper, "PRAGMA") {
			if err := commit(); err != nil {
				return err
			}
			if _, err := db.Exec(s); err != nil && !isAlreadyExistsErr(err) {
				return err
			}
			continue
		}
		if tx == nil {
			var err error
			tx, err = db.Begin()
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(s); err != nil {
			_ = tx.Rollback()
			tx = nil
			// SQLite reports "duplicate column name" when a column already
			// exists; treat that as already-applied rather than a hard
			// failure. The rolled-back batch re-runs cleanly on the next
			// open because every statement is idempotent under that rule.
			if isAlreadyExistsErr(err) {
				continue
			}
			return err
		}
	}
	return commit()
}

func stmtList(s any) []string {
	switch v := s.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	default:
		return nil
	}
}

func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}

func readSchemaVersion(db *sql.DB) (int, error) {
	// app_config.value is TEXT, so scan into a string and parse.
	var raw string
	err := db.QueryRow("SELECT value FROM app_config WHERE key = ?", migrationVersionKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid schema version %q: %w", raw, err)
	}
	return v, nil
}

func writeSchemaVersion(db *sql.DB, version int) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO app_config (key, value) VALUES (?, ?)",
		migrationVersionKey, strconv.Itoa(version),
	)
	return err
}

func logMigrationError(id int, err error) {
	log.Printf("schema migration %d warning: %v", id, err)
}

// insertDefaults inserts default configuration values if they don't exist.
// migrateV13NormalizeTagsAndDropMultiRepoMeta rewrites note tags into the
// canonical form produced by NormalizeTags and drops mining caches for
// multi-repo projects, whose activity and top contributors were mined from a
// non-git container directory and are therefore always zero. See migration
// v13's comment in upgradeSchema.
func migrateV13NormalizeTagsAndDropMultiRepoMeta(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`SELECT id, tags FROM project_notes WHERE tags LIKE '%,%' OR tags LIKE '%，%'`)
	if err != nil {
		return err
	}
	type tagFix struct {
		id   int64
		tags string
	}
	var fixes []tagFix
	for rows.Next() {
		var id int64
		var tags string
		if err := rows.Scan(&id, &tags); err != nil {
			rows.Close()
			return err
		}
		if n := NormalizeTags(tags); n != tags {
			fixes = append(fixes, tagFix{id, n})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, f := range fixes {
		if _, err := tx.Exec("UPDATE project_notes SET tags = ? WHERE id = ?", f.tags, f.id); err != nil {
			return err
		}
	}

	_, err = tx.Exec(`DELETE FROM repo_meta WHERE repository_id IN (
		SELECT r.id FROM repositories r
		WHERE r.project_id IS NOT NULL
		  AND (SELECT COUNT(*) FROM repositories r2 WHERE r2.project_id = r.project_id) > 1)`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func insertDefaults(db *sql.DB) error {
	defaults := map[string]string{
		"daily_code_standard": "500",
		"scan_depth":          "2",
		"auto_import":         "1",
	}

	for key, value := range defaults {
		_, err := db.Exec(
			"INSERT OR IGNORE INTO app_config (key, value) VALUES (?, ?)",
			key, value,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
