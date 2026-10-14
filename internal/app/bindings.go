package app

import (
	"context"
	"fmt"
	"time"

	"repotory/internal/bounds"
	pluginruntime "repotory/internal/core/plugin/runtime"
	"repotory/internal/db"
	"repotory/internal/domain"
	"repotory/internal/service"
	"repotory/internal/stats"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// --- Wails binding ↔ MCP tool audit (P32, 2026-10-02) -----------------------
//
// Every binding below is intentional — either backed by a repotory_* MCP tool
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
//   Projects: UpdateProjectLevel, ToggleStar, RefreshProjectHistory,
//             GetRepoCommits, GetProjectCommits
//   AI: AskAIWithEvidence, FileAnswerAsPage, StartAskStream (evidence-gated Q&A,
//       its query-loop filing, and the streaming variant; desktop UI consumers by
//       design — StartAskStream rides Wails events, so no MCP/HTTP twin)
//   Wiki: RunWikiLint (ADR-0014 W4 lint; read-only over pages, writes todos only)
//   Memory layers (ADR-0014 W5): LayeredProjectContext (orientation-first,
//             budget-bounded L3/L2/L1/L0 assembly; read-only)
//   Async jobs (ADR-0016): StartCompileJob, StartLintJob, GetCompileJob,
//             ListCompileJobs, CancelCompileJob — submission returns at once so a
//             multi-minute LLM batch never blocks a caller. One queue with two
//             kernels (待决 ①); ListCompileJobs filters by kind.
//   Wiki compile & review (ADR-0015): CompileNote, CompileProjectNotes,
//             ListPendingWikiPages, ApproveWikiPage, RejectWikiPage. Deliberately
//             no MCP twin: an agent able to write pending knowledge would bypass
//             the human gate the whole design rests on.
//   Workspace chrome: ResolveWorkspacePath — read-only path→project/repository
//             resolution so the IDE can show what it matched. No MCP twin by
//             design: agents pass `path` to repotory_context instead, and a second
//             client-side implementation of the matcher is the failure this avoids.
//   Knowledge graph (ADR-0019/0020/0021): FindMergeCandidates, MergeWikiPages,
//             RunCommunitySummary, ListWikiPageVersions, DiffWikiPageVersion.
//             Also no MCP twin, for two different reasons worth keeping apart:
//             MergeWikiPages destroys knowledge and must only ever follow a human
//             decision, while RunCommunitySummary writes model output into the
//             library and would defeat the review queue if an agent could call it.
//             The three read-only structural ones (candidates, versions, diff) stay
//             unexported to agents for the same reason the review queue does — they
//             are the surfaces a person looks at to decide.
//   Scan: GetScanStatus (progress polling)
//   Dashboard: GetSummary, GetHeatmapData, GetStatusBar, GetTodoCounts, GetNoteCounts
//   Notes lifecycle: DeleteNote, PinNote, MoveNote, ListNoteVersions,
//                    RestoreNoteVersion, DiffNoteVersions
//   Todos: ListTodos, CreateTodo, ToggleTodo, DeleteTodo, ReorderTodos
//   Config: GetConfig, UpdateConfig, UpdateScanRoots
//   Exports: GenerateLLMsTxt, ExportNoteAsMarkdown, ExportMemoryJSON
//   Plugins/knowledge sources: GetKnowledgeSources, TriggerKnowledgeImport,
//                              TriggerAllKnowledgeImports, ImportClaudeMemory,
//                              CaptureClaudeHandoff
//   Memory ring (D25 first screen): ListRecentHandoffs — content-free
//                    entries; reading a handoff stays on the knowledge page
//   IDE chrome: LatestHandoff (timestamp/title only; content stays behind notes_read)
//
// Conversely these MCP tools have no binding (agent-only flows, served straight
// off the service layer): repotory_ask, repotory_handoff, repotory_agent_score,
// repotory_integrity, repotory_notes_read.

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

// GetProjectCommits returns the merged cross-repo commit log for a project,
// newest first. Backs the commits tab's unified timeline.
func (a *App) GetProjectCommits(projectID int64, limit int) ([]stats.RecentCommit, error) {
	return a.svc.GetProjectCommits(projectID, limit)
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

// ImportClaudeMemory imports notes from Claude's memory directories.
func (a *App) ImportClaudeMemory() (*service.ImportResult, error) {
	return a.svc.ImportClaudeMemory()
}

// ListRecentHandoffs returns the first-screen memory ring's recent handoffs
// (content-free entries, newest first; D25). Desktop-only: the ring is the
// human entry point, agents read via repotory_notes_read / repotory_context.
func (a *App) ListRecentHandoffs(limit int) []service.HandoffEntry {
	return a.svc.ListRecentHandoffs(limit)
}

// CaptureClaudeHandoff runs an on-demand M1 session capture for one project
// (gated by the `claude_session_capture` config, default off). Desktop-only:
// agents push handoffs via repotory_handoff instead.
func (a *App) CaptureClaudeHandoff(projectID int64) (*service.HandoffResult, error) {
	return a.svc.CaptureClaudeHandoff(projectID)
}

// LatestHandoff returns a minimal status record for the most recent handoff
// (projectID>0 scopes it; 0 is global). It intentionally has no MCP twin: the
// body is available through notes_read, this binding is UI chrome only.
func (a *App) LatestHandoff(projectID int64) (*service.HandoffStatus, error) {
	return a.svc.LatestHandoff(projectID)
}

// RebuildEmbeddings (re)embeds every note into the semantic-search vector index.
// Desktop-only admin action; requires semantic_search enabled + a configured
// embedding endpoint (both default off — ADR-0012).
func (a *App) RebuildEmbeddings() (int, error) {
	return a.svc.RebuildEmbeddings()
}

// RebuildPageEmbeddings (re)embeds every APPROVED wiki page into the page vector
// index (ADR-0018 lane 2, schema v23). Desktop-only admin twin of
// RebuildEmbeddings: same semantic_search + endpoint gate, but it fills the
// separate page_embeddings index so graph-aware evidence can recall compiled
// pages. Kept as its own action because pages and notes embed in different index
// spaces and a user who only wants pages should not re-pay for every note.
func (a *App) RebuildPageEmbeddings() (int, error) {
	return a.svc.RebuildPageEmbeddings()
}

// AskAI forwards the question (with generated project context for projectID>0)
// to the OpenAI-compatible chat endpoint configured in Settings → AI, and
// returns the assistant reply for the Q&A panel.
func (a *App) AskAI(projectID int64, question string) (string, error) {
	return a.svc.AskAI(projectID, question)
}

// AskAIWithEvidence answers with a retrieved, citable context and returns the
// evidence it used, so the panel can render refs and offer 存为页面. AskAI stays
// as the plain path (and as the no-evidence fallback inside this one).
func (a *App) AskAIWithEvidence(projectID int64, question string) (*service.EvidenceAnswer, error) {
	return a.svc.AskAIWithEvidence(projectID, question, service.DefaultEvidenceBudget)
}

// AskStreamResult is the immediate acknowledgement of StartAskStream: the
// stream id the frontend uses to correlate the async events that follow.
type AskStreamResult struct {
	StreamID string `json:"stream_id"`
}

// StartAskStream launches the streaming evidence-backed ask and returns at
// once with a stream id; the answer then arrives progressively as Wails events
// named "ai.ask.stream", each payload tagged with that id:
//
//	{"stream_id": "...", "delta": "…"}                   ← content fragments
//	{"stream_id": "...", "done": true, "reply": "…",
//	 "evidence": {...}, "truncated": bool}               ← terminal success
//	{"stream_id": "...", "error": "…"}                   ← terminal failure
//
// Desktop-only transport: the browser/standalone runtime has no Wails event
// channel and uses the HTTP SSE endpoint (/api/ai/ask-stream) instead, so this
// binding is deliberately absent from the /api/rpc bridge (rpcBlockedMethods).
func (a *App) StartAskStream(projectID int64, question string) (*AskStreamResult, error) {
	streamID := fmt.Sprintf("ask-%d", time.Now().UnixNano())
	go func() {
		ans, err := a.svc.AskAIWithEvidenceStream(projectID, question, service.DefaultEvidenceBudget, func(delta string) {
			if a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "ai.ask.stream", map[string]any{
					"stream_id": streamID,
					"delta":     delta,
				})
			}
		})
		if a.ctx == nil {
			return
		}
		if err != nil {
			wailsruntime.EventsEmit(a.ctx, "ai.ask.stream", map[string]any{
				"stream_id": streamID,
				"error":     err.Error(),
			})
			return
		}
		wailsruntime.EventsEmit(a.ctx, "ai.ask.stream", map[string]any{
			"stream_id": streamID,
			"done":      true,
			"reply":     ans.Reply,
			"evidence":  ans.Evidence,
			"truncated": ans.Truncated,
		})
	}()
	return &AskStreamResult{StreamID: streamID}, nil
}

// FileAnswerAsPage closes ADR-0014's query loop: a good answer becomes a `query`
// wiki page linked to what it cited. evidence must be the SAME set the answer was
// generated from — refs are positional labels from that retrieval, so they can
// only be resolved against it (and an invented ref resolves to nothing).
func (a *App) FileAnswerAsPage(projectID int64, question, answer string, evidence *service.Evidence, citedRefs []string) (int64, error) {
	return a.svc.FileAnswerAsPage(projectID, question, answer, evidence, citedRefs)
}

// RunWikiLint runs ADR-0014 W4's five checks over projectID (0 = every page) and
// files findings as todos on the owning project. wantLLM asks for the
// model-backed contradiction/staleness pair, which additionally needs the
// wiki_lint_llm config key. It never rewrites a page — the whole safety argument
// for leaving lint on is that a finding is a suggestion with a checkbox on it.
func (a *App) RunWikiLint(projectID int64, wantLLM bool) (*service.WikiLintReport, error) {
	return a.svc.RunWikiLint(projectID, wantLLM)
}

// CompileNote runs the ingest-time compile for one note. Everything it writes
// lands as a pending page; nothing becomes searchable until a human approves it.
func (a *App) CompileNote(noteID int64) (*service.WikiCompileReport, error) {
	return a.svc.CompileNote(noteID)
}

// CompileProjectNotes compiles up to maxNotes notes of one project under one
// shared budget (<= 0 uses the default). Still a human action, never a timer.
func (a *App) CompileProjectNotes(projectID int64, maxNotes int) (*service.WikiCompileReport, error) {
	return a.svc.CompileProjectNotes(projectID, maxNotes)
}

// StartCompileJob queues a batch compile and returns the job id immediately: one
// note measured 173s on a local model, so this is the path the UI should use.
// CompileProjectNotes stays for callers that genuinely want to wait.
func (a *App) StartCompileJob(projectID int64, maxNotes int) (int64, error) {
	return a.svc.StartCompileJob(projectID, maxNotes)
}

// GetCompileJob is the poll endpoint for a single job's status and totals.
func (a *App) GetCompileJob(jobID int64) (*db.CompileJob, error) { return a.svc.GetCompileJob(jobID) }

// ListCompileJobs returns recent jobs (newest first) for the review panel.
// kind filters by queue ("compile" / "lint"); "" returns both, which is what the
// panel wants since it renders each row's own counters (see db.CompileJob.Kind).
func (a *App) ListCompileJobs(projectID int64, kind string, limit int) ([]db.CompileJob, error) {
	return a.svc.ListCompileJobs(projectID, kind, limit)
}

// StartLintJob enqueues the model-backed half of a lint pass and returns the job
// id without calling the model. Same shape as StartCompileJob and for the same
// measured reason: the model pass runs under a 10-minute ceiling, so a
// synchronous binding would either hang the UI or be cut off (ADR-0016 待决 ①).
func (a *App) StartLintJob(projectID int64) (int64, error) {
	return a.svc.StartLintJob(projectID)
}

// CancelCompileJob stops a job from taking further notes. An in-flight request is
// allowed to finish (its cost is already paid) — see ADR-0016 决策 3.
func (a *App) CancelCompileJob(jobID int64) (*db.CompileJob, error) {
	return a.svc.CancelCompileJob(jobID)
}

// ListPendingWikiPages is the review queue, each entry carrying its source notes
// and edges so a reviewer can compare before approving.
func (a *App) ListPendingWikiPages(projectID int64) ([]service.WikiPendingPage, error) {
	return a.svc.ListPendingWikiPages(projectID)
}

// ApproveWikiPage publishes a pending page into retrieval. This transition has no
// other caller: the compiler and lint cannot reach it.
func (a *App) ApproveWikiPage(pageID int64) error { return a.svc.ApproveWikiPage(pageID) }

// RejectWikiPage discards a pending page and its edges; it refuses approved pages.
// RejectWikiPage replaces a pending page with an error-page tombstone (its
// inbound links stay valid); it no longer deletes the row.
func (a *App) RejectWikiPage(pageID int64) error { return a.svc.RejectWikiPage(pageID) }

// ListRejectedWikiPages returns the error-page tombstones so the review tab can
// show what was rejected without hiding the nodes other pages still reference.
func (a *App) ListRejectedWikiPages(projectID int64) ([]service.WikiPendingPage, error) {
	return a.svc.ListRejectedWikiPages(projectID)
}

// DeleteCompiledPage permanently removes a never-published page (pending or
// rejected); the escape hatch that keeps rejected tombstones from piling up.
func (a *App) DeleteCompiledPage(pageID int64) error { return a.svc.DeleteCompiledPage(pageID) }

// ---- knowledge graph: merge, communities, page history (ADR-0019/0020/0021) ----

// FindMergeCandidates returns the pairs of pages that look like the same concept
// twice. Structural only — no model, no vector, no writes — so the review page can
// call it on load and show what is waiting before anyone enables anything
// (ADR-0019 决策 2). topK <= 0 uses the default.
func (a *App) FindMergeCandidates(projectID int64, topK int) (*service.MergeCandidateReport, error) {
	return a.svc.FindMergeCandidates(projectID, topK)
}

// MergeWikiPages absorbs loserID into winnerID: the loser's edges and source notes
// move to the winner and the loser becomes a tombstone. The winner's row is not
// touched (ADR-0019 决策 5).
//
// This is the ONLY merge entry point in the product and it exists for a human click
// — no job, no ticker and no MCP tool reaches it, which is the ADR's central claim
// and is pinned by TestMergeHasNoAutoPath. Deliberately no MCP twin: letting an
// agent destroy knowledge is the one thing this layer must never allow.
func (a *App) MergeWikiPages(loserID, winnerID int64) (*db.MergeResult, error) {
	return a.svc.MergeWikiPagesInto(loserID, winnerID)
}

// RunCommunitySummary detects topics among a project's approved pages and files one
// pending synthesis page per topic (ADR-0020). Synchronous by decision (决策 8 /
// 待决 4): it is bounded to ten communities and one call each, and job-ifying it
// would require rebuilding compile_jobs for a CHECK value.
//
// projectID 0 is the GLOBAL tier, not "every project" — so the review page must
// require an explicit project, exactly as StartCompileJob and StartLintJob do.
func (a *App) RunCommunitySummary(projectID int64) (*service.WikiCommunityReport, error) {
	return a.svc.RunCommunitySummary(projectID)
}

// ResolveWorkspacePath answers "which project is this folder, and which
// repository inside it" — the four-tier path resolution shared with the MCP tools.
//
// It exists as a binding so the IDE chrome can DISPLAY what was matched instead of
// reimplementing the matching in TypeScript. Two copies of "which project owns
// this directory" is precisely how an agent and the sidebar end up disagreeing
// about what knowledge a session gets. Read-only, and no MCP twin: an agent does
// not need it, because repotory_context takes `path` and resolves internally.
func (a *App) ResolveWorkspacePath(path string) (*service.ProjectResolution, error) {
	// Same limit as the agent-facing path argument, from the same package: the two
	// surfaces must not disagree about how long a path they will look at.
	if len(path) > bounds.MaxPathLen {
		return nil, fmt.Errorf("%s", bounds.TooLong("path", len(path), bounds.MaxPathLen))
	}
	return a.svc.ResolveProjectByPath(path), nil
}

// ListWikiPageVersions returns a page's approved-content snapshots, newest first
// (ADR-0021). Read-only, and the writer is a database trigger rather than a call
// anyone can forget to make.
func (a *App) ListWikiPageVersions(pageID int64) ([]db.PageVersion, error) {
	return a.svc.ListWikiPageVersions(pageID)
}

// DiffWikiPageVersion compares one snapshot against the page's current content, in
// the same line-diff format as DiffNoteVersions so one component renders both.
func (a *App) DiffWikiPageVersion(pageID, versionID int64) (string, error) {
	return a.svc.DiffWikiPageVersion(pageID, versionID)
}

// LayeredProjectContext assembles project memory from most stable to least
// (L3 cross-project profile, L2 project scenario, L1 curated notes, L0 raw
// transcripts) under per-layer budgets. An empty query means orientation only, so
// no note retrieval happens at all; raw transcripts are included only when a
// caller explicitly asks (ADR-0014 决策 6). Read-only.
func (a *App) LayeredProjectContext(projectID int64, query string, includeTranscripts bool) (*service.LayeredMemory, error) {
	if projectID < 0 {
		return nil, fmt.Errorf("project id must be >= 0 (0 = global profile only)")
	}
	return a.svc.BuildLayeredMemory(projectID, query, service.DefaultLayerBudget, includeTranscripts), nil
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
