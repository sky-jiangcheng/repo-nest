# ADR-0005: Service Layer Refactor (service / app / domain layering)

- Status: Accepted (the two items in Section 5 "Version and Data Paths" were superseded by the 2026-09-01 brand rename; see "Revisions" at the end)
- Date: 2026-08-17 (implemented in 1.7.0)
- Related: [ADR-0002](0002-c-end-repositioning.md), [ADR-0001](0001-plugin-platform.md) (the superseded M1-M4 layering vision)

## Background

The code shape before 1.7.0: the root `package main` piled up 14 handler files (roughly 1900 lines of business logic); the CLI (`cmd/repotory`) and MCP (`cmd/mcp`) each re-implemented "the same query + the same formatting" on top of `internal/db`; two competing scan pipelines existed, 5 nearly identical stats refresh loops, and a 1195-line monolithic `queries.go`. Exploratory digging also uncovered two production bugs hidden by the duplicated code (a repository ghost column, a go.mod block parsing panic).

## Decision

### 1. Three-layer division of labor

```
internal/app      Wails binding layer: 1-3 lines of delegation per method (transport glue)
internal/service  Business core: the only layer allowed to access db and the git Provider
cmd/*             CLI / MCP: another transport on top of service
```

Wails, CLI, and MCP **share the same service implementation**. The UI layer (app) never touches `*sql.DB`.

### 2. Remove the storage interface shim (overriding the M1 legacy)

`internal/core/storage` + the sqlite adapter layer (roughly 500 lines) was a pure forwarding shim, and half of the service calls bypassed it — keeping it would only push the "mixed access style" problem down one level. **Deleted**. Rationale: ADR-0002 settled on local-first + a single SQLite backend; PostgreSQL/ES was an M4 vision of the deprecated RFC (ADR-0001). The seam with real abstraction value is `core/git.Provider` (future remote Git hosting), which is retained.

### 3. Standalone domain types

Row types move into `internal/domain`, with db keeping type aliases for compatibility (`db.Project = domain.Project`), eliminating the reverse storage→db type dependency. The LCS diff moves into `internal/diff`.

### 4. Transactions and pipelines in their proper place

- The hand-written SQL transactions for project split/merge move down from handlers into `db.SplitProjectDown` / `db.MergeProjectUp` (testable)
- One scan pipeline: scanner → grouper → db transaction sync → cleanup → refresh; the grouping has a single implementation in grouper
- The 5 refresh loops merge into `service.refreshRepoStatsRange` (commit-awareness + all/mine dual-author rows)

### 5. Version and data paths

- Version number SSOT: `internal/version.Version` (shared by app/CLI/MCP/agent-score)
- **User data paths unchanged** (DB/log file names/plugin directory keep the `repotory` directory name) — the brand rename (Repotory) does not migrate data
- Go module path stays `repotory` (avoiding a repo-wide import rewrite; see the rename plan in TODO.md)

## Consequences

- Positive: CLI/MCP and desktop behavior stay identical forever; the root package keeps only main.go; new features are implemented once; unit tests can target service directly (fake git provider)
- Negative: the db alias layer is a temporary compatibility shim; new code should use domain types directly; service holds `*sql.DB` directly (an honest reflection of the single-backend reality; re-introducing repository interfaces would be required if multiple backends ever become necessary)
- Leftovers: brand/binary rename, frontend `useApiData` adoption, ESLint TS7 blockers, etc. — see [TODO.md](../../../TODO.md)
