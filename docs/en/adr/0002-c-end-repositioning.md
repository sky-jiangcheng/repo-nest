# ADR 0002: Product Repositioning to C-end "Second Brain for Code Projects" with In-process Plugins

- Status: Accepted
- Date: 2026-08-07
- Related: RFC 0001 (superseded by this ADR)

## Background

RFC 0001 planned the product as a "plugin platform" with B-end deployment capabilities: a resident HTTP Server / RBAC / AK-SK (M2), a plugin protocol gateway + scope permissions (M3), and PG/ES / K8s / repotory-server (M4).

Market and user feedback evaluation concluded that this positioning was overly complex and drifted away from the core value of a desktop tool. The real differentiation lies in: a visualization tool for local Git repositories that also accumulates into the user's "second brain for code projects" — cross-project notes, todos, and repository knowledge mining.

## Decision

1. **Core positioning**: the product is a C-end "second brain for code projects". The knowledge base (notes/todos/repository knowledge) is the primary entry point; the dashboard displays commit statistics.
2. **Plugin form**: adopt in-process plugins (loaded within the process, interfaces in `internal/core/plugin`) rather than an external plugin protocol gateway.
   - Rationale: an external gateway (M3) requires network protocols, authentication, and deployment — high complexity; a C-end desktop tool needs no multi-process isolation.
   - Plugins load inside the application process and access the knowledge base and storage, registering events and knowledge sources, through `PluginContext`.
3. **Deprecate M2/M3/M4**: the resident HTTP Server, RBAC, AK-SK, plugin protocol gateway, scope permissions, PG/ES, K8s, and repotory-server are all dropped.
4. **Keep the M1 abstraction layer**: as the foundation supporting additional plugin extension interfaces. Abstractions already landed, such as `storage.Stores`, `ScanTxer`, and the KB Facade, are retained and provide base capabilities for plugins.

## Impact

- Development focus shifts to the local knowledge base experience and AI-ready capabilities (llms.txt, `.md` export, local Q&A, MCP).
- No server-side components; single-binary distribution is preserved.
- The plugin mechanism focuses on on-machine extension: importing knowledge sources, listening for note/scan/import events.

## Plugin Loading Selection (verified conclusions from issue #33)

Two in-process plugin loading approaches were validated via prototypes:

| Dimension | Go plugin (`plugin.Open`) | yaegi (traefik/yaegi) |
|-----------|--------------------------|-----------------------|
| Cross-platform | Windows unsupported (Linux/macOS/FreeBSD only) | Pure Go, consistent across all three platforms |
| Loading | Requires precompiled `.so`, built with exactly the same Go version as the host | Loads `.go` scripts, no precompilation, decoupled from the host Go version |
| Panic isolation | recover can protect | recover can protect (verified) |
| Missing directory | Handle it yourself | Skipped gracefully (verified) |
| Ecosystem | Standard library, stable long-term | Actively maintained, interpreted execution |

**Decision: adopt yaegi.** The decisive factor is cross-platform support — Repotory explicitly ships to Windows / macOS / Linux, and Go plugin is unavailable on Windows, failing the acceptance criteria. yaegi is a pure Go interpreter that loads `.go` plugin scripts consistently on every target platform and requires no strict version binding between plugin and host.

## Rationale

- Lower implementation and maintenance complexity; shorter delivery cycles.
- Matches desktop tool users' expectations of privacy, offline use, and lightness.
- Concentrates differentiated resources on the knowledge base and AI integration rather than platform infrastructure.
