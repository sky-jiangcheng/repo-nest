# ADR-0011: 多 agent 记忆源导入——可复用的 importer 框架与各源可行性（M2）

- 状态：Proposed（框架已存在；**Codex、OpenCode、OpenClaw、Hermes(curated) 已落地**为 opt-in 手动源；Cursor 核验后暂缓、Hermes sessions 待 schema）
- 日期：2026-10-02
- 关联：[ADR-0002](0002-c-end-repositioning.md)（进程内插件运行时）、[ADR-0007](0007-session-memory-protocol.md)、[ADR-0010](0010-session-auto-capture.md)、TODO 会话记忆路线 M2

> **2026-10-11 锚点审计注**：本文代码锚点为 2026-10-02 决策前的现状快照（importer 框架与各源后续落地后多处已变），只作历史动机记录、不随代码漂移更新。

## 背景

RepoNest 已有一条干净的「外部记忆 → 知识库笔记」管线：`plugin.KnowledgeImporter` 产出 `[]plugin.ImportDoc`（`ProjectID/Title/Content/Tags/Kind/Source`，见 `internal/core/plugin/plugin.go`），经 `Runtime.RegisterSource`（`internal/core/plugin/runtime/runtime.go`）注册为内置源，导入时由 `TriggerImport → upsertDoc` 以 `(project, source, title)` 幂等 upsert 成笔记（`internal/service/plugin.go:100` 把 Claude 源接进去）。目前只有**一个**具体源：`internal/importers/claude`（读 `~/.claude/projects/*/memory/*.md`）。

M2 要把同一管线扩展到 Cursor / Codex / OpenCode 等。但 2026-10 现状盘点显示这些源的落盘形态**差异极大且多数非公开契约**：

- **Codex CLI**：会话以逐行 JSON 事件落盘（与 Claude Code 的 `*.jsonl` 同类，尾部 assistant 消息 + tool 调用），相对可解析，但路径/字段随版本漂移。
- **OpenCode**：会话/历史结构有第三方解析项目公开映射（claude-replay、obsidian-aisync、cli-continues 等），可按其已知形状适配。
- **Cursor**：聊天/Agent 历史主要存在 **`state.vscdb`（一个未公开的 SQLite 库里的序列化 blob）**，非文档化、跨 workspace、随版本重构——是四者中最脆、最不官方的一种。

结论：M2 的真正风险不是「写解析器」，而是**给未公开、易漂移的外部格式背长期维护债**。

## 决策

1. **不新建机制，复用现有 importer 管线**。每个新源都是一个实现 `KnowledgeImporter` 的 Go 原生 importer，走 `RegisterSource` + `upsertDoc`，与 Claude 同构（幂等、`Source` 字段区分来源、匹配到已有 project）。`ImportClaudeMemory` 已是范式样板。
2. **抽一层薄公共件，而非抽象大框架**：把「按 project 匹配目标 ProjectID」「ImportDoc 构造 + kind/tag 约定」「逐文件宽松失败」从 claude.go 里提出为 `internal/importers` 下的小 helper，供各源复用。**只在第 2、3 个源真的落地时才抽**，避免为想象中的通用性提前抽象。
3. **按「格式稳定性 × 获客价值」排序推进**，一次只立一个源，每个源立项前先做一次「拿真机样本验证格式」的门禁：
   - 先 **Codex**（与 Claude jsonl 最近、价值高）；
   - 再 **OpenCode**（有公开形状可循）；
   - **Cursor 缓行**：`state.vscdb` 属逆向、易碎，除非有明确需求信号，单独立项并在文档显著标注「非官方、随时可能失效」。
4. **每个源默认关、显式开 + 可 dry-run**：设置里逐源开关，导入前可预览将产生的笔记条数/标题；失败源在 `SourceStatus`（`runtime.go`）里如实报错，不静默吞。
5. **golden-file 回归锁格式**：每个源带一组「真实样本 → 期望 ImportDoc」的测试（Claude 已有 `claude_test.go`），上游改格式时测试先红，避免线上静默产出脏笔记。

## 理由

- 复用现成管线使每个新源是「一个 importer 文件 + 一份测试」的增量，不碰 service/app/domain 分层与冻结面（ADR-0006）。
- 反爬式解析未公开格式是纯维护负债；用「先验证样本再立项、一次一个、默认关」把风险面切到最小，Cursor 用最保守处置。
- golden-file 把「格式漂移」从线上事故降级为测试失败——对易变外部源这是唯一可持续的工程手段。

## 后果

- 正面：多 agent 记忆可汇入同一知识库与 `reponest_context`；新增源成本随公共件下沉递减。
- 负面：+N 个外部格式适配器，每个都绑定上游版本；需持续维护真机样本测试。
- **待决**：① project 匹配规则是否对所有源统一（Claude 现按路径/slug 匹配目标项目，Codex/Cursor 的项目边界未必同名）；② Cursor 是否值得做，或改走「让用户导出为 markdown 再导入」的离线兜底；③ 是否需要一个「记忆源」设置页的统一开关面板。

## 落地进度与各源真机核验（2026-10-02）

产品决策：M2 **全部源都做**（覆盖 claude/codex/opencode/cursor/openclaw）。逐源按 ADR-0011 决策 3 的真机格式核验结果：

- **claude**（已有）：读 `~/.claude/projects/*/memory/*.md`，AUTO 源（人工整理的记忆文档，可启动自动导入）。
- **codex**（已落地，opt-in MANUAL）：`~/.codex/sessions/**/rollout-*.jsonl` 流式解析，每会话→一条 log 笔记。真机核验：session_meta.payload.cwd + response_item message content 块。
- **opencode**（已落地，opt-in MANUAL）：`~/.local/share/opencode/storage/session/<hash>/ses_*.json`，字段已自带 `{id,projectID,directory,title,summary,time}`（已是摘要级，无需解析逐条消息），`directory` 末段驱动项目匹配。真机核验通过（v1.1.36 样本）。
- **cursor**（**核验后决定暂缓，不实现**）：真机只读核验 `state.vscdb`（`ItemTable`/`cursorDiskKV`/`composerHeaders` 三表）。**结论：低 ROI + 高脆弱，暂缓**——① 线程索引 `composerHeaders` 有干净列（composerId/workspaceId/时间/isSubagent），但**正文分散在 `cursorDiskKV` 的 `bubbleId:<id>` blob、由版本化 headers（`"value":17`）串联**，composer 输入又是 ProseMirror 文档树，逐条还原=多表 join + 内部版本 schema；② **项目归属拿不到**：`workspaceId` 常为 `empty-window`、DB 内无 folder 路径映射；③ 本机仅 4 条且全是空 draft，**无法对真实数据校验** parser。按 ADR-0011 决策 3/5（不背未公开易碎格式的长期债）与「先核验真机格式再写」的纪律，判定为当前**不值得**投入；有真实需求信号 + 能拿到稳定样例时再立项，届时也需 `cursor_project` 显式定向 + 默认关 + 标非官方契约。
- **openclaw**（**已落地，opt-in MANUAL**）：记忆 = `~/.openclaw-autoclaw/workspace/{IDENTITY,SOUL,USER,AGENTS,HEARTBEAT,TOOLS}.md`（真机核验 + 与 RepoNest 自身 awareness 同名同形）。**安全红线**：parent `~/.openclaw-autoclaw/` 含私钥（`office-plugin-tls/*.pem`）/`vault-roots.json`/`identity/`/`client-sign.json`——importer **硬 scoping 到 `workspace/*.md` 单层非递归**，绝不读 parent/子目录/非 md（有 allowlist 单测：parent `.md`、子目录 `.md`、非 md 一律不导入）。版本日期式 `lastTouchedVersion: 2026.x`，**无 1.0/2.0 判别位**→「只做 2.0」＝按当前布局实现。**全局记忆的项目归属**：走 `openclaw_project` 配置（项目名或 id），未设/未知 → ProjectID 0（被管线 skip），不硬塞、不乱撒。
- **hermes**（**已落地 curated-memory 层，opt-in MANUAL**）：**Nous Research 独立产品**（`com.nousresearch.hermes`），与 OpenClaw 两家——早先误判为 OpenClaw 运行时，已更正。经官网文档核验：root `~/.hermes/`（`$HERMES_HOME` 可覆盖），**curated 记忆 = `~/.hermes/memories/{MEMORY.md,USER.md}`**（Markdown）。同目录含 `.env`（密钥）/`mcp-tokens/`/`state.db`→**硬 scoping 到 `memories/*.md` 单层非递归**。项目归属同 openclaw（`hermes_project` 配置）。**Sessions 未导入**：`~/.hermes/sessions/` + `state.db` 记录 schema 无文档且本机不可核验，按「不猜活格式」纪律**暂缓**（见待决）。
