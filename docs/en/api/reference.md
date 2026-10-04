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

## Headless HTTP server (`reponest server`)

A JSON API for external runtimes such as the DeepSeek Harness dsh-plugin. It shares the same `internal/service` implementation and the same SQLite database as the desktop app, CLI, and MCP.

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
| `UpdateNote(noteID, content)` / `UpdateNoteMeta(noteID, title, tags, kind, pinned)` | update content / metadata |
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
| `UpdateConfig(key, value)` | allowed keys: `daily_code_standard` / `scan_depth` / `git_author` / `auto_import` (numeric keys are validated as numbers) |
| `UpdateScanRoots(roots[])` | atomically replace the scan root list |

## AI export and plugins

| Method | Description |
|------|------|
| `GenerateLLMsTxt()` | LLM-oriented Markdown overview of the knowledge base |
| `ExportNoteAsMarkdown(noteID)` | note Markdown with YAML frontmatter |
| `GetPluginStatuses()` / `ReloadPlugins()` | plugin load status / hot reload |
| `GetKnowledgeSources()` | knowledge source list (built-in claude + plugin-registered) |
| `TriggerKnowledgeImport(name)` / `TriggerAllKnowledgeImports()` | trigger imports (emits the `import.completed` frontend event) |
| `ImportClaudeMemory()` | one-click Claude memory import |
| `Health()` | `{status, version}` |

## Events (Wails → frontend)

| Event | Payload |
|------|------|
| `import.completed` | `{source, created, updated, skipped, error?}` |

---

## OpenAPI

For now, the tables on this page serve as the machine-readable contract (the hand-maintained openapi.json was removed in D10 and will return once CI generates it automatically). The desktop app does not expose a public HTTP service (see [TODO](https://github.com/sky-jiangcheng/repo-nest/blob/master/TODO.md)).
