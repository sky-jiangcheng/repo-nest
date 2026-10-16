---
title: API 参考
order: 22
---

# API 参考

Repotory 的对外接口是 **Wails 绑定面**：Go 方法经 Wails Bind 暴露给前端（`window.go.main.App.<方法名>`），方法名与 JSON 载荷即契约。桌面应用默认不监听 HTTP 端口；需要 HTTP 形态时由 `cmd/server` 提供仅限 loopback 的 JSON API（复用同一 service 层）。

> 绑定层是薄委托（`internal/app`），实现全部在 `internal/service`；CLI 与 MCP 复用同一实现。

## 1.7.0 契约变更

- `GetHeatmapData(projectId int64)`：新增参数，`0` 为全局，`>0` 限定该项目的仓库（此前项目详情页误用全局数据）
- 移除从未被前端调用的死方法：`ExportProjectStats`、`ExportHeatmapCSV`、`GetNoteVersion`、`ScanForRepositories`、`RefreshStats`、`RefreshAllStats`、`RefreshProjectStats`

---

## Headless HTTP 服务（`repotory server`）

面向 DeepSeek Harness dsh-plugin 等外部运行时的 JSON API，与桌面应用、CLI、MCP 共用同一 `internal/service` 实现和同一个 SQLite 数据库。

| 端点 | 方法 | 说明 |
|------|------|------|
| `/health` | GET | 服务与数据库健康状态 |
| `/api/ai_context` | GET/POST | 全知识库 Markdown（llms.txt 风格） |
| `/api/search?q=...&all=1` | GET | FTS5 全文搜索（默认仅笔记，`all=1` 含待办） |
| `/api/project/{id}/detail` | GET | 项目 + 仓库及历史统计 |
| `/api/project/{id}/overview` | GET | 知识挖掘结果（README/技术栈/依赖等） |
| `/api/project/{id}/stats?date=` | GET | 某日项目统计 |

> **信任边界**：该服务**无认证**，返回内容即用户完整本地知识库。安全性完全依赖 `cmd/server` 只绑定 `127.0.0.1`——可达性等价于「本机上的另一个进程」。**禁止**改为 `0.0.0.0` 或经反代暴露到网络；需要远程访问时应先补认证方案并提交 ADR。

### 可选：同源托管前端（`--web-dist`）

默认纯 API，与生产一致。加 `--web-dist <目录>`（或环境变量 `REPOTORY_WEB_DIST`）后，同一进程把构建好的前端一并服务，浏览器即可**单源**访问页面与 `/api/rpc`，无需另起开发服务器或跨域代理：

```bash
cd web && npm run build:server              # base /，输出 web/dist-server
cd .. && go build -o repotory-server ./cmd/server
./repotory-server --port 18766 --web-dist web/dist-server
```

- `/api/*` 与 `/health` 永远路由到 API mux（`cmd/server` 的 `isAPIPath` 与 `httpapi` 注册面由测试钉住同步）；其余路径优先匹配静态文件，未命中回退 `index.html`（客户端路由）
- 缺失的静态资源（带非 `.html` 扩展名的路径）返回真 `404`，不回退 HTML——资源缺失不会被伪装成 MIME 错误
- 相对路径在启动时解析为绝对路径，并在日志打印实际服务目录；路径穿越强制回落根，信任边界不变（仍仅 loopback）
- 前端配套只有 `build:server` 一种产物可配合此开关：桌面构建 `npm run build` 的 `web/dist` 是 base `./`（wails:// origin 专用），混用会让页面在一种运行时下白屏

---

## `/api/rpc`（JSON-RPC 桥）

`/api/rpc` 是一个**全量写面**：它把桌面 App 绑定的同一个 `App` 对象（`internal/app` 的全部导出方法）以 JSON-RPC 形式暴露出去，浏览器/独立前端模式下前端 transport 就走这条路。能力等同桌面 UI——下表列出的只读端点之外，`CreateNote`、`DeleteNote`、`UpdateConfig`、`UpdateScanRoots`、`TriggerScan`、`ReloadPlugins` 等全部方法都可经它调用。做安全评估时必须把它计入暴露面。

- **请求**：`POST /api/rpc`，体 `{"method": "<App 导出方法名>", "args": [按位置排列的 JSON 参数]}`；`args` 可省略（缺省参数为零值，`context.Context` 参数以请求上下文填充）
- **响应**：`{"result": <第一个返回值>}`；方法返回 `(T, error)` 且 error 非 nil 时为 `422` + `{"error": "..."}`
- **状态码**：`400` 参数不合法 / `403` 生命周期方法被拒 / `404` 方法不存在 / `405` 非 POST / `413` 请求体超过 1 MiB / `501` 不支持的签名（变参或非 `(T, error)` 双返回）/ `503` 绑定对象不可用
- **生命周期方法黑名单**：`Startup`、`Shutdown`、`Service` 属 Wails 运行时内部钩子，经 RPC 调用返回 `403`（此前一发 `{"method":"Shutdown"}` 即可无崩溃痕迹地关闭整个服务的数据库句柄）
- **请求体上限**：1 MiB（合法载荷——单条笔记/交接记录——远低于此）

---

## 项目

| 方法 | 签名 | 说明 |
|------|------|------|
| `GetProjects` | `(date string, starredOnly bool) → ProjectResponse[]` | 项目列表（默认昨天；今/昨无数据时按需触发单日刷新） |
| `GetProjectDetail` | `(id) → ProjectDetail` | 项目 + 仓库列表及各自历史统计 |
| `GetProjectStats` | `(id, date) → DailyStat[]` | 某日（默认昨天）项目统计 |
| `GetProjectOverview` | `(id) → ProjectOverview` | 知识挖掘：README/技术栈/语言/依赖/贡献者/活跃度/最近提交（缓存于 repo_meta，未命中异步挖掘） |
| `ResolveWorkspacePath` | `(path) → ProjectResolution` | 只读：把一个绝对目录解析为归属 project，以及当该目录落在被扫描仓库内时对应的 repository（四层路径解析）。供 IDE 侧栏显示命中范围；无 MCP 对应面，agent 改为把 `path` 传给 `repotory_context`。见 [AI 集成](../features/ai-integration.md) |
| `SearchProjects` | `(query) → ProjectResponse[]` | 名称/路径模糊搜索（含昨日统计增强） |
| `ToggleStar` | `(id) → bool` | 切换收藏，返回新状态（原子 UPDATE） |
| `UpdateProjectLevel` | `(id, "up"\|"down") → {success, new_level}` | 合并/拆分项目（单事务） |
| `RefreshProjectHistory` | `(id) → {success}` | 回填该项目近 365 天统计 |
| `ListOrganizeGroups` | `() → OrganizeGroup[]` | 知识库整理：列出仍是落点（`import_placeholder` / `default`）、还挂着未归类笔记的项目，含笔记数与「是否有被扫描仓库指向它」。只读，无 MCP 对应面（整理是人的动作，见 [ADR-0023](../adr/0023-note-ownership-knowledge-organize.md)） |
| `ReassignLandingNotes` | `(sourceId, targetId) → int` | 把某落点项目的全部笔记归口到目标真项目（或「未分类」），返回移动条数。源必须是落点项目，目标必须是扫描项目/默认。无 MCP 对应面 |
| `DeleteLandingProject` | `(id, deleteNotes bool) → error` | 删除一个 `import_placeholder` 落点项目；仍带笔记时须 `deleteNotes=true` 显式确认（连带删除其笔记）。默认项目与扫描项目一律拒绝——删除是确认动作，不自动级联。无 MCP 对应面 |

**OrganizeGroup**：`{ project_id, name, root_path, kind, note_count, repo_bound }`。

**ProjectResponse**（节选）：

```json
{
  "id": 1, "name": "my-project", "root_path": "/Users/me/code/my-project",
  "is_starred": true, "repo_count": 2, "project_kind": "scanned",
  "total_added": 1200, "total_deleted": 300,
  "my_added": 800, "my_deleted": 200, "my_files": 15,
  "is_workday": true, "below_standard": false
}
```

## 扫描

| 方法 | 签名 | 说明 |
|------|------|------|
| `TriggerScan` | `() → {success, task_id}` | 异步全量扫描，立即返回 |
| `GetScanStatus` | `() → ScanStatus` | `{running, backfilling, message, progress, total}` |

## 摘要与状态

| 方法 | 签名 | 说明 |
|------|------|------|
| `GetSummary` | `(date) → Summary` | 全局日摘要（团队/个人新增删除、文件、仓库数、是否工作日） |
| `GetHeatmapData` | `(projectId) → {days: HeatmapDay[]}` | 近一年热力图；`projectId>0` 限定项目 |
| `GetStatusBar` | `() → StatusBarData` | 当前时间 + 最近提交（30s 缓存） |
| `GetTodoCounts` / `GetNoteCounts` | `() → counts[]` | 各项目待办（未完成/总数）与笔记计数 |

## 搜索

| 方法 | 签名 | 说明 |
|------|------|------|
| `SearchNotes` | `(query) → SearchHit[]` | 笔记 FTS5 搜索（bm25 + snippet 高亮） |
| `SearchAll` | `(query) → SearchHit[]` | 笔记 + 待办联合搜索 |

## 笔记

| 方法 | 说明 |
|------|------|
| `ListNotes(projectID)` / `ListAllNotes()` / `ListAllTags()` | 项目笔记 / 全局笔记（含项目名）/ 全部标签 |
| `CreateNote(projectID, content)` | 创建（默认 kind=other, source=manual） |
| `CreateNoteWithMeta(projectID, title, content, tags, kind, source)` | 带元数据创建 |
| `UpdateNote(noteID, content)` / `UpdateNoteMeta(noteID, title, tags, kind, pinned)` | 更新内容 / 元数据 |
| `PinNote(noteID, pinned)` / `MoveNote(noteID, projectID)` / `DeleteNote(noteID)` | 置顶 / 迁移 / 删除 |
| `ListNoteVersions(noteID)` | 版本列表（最近 50） |
| `RestoreNoteVersion(noteID, versionID)` | 恢复到历史版本 |
| `DiffNoteVersions(noteID, versionID)` | 版本 vs 当前行级 diff（`+/-/空格` 前缀） |

创建成功会发出插件事件 `note.created`。

## 待办

`ListTodos(projectID)` / `CreateTodo(projectID, title)` / `ToggleTodo(id)` / `DeleteTodo(id)` / `ReorderTodos(ids[])`（单事务重排）。

## 配置

| 方法 | 说明 |
|------|------|
| `GetConfig()` | `{config: map, scan_roots: []}` |
| `UpdateConfig(key, value)` | 允许键：`daily_code_standard` / `scan_depth` / `git_author` / `auto_import`（数值键校验数字） |
| `UpdateScanRoots(roots[])` | 原子替换扫描根列表 |

## AI 导出与插件

| 方法 | 说明 |
|------|------|
| `GenerateLLMsTxt()` | 面向 LLM 的知识库总览 Markdown |
| `ExportNoteAsMarkdown(noteID)` | 带 YAML frontmatter 的笔记 Markdown |
| `GetPluginStatuses()` / `ReloadPlugins()` | 插件加载状态 / 热重载 |
| `GetKnowledgeSources()` | 知识源列表（内置 claude + 插件注册） |
| `TriggerKnowledgeImport(name)` / `TriggerAllKnowledgeImports()` | 触发导入（发出 `import.completed` 前端事件） |
| `ImportClaudeMemory()` | Claude 记忆一键导入 |
| `Health()` | `{status, version}` |

## 事件（Wails → 前端）

| 事件 | 载荷 |
|------|------|
| `import.completed` | `{source, created, updated, skipped, error?}` |

---

## OpenAPI

机器可读契约暂以本页表格为准（此前的手工 openapi.json 已随 D10 移除，待 CI 自动生成后回归）。桌面应用不提供公网 HTTP 服务（见 [TODO](https://github.com/sky-jiangcheng/repotory/blob/master/TODO.md)）。
