package service

import (
	"fmt"
	"log"
	"strings"

	"repotory/internal/db"
	"repotory/internal/domain"
)

// ListTodos returns all todo items for a project.
func (s *Service) ListTodos(projectID int64) []domain.Todo {
	todos, err := db.ListTodos(s.db, projectID)
	if err != nil {
		log.Printf("list todos error: %v", err)
		return nil
	}
	if todos == nil {
		todos = []domain.Todo{}
	}
	return todos
}

// CreateTodo creates a new todo for a project.
func (s *Service) CreateTodo(projectID int64, title string) (*domain.Todo, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	return db.CreateTodo(s.db, projectID, title)
}

// ToggleTodo flips the completed status of a todo.
func (s *Service) ToggleTodo(todoID int64) error {
	return db.ToggleTodo(s.db, todoID)
}

// DeleteTodo removes a todo.
func (s *Service) DeleteTodo(todoID int64) error {
	return db.DeleteTodo(s.db, todoID)
}

// ReorderTodos updates the sort_order for a list of todo IDs.
func (s *Service) ReorderTodos(todoIDs []int64) error {
	return db.ReorderTodos(s.db, todoIDs)
}
