---
title: Architecture Decision Records (ADR)
order: 21
---

# Architecture Decision Records (ADR)

| # | Title | Status |
|---|-------|--------|
| [0001](0001-plugin-platform.md) | Plugin platform (M1-M4: HTTP server / RBAC / PG-ES / K8s) | Superseded by 0002 |
| [0002](0002-c-end-repositioning.md) | C-end repositioning: local-first "second brain for code projects" + in-process plugins | Accepted |
| [0003](0003-fts5-search.md) | FTS5 trigram full-text search | Accepted |
| [0004](0004-block-editor.md) | Block editor (output stays pure Markdown) | Accepted |
| [0005](0005-service-layer.md) | Service layer refactor (service / app / domain layering) | Accepted |
| [0006](0006-scope-freeze.md) | Scope freeze and feature tiering (core loop first) | Accepted |
| [0007](0007-session-memory-protocol.md) | Session memory protocol (context / handoff dual tools) | Accepted |
| [0008](0008-pwa-removal.md) | PWA removed from the desktop main build (implements the deferred tier of ADR-0006) | Accepted |
| [0009](0009-ide-presence.md) | IDE presence — thin-client distribution strategy (one-command registration → VS Code extension → JetBrains deferred) | Accepted |
| [0010](0010-session-auto-capture.md) | Session auto-capture — zero-touch Claude Code session handoff (M1) | Accepted-in-principle |
| [0011](0011-multi-agent-memory-importers.md) | Multi-agent memory importers — reusable pipeline + per-source feasibility (M2) | Proposed |
| [0012](0012-semantic-search.md) | Semantic search — vector recall over FTS5, stay zero-CGO (M3) | Accepted-in-principle |
| [0013](0013-vector-database-selection.md) | Vector store selection (sqlite-vec) and onboarding guide | Accepted |
| [0014](0014-llm-wiki-knowledge-compiler.md) | LLM Wiki knowledge compiler — compile at ingest, gather evidence at query time (M6) | Proposed (W1/W1b/W4 landed, W2 core + UI landed; its promotion still needs a real labelled query set — see 0015 for W3) |
| [0015](0015-w3-minimal-compile-loop.md) | W3 minimal compile loop — the LLM writes pending pages only, humans gate what enters search | Proposed |
| [0016](0016-async-batch-jobs.md) | Async long-running batch jobs — submit returns, progress is queryable, results survive | Accepted |
| [0017](0017-one-kernel-two-entry-points.md) | One kernel, two entry points — product layering without forking the repo | Proposed |

## Conventions

- One ADR per major irreversible decision: Background → Decision → Consequences
- Superseded ADRs keep their original text and banner; they are never deleted
- New ADRs increment from `0017`, file name `NNNN-kebab-title.md`
