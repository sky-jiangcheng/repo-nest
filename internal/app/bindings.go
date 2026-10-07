package app

import (
	"context"

	pluginruntime "repo-nest/internal/core/plugin/runtime"
	"repo-nest/internal/domain"
	"repo-nest/internal/service"
	"repo-nest/internal/stats"
)

// --- Wails binding ↔ MCP tool audit (P32, 2026-10-02) -----------------------
//
// Every binding below is intentional — either backed by a reponest_* MCP tool
// (the agent-facing surface) or a deliberate desktop-only GUI/admin affordance.
// No dead or duplicate bindings were found. Sections map to the groups below.
//
// MCP-backed (same capability is exposed to agents too):
//   GetProjects→projects_list, GetProjectStats→projects_stats,
//   GetProjectOverview→context, TriggerScan→scan,
//   ListNotes/ListAllNotes→notes_list, SearchNotes/SearchAll→notes_search,
//   CreateNote/CreateNoteWithMeta→notes_create,
//   UpdateNote/UpdateNoteFull/UpdateNoteMeta→notes_update.
//
// Desktop-only (no MCP equivalent by design — GUI state, admin, or file UX):
//   Projects: UpdateProjectLevel, ToggleStar, RefreshProjectHistory
//   Scan: GetScanStatus (progress polling)
//   Dashboard: GetSummary, GetHeatmapData, GetStatusBar, GetTodoCounts, GetNoteCounts
//   Notes lifecycle: DeleteNote, PinNote, MoveNote, ListNoteVersions,
//                    RestoreNoteVersion, DiffNoteVersions
//   Todos: ListTodos, CreateTodo, ToggleTodo, DeleteTodo, ReorderTodos
//   Config: GetConfig, UpdateConfig, UpdateScanRoots
//   Exports: GenerateLLMsTxt, ExportNoteAsMarkdown, ExportMemoryJSON
//   Plugins/knowledge sources: GetPluginStatuses, GetKnowledgeSources,
//                              TriggerKnowledgeImport, TriggerAllKnowledgeImports,
//                              ReloadPlugins, ImportClaudeMemory, CaptureClaudeHandoff
//
// Conversely these MCP tools have no binding (agent-only flows, served straight
// off the service layer): reponest_ask, reponest_handoff, reponest_agent_score,
// reponest_integrity, reponest_notes_read.

// --- Projects ---------------------------------------------------------------

// GetProjects returns enriched project summaries, optionally filtered by date
// and starred status.
func (a *App) GetProjects(date string, starredOnly bool) []service.ProjectResponse {
	return a.svc.GetProjects(date, starredOnly)
}

// GetProjectDetail returns a project with all its repositories and stats.
func (a *App) GetProjectDetail(id int64) (*service.ProjectDetailResponse, error) {
	return a.svc.GetProjectDetail(id)
}

// GetProjectStats returns daily stats for a project, optionally by date.
func (a *App) GetProjectStats(id int64, date string) []domain.DailyStat {
	return a.svc.GetProjectStats(id, date)
}

// GetRepoCommits returns one repository's commit log, newest first. Backs the
// repo row's commit list: the stored daily_stats rows carry no message or SHA,
// so the list is read from git on demand and works without a forge remote.
func (a *App) GetRepoCommits(repoID int64, limit int) ([]stats.RecentCommit, error) {
	return a.svc.GetRepoCommits(repoID, limit)
}

// UpdateProjectLevel adjusts a project's grouping level up or down.
func (a *App) UpdateProjectLevel(id int64, direction string) (*service.LevelUpdateResult, error) {
	return a.svc.UpdateProjectLevel(id, direction)
}

// ToggleStar flips the starred status of a project.
func (a *App) ToggleStar(projectID int64) (bool, error) { return a.svc.ToggleStar(projectID) }

// RefreshProjectHistory triggers a full 365-day stats backfill for a single
// project's repositories.
func (a *App) RefreshProjectHistory(projectID int64) (map[string]any, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.svc.RefreshProjectHistory(ctx, projectID); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// SearchProjects searches for projects by name or path.
func (a *App) SearchProjects(query string) []service.ProjectResponse {
	return a.svc.SearchProjects(query)
}

// GetProjectOverview returns mined knowledge for a project detail page.
func (a *App) GetProjectOverview(projectID int64) (*service.ProjectOverview, error) {
	return a.svc.GetProjectOverview(projectID)
}

// --- Scan -------------------------------------------------------------------

// TriggerScan starts an async full repository scan and returns immediately.
func (a *App) TriggerScan() (*service.ScanResult, error) { return a.svc.TriggerScan() }

// GetScanStatus returns the current scan progress.
func (a *App) GetScanStatus() *service.ScanStatus { return a.svc.GetScanStatus() }

// --- Summary / heatmap / status ----------------------------------------------

// GetSummary returns aggregated stats for all repositories on a given date.
func (a *App) GetSummary(date string) (*service.SummaryData, error) { return a.svc.GetSummary(date) }

// GetHeatmapData returns daily commit stats for the past year. A positive
// projectID restricts the aggregation to that project's repositories.
func (a *App) GetHeatmapData(projectID int64) *service.HeatmapResponse {
	return a.svc.GetHeatmapData(projectID)
}

// GetStatusBar returns current status bar information (30s cached).
func (a *App) GetStatusBar() *service.StatusBarData { return a.svc.GetStatusBar() }

// GetTodoCounts returns incomplete and total todo counts per project.
func (a *App) GetTodoCounts() []domain.TodoCount { return a.svc.GetTodoCounts() }

// GetNoteCounts returns the count of notes per project.
func (a *App) GetNoteCounts() []domain.NoteCount { return a.svc.GetNoteCounts() }

// --- Search -----------------------------------------------------------------

// SearchNotes searches note content/title/tags across all projects.
func (a *App) SearchNotes(query string) []domain.SearchHit { return a.svc.SearchNotes(query) }

// SearchAll searches notes and todos together.
func (a *App) SearchAll(query string) []domain.SearchHit { return a.svc.SearchAll(query) }

// --- Notes ------------------------------------------------------------------

// ListNotes returns all notes for a project.
func (a *App) ListNotes(projectID int64) []domain.Note { return a.svc.ListNotes(projectID) }

// CreateNote creates a new note for a project.
func (a *App) CreateNote(projectID int64, content string) (*domain.Note, error) {
	return a.svc.CreateNote(projectID, content)
}

// CreateNoteWithMeta creates a note with explicit title, tags, kind and source.
func (a *App) CreateNoteWithMeta(projectID int64, title, content, tags, kind, source string) (*domain.Note, error) {
	return a.svc.CreateNoteWithMeta(projectID, title, content, tags, kind, source)
}

// UpdateNote updates the content of a note.
func (a *App) UpdateNote(noteID int64, content string) error {
	return a.svc.UpdateNote(noteID, content)
}

// UpdateNoteFull updates both content and metadata in a single transaction.
func (a *App) UpdateNoteFull(noteID int64, content, title, tags, kind string, pinned bool) error {
	return a.svc.UpdateNoteFull(noteID, content, title, tags, kind, pinned)
}

// DeleteNote removes a note.
func (a *App) DeleteNote(noteID int64) error { return a.svc.DeleteNote(noteID) }

// UpdateNoteMeta updates a note's editable metadata.
func (a *App) UpdateNoteMeta(noteID int64, title, tags, kind string, pinned bool) error {
	return a.svc.UpdateNoteMeta(noteID, title, tags, kind, pinned)
}

// PinNote sets or clears the pinned flag on a note.
func (a *App) PinNote(noteID int64, pinned bool) error { return a.svc.PinNote(noteID, pinned) }

// MoveNote reassigns a note to a different project.
func (a *App) MoveNote(noteID, projectID int64) error { return a.svc.MoveNote(noteID, projectID) }

// ListAllNotes returns every note across all projects with project info.
func (a *App) ListAllNotes() []domain.NoteWithProject { return a.svc.ListAllNotes() }

// ListAllTags returns the distinct set of tags used across all notes.
func (a *App) ListAllTags() []string { return a.svc.ListAllTags() }

// ListNoteVersions returns the recent version history for a note.
func (a *App) ListNoteVersions(noteID int64) []domain.NoteVersion {
	return a.svc.ListNoteVersions(noteID)
}

// RestoreNoteVersion restores a note to the content of a previous version.
func (a *App) RestoreNoteVersion(noteID, versionID int64) error {
	return a.svc.RestoreNoteVersion(noteID, versionID)
}

// DiffNoteVersions returns a line-based diff between a version and the note.
func (a *App) DiffNoteVersions(noteID, versionID int64) (string, error) {
	return a.svc.DiffNoteVersions(noteID, versionID)
}

// --- Todos ------------------------------------------------------------------

// ListTodos returns all todo items for a project.
func (a *App) ListTodos(projectID int64) []domain.Todo { return a.svc.ListTodos(projectID) }

// CreateTodo creates a new todo for a project.
func (a *App) CreateTodo(projectID int64, title string) (*domain.Todo, error) {
	return a.svc.CreateTodo(projectID, title)
}

// ToggleTodo flips the completed status of a todo.
func (a *App) ToggleTodo(todoID int64) error { return a.svc.ToggleTodo(todoID) }

// DeleteTodo removes a todo.
func (a *App) DeleteTodo(todoID int64) error { return a.svc.DeleteTodo(todoID) }

// ReorderTodos updates the sort_order for a list of todo IDs.
func (a *App) ReorderTodos(todoIDs []int64) error { return a.svc.ReorderTodos(todoIDs) }

// --- Config -----------------------------------------------------------------

// GetConfig returns all configuration settings and scan roots.
func (a *App) GetConfig() (*service.ConfigData, error) { return a.svc.GetConfig() }

// UpdateConfig sets a single configuration key-value pair.
func (a *App) UpdateConfig(key, value string) error { return a.svc.UpdateConfig(key, value) }

// UpdateScanRoots replaces the entire scan root list atomically.
//
// The submitted list is normalised first (see service.normalizeScanRoots): only
// real directories are stored, in canonical form, deduplicated. Entries that
// could not be accepted are reported in the result rather than failing the call,
// so a caller keeps the roots it asked for and learns which ones were dropped.
func (a *App) UpdateScanRoots(scanRoots []string) (*service.ScanRootsResult, error) {
	return a.svc.UpdateScanRoots(scanRoots)
}

// --- AI-facing exports --------------------------------------------------------

// GenerateLLMsTxt returns an aggregated Markdown document for AI consumption.
func (a *App) GenerateLLMsTxt() string { return a.svc.GenerateLLMsTxt() }

// ExportNoteAsMarkdown returns a single note as Markdown with YAML frontmatter.
func (a *App) ExportNoteAsMarkdown(noteID int64) string { return a.svc.ExportNoteAsMarkdown(noteID) }

// ExportMemoryJSON returns the knowledge base as an OMP-style portable memory
// JSON array (ADR-0013 interop seam; provisional, OMP is pre-v1). limit<=0 = all.
func (a *App) ExportMemoryJSON(limit int64) string { return a.svc.ExportMemoryJSON(int(limit)) }

// --- Plugins / knowledge sources ----------------------------------------------

// GetPluginStatuses returns the load status of every plugin directory.
func (a *App) GetPluginStatuses() []pluginruntime.PluginStatus { return a.svc.GetPluginStatuses() }

// GetKnowledgeSources returns registered knowledge importers with their state.
func (a *App) GetKnowledgeSources() []pluginruntime.SourceStatus { return a.svc.GetKnowledgeSources() }

// TriggerKnowledgeImport runs one knowledge source and returns its statistics.
func (a *App) TriggerKnowledgeImport(name string) (pluginruntime.ImportRun, error) {
	return a.svc.TriggerKnowledgeImport(name)
}

// TriggerAllKnowledgeImports runs every registered knowledge source.
func (a *App) TriggerAllKnowledgeImports() []pluginruntime.SourceRun {
	return a.svc.TriggerAllKnowledgeImports()
}

// ReloadPlugins rescans the plugins directory and reloads every plugin.
func (a *App) ReloadPlugins() []pluginruntime.PluginStatus { return a.svc.ReloadPlugins() }

// ImportClaudeMemory imports notes from Claude's memory directories.
func (a *App) ImportClaudeMemory() (*service.ImportResult, error) {
	return a.svc.ImportClaudeMemory()
}

// CaptureClaudeHandoff runs an on-demand M1 session capture for one project
// (gated by the `claude_session_capture` config, default off). Desktop-only:
// agents push handoffs via reponest_handoff instead.
func (a *App) CaptureClaudeHandoff(projectID int64) (*service.HandoffResult, error) {
	return a.svc.CaptureClaudeHandoff(projectID)
}

// RebuildEmbeddings (re)embeds every note into the semantic-search vector index.
// Desktop-only admin action; requires semantic_search enabled + a configured
// embedding endpoint (both default off — ADR-0012).
func (a *App) RebuildEmbeddings() (int, error) {
	return a.svc.RebuildEmbeddings()
}

// AskAI forwards the question (with generated project context for projectID>0)
// to the OpenAI-compatible chat endpoint configured in Settings → AI, and
// returns the assistant reply for the Q&A panel.
func (a *App) AskAI(projectID int64, question string) (string, error) {
	return a.svc.AskAI(projectID, question)
}

// ListAIModels probes the endpoint's /models (with path candidates and the
// localhost fallback) using the form's current values — no save required.
// ResolvedBaseURL is the base that actually worked, so the UI can adopt it.
func (a *App) ListAIModels(baseURL, apiKey string) (*service.AIModelListResult, error) {
	return service.ListAIModels(baseURL, apiKey)
}

// TestAIChat verifies a specific model responds to a minimal completion,
// using the form's current values.
func (a *App) TestAIChat(baseURL, model, apiKey string) (*service.AITestResult, error) {
	return service.TestAIChat(baseURL, model, apiKey)
}
