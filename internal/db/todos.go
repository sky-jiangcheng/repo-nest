package db

import "database/sql"

// CreateTodo inserts a new todo for a project, assigning the next sort_order.
// The MAX+INSERT pair runs in a transaction: two interleaved creates used to
// be able to observe the same MAX and hand out a duplicate sort_order.
func CreateTodo(db *sql.DB, projectID int64, title string) (*Todo, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var sortOrder int
	if err := tx.QueryRow(
		"SELECT COALESCE(MAX(sort_order), -1) + 1 FROM project_todos WHERE project_id = ?", projectID).
		Scan(&sortOrder); err != nil {
		return nil, err
	}
	res, err := tx.Exec(
		"INSERT INTO project_todos (project_id, title, sort_order) VALUES (?, ?, ?)",
		projectID, title, sortOrder)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getTodoByID(db, id)
}

// ListTodos returns all todos for a project ordered by sort_order.
func ListTodos(db *sql.DB, projectID int64) ([]Todo, error) {
	rows, err := db.Query(
		"SELECT id, project_id, title, completed, priority, sort_order, created_at, updated_at FROM project_todos WHERE project_id = ? ORDER BY sort_order ASC, id ASC",
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var todos []Todo
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Completed, &t.Priority, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

// ToggleTodo flips the completed status of a todo. An absent id yields an error.
func ToggleTodo(db *sql.DB, todoID int64) error {
	res, err := db.Exec("UPDATE project_todos SET completed = NOT completed, updated_at = CURRENT_TIMESTAMP WHERE id = ?", todoID)
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

// DeleteTodo removes a todo. Deleting an absent id is not an error.
func DeleteTodo(db *sql.DB, todoID int64) error {
	_, err := db.Exec("DELETE FROM project_todos WHERE id = ?", todoID)
	return err
}

// ReorderTodos assigns sort_order by the position of each id in the slice.
// All updates run in a single transaction to prevent partial writes on failure.
func ReorderTodos(db *sql.DB, todoIDs []int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for i, id := range todoIDs {
		if _, err := tx.Exec("UPDATE project_todos SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetTodoCounts returns per-project incomplete (Count) and total (Total) todos.
func GetTodoCounts(db *sql.DB) ([]TodoCount, error) {
	rows, err := db.Query(
		"SELECT project_id, SUM(CASE WHEN completed = 0 THEN 1 ELSE 0 END), COUNT(*) FROM project_todos GROUP BY project_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var counts []TodoCount
	for rows.Next() {
		var pid int64
		var count, total int64
		if err := rows.Scan(&pid, &count, &total); err != nil {
			return nil, err
		}
		counts = append(counts, TodoCount{ProjectID: pid, Count: int(count), Total: int(total)})
	}
	return counts, rows.Err()
}

func getTodoByID(db *sql.DB, id int64) (*Todo, error) {
	t := &Todo{}
	err := db.QueryRow(
		"SELECT id, project_id, title, completed, priority, sort_order, created_at, updated_at FROM project_todos WHERE id = ?", id).
		Scan(&t.ID, &t.ProjectID, &t.Title, &t.Completed, &t.Priority, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}
