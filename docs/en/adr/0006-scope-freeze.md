# ADR-0006: Scope Freeze and Feature Tiering (Core Loop First)

- Status: Accepted
- Date: 2026-08-20
- Related: [ADR-0002](0002-c-end-repositioning.md) (local-first positioning), [ADR-0001](0001-plugin-platform.md) (the deprecated platform route), [ADR-0005](0005-service-layer.md) (service layer unification), [ADR-0007](0007-session-memory-protocol.md) (session memory protocol, the implementation of this positioning)

## Background

Repotory simultaneously carries multiple product lines: commit statistics, knowledge base, block editor, plugin platform, PWA, SEO, CLI, MCP, agent-score, etc. (Issues #73/#74). The core value gets obscured by feature count, while peripheral capabilities keep adding maintenance cost. This ADR converges the product main line to:

> **A cross-agent project memory layer**: providing project-level context injection for AI sessions and structured handoff at session end, letting knowledge flow between agents.

"Local-first" remains the deployment and data boundary (data never leaves this machine, see ADR-0002), but the product value proposition upgrades from "a knowledge base both humans and AI can query" to "memory flowing between agents": `repotory_context` injects the full context with one call at session start, and `repotory_handoff` deposits it at zero cost at session end — the two ends close into a memory loop (protocol details in ADR-0007).

Core loop (every new feature must fit at least one stage):

```
Discover project → Understand project → Record knowledge → Retrieve knowledge → Inject into session → Deposit at session end
```

## Decision

### 1. Primary delivery form

**A local-first cross-platform desktop application** (Wails single binary). Web/PWA/HTTP server are no longer advanced as equal delivery forms.

### 2. Feature tiering

| Tier | Definition | Current features |
|------|-----------|------------------|
| **Core** | Directly constitutes the "Discover → Understand → Record → Retrieve → Inject → Deposit" loop | Automatic repo discovery, project grouping, repository knowledge mining, Markdown notes, FTS5 search, version history, global search, session context injection (repotory_context), session-end handoff (repotory_handoff), AI-ready interfaces (CLI/MCP/llms.txt) |
| **Core support** | Serves the comprehensibility of the loop without overshadowing it | Dashboard and statistics, workday checks, status bar, i18n, single-file cross-platform, Claude memory import |
| **Experimental** | Kept for real scenarios but **no further platform infrastructure expansion** | yaegi plugin system, block editor advanced blocks (Callout/Tabs/Mermaid and other structured blocks), OpenAPI/HTTP API spec (contract documentation, not a usable HTTP server) |
| **Deferred / to remove** | No further investment; evaluate removal from the desktop main build | PWA deep capabilities, SEO artifacts, Web-only interactions |

### 3. Scope freeze rules

1. **Platformization capabilities no longer expand by default**: plugin SPI, HTTP server, multi-backend storage, web deployment, etc. add no new infrastructure; genuine needs must first file an ADR arguing their contribution to the core loop.
2. **New features must state their tier**: new features must be labeled in the issue/PR as belonging to at least one stage of "Discover / Understand / Record / Retrieve / Inject into session / Deposit at session end"; unlabeled features stay out of the roadmap by default.
3. **Experimental feature gate**: capabilities marked experimental must never become hard dependencies of other features; their documentation must clearly state "experimental, interfaces may change".
4. **OpenAPI semantics**: `docs/api/openapi.json` is a contract/experimental document of the Wails binding surface, never marketed as a "usable HTTP server"; if an HTTP server mode ever returns, it must be implemented in sync.
5. **Editor boundary**: adding complex editor blocks is deferred; Markdown stability, search, and AI readability come first (output stays pure Markdown, see ADR-0004).

## Consequences

- Positive: the roadmap gains a clear trade-off basis; maintenance cost concentrates on the core loop; documentation matches real capabilities (tiering marks in the README, see [README](../../../README.md)).
- Negative: existing capabilities such as the plugin ecosystem, PWA, and SEO stop expanding, potentially losing some prospective users; the experimental boundary must be made explicit to users in the documentation.
- Leftovers: unifying `llms.txt`, CLI, and MCP fields is covered by the convergence work in Issue #76.
