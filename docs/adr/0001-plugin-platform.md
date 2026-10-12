---
status: Superseded（已被 ADR-0002 取代）
date: 2026-08（原 RFC 0001）
---

# RFC 0001: Repotory 架构演进 & 插件平台化

> **⚠️ Superseded by [ADR 0002](../adr/0002-c-end-repositioning.md)**
> 本 RFC 中的 M2（常驻 HTTP Server / RBAC / AK-SK）、M3（插件协议网关 + scope 权限）、M4（PG/ES / K8s / repotory-server）均已废弃。
> M1 抽象层保留，作为附加插件扩展接口的底层支撑。

| 项 | 值 |
|---|---|
| 状态 | **Superseded** (M1 已完成，M2-M4 废弃) |
| 设计版本 | v0.1 (M1) |
| 最近更新 | 2026-08-07 |
| 对应里程碑 | M1 抽象解耦 ✅ → M2/M3/M4 废弃（见 ADR 0002） |

## 1. 背景与目标

当前 Repotory 是 **Wails + SQLite 单二进制桌面应用**，代码高度耦合：

- `main.App` 持有 `*sql.DB`，所有 handler（`handlers_*.go`）直接调用 `internal/db/*.go` 的函数式 API；
- Git 操作在 `internal/stats/`、`internal/knowledge/` 中以包级函数形式实现，无法切换远程 Git 服务商；
- 存储层（SQLite）与业务逻辑强绑定，未来无法扩展到 PG/ES。

用户目标是将 Repotory 演进为「**核心底座 + 协议网关 + 插件生态**」的分层解耦架构，支持独立插件、多 Git 服务商适配、远端部署等场景。

本 RFC 固化 **M1（抽象解耦）→ M2（服务化底座）→ M3（插件协议闭环）→ M4（生态规模化）** 四个阶段的关键决策，作为后续 Issue 落地的蓝本。

## 2. 决策（Decision）

### 2.1 运行形态：桌面 ↔ 服务端双轨并行，同一套 Go 内核

- **不废弃 Wails**：桌面版继续是主力发布形态（零依赖、双击即用，对应当前用户核心价值）。
- **内核拆分**：`main.App` + `handlers_*.go` 的业务逻辑下沉到 `internal/core/`，Wails Bind 层仅做薄包装。
- **内嵌 HTTP 服务**：同一进程中，只要启动参数 `-server` 或桌面模式下**总是**监听 `127.0.0.1:18731`，对外暴露 `/api/v1/*` RESTful API。插件协议与前端 API 共享这套 HTTP。
- **独立后端部署**：后续提供 `repotory-server` 构建目标（去掉 Wails 启动、保留 HTTP + 插件宿主），复用 90%+ 代码。

### 2.2 存储升级时机：M4 引入，做成可插拔后端

- **M1–M3 全程 SQLite**：不引入 PG/ES，聚焦协议跑通。
- **M4 抽象分层后**：新增 `storage.postgres` + `search.elasticsearch` 两种实现，配置文件键 `storage.backend` / `search.backend` 运行时切换。
- **桌面版默认永远 SQLite**；PG/ES 只给 server 部署场景用。

### 2.3 插件运行时：三阶段 Tier 化演进

| Tier | 技术 | 启用时机 | 适用场景 |
|---|---|---|---|
| **Tier 1** | 进程内 Go plugin / WASM | M3 首发 | 官方基础插件集、打样 |
| **Tier 2** | 独立本地进程 + RESTful（HTTP2） | M4 中期 | 社区插件、支持 Node/Python 写插件 |
| **Tier 3** | K8s / Docker 容器 | 企业客户要求时 | 企业级服务端隔离部署 |

- M3 首个官方插件「项目管理插件」用 Tier 1。
- Webhook 协议在 Tier 1 下走 Go channel 直调、Tier 2+ 走 HTTP，抽象层统一接口语义。

## 3. M1 架构蓝图（本轮落地）

```
┌─────────────────────────────────────────────────┐
│  Wails Bind / HTTP API  (main 包，薄包装层)      │
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
│  (knowledge pkg) (SQLite 原实现)                │
└─────────────────────────────────────────────────┘
```

### 3.1 GitProvider 接口 (`internal/core/git`)

```go
type Provider interface {
    QueryStats(repoPath, date, author string) (*stats.Result, error)
    QueryStatsRange(repoPath, startDate, endDate, author string) ([]stats.DailyEntry, error)
    GetRecentCommits(repoPaths []string, filterAuthor string, limit int) ([]stats.RecentCommit, error)
    MineKnowledge(repoPath string) (*knowledge.RepoKnowledge, error)
}
```

- M1 默认实现 `LocalGitProvider`，直接调用 `internal/stats/` 和 `internal/knowledge/` 包级函数。
- 为 M4 的 `GitLabProvider` / `GitHubProvider` / `GiteaProvider` 留扩展点。

### 3.2 Storage 层接口 (`internal/core/storage`)

按领域拆分 Store 接口：

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

- M1 默认实现：`storage/sqlite` 包，**薄包装**现有 `internal/db` 函数式 API（避免重写 SQL 和测试）。
- `*sql.DB` 从 `main.App` 下移到 `storage/sqlite` 内部持有。

### 3.3 知识库领域模型 (`internal/core/kb`)

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

- 向上层（插件协议 scope `kb:space:*` / `kb:doc:*`）暴露统一 kb 语义。
- 向下 `Mapper` 层把 Space/Doc 与旧的 Project/Note 模型互转。

### 3.4 main.App 改造

```go
type App struct {
    ctx            context.Context
    gitUser        string
    // 新引入的抽象（M1 起注入这些而非 *sql.DB）
    Git            git.Provider
    Stores         storage.Stores
    KB             kb.Facade

    // 保留（过渡期兼容旧调用，M2 内清完）
    db             *sql.DB

    // 扫描/缓存状态
    scanMu         sync.Mutex
    // ...（保持不变）
}
```

- 过渡期：M1 内 `handlers_*.go` 不直接替换实现，而是新增接口字段；旧代码路径仍可用。
- 联调阶段：逐个 handler 把「直接 `db.Xxx()`」替换为「`a.Stores.Project.Xxx()`」风格。

## 4. 替代方案（Alternatives Considered）

### 4.1 一步到位上 gRPC + PG + ES + 容器化

**反对**：当前产品仍以桌面单二进制为核心价值，一步到位引入运维复杂度且没有首个插件验证需求。

### 4.2 废弃 SQLite、直接切换 PG

**反对**：PG 对桌面用户是纯负担，且破坏「零依赖单文件」卖点。存储抽象保留后，PG 是后续可选项而非强替换。

### 4.3 只支持 Tier 2（独立进程）插件形态

**反对**：桌面用户体验极差（需手动启动插件进程、配端口）。Tier 1 首发 + Tier 2 跟进是更合理的节奏。

## 5. 后果与风险（Consequences）

- **正面**：M1 后代码形态即支持未来的远程 Provider、可插拔存储、插件协议域模型；接口回归测试通过即等于行为零变化。
- **风险**：过渡期 App 同时持有接口与底层 `db` 字段，存在双写一致性隐患。M1 联调阶段需把所有调用路径完整迁移到接口。
- **度量**：M1 完成的判定标准——「`go test ./...` 全量通过」且「对 `main.App.db` 的直接引用在业务代码中降低到 0 或仅剩 SQLite 实现层内部」。

## 6. 关联 Issue

| # | 标题 | 里程碑 |
|---|---|---|
| 1 | 抽象 GitProvider 接口 + LocalGitProvider 实现 | M1 |
| 2 | 抽象 Storage 层 Store 接口，SQLite 默认实现 | M1 |
| 3 | 定义 Space/Doc 知识库领域模型与映射层 | M1 |
| 4 | 本 RFC 文档 | M1 |
| 5 | 桌面模式内嵌 HTTP 服务骨架（/api/v1/health） | M2 |
| 6 | RBAC 引擎（scope 中间件） | M2 |
| 7 | AK/SK 签名中间件 | M2 |
| … | 详见主里程碑 Issue 清单 | … |
