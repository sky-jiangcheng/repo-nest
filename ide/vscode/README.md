# RepoNest for VS Code

**Your local knowledge base, inside your IDE.** RepoNest keeps what your coding agents learn — why a decision was made, what gotcha you hit, what to do next — in a local, searchable store. This extension surfaces it where you work: load a project's full session context, write a structured handoff when you stop, and search everything you ever captured, without leaving VS Code.

One VSIX covers the whole fork family — **VS Code, Cursor and Windsurf**.

## What it does

| Surface | What it does |
|---|---|
| Command `RepoNest: Load Session Context` | Pulls the working context for your project (tech stack, README excerpt, recent commits, open todos, the last session handoff) via `reponest_context` and shows it in the RepoNest output channel |
| Command `RepoNest: Write Session Handoff` | Prompts for summary / changes / next steps, then records a structured handoff (`reponest_handoff`) — the next session, yours or any agent's, starts from it |
| Command `RepoNest: Search Knowledge` | Full-text search (FTS5) across every note you captured; hits render in the sidebar and open read-only |
| Sidebar "Knowledge Search" | Tree view bound to the search results |
| Status bar `⌒ RepoNest` | Click = load context; the tooltip shows the latest session handoff (title + time, refreshed every minute) |
| Command `RepoNest: Register MCP Server` | Runs `reponest-init --with-hook` in a terminal: registers the MCP server for Claude Code / Cursor / VS Code / Windsurf and installs the SessionEnd hook |

## Requirements

- The [`reponest-mcp`](https://github.com/sky-jiangcheng/repo-nest#quick-start) binary on your PATH — install it via Homebrew, Scoop or the GitHub Releases page. The desktop app is optional; the MCP server alone is enough.
- Optional: a running `reponest-server` (build it with `go build -o reponest-server ./cmd/server`) for the status-bar health check (set `reponest.serverUrl` if you changed the port).

## Settings

| Setting | Default | Description |
|---|---|---|
| `reponest.serverUrl` | `http://127.0.0.1:18765` | RepoNest headless service URL |
| `reponest.projectId` | *(empty)* | Pin a project id for context / handoff; empty = let the service resolve the current workspace |

## Install

**Marketplace**: search "RepoNest" in the Extensions view, or:

```bash
code --install-extension reponest-vscode.vsix
```

One VSIX works in VS Code, Cursor and Windsurf (use the fork's own CLI: `cursor --install-extension …`, `windsurf --install-extension …`).

The extension is a thin client — every answer comes from the same `internal/service` layer as the RepoNest desktop app, read from your local SQLite knowledge base. Nothing leaves your machine.

## Development

```bash
cd ide/vscode
npm install
npm run compile
# open this folder in VS Code and "Run Extension" (F5), or package:
npx @vscode/vsce package --no-dependencies   # → reponest-vscode-<version>.vsix
```

Architecture and scope: [ADR-0009](https://github.com/sky-jiangcheng/repo-nest/blob/master/docs/en/adr/0009-ide-presence.md). The extension calls `reponest-mcp` over stdio and the loopback-only headless service; zero business logic lives here.
