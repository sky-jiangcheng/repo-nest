package db

import (
	"database/sql"

	"repotory/internal/domain"
)

// ListLandingGroups returns every landing project — an import placeholder or the
// default ("Uncategorized") bucket — with how many notes it still holds and
// whether a scanned repository currently points at it. Empty placeholders are
// included so the organize surface can still offer to remove a shell the user has
// already emptied. Default sorts first, then placeholders by note count (ADR-0023).
func ListLandingGroups(db *sql.DB) ([]domain.OrganizeGroup, error) {
	rows, err := db.Query(`
		SELECT p.id, p.name, p.root_path, p.project_kind,
		       (SELECT COUNT(*) FROM project_notes n WHERE n.project_id = p.id) AS note_count,
		       EXISTS (SELECT 1 FROM repositories r WHERE r.project_id = p.id) AS repo_bound
		FROM projects p
		WHERE p.project_kind IN ('import_placeholder','default')
		ORDER BY CASE p.project_kind WHEN 'default' THEN 0 ELSE 1 END,
		         note_count DESC,
		         p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OrganizeGroup
	for rows.Next() {
		var g domain.OrganizeGroup
		if err := rows.Scan(&g.ProjectID, &g.Name, &g.RootPath, &g.Kind, &g.NoteCount, &g.RepoBound); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CountProjectNotes returns how many notes a project currently holds.
func CountProjectNotes(db *sql.DB, projectID int64) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM project_notes WHERE project_id = ?", projectID).Scan(&n)
	return n, err
}

// ReassignLandingNotes moves every note from source to target in one transaction
// and returns the count moved. The service layer validates the kinds before
// calling (source is a landing project; target is a real/default project); the
// UPDATE is the same primitive MergeProjectUp and MoveNote already use, just
// bulked to the whole source.
func ReassignLandingNotes(db *sql.DB, sourceID, targetID int64) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		"UPDATE project_notes SET project_id = ?, updated_at = strftime('%Y-%m-%d %H:%M:%f','now') WHERE project_id = ?",
		targetID, sourceID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(n), nil
}

// DeleteLandingProject removes a landing project; its notes go with it via the
// project_notes ON DELETE CASCADE. The service layer guarantees the id is an
// import_placeholder (never the default, never a scanned project) first. Returns
// how many notes were removed alongside the shell.
func DeleteLandingProject(db *sql.DB, id int64) (int, error) {
	notes, err := CountProjectNotes(db, id)
	if err != nil {
		return 0, err
	}
	res, err := db.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, sql.ErrNoRows
	}
	return notes, nil
}
