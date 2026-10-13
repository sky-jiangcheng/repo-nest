---
status: Superseded by ADR-0002
date: 2026-08 (originally RFC 0001)
---

# RFC 0001: Repotory Architecture Evolution & Plugin Platform

> **⚠️ Superseded by [ADR 0002](../adr/0002-c-end-repositioning.md)**
> M2 (resident HTTP Server / RBAC / AK-SK), M3 (plugin protocol gateway + scope permissions), and M4 (PG/ES / K8s / repotory-server) described in this RFC are deprecated.
> The M1 abstraction layer is retained as the foundation supporting additional plugin extension interfaces.

| Item | Value |
|------|-------|
| Status | **Superseded** (M1 completed; M2-M4 deprecated) |
| Design version | v0.1 (M1) |
| Last updated | 2026-08-07 |
| Related milestone | M1 abstraction decoupling ✅ → M2/M3/M4 deprecated (see ADR 0002) |

## 1. Background and Goals

Repotory is currently a **Wails + SQLite single-binary desktop application** with highly coupled code:

- `main.App` holds `*sql.DB`, and all handlers (`handlers_*.go`) directly call the functional API of `internal/db/*.go`;
- Git operations are implemented as package-level functions in `internal/stats/` and `internal/knowledge/`, with no way to switch remote Git providers;
- The storage layer (SQLite) is tightly bound to business logic, preventing future extension to PG/ES.

The goal is to evolve Repotory into a layered, decoupled architecture of "**core foundation + protocol gateway + plugin ecosystem**", supporting independent plugins, multi-Git-provider adaptation, and remote deployment scenarios.

This RFC codifies the key decisions across four phases — **M1 (abstraction decoupling) → M2 (service-oriented foundation) → M3 (plugin protocol loop) → M4 (ecosystem scale)** — as the blueprint for subsequent Issue implementation.

## 2. Decision

### 2.1 Runtime form: desktop ↔ server dual track, one Go kernel

- **Wails is retained**: the desktop version remains the primary release form (zero dependencies, double-click to run, matching the current core user value).
- **Kernel split**: business logic from `main.App` + `handlers_*.go` moves down into `internal/core/`, with the Wails Bind layer kept as a thin wrapper.
- **Embedded HTTP service**: within the same process, whenever started with the `-server` flag — or **always** in desktop mode — the app listens on `127.0.0.1:18731`, exposing the `/api/v1/*` RESTful API. The plugin protocol and the frontend API share this HTTP server.
  > **Factual correction (2026-10-13)**: three claims in this bullet no longer hold. The original text is kept so
  > later readers do not assume it was always wrong. (1) The desktop app embeds no HTTP server at all — the only
  > `ListenAndServe` in the repository lives in `cmd/server` (the standalone `repotory-server` binary) and
  > `internal/app` has no listener. (2) The port is **18765** (the `cmd/server` flag default, overridable via
  > `REPOTORY_HTTP_PORT`); 18731 was only ever the port `scripts/dev.sh` passed explicitly with `--port`, and it was
  > unified to 18765 on the same day. (3) The path is not `/api/v1/*` — main entry point of `httpapi` is now
  > `/api/rpc` (reflecting the bound App, covering every frontend call), with REST kept only for the subset the
  > `dsh` plugin needs. Separately, the in-process script plugin runtime of this ADR (yaegi) was removed under C11
  > option B; see the CHANGELOG and the status note on ADR-0002.
- **Standalone backend deployment**: a `repotory-server` build target will follow later (dropping the Wails bootstrap, keeping HTTP + plugin host), reusing 90%+ of the code.

### 2.2 Storage upgrade timing: introduced in M4 as pluggable backends

- **SQLite throughout M1–M3**: no PG/ES introduced; focus on getting the protocol working.
- **After M4 abstraction layering**: add `storage.postgres` + `search.elasticsearch` implementations, switchable at runtime via the `storage.backend` / `search.backend` config keys.
- **The desktop version always defaults to SQLite**; PG/ES are reserved for server deployment scenarios.

### 2.3 Plugin runtime: three-stage tiered evolution

| Tier | Technology | Availability | Use cases |
|------|-----------|--------------|-----------|
| **Tier 1** | In-process Go plugin / WASM | M3 launch | Official base plugin set, prototyping |
| **Tier 2** | Standalone local process + RESTful (HTTP2) | Mid M4 | Community plugins, Node/Python plugin support |
| **Tier 3** | K8s / Docker containers | When enterprise customers require it | Enterprise-grade server-side isolated deployment |

- The first official M3 plugin, the "project management plugin", uses Tier 1.
- Under Tier 1 the webhook protocol goes through direct Go channel calls; Tier 2+ goes through HTTP. The abstraction layer unifies the interface semantics.

## 3. M1 Architecture Blueprint (landed this round)

```
┌─────────────────────────────────────────────────┐
│  Wails Bind / HTTP API  (main pkg, thin wrapper)│
├─────────────────────────────────────────────────┤
│  internal/core/                                 │
│  ┌──────────┐  ┌────────────┐  ┌────────────┐  │
│  │   Git    │  │  Storage   │  │     KB     │  │
│  │ Provider │  │   Stores   │  │  Facade    │  │
│  └────┬─────┘  └─────┬──────┘  └─────┬──────┘  │
│       │              │               │         │
│       ▼              ▼               ▼         │
│  local git exec    adapter         mapper      │
│  (stats pkg)     internal/db    ↔project/note  │
│  (knowledge pkg) (original SQLite impl)        │
└─────────────────────────────────────────────────┘
```

### 3.1 GitProvider interface (`internal/core/git`)

```go
type Provider interface {
    QueryStats(repoPath, date, author string) (*stats.Result, error)
    QueryStatsRange(repoPath, startDate, endDate, author string) ([]stats.DailyEntry, error)
    GetRecentCommits(repoPaths []string, filterAuthor string, limit int) ([]stats.RecentCommit, error)
    MineKnowledge(repoPath string) (*knowledge.RepoKnowledge, error)
}
```

- The M1 default implementation is `LocalGitProvider`, directly calling the package-level functions of `internal/stats/` and `internal/knowledge/`.
- Extension points are reserved for M4's `GitLabProvider` / `GitHubProvider` / `GiteaProvider`.

### 3.2 Storage layer interfaces (`internal/core/storage`)

Store interfaces split by domain:

```go
type (
    ProjectStore interface { /* GetAll / GetByID / Search / Sync / ... */ }
    RepositoryStore interface { /* GetAll / GetByProjectID / Upsert / ... */ }
    DailyStatStore interface { /* Upsert / GetByProject / ... */ }
    NoteStore interface { /* Create / List / Update / Search / ... */ }
    TodoStore interface { /* Create / List / Toggle / Reorder / ... */ }
    RepoMetaStore interface { /* Get / Upsert */ }
    ConfigStore interface { /* Get / Set / All */ }
    ScanRootStore interface { /* Get / Replace */ }
    SearchStore interface { /* SearchNotes / SearchAll */ }
)

type Stores struct {
    Project    ProjectStore
    Repository RepositoryStore
    DailyStat  DailyStatStore
    Note       NoteStore
    Todo       TodoStore
    RepoMeta   RepoMetaStore
    Config     ConfigStore
    ScanRoot   ScanRootStore
    Search     SearchStore
}
```

- M1 default implementation: the `storage/sqlite` package, a **thin wrapper** over the existing `internal/db` functional API (avoiding SQL and test rewrites).
- `*sql.DB` moves down from `main.App` into `storage/sqlite` internals.

### 3.3 Knowledge base domain model (`internal/core/kb`)

```go
type Space struct {
    ID         int64  // ↔ project.id
    Name       string // ↔ project.name
    RootPath   string // ↔ project.root_path
    IsStarred  bool   // ↔ project.is_starred
    // ...
}
type Doc struct {
    ID         int64  // ↔ note.id
    SpaceID    int64  // ↔ note.project_id
    Title      string // ↔ note.title
    Content    string // ↔ note.content
    Tags       string // ↔ note.tags
    Kind       string // ↔ note.kind
    Pinned     bool   // ↔ note.pinned
    Source     string // ↔ note.source
}
```

- Exposes unified kb semantics upward (plugin protocol scopes `kb:space:*` / `kb:doc:*`).
- Downward, a `Mapper` layer converts between Space/Doc and the legacy Project/Note models.

### 3.4 main.App refactoring

```go
type App struct {
    ctx            context.Context
    gitUser        string
    // newly introduced abstractions (injected from M1 onward instead of *sql.DB)
    Git            git.Provider
    Stores         storage.Stores
    KB             kb.Facade

    // retained (transition-period compatibility with legacy calls, cleaned up within M2)
    db             *sql.DB

    // scan/cache state
    scanMu         sync.Mutex
    // ... (unchanged)
}
```

- Transition period: within M1, `handlers_*.go` are not directly replaced; interface fields are added instead, and the legacy code paths keep working.
- Integration phase: each handler is migrated one by one from "direct `db.Xxx()`" calls to the "`a.Stores.Project.Xxx()`" style.

## 4. Alternatives Considered

### 4.1 Going straight to gRPC + PG + ES + containerization

**Rejected**: the product's core value is still the desktop single binary; jumping straight in adds operational complexity with no first plugin to validate the demand.

### 4.2 Dropping SQLite and switching directly to PG

**Rejected**: PG is a pure burden for desktop users and breaks the "zero-dependency single file" selling point. With the storage abstraction in place, PG remains a future option rather than a forced replacement.

### 4.3 Supporting only Tier 2 (standalone process) plugins

**Rejected**: the desktop user experience is very poor (manually starting plugin processes, configuring ports). Tier 1 first + Tier 2 follow-up is a more reasonable cadence.

## 5. Consequences and Risks

- **Positive**: after M1, the code shape supports future remote Providers, pluggable storage, and the plugin protocol domain model; passing the interface regression tests equals zero behavior change.
- **Risk**: during the transition, App holds both the interfaces and the underlying `db` field, creating a dual-write consistency hazard. The M1 integration phase must fully migrate all call paths to the interfaces.
- **Metrics**: M1 completion criteria — "`go test ./...` passes in full" and "direct references to `main.App.db` in business code reduced to 0 or remaining only inside the SQLite implementation layer".

## 6. Related Issues

| # | Title | Milestone |
|---|-------|-----------|
| 1 | Abstract GitProvider interface + LocalGitProvider implementation | M1 |
| 2 | Abstract Storage layer Store interfaces, SQLite default implementation | M1 |
| 3 | Define Space/Doc knowledge base domain model and mapping layer | M1 |
| 4 | This RFC document | M1 |
| 5 | Desktop-mode embedded HTTP service skeleton (/api/v1/health) | M2 |
| 6 | RBAC engine (scope middleware) | M2 |
| 7 | AK/SK signature middleware | M2 |
| … | See the main milestone issue list for details | … |
