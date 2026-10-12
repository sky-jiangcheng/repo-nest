package service

import (
	"fmt"
	"log"
	"strings"

	"repotory/internal/db"
	"repotory/internal/domain"
)

// SearchNotes searches note content/title/tags across all projects,
// returning ranked hits with context snippets.
func (s *Service) SearchNotes(query string) []domain.SearchHit {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	results, err := db.SearchNotes(s.db, query)
	if err != nil {
		log.Printf("search notes error: %v", err)
		return nil
	}
	if results == nil {
		results = []domain.SearchHit{}
	}
	// M3-A: optionally fuse vector recall (default off; a no-op bypass when off).
	return s.fuseSemantic(results, query)
}

// SearchAll searches notes and todos together, returning ranked unified hits.
func (s *Service) SearchAll(query string) []domain.SearchHit {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	results, err := db.SearchAll(s.db, query)
	if err != nil {
		log.Printf("search all error: %v", err)
		return nil
	}
	if results == nil {
		results = []domain.SearchHit{}
	}
	return results
}

// FormatSearchAnswer renders up to limit search hits in the shared
// "[type] title\nsnippet" text format used by the CLI ask command and the MCP
// ask tool.
func FormatSearchAnswer(hits []domain.SearchHit, limit int) []string {
	parts := make([]string, 0, limit)
	for i, h := range hits {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("[%s] %s\n%s", h.Type, h.Title, h.Snippet))
	}
	return parts
}
