package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	searchProjectsLimit = 50
	projectColumns      = "id, name, root_path, level_override, is_auto_grouped, is_starred, created_at"
)

// GetAllProjects returns all projects, starred first then by name.
func GetAllProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query("SELECT " + projectColumns + " FROM projects ORDER BY is_starred DESC, name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.LevelOverride, &p.IsAutoGrouped, &p.IsStarred, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetStarredProjects returns only starred projects, ordered by name.
func GetStarredProjects(db *sql.DB) ([]Project, error) {
	rows, err := db.Query("SELECT " + projectColumns + " FROM projects WHERE is_starred = 1 ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.LevelOverride, &p.IsAutoGrouped, &p.IsStarred, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// GetProjectByID returns a single project by id.
func GetProjectByID(db *sql.DB, id int64) (*Project, error) {
	p := &Project{}
	err := db.QueryRow("SELECT "+projectColumns+" FROM projects WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.RootPath, &p.LevelOverride, &p.IsAutoGrouped, &p.IsStarred, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// SearchProjects searches projects by name or root_path. An empty or
// whitespace-only query returns nil. LIKE wildcards in the query are escaped.
func SearchProjects(db *sql.DB, query string) ([]Project, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	pattern := "%" + escapeLike(query) + "%"
	rows, err := db.Query(
		"SELECT "+projectColumns+" FROM projects WHERE name LIKE ? ESCAPE '\\' OR root_path LIKE ? ESCAPE '\\' ORDER BY name ASC LIMIT ?",
		pattern, pattern, searchProjectsLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.LevelOverride, &p.IsAutoGrouped, &p.IsStarred, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ToggleProjectStar flips the starred flag of a project atomically and returns
// the new value. Uses a single UPDATE with NOT to avoid TOCTOU race conditions
// between the read and write.
func ToggleProjectStar(db *sql.DB, projectID int64) (bool, error) {
	var newStarred bool
	err := db.QueryRow(
		"UPDATE projects SET is_starred = NOT is_starred WHERE id = ? RETURNING is_starred",
		projectID).Scan(&newStarred)
	if err == nil {
		return newStarred, nil
	}
	// Fallback for SQLite < 3.35 which lacks RETURNING: use a transaction.
	tx, txErr := db.Begin()
	if txErr != nil {
		return false, txErr
	}
	defer tx.Rollback() //nolint:errcheck

	var starred bool
	if err := tx.QueryRow("SELECT is_starred FROM projects WHERE id = ?", projectID).Scan(&starred); err != nil {
		return false, err
	}
	newStarred = !starred
	if _, err := tx.Exec("UPDATE projects SET is_starred = ? WHERE id = ?", newStarred, projectID); err != nil {
		return false, err
	}
	return newStarred, tx.Commit()
}

// SyncProjectTx inserts a project for rootPath if absent, or updates an existing
// one. The starred flag is always preserved. is_auto_grouped is only refreshed
// when the existing project is still auto-grouped (manually adjusted projects
// stay manual). Returns the project id.
func SyncProjectTx(tx *sql.Tx, name, rootPath string, level int, isAuto bool) (int64, error) {
	var id int64
	var existingAuto bool
	err := tx.QueryRow("SELECT id, is_auto_grouped FROM projects WHERE root_path = ?", rootPath).Scan(&id, &existingAuto)
	if err == sql.ErrNoRows {
		res, err := tx.Exec(
			"INSERT INTO projects (name, root_path, level_override, is_auto_grouped) VALUES (?, ?, ?, ?)",
			name, rootPath, level, isAuto)
		if err != nil {
			return 0, err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		return newID, nil
	}
	if err != nil {
		return 0, err
	}
	newAuto := existingAuto && isAuto
	if _, err := tx.Exec(
		"UPDATE projects SET name = ?, level_override = ?, is_auto_grouped = ? WHERE id = ?",
		name, level, newAuto, id); err != nil {
		return 0, err
	}
	return id, nil
}

// GetCollectedProjectIDs returns the ids of all projects marked as collected.
func GetCollectedProjectIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, "SELECT id FROM projects WHERE collected = TRUE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkProjectCollectedTx marks a project as collected inside the scan
// transaction, stamping collected_at so the controlled scan flow can refresh
// its full history window after the scan completes.
func MarkProjectCollectedTx(tx *sql.Tx, projectID int64) error {
	_, err := tx.Exec(
		"UPDATE projects SET collected = TRUE, collected_at = COALESCE(collected_at, datetime('now')) WHERE id = ?",
		projectID)
	return err
}

// SplitProjectDown adjusts a project's grouping level downwards: every repo
// except the first becomes its own project (keeping the original for the first
// repo so its notes/todos survive). The whole operation runs in a single
// transaction so the project graph never ends up half-changed. Returns the new
// level override.
func SplitProjectDown(db *sql.DB, id int64) (int, error) {
	project, err := GetProjectByID(db, id)
	if err != nil {
		// sql.ErrNoRows is preserved so callers can distinguish "absent"
		// from a storage failure (do not collapse into a bare message).
		return 0, fmt.Errorf("project not found: %w", err)
	}
	repos, err := GetRepositoriesByProjectID(db, id)
	if err != nil {
		return 0, fmt.Errorf("failed to load repos")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	if len(repos) <= 1 {
		if _, err := tx.Exec("UPDATE projects SET is_auto_grouped = 0 WHERE id = ?", id); err != nil {
			return 0, fmt.Errorf("failed to update project")
		}
	} else {
		for _, repo := range repos[1:] {
			res, err := tx.Exec(
				"INSERT INTO projects (name, root_path, level_override, is_auto_grouped) VALUES (?, ?, ?, 0)",
				filepath.Base(repo.Path), repo.Path, project.LevelOverride-1,
			)
			if err != nil {
				return 0, fmt.Errorf("failed to create sub-project: %w", err)
			}
			newID, err := res.LastInsertId()
			if err != nil {
				return 0, fmt.Errorf("failed to get new project id: %w", err)
			}
			if _, err := tx.Exec("UPDATE repositories SET project_id = ? WHERE id = ?", newID, repo.ID); err != nil {
				return 0, fmt.Errorf("failed to reassign repo: %w", err)
			}
		}
		// Keep the original project bound to its first repo.
		if _, err := tx.Exec("UPDATE projects SET is_auto_grouped = 0, root_path = ? WHERE id = ?", repos[0].Path, id); err != nil {
			return 0, fmt.Errorf("failed to update project: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit level change: %w", err)
	}
	return project.LevelOverride - 1, nil
}

// MergeProjectUp adjusts a project's grouping level upwards: it absorbs
// sibling projects that share the same parent directory, moving their repos,
// notes, and todos along, then deletes the emptied siblings. Runs in a single
// transaction. Returns the new level override.
func MergeProjectUp(db *sql.DB, id int64) (int, error) {
	project, err := GetProjectByID(db, id)
	if err != nil {
		// sql.ErrNoRows is preserved so callers can distinguish "absent"
		// from a storage failure (do not collapse into a bare message).
		return 0, fmt.Errorf("project not found: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck

	parentDir := filepath.Dir(project.RootPath)
	if parentDir != "" && parentDir != "/" && parentDir != "." {
		// Sibling match is an exact filepath.Dir comparison over all projects.
		// This used to pre-filter with root_path LIKE parentDir || '/%', but
		// the LIKE pattern is never escaped: a '%' or '_' in the directory
		// name over-matched (saved only by the exact re-check), and on Windows
		// the '\' in every path acted as the LIKE escape character, so the
		// pattern matched nothing and merge silently became a no-op.
		rows, err := tx.Query("SELECT id, root_path FROM projects WHERE id != ?", id)
		if err != nil {
			return 0, fmt.Errorf("failed to query siblings: %w", err)
		}
		var siblingIDs []int64
		for rows.Next() {
			var sid int64
			var sroot string
			if err := rows.Scan(&sid, &sroot); err != nil {
				rows.Close()
				return 0, fmt.Errorf("failed to scan sibling: %w", err)
			}
			if filepath.Dir(sroot) == parentDir {
				siblingIDs = append(siblingIDs, sid)
			}
		}
		rows.Close()

		for _, sid := range siblingIDs {
			if _, err := tx.Exec("UPDATE repositories SET project_id = ? WHERE project_id = ?", id, sid); err != nil {
				return 0, fmt.Errorf("failed to move repos: %w", err)
			}
			if _, err := tx.Exec("UPDATE project_notes SET project_id = ? WHERE project_id = ?", id, sid); err != nil {
				return 0, fmt.Errorf("failed to move notes: %w", err)
			}
			if _, err := tx.Exec("UPDATE project_todos SET project_id = ? WHERE project_id = ?", id, sid); err != nil {
				return 0, fmt.Errorf("failed to move todos: %w", err)
			}
			if _, err := tx.Exec("DELETE FROM projects WHERE id = ?", sid); err != nil {
				return 0, fmt.Errorf("failed to remove merged project: %w", err)
			}
		}
		if _, err := tx.Exec("UPDATE projects SET is_auto_grouped = 0, root_path = ?, name = ? WHERE id = ?", parentDir, filepath.Base(parentDir), id); err != nil {
			return 0, fmt.Errorf("failed to update project: %w", err)
		}
	} else if _, err := tx.Exec("UPDATE projects SET is_auto_grouped = 0 WHERE id = ?", id); err != nil {
		return 0, fmt.Errorf("failed to update project: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit level change: %w", err)
	}
	return project.LevelOverride + 1, nil
}
