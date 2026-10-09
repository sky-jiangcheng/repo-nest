package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"repo-nest/internal/db"
)

// handoffTag is the reserved tag marking a note as a structured session
// handoff. BuildProjectContext sorts these notes first, so the next session
// reads the previous one's exit record before anything else.
const handoffTag = "handoff"

// Handoff write bounds. A handoff is agent-authored JSON arriving over MCP,
// so it is untrusted input: without caps a runaway agent could write a
// multi-megabyte summary or thousands of bullets in one call, and the next
// reponest_context would embed all of it. The note-level bounds still apply
// on top of these (the rendered Markdown is written through CreateNoteWithMeta).
const (
	maxHandoffSummaryLen = 4_000
	maxHandoffItems      = 20    // items per section
	maxHandoffItemLen    = 1_000 // bytes per bullet
)

// HandoffInput is the structured record of what an AI session accomplished.
// Sections are optional except Summary, but an empty record carries no value
// for the next session, so at least one section must be non-empty.
type HandoffInput struct {
	ProjectID int64
	// Agent names the tool that produced the handoff, e.g. "claude-code" or
	// "cursor". Optional; recorded in the note header for provenance.
	Agent     string
	Summary   string
	Changes   []string
	Decisions []string
	Gotchas   []string
	NextSteps []string
	// Tags appends user-supplied tags to the reserved "handoff" tag.
	Tags string
}

// CreateHandoffNote persists a session handoff as a knowledge note rendered
// from a fixed Markdown template. A stable template matters: the note is
// consumed by both humans (desktop UI) and agents (reponest_context), and the
// section headings are the contract between the writer and every future
// reader.
func (s *Service) CreateHandoffNote(in HandoffInput) (*HandoffResult, error) {
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return nil, fmt.Errorf("summary is required")
	}
	if !handoffHasContent(in) {
		return nil, fmt.Errorf("at least one of changes, decisions, gotchas, next_steps must be non-empty")
	}
	if err := validateHandoffBounds(in, summary); err != nil {
		return nil, err
	}

	title := fmt.Sprintf("Session Handoff %s", time.Now().Format("2006-01-02 15:04"))
	tags := handoffTag
	if extra := strings.TrimSpace(in.Tags); extra != "" {
		tags = tags + ", " + extra
	}

	note, err := s.CreateNoteWithMeta(in.ProjectID, title, renderHandoffMarkdown(in, summary), tags, "knowledge", "mcp")
	if err != nil {
		return nil, err
	}
	return &HandoffResult{NoteID: note.ID, Title: note.Title, Tags: tags}, nil
}

// validateHandoffBounds caps the structured sections before rendering, so
// the failure message names the section and the limit the agent hit.
func validateHandoffBounds(in HandoffInput, summary string) error {
	if len(summary) > maxHandoffSummaryLen {
		return fmt.Errorf("summary too long: %d bytes (max %d)", len(summary), maxHandoffSummaryLen)
	}
	for name, section := range map[string][]string{
		"changes": in.Changes, "decisions": in.Decisions,
		"gotchas": in.Gotchas, "next_steps": in.NextSteps,
	} {
		items := nonEmptyLines(section)
		if len(items) > maxHandoffItems {
			return fmt.Errorf("%s has too many items: %d (max %d)", name, len(items), maxHandoffItems)
		}
		for _, item := range items {
			if len(item) > maxHandoffItemLen {
				return fmt.Errorf("%s item too long: %d bytes (max %d)", name, len(item), maxHandoffItemLen)
			}
		}
	}
	return nil
}

// HandoffResult reports what was persisted so the calling agent can confirm
// the handoff landed instead of assuming.
type HandoffResult struct {
	NoteID int64  `json:"note_id"`
	Title  string `json:"title"`
	Tags   string `json:"tags"`
}

// HandoffStatus is the minimal status surface for UI chrome (currently the VS
// Code status bar). It intentionally excludes content: status updates may poll
// periodically, and the last handoff's body belongs to the knowledge reader.
type HandoffStatus struct {
	NoteID    int64  `json:"note_id"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

// LatestHandoff returns the newest handoff note's status. projectID>0 scopes
// to one project; zero means the newest across the knowledge base. A missing
// handoff is (nil, nil), not an error.
func (s *Service) LatestHandoff(projectID int64) (*HandoffStatus, error) {
	note, err := db.LatestHandoffNote(s.db, projectID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &HandoffStatus{
		NoteID:    note.ID,
		ProjectID: note.ProjectID,
		Title:     note.Title,
		UpdatedAt: note.UpdatedAt,
	}, nil
}

// handoffHasContent checks whether any structured section carries information.
func handoffHasContent(in HandoffInput) bool {
	for _, section := range [][]string{in.Changes, in.Decisions, in.Gotchas, in.NextSteps} {
		for _, item := range section {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	}
	return false
}

// renderHandoffMarkdown renders the canonical handoff template. Sections with
// no content are omitted entirely: an absent heading tells the reader "the
// previous session recorded nothing here" without a wall of empty bullets.
func renderHandoffMarkdown(in HandoffInput, summary string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Session Handoff\n\n"))
	if agent := strings.TrimSpace(in.Agent); agent != "" {
		b.WriteString(fmt.Sprintf("> Written by `%s`\n\n", agent))
	}
	b.WriteString("## Summary\n\n")
	b.WriteString(summary)
	b.WriteString("\n\n")

	writeSection := func(heading string, items []string) {
		lines := nonEmptyLines(items)
		if len(lines) == 0 {
			return
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", heading))
		for _, item := range lines {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
		b.WriteString("\n")
	}
	writeSection("Changes", in.Changes)
	writeSection("Decisions", in.Decisions)
	writeSection("Gotchas", in.Gotchas)
	writeSection("Next Steps", in.NextSteps)

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// nonEmptyLines drops blank items and trims whitespace so template output
// stays tidy regardless of how the caller formatted the sections.
func nonEmptyLines(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if line := strings.TrimSpace(item); line != "" {
			out = append(out, line)
		}
	}
	return out
}
