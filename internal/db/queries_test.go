package db

import (
	"errors"
	"fmt"
	"testing"
)

// -- Todo tests --

func TestCreateAndListTodos(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "test-project")

	// Create two todos
	t1, err := CreateTodo(db, pid, "Fix login bug")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}
	if t1.Title != "Fix login bug" || t1.Completed {
		t.Errorf("unexpected todo values: %+v", t1)
	}

	t2, err := CreateTodo(db, pid, "Add unit tests")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}
	_ = t2

	// List should return both in order
	todos, err := ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 2 {
		t.Errorf("expected 2 todos, got %d", len(todos))
	}
	if todos[0].Title != "Fix login bug" {
		t.Errorf("first todo should be 'Fix login bug', got '%s'", todos[0].Title)
	}
	if todos[1].Title != "Add unit tests" {
		t.Errorf("second todo should be 'Add unit tests', got '%s'", todos[1].Title)
	}
	// sort_order should be sequential
	if todos[0].SortOrder != 0 || todos[1].SortOrder != 1 {
		t.Errorf("unexpected sort_order: %d, %d", todos[0].SortOrder, todos[1].SortOrder)
	}
}

func TestToggleTodo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "toggle-test")

	todo, err := CreateTodo(db, pid, "Test toggle")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}

	if todo.Completed {
		t.Error("new todo should not be completed")
	}

	// Toggle to completed
	if err := ToggleTodo(db, todo.ID); err != nil {
		t.Fatalf("ToggleTodo failed: %v", err)
	}

	todos, err := ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if !todos[0].Completed {
		t.Error("todo should be completed after toggle")
	}

	// Toggle back to incomplete
	if err := ToggleTodo(db, todo.ID); err != nil {
		t.Fatalf("second ToggleTodo failed: %v", err)
	}
	todos, err = ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if todos[0].Completed {
		t.Error("todo should be incomplete after second toggle")
	}
}

func TestToggleNonExistentTodo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	err := ToggleTodo(db, 99999)
	if err == nil {
		t.Error("expected error when toggling non-existent todo")
	}
}

func TestDeleteTodo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "delete-test")

	todo, err := CreateTodo(db, pid, "Delete me")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}

	if err := DeleteTodo(db, todo.ID); err != nil {
		t.Fatalf("DeleteTodo failed: %v", err)
	}

	todos, err := ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 0 {
		t.Errorf("expected 0 todos after delete, got %d", len(todos))
	}

	// Deleting a non-existent todo should not error
	if err := DeleteTodo(db, 99999); err != nil {
		t.Errorf("DeleteTodo on non-existent id should not error: %v", err)
	}
}

func TestReorderTodos(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "reorder-test")

	// Create 3 todos
	t1, _ := CreateTodo(db, pid, "A")
	t2, _ := CreateTodo(db, pid, "B")
	t3, _ := CreateTodo(db, pid, "C")

	// Reverse the order
	if err := ReorderTodos(db, []int64{t3.ID, t2.ID, t1.ID}); err != nil {
		t.Fatalf("ReorderTodos failed: %v", err)
	}

	todos, err := ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(todos))
	}
	// After reorder, C should be first, A should be last
	if todos[0].Title != "C" {
		t.Errorf("first todo should be 'C', got '%s'", todos[0].Title)
	}
	if todos[2].Title != "A" {
		t.Errorf("last todo should be 'A', got '%s'", todos[2].Title)
	}
}

func TestListTodosEmptyProject(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "empty-project")

	todos, err := ListTodos(db, pid)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 0 {
		t.Errorf("expected empty list, got %d items", len(todos))
	}
}

// -- Note tests --

func TestCreateAndListNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "note-test")

	n1, err := CreateNote(db, pid, "# Meeting notes\n- Discussed architecture")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	if n1.Content != "# Meeting notes\n- Discussed architecture" {
		t.Errorf("unexpected note content: %s", n1.Content)
	}
	if n1.CreatedAt != n1.UpdatedAt {
		t.Error("new note should have same created_at and updated_at")
	}

	notes, err := ListNotes(db, pid)
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}

func TestUpdateNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "update-note-test")

	note, err := CreateNote(db, pid, "Original content")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	originalUpdatedAt := note.UpdatedAt

	if err := UpdateNote(db, note.ID, "Updated content"); err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}

	notes, err := ListNotes(db, pid)
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "Updated content" {
		t.Errorf("expected 'Updated content', got '%s'", notes[0].Content)
	}
	if notes[0].UpdatedAt == originalUpdatedAt {
		t.Error("updated_at should change after update")
	}
}

func TestUpdateNonExistentNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	err := UpdateNote(db, 99999, "content")
	if err == nil {
		t.Error("expected error when updating non-existent note")
	}
}

func TestMoveNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid1 := createTestProject(t, db, "move-note-from")
	pid2 := createTestProject(t, db, "move-note-to")

	note, err := CreateNote(db, pid1, "Relocatable content")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	if err := MoveNote(db, note.ID, pid2); err != nil {
		t.Fatalf("MoveNote failed: %v", err)
	}

	from, err := ListNotes(db, pid1)
	if err != nil {
		t.Fatalf("ListNotes(from) failed: %v", err)
	}
	if len(from) != 0 {
		t.Errorf("expected no notes in source project, got %d", len(from))
	}

	to, err := ListNotes(db, pid2)
	if err != nil {
		t.Fatalf("ListNotes(to) failed: %v", err)
	}
	if len(to) != 1 || to[0].ID != note.ID {
		t.Errorf("expected moved note in target project, got %+v", to)
	}
	if to[0].ProjectID != pid2 {
		t.Errorf("expected project_id %d, got %d", pid2, to[0].ProjectID)
	}
}

func TestMoveNonExistentNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "move-nonexistent")

	err := MoveNote(db, 99999, pid)
	if err == nil {
		t.Error("expected error when moving non-existent note")
	}
}

func TestDeleteNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "delete-note-test")

	note, err := CreateNote(db, pid, "Delete me")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	if err := DeleteNote(db, note.ID); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}

	notes, err := ListNotes(db, pid)
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after delete, got %d", len(notes))
	}
}

func TestNotesEmptyProject(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "empty-note-project")

	notes, err := ListNotes(db, pid)
	if err != nil {
		t.Fatalf("ListNotes failed: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected empty notes list, got %d", len(notes))
	}
}

// -- CASCADE delete test --

func TestCascadeDeleteProject(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	pid := createTestProject(t, db, "cascade-test")

	// Create todo and note
	_, err := CreateTodo(db, pid, "Todo under cascade")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}
	_, err = CreateNote(db, pid, "Note under cascade")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}

	// Delete the project
	_, err = db.Exec("DELETE FROM projects WHERE id = ?", pid)
	if err != nil {
		t.Fatalf("delete project failed: %v", err)
	}

	// Todos and notes should be cascade-deleted
	todos, _ := ListTodos(db, pid)
	if len(todos) != 0 {
		t.Errorf("expected 0 todos after cascade delete, got %d", len(todos))
	}

	notes, _ := ListNotes(db, pid)
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after cascade delete, got %d", len(notes))
	}
}

// -- GetTodoCounts test --

func TestGetTodoCounts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	p1 := createTestProject(t, db, "counts-p1")
	p2 := createTestProject(t, db, "counts-p2")

	// Project 1: 3 todos, 1 completed
	CreateTodo(db, p1, "T1") //nolint:errcheck
	CreateTodo(db, p1, "T2") //nolint:errcheck
	t3, _ := CreateTodo(db, p1, "T3")
	ToggleTodo(db, t3.ID) //nolint:errcheck

	// Project 2: 1 todo, 0 completed
	CreateTodo(db, p2, "T4") //nolint:errcheck

	counts, err := GetTodoCounts(db)
	if err != nil {
		t.Fatalf("GetTodoCounts failed: %v", err)
	}

	if len(counts) != 2 {
		t.Fatalf("expected 2 project counts, got %d", len(counts))
	}

	for _, c := range counts {
		switch c.ProjectID {
		case p1:
			if c.Count != 2 || c.Total != 3 {
				t.Errorf("project 1: expected count=2 total=3, got count=%d total=%d", c.Count, c.Total)
			}
		case p2:
			if c.Count != 1 || c.Total != 1 {
				t.Errorf("project 2: expected count=1 total=1, got count=%d total=%d", c.Count, c.Total)
			}
		default:
			t.Errorf("unexpected project ID: %d", c.ProjectID)
		}
	}
}

func TestGetTodoCountsEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	counts, err := GetTodoCounts(db)
	if err != nil {
		t.Fatalf("GetTodoCounts failed: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty counts, got %d", len(counts))
	}
}

// -- SyncProjectTx tests --

func TestSyncProjectTx_CreateNew(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	id, err := SyncProjectTx(tx, "my-project", "/repos/my-project", 0, true)
	if err != nil {
		t.Fatalf("SyncProjectTx failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero project ID")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// Verify the project was created
	p, err := GetProjectByID(db, id)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if p.Name != "my-project" || p.RootPath != "/repos/my-project" {
		t.Errorf("unexpected project: %+v", p)
	}
}

func TestSyncProjectTx_PreservesExisting(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a project with is_starred = 1 and is_auto_grouped = 0 (manually adjusted)
	res, err := db.Exec(
		"INSERT INTO projects (name, root_path, is_auto_grouped, is_starred) VALUES (?, ?, ?, ?)",
		"old-name", "/repos/proj", 0, 1,
	)
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	origID, _ := res.LastInsertId()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Sync with new name and is_auto_grouped=true
	id, err := SyncProjectTx(tx, "new-name", "/repos/proj", 0, true)
	if err != nil {
		t.Fatalf("SyncProjectTx failed: %v", err)
	}
	if id != origID {
		t.Errorf("expected same project ID %d, got %d", origID, id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// is_starred should be preserved, is_auto_grouped should stay 0 (manually adjusted)
	p, err := GetProjectByID(db, id)
	if err != nil {
		t.Fatalf("GetProjectByID failed: %v", err)
	}
	if !p.IsStarred {
		t.Error("is_starred should be preserved as true")
	}
	if p.IsAutoGrouped {
		t.Error("is_auto_grouped should remain false for manually adjusted projects")
	}
}

func TestSyncProjectTx_UpdatesAutoGrouped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create an auto-grouped project
	res, _ := db.Exec(
		"INSERT INTO projects (name, root_path, is_auto_grouped, is_starred) VALUES (?, ?, ?, ?)",
		"old-name", "/repos/proj", 1, 0,
	)
	origID, _ := res.LastInsertId()

	tx, _ := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	id, err := SyncProjectTx(tx, "new-name", "/repos/proj", 0, true)
	if err != nil {
		t.Fatalf("SyncProjectTx failed: %v", err)
	}
	tx.Commit() //nolint:errcheck

	p, _ := GetProjectByID(db, id)
	if p.Name != "new-name" {
		t.Errorf("expected name 'new-name', got '%s'", p.Name)
	}
	if p.IsAutoGrouped != true {
		t.Error("is_auto_grouped should be updated to true for auto-grouped projects")
	}
	if p.IsStarred {
		t.Error("is_starred should remain false")
	}
	if id != origID {
		t.Errorf("expected same ID, got different")
	}
}

// -- CleanupStaleDataTx tests --

func TestCleanupStaleDataTx_RemovesStaleRepos(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create two projects with repos
	p1 := createTestProject(t, db, "proj1")
	p2 := createTestProject(t, db, "proj2")

	db.Exec("INSERT INTO repositories (path, project_id) VALUES (?, ?)", "/repos/p1", p1) //nolint:errcheck
	db.Exec("INSERT INTO repositories (path, project_id) VALUES (?, ?)", "/repos/p2", p2) //nolint:errcheck

	// Add stats for both repos
	var r1ID, r2ID int64
	db.QueryRow("SELECT id FROM repositories WHERE path = '/repos/p1'").Scan(&r1ID)
	db.QueryRow("SELECT id FROM repositories WHERE path = '/repos/p2'").Scan(&r2ID)
	db.Exec("INSERT INTO daily_stats (repository_id, stat_date, author, lines_added) VALUES (?, ?, ?, ?)",
		r1ID, "2024-01-01", "all", 100) //nolint:errcheck
	db.Exec("INSERT INTO daily_stats (repository_id, stat_date, author, lines_added) VALUES (?, ?, ?, ?)",
		r2ID, "2024-01-01", "all", 200) //nolint:errcheck

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Only p1's repo is in the scanned set
	err = CleanupStaleDataTx(tx, []string{"/repos/p1"})
	if err != nil {
		t.Fatalf("CleanupStaleDataTx failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// p2's repo should be deleted
	var count int
	db.QueryRow("SELECT COUNT(*) FROM repositories WHERE path = '/repos/p2'").Scan(&count)
	if count != 0 {
		t.Error("stale repo should be deleted")
	}

	// p1's repo should still exist
	db.QueryRow("SELECT COUNT(*) FROM repositories WHERE path = '/repos/p1'").Scan(&count)
	if count != 1 {
		t.Error("scanned repo should still exist")
	}

	// p2's stats should be deleted
	db.QueryRow("SELECT COUNT(*) FROM daily_stats WHERE repository_id = ?", r2ID).Scan(&count)
	if count != 0 {
		t.Error("stale repo stats should be deleted")
	}
}

func TestCleanupStaleDataTx_PreservesProjectsWithNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a project with a note but no repos (simulating a project whose repos disappeared)
	p1 := createTestProject(t, db, "proj-with-notes")
	CreateNote(db, p1, "important note") //nolint:errcheck

	// Create a project with no notes/repos (orphaned)
	p2 := createTestProject(t, db, "orphaned")

	tx, _ := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	// Non-matching scanned paths - all repos are stale
	err := CleanupStaleDataTx(tx, []string{"/nonexistent"})
	if err != nil {
		t.Fatalf("CleanupStaleDataTx failed: %v", err)
	}
	tx.Commit() //nolint:errcheck

	// Project with notes should be preserved
	var count int
	db.QueryRow("SELECT COUNT(*) FROM projects WHERE id = ?", p1).Scan(&count)
	if count != 1 {
		t.Error("project with notes should be preserved")
	}

	// Orphaned project should be deleted
	db.QueryRow("SELECT COUNT(*) FROM projects WHERE id = ?", p2).Scan(&count)
	if count != 0 {
		t.Error("orphaned project should be deleted")
	}
}

// An empty scanned-path set used to mean "delete every repository and stat".
// That is indistinguishable from "the scan walk failed" — an unreadable root,
// a vanished symlink, an unmounted volume — and the deleted stats cannot be
// rebuilt without re-walking git history in every project. The cleanup now
// refuses instead, so this asserts the refusal AND that nothing was deleted.
func TestCleanupStaleDataTx_RefusesEmptyPathsAndKeepsData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	p1 := createTestProject(t, db, "keeper")
	db.Exec("INSERT INTO repositories (path, project_id) VALUES (?, ?)", "/repos/keeper", p1) //nolint:errcheck
	var rID int64
	db.QueryRow("SELECT id FROM repositories WHERE path = '/repos/keeper'").Scan(&rID)
	db.Exec("INSERT INTO daily_stats (repository_id, stat_date, author, lines_added) VALUES (?, ?, ?, ?)",
		rID, "2024-01-01", "all", 100) //nolint:errcheck

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := CleanupStaleDataTx(tx, []string{}); !errors.Is(err, ErrNoScannedPaths) {
		t.Fatalf("expected ErrNoScannedPaths, got %v", err)
	}

	// Roll the transaction back before querying through the pool. A ":memory:"
	// database has a single connection, so a read on the still-open tx would
	// hold the only connection and block forever.
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// The committed data must be intact — this is the whole point of refusing.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM repositories WHERE path = '/repos/keeper'").Scan(&count); err != nil {
		t.Fatalf("count repos after rollback: %v", err)
	}
	if count != 1 {
		t.Error("repository must survive after rollback")
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM daily_stats WHERE repository_id = ?", rID).Scan(&count); err != nil {
		t.Fatalf("count daily stats after rollback: %v", err)
	}
	if count != 1 {
		t.Error("daily stats must survive a refused cleanup")
	}
}

func TestSearchProjects_Basic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES (?, ?, 1)",
		"my-app", "/workspace/my-app")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES (?, ?, 1)",
		"other-tool", "/home/user/other-tool")
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	results, err := SearchProjects(db, "my")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'my', got %d", len(results))
	}
	if results[0].Name != "my-app" {
		t.Errorf("expected 'my-app', got '%s'", results[0].Name)
	}
}

func TestSearchProjects_ByPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('alpha', '/code/alpha', 1)")
	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('beta', '/workspace/beta', 1)")

	results, err := SearchProjects(db, "workspace")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "beta" {
		t.Fatalf("expected 1 result (beta) for 'workspace', got %d", len(results))
	}
}

func TestSearchProjects_EscapedWildcards(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('test_2024', '/p/test_2024', 1)")
	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('test2024', '/p/test2024', 1)")
	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('100%done', '/p/100pct-done', 1)")
	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('100_percent', '/p/100_percent', 1)")

	// Underscore should be literal, not a single-char wildcard
	results, err := SearchProjects(db, "test_")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "test_2024" {
		t.Fatalf("underscore should be literal: expected 1 result (test_2024), got %d", len(results))
	}

	// Percent should be literal, not a multi-char wildcard
	results, err = SearchProjects(db, "100%")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "100%done" {
		t.Fatalf("percent should be literal: expected 1 result (100%%done), got %d", len(results))
	}
}

func TestSearchProjects_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES ('a', '/a', 1)")

	results, err := SearchProjects(db, "")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if results != nil {
		t.Error("empty query should return nil results")
	}

	results, err = SearchProjects(db, "   ")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if results != nil {
		t.Error("whitespace-only query should return nil results")
	}
}

func TestSearchProjects_Limit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("proj-%02d", i)
		db.Exec("INSERT INTO projects (name, root_path, is_auto_grouped) VALUES (?, ?, 1)", name, "/p/"+name)
	}

	results, err := SearchProjects(db, "proj")
	if err != nil {
		t.Fatalf("SearchProjects failed: %v", err)
	}
	if len(results) > searchProjectsLimit {
		t.Errorf("results should be limited to %d, got %d", searchProjectsLimit, len(results))
	}
}
