# Repotory for VS Code

**Your local knowledge base, inside your IDE.** Repotory keeps what your coding agents learn — why a decision was made, what gotcha you hit, what to do next — in a local, searchable store. This extension surfaces it where you work: load a project's full session context, write a structured handoff when you stop, and search everything you ever captured, without leaving VS Code.

One VSIX covers the whole fork family — **VS Code, Cursor and Windsurf**.

## What it does

| Surface | What it does |
|---|---|
| Command `Repotory: Load Session Context` | Pulls the working context for your project (tech stack, README excerpt, recent commits, open todos, the last session handoff) via `repotory_context` and shows it in the Repotory output channel |
| Command `Repotory: Write Session Handoff` | Prompts for summary / changes / next steps, then records a structured handoff (`repotory_handoff`) — the next session, yours or any agent's, starts from it |
| Command `Repotory: Search Knowledge` | Full-text search (FTS5) across every note you captured; hits render in the sidebar and open read-only |
| Sidebar "Knowledge Search" | Tree view bound to the search results |
| Status bar `⌒ Repotory` | Click = load context; tooltip shows headless-service reachability |
| Command `Repotory: Register MCP Server` | Runs `repotory-init --with-hook` in a terminal: registers the MCP server for Claude Code / Cursor / VS Code / Windsurf and installs the SessionEnd hook |

## Requirements

- The [`repotory-mcp`](https://github.com/sky-jiangcheng/repo-nest#quick-start) binary on your PATH — install it via Homebrew, Scoop or the GitHub Releases page. The desktop app is optional; the MCP server alone is enough.
- Optional: a running `repotory server` for the status-bar health check (set `repotory.serverUrl` if you changed the port).

## Settings

| Setting | Default | Description |
|---|---|---|
| `repotory.serverUrl` | `http://127.0.0.1:18765` | Repotory headless service URL |
| `repotory.projectId` | *(empty)* | Pin a project id for context / handoff; empty = let the service resolve the current workspace |

## Install

**Marketplace**: search "Repotory" in the Extensions view, or:

```bash
code --install-extension repotory-vscode.vsix
```

One VSIX works in VS Code, Cursor and Windsurf (use the fork's own CLI: `cursor --install-extension …`, `windsurf --install-extension …`).

The extension is a thin client — every answer comes from the same `internal/service` layer as the Repotory desktop app, read from your local SQLite knowledge base. Nothing leaves your machine.

## Development

```bash
cd ide/vscode
npm install
npm run compile
# open this folder in VS Code and "Run Extension" (F5), or package:
npx @vscode/vsce package --no-dependencies   # → repotory-vscode-<version>.vsix
```

Architecture and scope: [ADR-0009](https://github.com/sky-jiangcheng/repo-nest/blob/master/docs/en/adr/0009-ide-presence.md). The extension calls `repotory-mcp` over stdio and the loopback-only headless service; zero business logic lives here.
