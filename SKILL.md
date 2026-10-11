# RepoNest Skill

RepoNest is a local-first, cross-agent project memory layer. It discovers local Git repositories, maintains a cross-project knowledge base (Markdown notes, FTS5 search, version history), mines repository knowledge, and exposes it to AI agents via the MCP server and llms.txt export.

## Session Memory Protocol

The tools form a session loop. Use it in every working session on a RepoNest-tracked project:

```mermaid
flowchart TB
    A["reponest_context<br/>session start"] --> W(["…work…"])
    W --> B["reponest_handoff<br/>session end"]
    B -.->|"next session reads it first"| A
    classDef ctx fill:#f0fdf4,stroke:#22c55e,color:#14532d
    classDef rec fill:#fdf2f8,stroke:#ec4899,color:#831843
    class A ctx
    class B rec
```

0. **First run**: on a fresh install call `reponest_scan` once — it seeds the default scan roots and scans them synchronously, so the knowledge base is populated without ever opening the desktop app. Call it again any time `reponest_context` reports no projects. (Humans: `node scripts/reponest-init/index.mjs --with-hook` registers the MCP server and the SessionEnd handoff hook in one command — see ADR-0009.)
1. **Start**: call `reponest_context` once — it returns tech stack, README excerpt, dependencies, recent commits, open todos and the most relevant notes (previous session handoffs lead). This replaces chaining `projects_list` → `notes_search` → `notes_read`.
2. **End**: call `reponest_handoff` with `summary` (required) plus any of `changes` / `decisions` / `gotchas` / `next_steps`. The note is tagged `handoff` and the next session reads it first — regardless of which agent wrote it.
   - Handoff notes are protocol records: `reponest_notes_update` refuses to overwrite them (write a new handoff instead), and `reponest_context` renders the latest one in full.
   - To make step 2 fire automatically instead of relying on the agent remembering, wire a Claude Code `SessionEnd` hook — one bounded headless turn at session end (see `docs/features/ai-integration.md`).

## Recommended AI Workflow

```
0. First run        → reponest_scan                  → discover repos, populate the knowledge base
1. Load context     → reponest_context               → session cold start, one call
2. Search projects  → reponest_projects_list        → find relevant repos
3. Search knowledge → reponest_notes_search / reponest_ask  → find existing notes
4. Read details     → reponest_notes_read            → read a specific note
5. Create knowledge → reponest_notes_create          → capture new insights
6. Update knowledge → reponest_notes_update          → refine existing notes
7. End session      → reponest_handoff               → structured handoff for the next session
8. Check readiness  → reponest_agent_score           → verify AI integration health
9. Check the data   → reponest_integrity             → when results look incomplete
```

**When search comes back suspiciously thin**, run `reponest_integrity` before
concluding the knowledge base is thin. An FTS index that drifted out of sync
with the notes table makes every search silently under-report, and
`reponest_agent_score` will still say everything is fine — it checks
installation readiness, not whether the data is true.

**Typical scenario**: "Help me understand project X"
1. `reponest_context({ project_name: "X" })` — one call loads everything known about the project
2. `reponest_notes_read({ id })` — expand on any note the context surfaced
3. `reponest_handoff({ project_id, summary, changes, next_steps })` — save what you learned when done

**Typical scenario**: "What do I know about topic Y?"
1. `reponest_notes_search({ query: "Y" })` — full-text search across all notes
2. `reponest_ask({ query: "Y" })` — FTS5 search across notes + todos, returned as a formatted top-5 answer
3. `reponest_notes_read({ id })` — read the most relevant note in detail

## What RepoNest Does

- **Repository discovery**: Automatically finds Git repos under configurable scan roots
- **Daily activity tracking**: Lines added/deleted, files changed, commit counts per repo (365-day backfill on demand)
- **Knowledge base**: Cross-project Markdown notes with tags, pins, version history + LCS diff
- **Full-text search**: SQLite FTS5 trigram + bm25 ranking across notes and todos
- **Agent memory import**: idempotent import from six built-in sources (claude / codex / opencode / openclaw / hermes / cursor)
- **Repo knowledge mining**: README excerpt, tech stack, languages, dependencies, contributors, activity
- **AI context export**: `llms.txt` and per-note `.md` export

## MCP Server (`reponest-mcp`)

MCP is the single AI execution interface (the `reponest` CLI is not shipped). stdio server, opens the database once per process. The desktop app is **not** required — `reponest-mcp` reads the same local SQLite database.

Install from a release (preferred), then register:

```bash
# No prerequisites — download reponest-mcp-<target>.tar.gz / .zip from Releases
# Or use a package manager:
#   macOS   : brew tap sky-jiangcheng/repo && brew install --cask sky-jiangcheng/repo/reponest-mcp
#   Linux   : brew tap sky-jiangcheng/repo && brew install sky-jiangcheng/repo/reponest-mcp
#   Windows : scoop bucket add repo https://github.com/sky-jiangcheng/scoop-repo && scoop install repo/reponest-mcp
```

Build from source if none of the above apply:

```bash
go build -o /usr/local/bin/reponest-mcp ./cmd/mcp/
```

### MCP Tools

> **Usage patterns**: `reponest_ask` is the primary entry point for most queries (FTS5 over notes + todos, returned as a formatted top-5 answer). Use `reponest_notes_search` for precise FTS5 hits. For write operations, always read the existing note first to avoid overwriting.

| Tool | Description | Key Parameters | Example |
|------|-------------|----------------|--------|
| `reponest_scan` | Discover local Git repos and populate the knowledge base (first run; headless) | none (seeds default scan roots, then scans synchronously) | `{}` |
| `reponest_context` | Load full project context in one call (session start) | `project_id?`, `project_name?` (fuzzy); none = auto-resolve when only one project | `{ project_name: "auth" }` |
| `reponest_handoff` | Record a structured session handoff (session end) | `project_id`, `summary`, `changes?`, `decisions?`, `gotchas?`, `next_steps?`, `agent?`, `tags?` | `{ project_id: 1, summary: "...", gotchas: ["..."], next_steps: ["..."] }` |
| `reponest_ask` | Ask a question; FTS5 over notes + todos, returns a formatted top-5 answer | `query` (string, supports CJK) | `{ query: "数据库迁移方案" }` |
| `reponest_notes_search` | FTS5 full-text search across notes | `query` (string, trigram + bm25) | `{ query: "react hooks" }` |
| `reponest_notes_read` | Read one note by ID | `id` (number) | `{ id: 42 }` |
| `reponest_notes_create` | Create a new note | `project_id`, `title`, `content`, `category?`, `tags?` | `{ project_id: 1, title: "API Design", content: "...", category: "knowledge" }` |
| `reponest_notes_update` | Update note content/metadata (refuses notes tagged `handoff`) | `id`, `content?`, `title?`, `tags?`, `category?` | `{ id: 42, content: "updated text" }` |
| `reponest_notes_list` | List all notes (paginated) | `limit?`, `offset?` | `{ limit: 20 }` |
| `reponest_projects_list` | List all projects with stats | `starred_only?` | `{ starred_only: true }` |
| `reponest_projects_stats` | Get stats for one project | `id` (number) | `{ id: 1 }` |
| `reponest_agent_score` | Check AI-readiness score (0-100) | none | `{}` |
| `reponest_integrity` | Audit data trustworthiness (index drift, orphans, staleness) | none | `{}` |

### Registering with clients

**Claude Code** (recommended):

```bash
claude mcp add reponest -- /usr/local/bin/reponest-mcp
```

Or in a project-level `.mcp.json` / user-level MCP config (Cursor: Settings → MCP → Add Server):

```json
{
  "mcpServers": {
    "reponest": { "command": "/usr/local/bin/reponest-mcp", "args": [] }
  }
}
```

> Windows path example: `"command": "C:\\Tools\\reponest-mcp.exe"`.

## Key File Paths

| Content | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Database | `~/Library/Application Support/reponest/dashboard.db` | `~/.config/reponest/dashboard.db` | `%APPDATA%\reponest\dashboard.db` |
| Plugins | `…/reponest/plugins/` | `…/reponest/plugins/` | `…\reponest\plugins\` |
| Log | `~/Library/Logs/reponest.log` | `$XDG_STATE_HOME/reponest/reponest.log` (default `~/.local/state/reponest/`) | `%APPDATA%\reponest\logs\reponest.log` |
| Claude memory | `~/.claude/projects/*/memory/*.md` | same | same |

## Architecture

- **Backend**: Go + SQLite (modernc, zero CGO), Wails v2 desktop app
- **Layering**: `internal/app` (thin Wails bindings) → `internal/service` (business core, shared by desktop/MCP) → `internal/db` + `internal/core/git`
- **Frontend**: React 19 + Vite 8 + TypeScript 6, hand-rolled CSS design system
- **Search**: FTS5 trigram + bm25, LIKE fallback for short CJK queries
- **i18n**: react-i18next, zh-CN + en
- **Markdown**: Mermaid, KaTeX, callouts, highlight.js
- **Plugins**: the six built-in knowledge sources are plain Go implementations (no yaegi); yaegi only runs user-written script plugins (see docs/plugins/overview.md)

Docs: <https://sky-jiangcheng.github.io/repo-nest/> · Repo: <https://github.com/sky-jiangcheng/repo-nest>
