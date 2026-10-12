# ADR-0005: 服务层重构（service / app / domain 分层）

- 状态：Accepted（第 5 节「版本与数据路径」的两条已被 2026-09-01 的品牌更名取代，见文末「修订」）
- 日期：2026-08-17（实现于 1.7.0）
- 关联：[ADR-0002](0002-c-end-repositioning.md)、[ADR-0001](0001-plugin-platform.md)（Superseded 的 M1-M4 分层设想）

## 背景

1.7.0 前的代码形态：根目录 `package main` 堆积 14 个 handler 文件（约 1900 行业务逻辑），CLI（`cmd/repotory`）与 MCP（`cmd/mcp`）各自用 `internal/db` 重复实现"同一查询 + 同一格式化"；存在两条竞争的扫描管线、5 处近乎相同的统计刷新循环、1195 行单体 `queries.go`。探索性深挖还发现两个被重复代码掩盖的生产 bug（repository 幽灵列、go.mod 块解析 panic）。

## 决策

### 1. 三层分工

```
internal/app      Wails 绑定层：每方法 1-3 行委托（transport glue）
internal/service  业务核心：唯一有权访问 db 与 git Provider 的层
cmd/*             CLI / MCP：service 的另一种 transport
```

Wails、CLI、MCP **共享同一 service 实现**。UI 层（app）不接触 `*sql.DB`。

### 2. 移除 storage 接口垫片（推翻 M1 遗留）

`internal/core/storage` + sqlite 适配层（约 500 行）是纯转发垫片，且 service 一半调用绕过它——保留只会把"混合访问风格"问题下移一层。**删除**。理由：ADR-0002 已确定本地优先 + SQLite 单后端，PostgreSQL/ES 是已废弃 RFC（ADR-0001）的 M4 设想；真正有抽象价值的接缝是 `core/git.Provider`（未来远端 Git 托管），保留。

### 3. 领域类型独立

行类型移入 `internal/domain`，db 以类型别名兼容（`db.Project = domain.Project`），消除 storage→db 的反向类型依赖。LCS diff 移入 `internal/diff`。

### 4. 事务与管线归位

- 项目升降级的手写 SQL 事务从 handler 下沉为 `db.SplitProjectDown` / `db.MergeProjectUp`（可测试）
- 扫描唯一管线：scanner → grouper → db 事务同步 → 清理 → 刷新；分组唯一实现在 grouper
- 5 处刷新循环合并为 `service.refreshRepoStatsRange`（取消感知 + all/mine 双作者行）

### 5. 版本与数据路径

- 版本号 SSOT：`internal/version.Version`（app/CLI/MCP/agent-score 共用）
- **用户数据路径不变**（DB/日志文件名/插件目录沿用 `repotory` 目录名）——品牌更名（Repotory）不迁移数据
- Go module 路径保持 `repotory`（避免全库 import 改写；见 TODO.md 更名计划）

## 后果

- 正面：CLI/MCP 与桌面端行为永远一致；根 package 只剩 main.go；新功能只需实现一次；单测可直接打在 service 上（fake git provider）
- 负面：db 别名层是暂时兼容物，新代码应直接用 domain 类型；service 直接持有 `*sql.DB`（诚实反映单后端现实，若未来真需要多后端需重新引入仓储接口）
- 遗留：品牌/二进制更名、前端 `useApiData` 接入、ESLint TS7 阻塞等见 [TODO.md](../../TODO.md)
