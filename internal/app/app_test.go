package app

import (
	"testing"

	"repotory/internal/db"
	"repotory/internal/service"
)

// setupTestApp creates an in-memory App for integration testing, using the
// real schema via db.InitDB so tests cannot drift from migrations.
func setupTestApp(t *testing.T) *App {
	t.Helper()
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Seed one project for FK targets through the production sync helper.
	tx, err := database.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	if _, err := db.SyncProjectTx(tx, "test-project", "/tmp/test", 0, true); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit seed: %v", err)
	}
	return New(service.New(database, "testuser"))
}

func TestAppCreateAndListTodos(t *testing.T) {
	a := setupTestApp(t)

	todo, err := a.CreateTodo(1, "Fix bug")
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}
	if todo.Title != "Fix bug" {
		t.Errorf("expected 'Fix bug', got '%s'", todo.Title)
	}

	todos := a.ListTodos(1)
	if len(todos) != 1 {
		t.Errorf("expected 1 todo, got %d", len(todos))
	}
}

func TestAppCreateTodoEmptyTitle(t *testing.T) {
	a := setupTestApp(t)

	if _, err := a.CreateTodo(1, ""); err == nil {
		t.Error("expected error for empty title")
	}
	if _, err := a.CreateTodo(1, "   "); err == nil {
		t.Error("expected error for whitespace-only title")
	}
}

func TestAppToggleTodo(t *testing.T) {
	a := setupTestApp(t)

	todo, _ := a.CreateTodo(1, "Toggle me")
	if err := a.ToggleTodo(todo.ID); err != nil {
		t.Fatalf("ToggleTodo failed: %v", err)
	}

	todos := a.ListTodos(1)
	if !todos[0].Completed {
		t.Error("todo should be completed after toggle")
	}
}

func TestAppReorderTodos(t *testing.T) {
	a := setupTestApp(t)

	t1, _ := a.CreateTodo(1, "A")
	t2, _ := a.CreateTodo(1, "B")
	t3, _ := a.CreateTodo(1, "C")

	if err := a.ReorderTodos([]int64{t3.ID, t2.ID, t1.ID}); err != nil {
		t.Fatalf("ReorderTodos failed: %v", err)
	}

	todos := a.ListTodos(1)
	if todos[0].Title != "C" {
		t.Errorf("first todo should be 'C', got '%s'", todos[0].Title)
	}
}

func TestAppDeleteTodo(t *testing.T) {
	a := setupTestApp(t)

	todo, _ := a.CreateTodo(1, "Delete me")
	if err := a.DeleteTodo(todo.ID); err != nil {
		t.Fatalf("DeleteTodo failed: %v", err)
	}

	if todos := a.ListTodos(1); len(todos) != 0 {
		t.Errorf("expected 0 todos, got %d", len(todos))
	}
}

func TestAppCreateAndUpdateNote(t *testing.T) {
	a := setupTestApp(t)

	note, err := a.CreateNote(1, "# Hello")
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	if note.Content != "# Hello" {
		t.Errorf("expected '# Hello', got '%s'", note.Content)
	}

	if err := a.UpdateNote(note.ID, "# Updated"); err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}

	notes := a.ListNotes(1)
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "# Updated" {
		t.Errorf("expected '# Updated', got '%s'", notes[0].Content)
	}
}

func TestAppCreateNoteEmptyContent(t *testing.T) {
	a := setupTestApp(t)

	if _, err := a.CreateNote(1, ""); err == nil {
		t.Error("expected error for empty content")
	}
	if err := a.UpdateNote(1, "   "); err == nil {
		t.Error("expected error for empty content in update")
	}
}

func TestAppDeleteNote(t *testing.T) {
	a := setupTestApp(t)

	note, _ := a.CreateNote(1, "Delete me")
	if err := a.DeleteNote(note.ID); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}

	if notes := a.ListNotes(1); len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
}

func TestAppGetTodoCounts(t *testing.T) {
	a := setupTestApp(t)

	_, _ = a.CreateTodo(1, "T1")
	_, _ = a.CreateTodo(1, "T2")
	t3, _ := a.CreateTodo(1, "T3")
	_ = a.ToggleTodo(t3.ID)

	counts := a.GetTodoCounts()
	if len(counts) != 1 {
		t.Fatalf("expected 1 count entry, got %d", len(counts))
	}
	if counts[0].Count != 2 || counts[0].Total != 3 {
		t.Errorf("expected count=2 total=3, got count=%d total=%d", counts[0].Count, counts[0].Total)
	}
}

func TestAppGetTodoCountsEmpty(t *testing.T) {
	a := setupTestApp(t)

	if counts := a.GetTodoCounts(); len(counts) != 0 {
		t.Errorf("expected empty counts, got %d", len(counts))
	}
}

func TestAppHealth(t *testing.T) {
	a := setupTestApp(t)
	h := a.Health()
	if h["status"] != "ok" {
		t.Errorf("expected status ok, got %v", h["status"])
	}
	if h["version"] == "" {
		t.Error("expected non-empty version")
	}
}
