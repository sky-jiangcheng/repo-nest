// api/endpoints.ts — Every backend call, one endpoint per function, routed
// through the shared transport. Names are the stable public surface imported
// across the app.

import { call } from './transport'
import type {
  AppConfig,
  CompileJob,
  DailyStat,
  Evidence,
  EvidenceAnswer,
  HeatmapResponse,
  ImportResult,
  ImportRun,
  HandoffResult,
  Note,
  NoteCount,
  NoteVersion,
  NoteWithProject,
  Project,
  ProjectDetail,
  ProjectOverview,
  RepoCommit,
  ScanStatus,
  SearchHit,
  SourceStatus,
  StatusBarData,
  Summary,
  Todo,
  TodoCount,
  WikiPendingPage,
} from './types'

const JSON_HEADERS = { 'Content-Type': 'application/json' }
const jsonInit = (method: string, body: unknown): RequestInit => ({
  method,
  headers: JSON_HEADERS,
  body: JSON.stringify(body),
})

// --- Projects -----------------------------------------------------------------

export function getProjects(date?: string, starredOnly = false): Promise<Project[]> {
  const params = new URLSearchParams()
  if (date) params.set('date', date)
  if (starredOnly) params.set('starred', '1')
  return call<Project[]>({
    method: 'GetProjects',
    args: [date ?? '', starredOnly],
    path: `/projects?${params.toString()}`,
  }).then(d => d ?? [])
}

export function getProjectDetail(id: number): Promise<ProjectDetail> {
  return call<ProjectDetail>({ method: 'GetProjectDetail', args: [id], path: `/projects/${id}` })
}

/**
 * One repository's commit log, newest first. Read from git on demand: the
 * stored daily_stats rows carry no message or SHA, so a commit list cannot be
 * assembled from the database.
 */
export function getRepoCommits(repoId: number, limit = 50): Promise<RepoCommit[]> {
  return call<RepoCommit[]>({ method: 'GetRepoCommits', args: [repoId, limit], path: `/repos/${repoId}/commits` })
}

/**
 * A project's merged commit log across all its repositories, newest first.
 * Same on-demand git read as getRepoCommits, all authors, so the merged and
 * per-repo views agree.
 */
export function getProjectCommits(projectId: number, limit = 50): Promise<RepoCommit[]> {
  return call<RepoCommit[]>({ method: 'GetProjectCommits', args: [projectId, limit], path: `/projects/${projectId}/commits` })
}

export function getProjectStats(id: number, date?: string): Promise<DailyStat[]> {
  const params = date ? `?date=${date}` : ''
  return call<DailyStat[]>({
    method: 'GetProjectStats',
    args: [id, date ?? ''],
    path: `/projects/${id}/stats${params}`,
  }).then(d => d ?? [])
}

export function toggleStar(id: number): Promise<boolean> {
  // The Wails binding returns a bare bool (not {starred}), so unwrap it
  // directly. `!!r` normalizes undefined from a failed call to false.
  return call<boolean>({
    method: 'ToggleStar',
    args: [id],
    path: `/projects/${id}/star`,
    init: { method: 'POST' },
  }).then(r => !!r)
}

export function refreshProjectHistory(id: number): Promise<{ success: boolean }> {
  return call<{ success: boolean }>({
    method: 'RefreshProjectHistory',
    args: [id],
    path: `/projects/${id}/refresh-history`,
    init: { method: 'POST' },
  })
}

export function updateProjectLevel(
  id: number,
  direction: 'up' | 'down'
): Promise<{ success: boolean; new_level: number }> {
  return call({
    method: 'UpdateProjectLevel',
    args: [id, direction],
    path: `/projects/${id}/level`,
    init: jsonInit('POST', { direction }),
  })
}

export function searchProjects(query: string): Promise<Project[]> {
  return call<Project[]>({
    method: 'SearchProjects',
    args: [query],
    path: `/projects/search?q=${encodeURIComponent(query)}`,
  }).then(d => d ?? [])
}

export function getProjectOverview(projectId: number): Promise<ProjectOverview> {
  return call<ProjectOverview>({
    method: 'GetProjectOverview',
    args: [projectId],
    path: `/projects/${projectId}/overview`,
  }).then(d => d ?? {
    readme_excerpt: '', tech_stack: [], languages: [], recent_commits: [], cached: false,
    dependencies: [], top_contributors: [], activity: null,
  })
}

// --- Scan -----------------------------------------------------------------------

export function triggerScan(): Promise<{ success: boolean }> {
  return call({ method: 'TriggerScan', path: '/scan', init: { method: 'POST' } })
}

export function getScanStatus(): Promise<ScanStatus> {
  const empty: ScanStatus = { running: false, backfilling: false, message: '', progress: 0, total: 0 }
  return call<ScanStatus>({ method: 'GetScanStatus', path: '/scan/status' }).then(d => d ?? empty)
}

// --- Config ---------------------------------------------------------------------

export function getConfig(): Promise<AppConfig> {
  return call<AppConfig>({ method: 'GetConfig', path: '/config' })
}

export function updateConfig(key: string, value: string): Promise<{ success: boolean }> {
  return call<{ success: boolean }>({
    method: 'UpdateConfig',
    args: [key, value],
    path: '/config',
    init: jsonInit('PUT', { key, value }),
  })
}

/**
 * Why one submitted scan root was refused. `reason` is a stable code the UI maps
 * to localised text; `detail` is the backend's English fallback.
 */
export interface ScanRootRejection {
  path: string
  reason: 'empty' | 'relative' | 'not_found' | 'not_a_directory' | 'duplicate' | 'inaccessible' | string
  detail: string
}

/** What the backend actually stored, plus anything it refused and why. */
export interface ScanRootsResult {
  scan_roots: string[]
  rejected: ScanRootRejection[]
}

export function updateScanRoots(scan_roots: string[]): Promise<ScanRootsResult> {
  return call<ScanRootsResult>({
    method: 'UpdateScanRoots',
    args: [scan_roots],
    path: '/config',
    init: jsonInit('PUT', { scan_roots }),
  })
}

// --- Summary / heatmap / status bar -----------------------------------------------

export function getSummary(date?: string): Promise<Summary> {
  const empty: Summary = {
    date: '', repo_count: 0, total_files: 0, total_added: 0,
    total_deleted: 0, my_added: 0, my_deleted: 0, my_files: 0, is_workday: false,
  }
  const params = date ? `?date=${date}` : ''
  return call<Summary>({
    method: 'GetSummary',
    args: [date ?? ''],
    path: `/summary${params}`,
  }).then(d => d ?? empty)
}

export function getHeatmapData(projectId = 0): Promise<HeatmapResponse> {
  return call<HeatmapResponse>({
    method: 'GetHeatmapData',
    args: [projectId],
    path: `/heatmap?project_id=${projectId}`,
  }).then(d => d ?? { days: [] })
}

export function getStatusBar(): Promise<StatusBarData> {
  const empty: StatusBarData = {
    current_time: '', last_commit_time: '', last_commit_repo: '',
    last_commit_branch: '', last_commit_msg: '',
  }
  return call<StatusBarData>({ method: 'GetStatusBar', path: '/status-bar' }).then(d => d ?? empty)
}

export function getTodoCounts(): Promise<TodoCount[]> {
  return call<TodoCount[]>({ method: 'GetTodoCounts', path: '/todo-counts' }).then(d => d ?? [])
}

export function getNoteCounts(): Promise<NoteCount[]> {
  return call<NoteCount[]>({ method: 'GetNoteCounts', path: '/note-counts' }).then(d => d ?? [])
}

// --- Search ---------------------------------------------------------------------

export function searchNotes(query: string): Promise<SearchHit[]> {
  return call<SearchHit[]>({
    method: 'SearchNotes',
    args: [query],
    path: `/notes/search?q=${encodeURIComponent(query)}`,
  }).then(d => d ?? [])
}

export function searchAll(query: string): Promise<SearchHit[]> {
  return call<SearchHit[]>({
    method: 'SearchAll',
    args: [query],
    path: `/search?q=${encodeURIComponent(query)}`,
  }).then(d => d ?? [])
}

// --- Todos ------------------------------------------------------------------------

export function listTodos(projectId: number): Promise<Todo[]> {
  return call<Todo[]>({
    method: 'ListTodos',
    args: [projectId],
    path: `/todos?project_id=${projectId}`,
  }).then(d => d ?? [])
}

export interface AIModelListResult {
  models: string[]
  resolved_base_url: string
}

export interface AITestResult {
  reply: string
  resolved_base_url: string
}

export function listAIModels(baseURL: string, apiKey: string): Promise<AIModelListResult> {
  return call<AIModelListResult>({
    method: 'ListAIModels',
    args: [baseURL, apiKey],
  }).then(d => d ?? { models: [], resolved_base_url: '' })
}

export function testAIChat(baseURL: string, model: string, apiKey: string): Promise<AITestResult> {
  return call<AITestResult>({
    method: 'TestAIChat',
    args: [baseURL, model, apiKey],
  }).then(d => d ?? { reply: '', resolved_base_url: '' })
}

export function askAI(projectId: number, question: string): Promise<string> {
  return call<string>({
    method: 'AskAI',
    args: [projectId, question],
  })
}

// askAIWithEvidence answers through the W2 retrieval path and returns the
// evidence alongside the reply, so the panel can show what was cited and file the
// answer back with those same refs. AskAI stays as the plain call.
export function askAIWithEvidence(projectId: number, question: string): Promise<EvidenceAnswer> {
  return call<EvidenceAnswer>({
    method: 'AskAIWithEvidence',
    args: [projectId, question],
  }).then(d => d ?? { reply: '', evidence: { items: [], dropped: 0, elapsed_ms: 0, truncated: false } })
}

// fileAnswerAsPage closes the query loop: the answer becomes a `query` wiki page
// linked to the pages it cited. evidence must be the exact set the reply was
// generated from — refs are positional labels of that one retrieval, and the
// backend resolves them only against it (an invented ref links to nothing).
export function fileAnswerAsPage(
  projectId: number,
  question: string,
  answer: string,
  evidence: Evidence,
  citedRefs: string[],
): Promise<number> {
  return call<number>({
    method: 'FileAnswerAsPage',
    args: [projectId, question, answer, evidence, citedRefs],
  })
}

export function createTodo(projectId: number, title: string): Promise<Todo> {
  return call<Todo>({
    method: 'CreateTodo',
    args: [projectId, title],
    path: '/todos',
    init: jsonInit('POST', { project_id: projectId, title }),
  })
}

export function toggleTodo(todoId: number): Promise<void> {
  return call<void>({ method: 'ToggleTodo', args: [todoId], path: `/todos/${todoId}/toggle`, init: { method: 'POST' } })
}

export function deleteTodo(todoId: number): Promise<void> {
  return call<void>({ method: 'DeleteTodo', args: [todoId], path: `/todos/${todoId}`, init: { method: 'DELETE' } })
}

export function reorderTodos(todoIds: number[]): Promise<void> {
  return call<void>({
    method: 'ReorderTodos',
    args: [todoIds],
    path: '/todos/reorder',
    init: jsonInit('POST', { todo_ids: todoIds }),
  })
}

// --- Notes ------------------------------------------------------------------------

export function listNotes(projectId: number): Promise<Note[]> {
  return call<Note[]>({
    method: 'ListNotes',
    args: [projectId],
    path: `/notes?project_id=${projectId}`,
  }).then(d => d ?? [])
}

export function createNote(projectId: number, content: string): Promise<Note> {
  return call<Note>({
    method: 'CreateNote',
    args: [projectId, content],
    path: '/notes',
    init: jsonInit('POST', { project_id: projectId, content }),
  })
}

export interface NoteCreateInput {
  title?: string
  tags?: string
  kind?: string
  source?: string
}

export function createNoteWithMeta(
  projectId: number,
  content: string,
  meta: NoteCreateInput = {}
): Promise<Note> {
  const title = meta.title ?? ''
  const tags = meta.tags ?? ''
  const kind = meta.kind ?? ''
  const source = meta.source ?? ''
  return call<Note>({
    method: 'CreateNoteWithMeta',
    args: [projectId, title, content, tags, kind, source],
    path: '/notes',
    init: jsonInit('POST', { project_id: projectId, content, title, tags, kind, source }),
  })
}

export function updateNote(noteId: number, content: string): Promise<void> {
  return call<void>({
    method: 'UpdateNote',
    args: [noteId, content],
    path: `/notes/${noteId}`,
    init: jsonInit('PUT', { content }),
  })
}

export function updateNoteFull(
  noteId: number,
  content: string,
  title: string,
  tags: string,
  kind: string,
  pinned: boolean
): Promise<void> {
  return call<void>({
    method: 'UpdateNoteFull',
    args: [noteId, content, title, tags, kind, pinned],
    path: `/notes/${noteId}`,
    init: jsonInit('PUT', { content, title, tags, kind, pinned }),
  })
}

export function updateNoteMeta(
  noteId: number,
  title: string,
  tags: string,
  kind: string,
  pinned: boolean
): Promise<void> {
  return call<void>({
    method: 'UpdateNoteMeta',
    args: [noteId, title, tags, kind, pinned],
    path: `/notes/${noteId}/meta`,
    init: jsonInit('PUT', { title, tags, kind, pinned }),
  })
}

export function pinNote(noteId: number, pinned: boolean): Promise<void> {
  return call<void>({
    method: 'PinNote',
    args: [noteId, pinned],
    path: `/notes/${noteId}/pin`,
    init: jsonInit('POST', { pinned }),
  })
}

export function deleteNote(noteId: number): Promise<void> {
  return call<void>({ method: 'DeleteNote', args: [noteId], path: `/notes/${noteId}`, init: { method: 'DELETE' } })
}

export function moveNote(noteId: number, projectId: number): Promise<void> {
  return call<void>({
    method: 'MoveNote',
    args: [noteId, projectId],
    path: `/notes/${noteId}/move`,
    init: jsonInit('POST', { project_id: projectId }),
  })
}

// --- Note version history -----------------------------------------------------------

export function listNoteVersions(noteId: number): Promise<NoteVersion[]> {
  return call<NoteVersion[]>({
    method: 'ListNoteVersions',
    args: [noteId],
    path: `/notes/${noteId}/versions`,
  }).then(d => d ?? [])
}

export function restoreNoteVersion(noteId: number, versionId: number): Promise<void> {
  return call<void>({
    method: 'RestoreNoteVersion',
    args: [noteId, versionId],
    path: `/notes/${noteId}/versions/${versionId}/restore`,
    init: { method: 'POST' },
  })
}

export function diffNoteVersions(noteId: number, versionId: number): Promise<string> {
  return call<string>({
    method: 'DiffNoteVersions',
    args: [noteId, versionId],
    path: `/notes/${noteId}/versions/${versionId}/diff`,
  }).then(d => d ?? '')
}

// --- Knowledge hub -------------------------------------------------------------------

export function listAllNotes(): Promise<NoteWithProject[]> {
  return call<NoteWithProject[]>({ method: 'ListAllNotes', path: '/notes/all' }).then(d => d ?? [])
}

export function listAllTags(): Promise<string[]> {
  return call<string[]>({ method: 'ListAllTags', path: '/notes/tags' }).then(d => d ?? [])
}

export function importClaudeMemory(): Promise<ImportResult> {
  return call<ImportResult>({
    method: 'ImportClaudeMemory',
    path: '/knowledge/import',
    init: { method: 'POST' },
  }).then(d => d ?? { synced: 0, updated: 0, skipped: 0 })
}

// captureClaudeHandoff runs an on-demand M1 capture for one project: it reads
// that project's latest Claude Code session transcript and persists a handoff
// note. Backend-gated by the `claude_session_capture` config (default off), so
// it rejects unless the user has explicitly enabled capture.
export function captureClaudeHandoff(projectId: number): Promise<HandoffResult> {
  return call<HandoffResult>({
    method: 'CaptureClaudeHandoff',
    args: [projectId],
    path: `/project/${projectId}/capture-claude-handoff`,
    init: { method: 'POST' },
  })
}

// --- AI-facing exports ------------------------------------------------------------------

export function generateLLMsTxt(): Promise<string> {
  return call<string>({ method: 'GenerateLLMsTxt', path: '/llms' })
}

export function exportNoteAsMarkdown(noteId: number): Promise<string> {
  return call<string>({
    method: 'ExportNoteAsMarkdown',
    args: [noteId],
    path: `/notes/${noteId}/markdown`,
  }).then(d => d ?? '')
}

// exportMemoryJSON returns the knowledge base as an OMP-style portable memory
// JSON string (ADR-0013 interop seam). limit<=0 = all notes.
export function exportMemoryJSON(limit: number): Promise<string> {
  return call<string>({
    method: 'ExportMemoryJSON',
    args: [limit],
    path: `/memory/export.json?limit=${limit}`,
  }).then(d => d ?? '[]')
}

// --- Plugins -------------------------------------------------------------------------------

export function getKnowledgeSources(): Promise<SourceStatus[]> {
  return call<SourceStatus[]>({ method: 'GetKnowledgeSources', path: '/plugins/sources' }).then(d => d ?? [])
}

export function triggerKnowledgeImport(name: string): Promise<ImportRun> {
  const empty: ImportRun = { created: 0, updated: 0, skipped: 0 }
  return call<ImportRun>({
    method: 'TriggerKnowledgeImport',
    args: [name],
    path: '/plugins/import',
    init: jsonInit('POST', { source: name }),
  }).then(d => d ?? empty)
}

// --- Wiki review & compile jobs ------------------------------------------------------------

// listPendingWikiPages feeds the review tab: unreviewed local-model output, each
// with its source notes so a reviewer can compare before approving.
export function listPendingWikiPages(projectId: number): Promise<WikiPendingPage[]> {
  return call<WikiPendingPage[]>({ method: 'ListPendingWikiPages', args: [projectId] }).then(d => d ?? [])
}

// listRejectedWikiPages returns the error-page tombstones (a rejected result is
// replaced by an error page, not deleted, so inbound links keep resolving).
export function listRejectedWikiPages(projectId: number): Promise<WikiPendingPage[]> {
  return call<WikiPendingPage[]>({ method: 'ListRejectedWikiPages', args: [projectId] }).then(d => d ?? [])
}

export function approveWikiPage(pageId: number): Promise<void> {
  return call<void>({ method: 'ApproveWikiPage', args: [pageId] })
}

export function rejectWikiPage(pageId: number): Promise<void> {
  return call<void>({ method: 'RejectWikiPage', args: [pageId] })
}

export function deleteCompiledPage(pageId: number): Promise<void> {
  return call<void>({ method: 'DeleteCompiledPage', args: [pageId] })
}

// startCompileJob queues a batch compile and returns the job id immediately; the
// caller polls. Submission is not supposed to block on the model.
export function startCompileJob(projectId: number, maxNotes: number): Promise<number> {
  return call<number>({ method: 'StartCompileJob', args: [projectId, maxNotes] }).then(d => d ?? 0)
}

export function getCompileJob(jobId: number): Promise<CompileJob | null> {
  return call<CompileJob | null>({ method: 'GetCompileJob', args: [jobId] })
}

// kind: '' lists both queues, 'compile' / 'lint' filter to one. The review panel
// asks for '' and renders each row against its own counters.
export function listCompileJobs(projectId: number, kind: string, limit: number): Promise<CompileJob[]> {
  return call<CompileJob[]>({ method: 'ListCompileJobs', args: [projectId, kind, limit] }).then(d => d ?? [])
}

export function startLintJob(projectId: number): Promise<number> {
  return call<number>({ method: 'StartLintJob', args: [projectId] })
}

export function cancelCompileJob(jobId: number): Promise<CompileJob | null> {
  return call<CompileJob | null>({ method: 'CancelCompileJob', args: [jobId] })
}

