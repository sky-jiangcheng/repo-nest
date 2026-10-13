package app

// Binding surface registry (P32, machine-checked).
//
// Why this file exists. The audit of "which binding has an MCP twin, and which is
// deliberately desktop-only" used to live in a hand-maintained comment block dated
// 2026-10-02 at the top of bindings.go. The MCP side already had an automated pin
// (TestToolsRegistered asserts the exact tool list and its count); the binding
// side had prose that must be remembered. Every binding added since then meant
// editing a comment, and forgetting meant the next reader trusted a stale claim —
// which is the failure this repo has now hit repeatedly enough that it keeps
// writing it down.
//
// So: the classification is data here, and surface_test.go asserts it is
// COMPLETE (every exported *App method appears), CURRENT (no entry names a method
// that no longer exists), EXPLAINED (a desktop-only binding must carry a reason),
// and CONSISTENT with the HTTP blocklist. A new binding with no entry fails the
// build instead of silently drifting the documentation.
//
// Reading the categories:
//
//   - SurfaceMCPBacked — an agent can reach the same capability through the named
//     MCP tool. The tool name is recorded, and cmd/mcp's registry test cross-checks
//     that it actually exists.
//   - SurfaceDesktopOnly — no MCP twin, on purpose. The Reason is not a comment:
//     the test requires it to be non-empty, and it is the answer to "why is this
//     not exposed to agents". Most of these are the human gate (compile, approve,
//     merge, community synthesis) or UI chrome whose shape is deliberately lossy.
//   - SurfaceLifecycle — process control, never an agent capability. Kept in sync
//     with httpapi's RPC blocklist by test assertion rather than by hope.

// SurfaceCategory classifies how a binding is reachable outside the desktop UI.
type SurfaceCategory string

const (
	// SurfaceMCPBacked has an agent-facing MCP twin (see the tool name).
	SurfaceMCPBacked SurfaceCategory = "mcp-backed"
	// SurfaceDesktopOnly is intentionally unreachable by agents.
	SurfaceDesktopOnly SurfaceCategory = "desktop-only"
	// SurfaceLifecycle controls the process; blocked on the HTTP bridge.
	SurfaceLifecycle SurfaceCategory = "lifecycle"
)

// BindingSurface is one registry entry.
type BindingSurface struct {
	Category SurfaceCategory
	// MCPTool is the agent-facing twin. Required for SurfaceMCPBacked, forbidden
	// otherwise (an entry claiming desktop-only while naming a tool is exactly the
	// stale-audit shape this file replaces).
	MCPTool string
	// Reason is required for SurfaceDesktopOnly and SurfaceLifecycle.
	Reason string
}

// bindingSurfaces must cover every exported method of *App. Enforced by
// TestBindingSurfaceRegistryIsComplete.
var bindingSurfaces = map[string]BindingSurface{
	// ---- lifecycle -----------------------------------------------------------
	"Startup":  {SurfaceLifecycle, "", "process start; a remote call would boot subsystems the caller never asked for"},
	"Shutdown": {SurfaceLifecycle, "", "closes the database — one POST must not be able to do that"},
	"Service":  {SurfaceLifecycle, "", "returns the *Service itself; not a serialisable capability"},
	"Health":   {SurfaceMCPBacked, "repotory_integrity", "liveness probe; /health is its REST twin"},

	// ---- projects ------------------------------------------------------------
	"GetProjects":           {SurfaceMCPBacked, "repotory_projects_list", ""},
	"GetProjectStats":       {SurfaceMCPBacked, "repotory_projects_stats", ""},
	"GetProjectDetail":      {SurfaceDesktopOnly, "", "GUI drill-down screen; agents get the same facts from repotory_context in one call"},
	"GetProjectOverview":    {SurfaceMCPBacked, "repotory_context", "mined project knowledge; context is the agent-facing render of it"},
	"GetRepoCommits":        {SurfaceDesktopOnly, "", "commit table for the detail page"},
	"GetProjectCommits":     {SurfaceDesktopOnly, "", "commit table for the detail page"},
	"UpdateProjectLevel":    {SurfaceDesktopOnly, "", "project grouping is a human organising act"},
	"ToggleStar":            {SurfaceDesktopOnly, "", "collection is UI state"},
	"RefreshProjectHistory": {SurfaceDesktopOnly, "", "long-running git refresh driven from the detail page"},
	"SearchProjects":        {SurfaceDesktopOnly, "", "project picker in the UI"},

	// ---- scan ----------------------------------------------------------------
	"TriggerScan":   {SurfaceMCPBacked, "repotory_scan", ""},
	"GetScanStatus": {SurfaceDesktopOnly, "", "progress polling for the scan UI; the agent-facing scan tool waits and reports"},

	// ---- dashboard -----------------------------------------------------------
	"GetSummary":     {SurfaceDesktopOnly, "", "today/week counters for the dashboard header; agents get their numbers from repotory_context and repotory_integrity"},
	"GetHeatmapData": {SurfaceDesktopOnly, "", "heatmap grid for the dashboard; activity is already in repotory_context's Activity section"},
	"GetStatusBar":   {SurfaceDesktopOnly, "", "the dashboard status strip; it bundles counters no agent needs as a single call"},
	"GetTodoCounts":  {SurfaceDesktopOnly, "", "count badges on the dashboard tiles; open todos are already inside repotory_context"},
	"GetNoteCounts":  {SurfaceDesktopOnly, "", "per-level note counters for the dashboard tiles"},

	// ---- search --------------------------------------------------------------
	"SearchNotes": {SurfaceMCPBacked, "repotory_notes_search", ""},
	"SearchAll":   {SurfaceMCPBacked, "repotory_notes_search", ""},

	// ---- notes ---------------------------------------------------------------
	"ListNotes":          {SurfaceMCPBacked, "repotory_notes_list", ""},
	"ListAllNotes":       {SurfaceMCPBacked, "repotory_notes_list", ""},
	"ListAllTags":        {SurfaceDesktopOnly, "", "tag filter chips in the knowledge page"},
	"CreateNote":         {SurfaceMCPBacked, "repotory_notes_create", ""},
	"CreateNoteWithMeta": {SurfaceMCPBacked, "repotory_notes_create", ""},
	"UpdateNote":         {SurfaceMCPBacked, "repotory_notes_update", ""},
	"UpdateNoteFull":     {SurfaceMCPBacked, "repotory_notes_update", ""},
	"UpdateNoteMeta":     {SurfaceMCPBacked, "repotory_notes_update", ""},
	"DeleteNote":         {SurfaceDesktopOnly, "", "deletion is a human action; no agent path deletes knowledge"},
	"PinNote":            {SurfaceDesktopOnly, "", "pinning changes the UI sort order only; it alters no knowledge content"},
	"MoveNote":           {SurfaceDesktopOnly, "", "moving a note between projects re-files knowledge; a human decides where it belongs"},
	"ListNoteVersions":   {SurfaceDesktopOnly, "", "version-history panel; agents read the live note"},
	"RestoreNoteVersion": {SurfaceDesktopOnly, "", "restoring overwrites current content — human only"},
	"DiffNoteVersions":   {SurfaceDesktopOnly, "", "version diff for the note history panel; agents read the live note via notes_read"},

	// ---- todos ---------------------------------------------------------------
	"ListTodos":    {SurfaceDesktopOnly, "", "todo panel; repotory_context already surfaces open todos"},
	"CreateTodo":   {SurfaceDesktopOnly, "", "todo creation is a human planning act in this UI"},
	"ToggleTodo":   {SurfaceDesktopOnly, "", "checking off is the human decision lint exists to support"},
	"DeleteTodo":   {SurfaceDesktopOnly, "", "deletion is a human action"},
	"ReorderTodos": {SurfaceDesktopOnly, "", "manual todo ordering is a personal prioritisation act, not a data operation"},

	// ---- config --------------------------------------------------------------
	"GetConfig":       {SurfaceDesktopOnly, "", "settings page; secrets are masked for it and it is still not an agent surface"},
	"UpdateConfig":    {SurfaceDesktopOnly, "", "changing privacy gates must never be something an agent can do to itself"},
	"UpdateScanRoots": {SurfaceDesktopOnly, "", "granting new filesystem scope is a human decision"},

	// ---- exports -------------------------------------------------------------
	"GenerateLLMsTxt":      {SurfaceDesktopOnly, "", "the /api/ai_context REST twin serves agents and the dsh plugin; the binding feeds the UI preview"},
	"ExportNoteAsMarkdown": {SurfaceDesktopOnly, "", "writes to a path chosen in a native save dialog; an agent should not pick where user files land"},
	"ExportMemoryJSON":     {SurfaceDesktopOnly, "", "file save; OMP export is provisional (ADR-0011)"},

	// ---- knowledge sources / memory -----------------------------------------
	"GetKnowledgeSources":        {SurfaceDesktopOnly, "", "plugins settings tab"},
	"TriggerKnowledgeImport":     {SurfaceDesktopOnly, "", "importing another tool's memory is an explicit human opt-in (ADR-0011 privacy gate)"},
	"TriggerAllKnowledgeImports": {SurfaceDesktopOnly, "", "batch import of other tools' memory files; same per-click consent as the single-source triggers"},
	"ImportClaudeMemory":         {SurfaceDesktopOnly, "", "reads another tool's memory files off disk; the explicit per-click consent is the point (ADR-0011 decision 4)"},
	"CaptureClaudeHandoff":       {SurfaceDesktopOnly, "", "reads another tool's session transcript; consent is per-click in settings"},
	"ListRecentHandoffs":         {SurfaceDesktopOnly, "", "memory-ring chrome: content-free entries, reading a handoff stays behind notes_read"},
	"LatestHandoff":              {SurfaceDesktopOnly, "", "IDE status chrome: timestamp/title only by design, the body belongs to the knowledge reader"},
	"RebuildEmbeddings":          {SurfaceDesktopOnly, "", "bulk sends note text to a configured endpoint; the gate lives in settings, not in an agent call"},

	// ---- AI ------------------------------------------------------------------
	"AskAI":                 {SurfaceDesktopOnly, "", "the agent-facing twin is repotory_ask, which has no binding (served straight off the service)"},
	"AskAIWithEvidence":     {SurfaceDesktopOnly, "", "AI ask panel; agents use repotory_ask"},
	"StartAskStream":        {SurfaceDesktopOnly, "", "replies over Wails events, so an HTTP/MCP caller would get a stream id nobody can read; the browser path is the SSE endpoint"},
	"FileAnswerAsPage":      {SurfaceDesktopOnly, "", "filing an answer into the knowledge layer is a human decision (ADR-0014 decision 3)"},
	"RunWikiLint":           {SurfaceDesktopOnly, "", "lint writes todos; scheduled and manual paths are the app's own"},
	"LayeredProjectContext": {SurfaceDesktopOnly, "", "agent-side assembly is repotory_context; this binding is the UI/CLI preview"},
	"ListAIModels":          {SurfaceDesktopOnly, "", "model picker in the settings tab; it enumerates a configured endpoint and is not a knowledge capability"},
	"TestAIChat":            {SurfaceDesktopOnly, "", "settings 'test connection' button"},

	// ---- wiki compile & review (the human gate) ------------------------------
	"CompileNote":           {SurfaceDesktopOnly, "", "an agent able to write pending knowledge would bypass the human gate the whole design rests on (ADR-0015)"},
	"CompileProjectNotes":   {SurfaceDesktopOnly, "", "same gate; batch form"},
	"StartCompileJob":       {SurfaceDesktopOnly, "", "same gate, queued form"},
	"GetCompileJob":         {SurfaceDesktopOnly, "", "job progress for the review page"},
	"ListCompileJobs":       {SurfaceDesktopOnly, "", "job list for the review page"},
	"StartLintJob":          {SurfaceDesktopOnly, "", "same gate as RunWikiLint"},
	"CancelCompileJob":      {SurfaceDesktopOnly, "", "UI control of a user-started job"},
	"ListPendingWikiPages":  {SurfaceDesktopOnly, "", "the review queue IS the human gate's UI"},
	"ApproveWikiPage":       {SurfaceDesktopOnly, "", "publishing into retrieval is a human act and has no other caller"},
	"RejectWikiPage":        {SurfaceDesktopOnly, "", "reject writes an error-page tombstone rather than deleting; the review decision is human-only (ADR-0015 decision 3)"},
	"ListRejectedWikiPages": {SurfaceDesktopOnly, "", "the error-page list in the review tab, so tombstones stay visible to the person who can clean them up"},
	"DeleteCompiledPage":    {SurfaceDesktopOnly, "", "permanent removal of never-published pages: human only"},

	// ---- knowledge graph (ADR-0019 / 0020 / 0021) ----------------------------
	"FindMergeCandidates":  {SurfaceDesktopOnly, "", "read-only, but it is the surface a person looks at in order to decide — exposing it invites an agent to merge without the human"},
	"MergeWikiPages":       {SurfaceDesktopOnly, "", "a merge destroys information and may only follow a human decision (ADR-0019 decision 1, pinned by TestMergeHasNoAutoPath)"},
	"RunCommunitySummary":  {SurfaceDesktopOnly, "", "writes model output into the library; reachable by an agent it would defeat the review queue (ADR-0020 decision 9)"},
	"ResolveWorkspacePath": {SurfaceDesktopOnly, "", "agents pass `path` to repotory_context instead; a second client-side matcher is the disagreement this avoids"},
	"ListWikiPageVersions": {SurfaceDesktopOnly, "", "the page history panel (ADR-0021); snapshots exist so a HUMAN can notice stale approved content, not so an agent can act on it"},
	"DiffWikiPageVersion":  {SurfaceDesktopOnly, "", "snapshot-vs-current diff on the review page; it supports a human approve/reject decision"},
}

// mcpBackedBindings returns the bindings that claim an MCP twin, for the
// cross-check in cmd/mcp (which can import this package without a cycle).
func mcpBackedBindings() map[string]string {
	out := map[string]string{}
	for name, s := range bindingSurfaces {
		if s.Category == SurfaceMCPBacked && s.MCPTool != "" {
			out[name] = s.MCPTool
		}
	}
	return out
}
