package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Note write bounds, enforced here because this package is the chokepoint
// every writer converges on: the service layer (desktop UI, MCP tools), the
// plugin runtime's import upserts (runtime.upsertDoc calls this package
// directly), and script plugins holding ctx.DB(). Validating only in the
// service layer left the import path unbounded — a plugin could upsert a
// multi-megabyte doc that the next reponest_context would embed in full.
// The limits are generous enough for real Markdown notes.
const (
	MaxNoteContentLen = 100_000 // ~100 KB of Markdown per note
	MaxNoteTitleLen   = 200
	MaxNoteTagsLen    = 500
	MaxNoteTagCount   = 20
)

// ValidateNoteBounds rejects oversized note fields before they reach the
// database. Content may legitimately be empty on metadata-only updates
// (UpdateNoteMeta does not touch content) — it is still bounded when present.
func ValidateNoteBounds(title, content, tags string) error {
	if len(content) > MaxNoteContentLen {
		return fmt.Errorf("content too long: %d bytes (max %d)", len(content), MaxNoteContentLen)
	}
	if len(title) > MaxNoteTitleLen {
		return fmt.Errorf("title too long: %d bytes (max %d)", len(title), MaxNoteTitleLen)
	}
	if len(tags) > MaxNoteTagsLen {
		return fmt.Errorf("tags too long: %d bytes (max %d)", len(tags), MaxNoteTagsLen)
	}
	if n := countTags(tags); n > MaxNoteTagCount {
		return fmt.Errorf("too many tags: %d (max %d)", n, MaxNoteTagCount)
	}
	return nil
}

// countTags counts comma-separated tag entries, ignoring blanks so ",a,,b,"
// counts as two tags.
func countTags(tags string) int {
	n := 0
	for _, t := range strings.Split(tags, ",") {
		if strings.TrimSpace(t) != "" {
			n++
		}
	}
	return n
}

// NormalizeTags splits a raw tags string on ASCII and full-width commas,
// trims each entry, drops empties and duplicates (first occurrence wins) and
// rejoins with ", " — the canonical form the frontend's parseTags/joinTags
// pair produces. Writers used to store the raw input, so a note created with
// "cbipay, 样例测试, 支付" kept the whole string as ONE tag: the tag list then
// offered a single joined chip that the frontend filter (which splits tags
// on commas) could never match, hiding the note behind its own tag.
func NormalizeTags(tags string) string {
	parts := strings.FieldsFunc(tags, func(r rune) bool {
		return r == ',' || r == '，'
	})
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, t := range parts {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return strings.Join(out, ", ")
}

// CreateNote inserts a new note for a project.
func CreateNote(db *sql.DB, projectID int64, content string) (*Note, error) {
	return CreateNoteEx(db, projectID, "", content, "", "other", "manual")
}

// CreateNoteEx inserts a new note with explicit metadata. Tags are
// normalized before storage — every writer (desktop UI, MCP, plugin runtime,
// importers) converges here, so the canonical form is guaranteed at the
// chokepoint the same way the bounds are.
func CreateNoteEx(db *sql.DB, projectID int64, title, content, tags, kind, source string) (*Note, error) {
	if err := ValidateNoteBounds(title, content, tags); err != nil {
		return nil, err
	}
	tags = NormalizeTags(tags)
	res, err := db.Exec(
		"INSERT INTO project_notes (project_id, title, content, tags, kind, source) VALUES (?, ?, ?, ?, ?, ?)",
		projectID, title, content, tags, kind, source)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return getNoteByID(db, id)
}

func getNoteByID(db *sql.DB, id int64) (*Note, error) {
	n := &Note{}
	err := db.QueryRow(
		"SELECT id, project_id, title, content, tags, kind, pinned, source, sort_order, created_at, updated_at FROM project_notes WHERE id = ?", id).
		Scan(&n.ID, &n.ProjectID, &n.Title, &n.Content, &n.Tags, &n.Kind, &n.Pinned, &n.Source, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// GetNoteByID returns a single note by its ID.
func GetNoteByID(db *sql.DB, id int64) (*Note, error) {
	return getNoteByID(db, id)
}

// ListNotes returns all notes for a project, pinned first then by recency.
func ListNotes(db *sql.DB, projectID int64) ([]Note, error) {
	rows, err := db.Query(
		"SELECT id, project_id, title, content, tags, kind, pinned, source, sort_order, created_at, updated_at FROM project_notes WHERE project_id = ? ORDER BY pinned DESC, sort_order ASC, created_at ASC, id ASC",
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.Title, &n.Content, &n.Tags, &n.Kind, &n.Pinned, &n.Source, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// LatestHandoffNote returns the most recently updated handoff-tagged note.
// projectID>0 scopes the result to one project; zero means the latest across
// all projects. It returns sql.ErrNoRows when none exists.
func LatestHandoffNote(db *sql.DB, projectID int64) (*Note, error) {
	query := "SELECT id, project_id, title, content, tags, kind, pinned, source, sort_order, created_at, updated_at " +
		"FROM project_notes WHERE ',' || REPLACE(LOWER(tags), ' ', '') || ',' LIKE '%,handoff,%'"
	args := []any{}
	if projectID > 0 {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY updated_at DESC, id DESC LIMIT 1"
	note := &Note{}
	err := db.QueryRow(query, args...).Scan(
		&note.ID, &note.ProjectID, &note.Title, &note.Content, &note.Tags,
		&note.Kind, &note.Pinned, &note.Source, &note.SortOrder,
		&note.CreatedAt, &note.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return note, nil
}

// UpdateNote updates the content of a note. An absent id yields an error.
func UpdateNote(db *sql.DB, noteID int64, content string) error {
	if err := ValidateNoteBounds("", content, ""); err != nil {
		return err
	}
	res, err := db.Exec(
		"UPDATE project_notes SET content = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?",
		content, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateNoteFull updates both content and metadata in a single transaction,
// ensuring the note_versions snapshot trigger fires once with consistent data.
func UpdateNoteFull(db *sql.DB, noteID int64, content, title, tags, kind string, pinned bool) error {
	if err := ValidateNoteBounds(title, content, tags); err != nil {
		return err
	}
	tags = NormalizeTags(tags)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		"UPDATE project_notes SET content = ?, title = ?, tags = ?, kind = ?, pinned = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?",
		content, title, tags, kind, pinned, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// DeleteNote removes a note. Deleting an absent id is not an error. The
// derived vector index is kept in sync best-effort: its rows are keyed by
// note id, and without this cleanup deleted notes leave orphaned vectors
// that consume KNN's recall budget until the next full rebuild.
func DeleteNote(db *sql.DB, noteID int64) error {
	if _, err := db.Exec("DELETE FROM project_notes WHERE id = ?", noteID); err != nil {
		return err
	}
	// The note IS deleted at this point, so an embedding cleanup failure is
	// not worth failing the call over — the index is a rebuildable cache.
	if VectorIndexReady(db) {
		_ = DeleteNoteEmbedding(db, noteID) //nolint:errcheck // repairable via RebuildEmbeddings
	}
	return nil
}

// UpdateNoteMeta updates a note's editable metadata.
func UpdateNoteMeta(db *sql.DB, noteID int64, title, tags, kind string, pinned bool) error {
	if err := ValidateNoteBounds(title, "", tags); err != nil {
		return err
	}
	tags = NormalizeTags(tags)
	res, err := db.Exec(
		"UPDATE project_notes SET title = ?, tags = ?, kind = ?, pinned = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?",
		title, tags, kind, pinned, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MoveNote reassigns a note to a different project.
func MoveNote(db *sql.DB, noteID, projectID int64) error {
	res, err := db.Exec(
		"UPDATE project_notes SET project_id = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?",
		projectID, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// PinNote sets or clears the pinned flag on a note.
func PinNote(db *sql.DB, noteID int64, pinned bool) error {
	res, err := db.Exec(
		"UPDATE project_notes SET pinned = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE id = ?",
		pinned, noteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetNoteBySourceTitle returns a note matching a project, source and title.
func GetNoteBySourceTitle(db *sql.DB, projectID int64, source, title string) (*Note, error) {
	n := &Note{}
	err := db.QueryRow(
		"SELECT id, project_id, title, content, tags, kind, pinned, source, sort_order, created_at, updated_at FROM project_notes WHERE project_id = ? AND source = ? AND title = ?",
		projectID, source, title).
		Scan(&n.ID, &n.ProjectID, &n.Title, &n.Content, &n.Tags, &n.Kind, &n.Pinned, &n.Source, &n.SortOrder, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ListAllNotes returns notes across all projects joined with project name,
// ordered pinned first then most recently updated. A positive limit caps the
// rows (<= 0 means no limit) so callers that display a bounded window (MCP
// notes_list, llms.txt) never load the whole knowledge base into memory; a
// non-empty kind filters to that note kind.
func ListAllNotes(db *sql.DB, limit int, kind string) ([]NoteWithProject, error) {
	query := "SELECT n.id, n.project_id, n.title, n.content, n.tags, n.kind, n.pinned, n.source, n.sort_order, n.created_at, n.updated_at, p.name FROM project_notes n JOIN projects p ON p.id = n.project_id"
	args := []any{}
	if kind != "" {
		query += " WHERE n.kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY n.pinned DESC, n.updated_at DESC"
	if limit > 0 {
		// limit is a validated int from Go code, not user input, so
		// interpolating it keeps the query shape static (SQLite has no
		// parameter placeholder for LIMIT in every build).
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []NoteWithProject
	for rows.Next() {
		var np NoteWithProject
		if err := rows.Scan(&np.ID, &np.ProjectID, &np.Title, &np.Content, &np.Tags, &np.Kind, &np.Pinned, &np.Source, &np.SortOrder, &np.CreatedAt, &np.UpdatedAt, &np.ProjectName); err != nil {
			return nil, err
		}
		notes = append(notes, np)
	}
	return notes, rows.Err()
}

// CountNotes returns the total number of notes across all projects, without
// loading any rows: a count is aggregate work for SQLite but a full scan of
// every note's content for the caller.
func CountNotes(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM project_notes").Scan(&n)
	return n, err
}

// ListAllTags returns the distinct set of individual tags across all notes,
// sorted. Stored values are comma-joined (one string per note), so the raw
// DISTINCT list would offer one chip per NOTE — e.g. "cbipay, 支付" — which
// the frontend filter (splitting each note's tags on commas) could never
// match. Splitting here is what makes a tag chip point back at its notes.
func ListAllTags(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT DISTINCT tags FROM project_notes WHERE tags != ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[string]bool)
	var tags []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		for _, t := range strings.Split(v, ",") {
			t = strings.TrimSpace(t)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			tags = append(tags, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(tags)
	return tags, nil
}

// GetNoteCounts returns the number of notes per project.
func GetNoteCounts(db *sql.DB) ([]NoteCount, error) {
	rows, err := db.Query("SELECT project_id, COUNT(*) FROM project_notes GROUP BY project_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var counts []NoteCount
	for rows.Next() {
		var pid int64
		var c int64
		if err := rows.Scan(&pid, &c); err != nil {
			return nil, err
		}
		counts = append(counts, NoteCount{ProjectID: pid, Count: int(c)})
	}
	return counts, rows.Err()
}
