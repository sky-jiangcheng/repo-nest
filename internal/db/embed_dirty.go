package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Incremental embedding work queue (ADR-0012's missing half: 笔记增删改即时生效,
// instead of "全量重建 or stale").
//
// Why a trigger-maintained table rather than hooks in the service layer:
// internal/db is the chokepoint every note writer converges on — the desktop UI,
// MCP tools, the CLI, the session-capture hook and, critically, the plugin
// runtime's direct upserts. The five agent-memory importers never call
// service.CreateNote*, so a service-layer hook would silently skip the single
// largest writer. This is the same trap FTS5 avoided with its ai/ad/au triggers
// (see ftsSchemaStatements), so the repair uses the same shape.
//
// The table is a work queue, not data: it is derived from writes and is always
// safe to truncate (a full rebuild covers everything). A row whose note is gone
// means "delete its vector" — the drainer resolves each id against
// project_notes to tell "re-embed" from "remove".
//
// The update trigger fires only when the embedding input actually changed.
// `AFTER UPDATE OF title, content` looked like the right spelling but is a
// SYNTACTIC test: it fires whenever a statement mentions those columns, even
// when the new value equals the old one — and UpdateNoteMeta resends `title` on
// every tags/kind/pinned save, so every metadata write would have queued a wasted
// embedding request. A `WHEN OLD.x IS NOT NEW.x` guard is the semantic test
// (`IS NOT` also treats NULL correctly), so pinning, reordering, retagging or
// moving a note costs nothing while any text change costs exactly one.
//
// Each trigger is dropped before it is created rather than relying on
// IF NOT EXISTS: a trigger holds no state, so recreating it is free, and it means
// a definition change here is actually applied to databases that already stamped
// this version instead of being silently skipped forever.
var noteEmbedDirtyStatements = []string{
	`CREATE TABLE IF NOT EXISTS note_embed_dirty (
		note_id INTEGER PRIMARY KEY,
		marked_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`DROP TRIGGER IF EXISTS project_notes_embed_ai`,
	`CREATE TRIGGER project_notes_embed_ai AFTER INSERT ON project_notes BEGIN
		INSERT OR REPLACE INTO note_embed_dirty(note_id) VALUES (new.id);
	END`,
	`DROP TRIGGER IF EXISTS project_notes_embed_au`,
	`CREATE TRIGGER project_notes_embed_au AFTER UPDATE ON project_notes
		WHEN OLD.title IS NOT NEW.title OR OLD.content IS NOT NEW.content BEGIN
		INSERT OR REPLACE INTO note_embed_dirty(note_id) VALUES (new.id);
	END`,
	`DROP TRIGGER IF EXISTS project_notes_embed_ad`,
	`CREATE TRIGGER project_notes_embed_ad AFTER DELETE ON project_notes BEGIN
		INSERT OR REPLACE INTO note_embed_dirty(note_id) VALUES (old.id);
	END`,
}

// EnsureNoteEmbedDirty creates the dirty table and its triggers. Idempotent, and
// exported so tests (and any future caller) can set it up without running the
// full migration machinery — same contract as EnsureFTSIndex.
func EnsureNoteEmbedDirty(db *sql.DB) error {
	for _, stmt := range noteEmbedDirtyStatements {
		if _, err := db.Exec(stmt); err != nil {
			if isAlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("db: ensure note embed dirty queue: %w", err)
		}
	}
	return nil
}

// noteEmbedDirtyMigration is the v14 schema change. Kept as its own exported-by-
// nothing helper so migrate.go stays a list of ids and this file owns the
// concept.
func noteEmbedDirtyMigration(db *sql.DB) error { return EnsureNoteEmbedDirty(db) }

// NoteEmbeddingText builds the exact string handed to the embedding endpoint.
// Both the full rebuild and the incremental drainer call this: if the two ever
// derived the text differently, a note's vector would silently change meaning
// depending on which path wrote it, and RRF would then be ranking a mix of two
// different representations.
func NoteEmbeddingText(title, content string) string {
	if len(content) > MaxNoteContentLen {
		content = content[:MaxNoteContentLen]
	}
	return strings.TrimSpace(title + "\n" + content)
}

// DirtyEmbedWork is one drain round's worth of work, split by what the note's
// current state demands.
type DirtyEmbedWork struct {
	// ToEmbed are live notes needing a (re)computed vector.
	ToEmbed []NoteEmbeddingInput
	// Gone are dirty ids whose note no longer exists: their vectors must be
	// removed, or a deleted note keeps being recalled by search forever.
	Gone []int64
}

// NextDirtyEmbedWork pops up to `limit` queued ids (lowest id first, so the
// order is stable and testable) and resolves each against project_notes. One
// round trip: the LEFT JOIN carries the note text when the row still exists.
func NextDirtyEmbedWork(db *sql.DB, limit int) (*DirtyEmbedWork, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := db.Query(
		`SELECT d.note_id, n.title, n.content
		   FROM note_embed_dirty d LEFT JOIN project_notes n ON n.id = d.note_id
		  ORDER BY d.note_id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("db: next dirty embed work: %w", err)
	}
	defer rows.Close()

	work := &DirtyEmbedWork{}
	for rows.Next() {
		var (
			id             int64
			title, content sql.NullString
		)
		if err := rows.Scan(&id, &title, &content); err != nil {
			return nil, err
		}
		if !title.Valid && !content.Valid {
			work.Gone = append(work.Gone, id)
			continue
		}
		work.ToEmbed = append(work.ToEmbed, NoteEmbeddingInput{
			ID:   id,
			Text: NoteEmbeddingText(title.String, content.String),
		})
	}
	return work, rows.Err()
}

// ClearDirtyNoteIDs removes resolved ids from the queue. Only called for work
// that actually landed: if an embedding request fails, the rows stay queued and
// the next drain retries them.
func ClearDirtyNoteIDs(db *sql.DB, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if _, err := db.Exec(
		"DELETE FROM note_embed_dirty WHERE note_id IN ("+placeholders+")", args...); err != nil {
		return fmt.Errorf("db: clear dirty note ids: %w", err)
	}
	return nil
}

// TruncateDirtyNoteIDs empties the queue (called after a successful full
// rebuild, which has already covered everything still queued).
func TruncateDirtyNoteIDs(db *sql.DB) error {
	if _, err := db.Exec("DELETE FROM note_embed_dirty"); err != nil {
		return fmt.Errorf("db: truncate dirty note queue: %w", err)
	}
	return nil
}
