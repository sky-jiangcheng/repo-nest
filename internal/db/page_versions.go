package db

import (
	"database/sql"
	"fmt"

	"repo-nest/internal/diff"
)

// ListPageVersions returns the recent version history for a page, ordered by
// created_at descending. At most 50 versions are kept by the cleanup trigger
// (ADR-0021, same budget as note_versions).
func ListPageVersions(db *sql.DB, pageID int64) ([]PageVersion, error) {
	rows, err := db.Query(
		// id DESC breaks same-second ties: created_at is second-precision, so
		// within one second the rowid is the only deterministic "newer" signal
		// (same discipline as the wiki neighbour ordering).
		"SELECT id, page_id, title, content, kind, created_at FROM page_versions WHERE page_id = ? ORDER BY created_at DESC, id DESC LIMIT 50",
		pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []PageVersion
	for rows.Next() {
		var v PageVersion
		if err := rows.Scan(&v.ID, &v.PageID, &v.Title, &v.Content, &v.Kind, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// GetPageVersion returns a single version by ID.
func GetPageVersion(db *sql.DB, versionID int64) (*PageVersion, error) {
	v := &PageVersion{}
	err := db.QueryRow(
		"SELECT id, page_id, title, content, kind, created_at FROM page_versions WHERE id = ?", versionID).
		Scan(&v.ID, &v.PageID, &v.Title, &v.Content, &v.Kind, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// RestorePageVersion rolls a page back to the content of a previous version
// (ADR-0021 决策 3): the current content is snapshotted FIRST — explicitly,
// because the page_versions_snap trigger snapshots NEW rows (the v8 note
// precedent), so relying on it would lose the pre-rollback state — then the
// old content is applied through the same UpdateWikiPage write path. History
// only ever grows, never shrinks.
//
// Replaying the same version is a no-op: when the page already sits at that
// version's content nothing is written and nothing is snapshotted.
func RestorePageVersion(db *sql.DB, pageID, versionID int64) error {
	v, err := GetPageVersion(db, versionID)
	if err != nil {
		return err
	}
	if v.PageID != pageID {
		return fmt.Errorf("version %d does not belong to page %d", versionID, pageID)
	}
	current, err := GetWikiPageByID(db, pageID)
	if err != nil {
		return err
	}
	if current.Title == v.Title && current.Kind == v.Kind && current.Content == v.Content {
		// Already at this version: applying it would be a no-op UPDATE that
		// snapshots nothing (the WHEN clause), so there is nothing to preserve
		// either. Return before writing to keep replays byte-identical.
		return nil
	}
	// Snapshot the pre-rollback state as its own version (the trigger cannot:
	// it fires on the applied row, not the displaced one).
	if _, err := db.Exec(
		"INSERT INTO page_versions(page_id, title, content, kind) SELECT id, title, content, kind FROM wiki_pages WHERE id = ?",
		pageID); err != nil {
		return fmt.Errorf("db: snapshot page before rollback: %w", err)
	}
	// Apply the old version through the normal write path — the trigger
	// snapshots the applied state as the newest version.
	return UpdateWikiPage(db, pageID, v.Title, v.Kind, v.Content)
}

// DiffPageVersions produces a line-based diff between a historical version and
// the current page content (the NoteDiff twin, same diff.Lines algorithm).
func DiffPageVersions(db *sql.DB, pageID int64, versionID int64) (string, error) {
	current, err := GetWikiPageByID(db, pageID)
	if err != nil {
		return "", err
	}
	v, err := GetPageVersion(db, versionID)
	if err != nil {
		return "", err
	}
	if v.PageID != pageID {
		return "", fmt.Errorf("version %d does not belong to page %d", versionID, pageID)
	}
	return diff.Lines(v.Content, current.Content), nil
}
