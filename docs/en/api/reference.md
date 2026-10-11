---
title: API Reference
order: 22
---

# API Reference

RepoNest's external interface is the **Wails binding surface**: Go methods are exposed to the frontend via Wails Bind (`window.go.main.App.<MethodName>`), and the method names plus JSON payloads are the contract. The desktop app does not listen on an HTTP port by default; when an HTTP shape is needed, `cmd/server` provides a loopback-only JSON API (reusing the same service layer).

> The binding layer is a thin delegation (`internal/app`); all implementations live in `internal/service`, shared by the CLI and MCP.

## Contract changes in 1.7.0

- `GetHeatmapData(projectId int64)`: new parameter — `0` means global, `>0` limits to that project's repos (the project detail page previously used global data by mistake)
- Removed dead methods that were never called by the frontend: `ExportProjectStats`, `ExportHeatmapCSV`, `GetNoteVersion`, `ScanForRepositories`, `RefreshStats`, `RefreshAllStats`, `RefreshProjectStats`

---

## Headless HTTP server (`reponest-server`)

A JSON API for external runtimes such as the DeepSeek Harness dsh-plugin. It shares the same `internal/service` implementation and the same SQLite database as the desktop app, CLI, and MCP.

Build and run: `go build -o reponest-server ./cmd/server && ./reponest-server --port 18765` (or `REPONEST_HTTP_PORT`). The root `reponest` binary is a Wails app with **no subcommand dispatch at all** — there is no `reponest server` command; don't type it just because older docs did.

| Endpoint | Method | Description |
|------|------|------|
| `/health` | GET | service and database health |
| `/api/ai_context` | GET/POST | the entire knowledge base as Markdown (llms.txt style) |
| `/api/search?q=...&all=1` | GET | FTS5 full-text search (notes only by default; `all=1` includes todos) |
| `/api/project/{id}/detail` | GET | project + repos with historical stats |
| `/api/project/{id}/overview` | GET | knowledge mining results (README / tech stack / dependencies, etc.) |
| `/api/project/{id}/stats?date=` | GET | project stats for a given day |

> **Trust boundary**: this server has **no authentication**, and everything it returns is the user's complete local knowledge base. Security relies entirely on `cmd/server` binding to `127.0.0.1` only — reachability is equivalent to "another process on this machine". **Never** change this to `0.0.0.0` or expose it to the network through a reverse proxy; for remote access, add an authentication scheme and submit an ADR first.

---

## `/api/rpc` (JSON-RPC bridge)

`/api/rpc` is a **full write surface**: it exposes the same `App` object the desktop UI binds (every exported method of `internal/app`) over JSON-RPC, and the web frontend's transport uses it in browser/standalone mode. Its capability equals the desktop UI — beyond the read-only endpoints listed above, `CreateNote`, `DeleteNote`, `UpdateConfig`, `UpdateScanRoots`, `TriggerScan`, `ReloadPlugins` and every other method are callable through it. Any security assessment must count this endpoint in the exposure.

- **Request**: `POST /api/rpc` with `{"method": "<App method name>", "args": [positional JSON args]}`; `args` may be omitted (missing parameters become zero values; a `context.Context` parameter receives the request context)
- **Response**: `{"result": <first return value>}`; a method returning `(T, error)` with a non-nil error yields `422` + `{"error": "..."}`
- **Status codes**: `400` bad args / `403` blocked lifecycle method / `404` unknown method / `405` not POST / `413` body over 1 MiB / `501` unsupported signature (variadic or non-`(T, error)` pair) / `503` binding unavailable
- **Lifecycle blocklist**: `Startup`, `Shutdown`, `Service` are Wails runtime internals and return `403` (previously a single `{"method":"Shutdown"}` could silently close the server's database handle)
- **Body limit**: 1 MiB (legitimate payloads — a note or handoff record — sit far below it)

---

## Projects

| Method | Signature | Description |
|------|------|------|
| `GetProjects` | `(date string, starredOnly bool) → ProjectResponse[]` | project list (yesterday by default; triggers an on-demand single-day refresh when neither today nor yesterday has data) |
| `GetProjectDetail` | `(id) → ProjectDetail` | project + repo list, each with historical stats |
| `GetProjectStats` | `(id, date) → DailyStat[]` | project stats for a given day (yesterday by default) |
| `GetRepoCommits` / `GetProjectCommits` | `(repoID / projectID, limit) → RecentCommit[]` | latest commits for a repo / project |
| `GetProjectOverview` | `(id) → ProjectOverview` | knowledge mining: README / tech stack / languages / dependencies / contributors / activity / recent commits (cached in repo_meta; mined asynchronously on cache miss) |
| `SearchProjects` | `(query) → ProjectResponse[]` | fuzzy search by name/path (enriched with yesterday's stats) |
| `ToggleStar` | `(id) → bool` | toggle star, returns the new state (atomic UPDATE) |
| `UpdateProjectLevel` | `(id, "up"\|"down") → {success, new_level}` | merge/split projects (single transaction) |
| `RefreshProjectHistory` | `(id) → {success}` | backfill the last 365 days of stats for this project |

**ProjectResponse** (excerpt):

```json
{
  "id": 1, "name": "my-project", "root_path": "/Users/me/code/my-project",
  "is_starred": true, "repo_count": 2,
  "total_added": 1200, "total_deleted": 300,
  "my_added": 800, "my_deleted": 200, "my_files": 15,
  "is_workday": true, "below_standard": false
}
```

## Scanning

| Method | Signature | Description |
|------|------|------|
| `TriggerScan` | `() → {success, task_id}` | full async scan, returns immediately |
| `GetScanStatus` | `() → ScanStatus` | `{running, backfilling, message, progress, total}` |

## Summary and status

| Method | Signature | Description |
|------|------|------|
| `GetSummary` | `(date) → Summary` | global daily summary (team/personal additions & deletions, files, repo count, workday flag) |
| `GetHeatmapData` | `(projectId) → {days: HeatmapDay[]}` | heatmap for the last year; `projectId>0` limits to a project |
| `GetStatusBar` | `() → StatusBarData` | current time + latest commit (30s cache) |
| `GetTodoCounts` / `GetNoteCounts` | `() → counts[]` | per-project todos (incomplete/total) and note counts |

## Search

| Method | Signature | Description |
|------|------|------|
| `SearchNotes` | `(query) → SearchHit[]` | FTS5 note search (bm25 + snippet highlighting) |
| `SearchAll` | `(query) → SearchHit[]` | combined search over notes + todos |

## Notes

| Method | Description |
|------|------|
| `ListNotes(projectID)` / `ListAllNotes()` / `ListAllTags()` | project notes / all notes (with project name) / all tags |
| `CreateNote(projectID, content)` | create (defaults: kind=other, source=manual) |
| `CreateNoteWithMeta(projectID, title, content, tags, kind, source)` | create with metadata |
| `UpdateNote(noteID, content)` / `UpdateNoteFull(noteID, content, title, tags, kind, pinned)` | update content / content and all metadata in one call |
| `PinNote(noteID, pinned)` / `MoveNote(noteID, projectID)` / `DeleteNote(noteID)` | pin / move / delete |
| `ListNoteVersions(noteID)` | version list (latest 50) |
| `RestoreNoteVersion(noteID, versionID)` | restore to a historical version |
| `DiffNoteVersions(noteID, versionID)` | line-level diff of version vs current (`+/-/space` prefix) |

A successful create emits the plugin event `note.created`.

## Todos

`ListTodos(projectID)` / `CreateTodo(projectID, title)` / `ToggleTodo(id)` / `DeleteTodo(id)` / `ReorderTodos(ids[])` (reordered in a single transaction).

## Configuration

| Method | Description |
|------|------|
| `GetConfig()` | `{config: map, scan_roots: []}` |
| `UpdateConfig(key, value)` | 22 allowed keys — core: `daily_code_standard` / `scan_depth` / `git_author` / `auto_import` / `claude_session_capture` / `openclaw_project` / `hermes_project`; semantic search & vector store: `semantic_search` / `embedding_base_url` / `embedding_model` / `embedding_api_key` / `embedding_dim` / `vector_store` / `vector_store_url` / `vector_store_api_key` / `vector_store_collection`; AI & knowledge graph: `ai_chat_base_url` / `ai_chat_model` / `ai_chat_api_key` / `wiki_compile` / `wiki_lint_llm` / `wiki_graph_search` (non-string keys are validated as numbers; secret keys come back masked from `GetConfig`. Per-key defaults and "what turning this on sends out" live in the [AI Integration](features/ai-integration.md) config table) |
| `UpdateScanRoots(roots[])` | atomically replace the scan root list |

## AI export and plugins

| Method | Description |
|------|------|
| `GenerateLLMsTxt()` | LLM-oriented Markdown overview of the knowledge base |
| `ExportNoteAsMarkdown(noteID)` | note Markdown with YAML frontmatter |
| `GetPluginStatuses()` / `ReloadPlugins()` | plugin load status / hot reload |
| `GetKnowledgeSources()` | knowledge source list (six built-in agent memory sources claude / codex / opencode / openclaw / hermes / cursor + plugin-registered) |
| `TriggerKnowledgeImport(name)` / `TriggerAllKnowledgeImports()` | trigger imports (emits the `import.completed` frontend event) |
| `ImportClaudeMemory()` | one-click Claude memory import (legacy entry point; identical to triggering the `claude` source by name) |
| `Health()` | `{status, version}` |

## AI and knowledge graph (M6/M7)

> All are desktop bindings. MCP deliberately excludes the wiki write surface — an agent that can write pending knowledge itself would bypass the human in the loop (ADR-0015 decision 6); `StartAskStream` likewise lives only on the desktop + SSE and never crosses `/api/rpc`.

| Method | Description |
|------|------|
| `AskAI(projectID, question)` / `AskAIWithEvidence(projectID, question)` | plain chat / evidence-backed Q&A (two-arm recall + `[P#]/[N#]` citations + `Truncated` marker) |
| `StartAskStream(projectID, question)` | streaming Q&A: returns a `stream_id` first, then the answer arrives progressively via `ai.ask.stream` events (or the `/api/ai/ask-stream` SSE endpoint in browser mode) |
| `FileAnswerAsPage(projectID, question, answer, evidence, citedRefs)` | file an answer back as a `query` page (linked to the cited pages and notes; only citation numbers resolved from that retrieval are accepted) |
| `RebuildEmbeddings()` | recompute all note embeddings (runs only after `semantic_search` is on; day-to-day increments go through the `note_embed_dirty` queue) |
| `ListAIModels(baseURL, apiKey)` / `TestAIChat(baseURL, model, apiKey)` | list models / test an endpoint (`finish_reason` is surfaced so truncation and format errors look different) |
| `CaptureClaudeHandoff(projectID)` / `LatestHandoff(projectID)` | on-demand Claude handoff capture / latest handoff status (same source as the VS Code status bar) |
| `ExportMemoryJSON(limit)` | OMP-style Memory Object export (provisional, export-only) |
| `RunWikiLint(projectID, wantLLM)` | the five lint checks; findings only write `project_todos` — **no code path can mutate a page** |
| `CompileNote(noteID)` / `CompileProjectNotes(projectID, maxNotes)` | synchronous compile (output is always `status=pending` with the source note auto-attached) |
| `StartCompileJob` / `GetCompileJob` / `ListCompileJobs` / `StartLintJob` / `CancelCompileJob` | async compile / lint jobs (ADR-0016; the three structural checks never enter a job) |
| `ListPendingWikiPages(projectID)` / `ListRejectedWikiPages(projectID)` | pending / disqualified page lists (the Review tab's data source, with source notes and in/out links) |
| `ApproveWikiPage(pageID)` / `RejectWikiPage(pageID)` / `DeleteCompiledPage(pageID)` | approve (also extracts body `[[wikilink]]`s into idempotent `mentions` edges) / mark disqualified (keeps inbound links, never deletes the row) / permanently delete an unpublished page |
| `LayeredProjectContext(projectID, query, includeTranscripts)` | L3 global profile → L2 project scene → L1 notes → L0 transcripts assembly (per-layer independent budgets) |

## Events (Wails → frontend)

| Event | Payload |
|------|------|
| `import.completed` | `{source, created, updated, skipped, error?}` |
| `ai.ask.stream` | streaming Q&A fragments: `{stream_id, delta}` × N, terminal `{stream_id, done, reply, evidence, truncated}` or `{stream_id, error}` |

---

## OpenAPI

For now, the tables on this page serve as the machine-readable contract (the hand-maintained openapi.json was removed in D10 and will return once CI generates it automatically). The desktop app does not expose a public HTTP service (see [TODO](https://github.com/sky-jiangcheng/repo-nest/blob/master/TODO.md)).
