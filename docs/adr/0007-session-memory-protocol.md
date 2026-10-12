# ADR-0007: 会话记忆协议（context / handoff 双工具）

- 状态：Accepted
- 日期：2026-09-28
- 关联：[ADR-0006](0006-scope-freeze.md)（核心闭环优先）、[ADR-0005](0005-service-layer.md)（服务层统一）

## 背景

ADR-0006 将核心闭环定为「发现 → 理解 → 记录 → 检索 → AI 使用」，但闭环的两端在 agent 视角下是断的：

1. **会话开始**：agent 拿到一个新任务后，需要自己链式调用 `projects_list` → `notes_search` → `notes_read` 3-4 次才能拼出项目上下文；多数 agent 直接跳过这一步，带着零上下文开工。
2. **会话结束**：agent 学到的东西（决策、坑、下一步）随会话消失。`notes_create` 存在但没有协议——agent 不知道何时写、写什么结构，人类也无法预测产出格式。

「AI 项目记忆层」的叙事要成立，记忆必须在**会话边界自动发生**，而不是依赖用户记得去保存。

## 决策

新增三个 MCP 工具，构成会话记忆协议（含冷启动）：

### `repotory_context`（会话开始）

一次调用返回项目完整上下文的 Markdown 文档：

- 技术栈 / README 摘要 / 语言占比 / 依赖 / 贡献者 / 活跃度（来自 `repo_meta` 挖掘缓存，未缓存时后台异步挖掘并在文档中声明）
- 最近提交（实时 git log）
- 开放待办
- 高相关知识笔记；**带 `handoff` 标签的笔记排最前**（它们记录上次会话如何结束）

项目解析协议（`ResolveProject`）：`project_id` 精确解析 → `project_name` 名称/路径模糊匹配 → 无参且仅一个项目时自动解析。多匹配时返回项目目录（含 ID 表格）让 agent 选择，绝不猜测注入错误项目的上下文。

### `repotory_handoff`（会话结束）

结构化交接协议，入参为 `summary`（必填）+ `changes` / `decisions` / `gotchas` / `next_steps`（数组，至少一项非空）+ `agent`（写入方标识）。渲染为固定 Markdown 模板落库（`kind=knowledge`、`source=mcp`、tags 自动含 `handoff`）。

固定模板是刻意的：标题结构是写入方与所有未来读者（人类 + agent）之间的契约，`repotory_context` 靠 `handoff` 标签将其排序置顶。

### `repotory_scan`（冷启动）

会话记忆协议要成立，前提是数据库里已有项目。原先只有桌面应用能完成「播种扫描根 + 扫描」，导致文档宣称的「MCP 不需要桌面应用」是空话——纯 MCP 安装的 `repotory_context` 只会返回「无项目」。为此把扫描能力下沉到服务层并暴露为第三个工具：

- `Service.EnsureDefaultScanRoots`：首次运行播种平台默认扫描根（`scan_roots_seeded` 标记；已有扫描根时不覆盖）；
- `Service.ScanNow`：同步执行完整扫描管线并返回仓库/项目计数（`TriggerScan` 保留给桌面端的后台异步场景，二者共用 `scanning` 互斥）；
- `repotory_scan`：调用两者，使首次可用漏斗从「安装→扫描→收藏→刷新→使用」压缩为「安装→`repotory_scan`→`repotory_context`」。

桌面端首启也改为复用 `Service.EnsureDefaultScanRoots`，两端行为一致。

## 理由

- **零参数即可用**：单项目安装直接 `repotory_context()` 拿全上下文，把「记忆加载」的成本降到一次调用。
- **跨 agent**：交接落在本地 SQLite 而非任何 agent 私有记忆格式，Claude Code 写的交接 Cursor 直接读。
- **服务层共享**：三个工具实现在 `internal/service`（`context.go` / `handoff.go` / `scan.go`），桌面端复用同一实现（ADR-0005 分层）。
- **多匹配不猜测**：错误上下文比没有上下文更危险，歧义时返回目录由 agent 二次选择。

## 后续方向（未决）

- 会话自动捕捉：解析 `~/.claude/projects/*/*.jsonl` 生成会话摘要（零人工参与，需评估隐私与体积）
- 多源记忆导入：Cursor、Codex、OpenCode 的记忆格式
- 语义检索：本地 embedding（评估 sqlite-vec），补充 FTS5 的字面匹配盲区
- Claude Code hook 集成文档：SessionEnd hook 自动触发 `repotory_handoff`
