# ADR-0022: 自动发现与定时同步 agent 记忆——来源类型用户可见

- 状态：Proposed（方案待评审；其中 CodeBuddy 接线为「解除当前未完成操作」的独立先行项，可不等整体批准即落地）
- 日期：2026-10-14
- 关联：[ADR-0011](0011-multi-agent-memory-importers.md)（多 agent 记忆源导入）、[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（LLM Wiki 知识编译层）、[ADR-0015](0015-w3-minimal-compile-loop.md)（W3 最小编译闭环）、[ADR-0007](0007-session-memory-protocol.md)（会话记忆协议）、[ADR-0009](0009-ide-presence.md)（IDE 存在感）、TODO 会话记忆路线 M2/M8

## 背景

用户诉求（目标环境为 **macOS 真机**，宿主为 Wails 桌面应用；agent 记忆目录随 OS 不同，发现路径按 OS 分表、本 ADR 以 macOS 为准）：本机安装了多种自带对话记忆能力的 agent 工具——CodeBuddy 这类 **IDEA/VS Code/JetBrains 插件**（会话转录与 curated 记忆分别落在 `~/Library/Application Support/CodeBuddyExtension/Data/...` 与 `~/.codebuddy/memery/*.md`），以及 Claude Code / Codex / OpenCode / OpenClaw / Hermes / Cursor 等**命令行 agent**。希望应用能做到四点：

1. **自动发现**本机这些 agent 工具的对话记忆；
2. **定时同步**这些记忆；
3. 把同步内容**组织进应用自有知识库**；
4. 每条知识的**来源类型对用户可见**（如外部导入 / 项目提取 / 用户对话等）。

2026-10-14 现状盘点显示，第 3、4 点已有地基，第 1、2 点是纯缺口：

- **导入管线已就绪（ADR-0011）**：`plugin.KnowledgeImporter` 产出 `[]plugin.ImportDoc{ProjectID,Title,Content,Tags,Kind,Source}`，经 `Runtime.RegisterSource(/Manual)`（`internal/core/plugin/runtime/runtime.go`）注册，`TriggerImport → upsertDoc` 以 `(project_id, source, title)` 幂等 upsert 进 `project_notes`（`source TEXT DEFAULT 'manual'`）。已注册六源：`claude`（AUTO，唯一启动自动导入）、`codex/opencode/openclaw/hermes/cursor`（MANUAL，默认关，仅显式触发）。`internal/service/plugin.go:89 registerBuiltinImporters` 为唯一注册点。
- **CodeBuddy 导入器代码已写完、未接线、未提交**：`internal/importers/codebuddy/codebuddy.go`（621 行，git 未跟踪）实现双源——`codebuddy`（会话转录）+ `codebuddy-memory`（curated 记忆，`codebuddy_project` 目标项目配置），但 `plugin.go` 没有 import 与注册，注释仍写「六个内置源」。这是当前仓库里唯一「半完成」的操作。
- **没有自动发现**：源的可用性靠用户自己知道路径；`SourceStatus.Enabled` 目前只是 `lastErr == nil` 的派生值，不是「本机是否装了该 agent、记忆目录是否可读」的检测结果。用户装好 CodeBuddy 后，设置页不会自己冒出该源。
- **没有定时同步**：只有启动 auto-import（仅 claude）与设置页手动点按；服务层已有 `time.NewTicker` 惯例（`startEmbedDrainer` 5s、`startWikiLintTicker` 6h、page drain、compile poll，均在 `internal/service/`），但知识导入没有周期通道。
- **来源标注只有原始字符串，无分类、几乎不可见**：`project_notes.source` 存 importer 名（claude/codex/…/manual），前端仅 `NoteSection.tsx:274` 给 `source==='claude'` 画了一个 badge，其余来源、`manual`、`handoff`（会话交接）一律不显示；不存在「外部导入 / 项目提取 / 用户对话」的分类概念，也没有按来源过滤的入口。项目扫描（scan 管线）只产出 `repo_meta`（README excerpt + stats，进 llm 上下文），不产知识笔记，因此「项目提取」目前是空类别，需要预留。

## 决策

1. **发现 = 每个源实现 `Probe()`，注册即探活**。在 `plugin.KnowledgeImporter` 上新增 `Probe() ProbeResult`（`Detected bool` + `Detail`：根路径、匹配文件数、错误摘要）；`Runtime` 在 `RegisterSource` 时立即对每个源跑一次只读探活，`SourceStatus` 扩展为 `{name, plugin, detected, enabled, detail}`（`enabled` 保留原语义：上次导入是否成功）。探活沿用 openclaw/hermes 的**硬 scoping 纪律**：只检查各源声明根目录（如 `~/.codebuddy/...`、`~/.claude/...`）内是否存在至少一个记忆文件，绝不读 parent/sibling/子目录之外的内容。设置页来源列表从「列注册表」变成「本机实际发现了什么」。

2. **CodeBuddy 接线即首个新增源（先行项）**。提交 `codebuddy.go` 入库，在 `registerBuiltinImporters` 增加 `RegisterSourceManual(codebuddy.SourceName, codebuddy.New(s.db))` 与 `RegisterSourceManual(codebuddy.MemorySourceName, codebuddy.NewMemory(s.db))`（保持 MANUAL，会话转录隐私敏感，遵守 ADR-0011 决策 4 默认关）；`codebuddy_project` 与 `openclaw_project`/`hermes_project` 同列设置页「全局记忆源目标项目」输入组；同步更新 `plugin.go`/`runtime.go` 注释「六个」为「八个」。Cost：约一个提交 + 十行注册，最快解除「未完成操作」。

3. **定时同步 = 服务层 sync ticker + 逐源持久化开关，默认关**。新增 `internal/service/knowledge_sync.go`：`startKnowledgeSyncTicker`（沿用 `time.NewTicker` 惯例，**首跳推迟一个周期**，避免启动即全量导入，同 `startWikiLintTicker`）。参数：
   - 周期配置 `knowledge_sync_interval_minutes`（默认 `240`，即 4 小时）；
   - 逐源开关 `sync_source_<name>`（`'1'`/`'0'`，默认 `'0'` 关；`claude` 延续 AUTO 语义默认 `'1'`）——**隐私默认关**是 ADR-0011 决策 4「每个源默认关、显式开」向周期通道的延伸；
   - 每轮同步 = 对所有 `sync_source_*='1'` 的源依次 `TriggerImport`（`importMu` 已串行，与设置页手动导入互斥不重复）；
   - 结果走既有 `emitImportEvent → import.completed` toast，并在 `SourceStatus` 带上 `lastRun`/`lastError` 供设置页展示上次同步时间与错误。

4. **来源类型 = 服务端派生分类**。新增枚举（不新增 DB 列，`project_notes.source` 已存在，分类是展示/过滤用的派生语义）：

   | 来源类型 | 含义（用户可见文案） | 映射的 source 值 |
   |---|---|---|
   | `agent-import` | 外部导入（agent 记忆导入） | `claude, codex, opencode, openclaw, hermes, cursor, codebuddy, codebuddy-memory` |
   | `user-conversation` | 用户对话（会话交接沉淀） | `handoff`（现有会话交接标签笔记）、未来结构化对话导入 |
   | `project-extract` | 项目提取（从项目/仓库提取） | 预留空类别；当前扫描只产 `repo_meta` 不产笔记，未来 README/代码摘要等经编译成笔记时归此类 |
   | `manual` | 手动（用户手写/编辑） | `manual` 及用户直建笔记 |

   实现为 `internal/service`（或 `internal/core`）单一 `SourceType(source string) (type, label)` 函数，未知 source 兜底显示其原始字符串、类型归 `manual`；前端通过现有 bindings 拿 `{source, source_type, label}`，i18n 文案走 react-i18next。

5. **来源用户可见 = 三层 UI**：
   - **笔记卡片**：`NoteSection.tsx` 现有 claude 专属 badge（`badgeSource`）泛化为「来源 badge」——所有 source 按类型显示 label（如「CodeBuddy 导入」「会话交接」「手动」）+ 类型色调，与现有 kind badge 并列；
   - **设置页来源列表**：`PluginsTab.tsx` 每源一行增强为：发现状态（已检测/未检测 + 根路径）→ 来源类型 → 定时同步开关 → 立即导入 → 上次同步时间；全局记忆源的目标项目输入随决策 2 增加 `codebuddy_project`；
   - **知识库浏览/检索**：笔记列表、搜索结果复用同一来源 badge 组件，用户在任何看到知识的界面都能回答「这条知识来自哪里」。

6. **组织进自有知识库 = 保持两层管线，不新建存储**。导入 → `project_notes`（现有 upsertDoc）→ M6 wiki 编译（ADR-0014/0015）产出 wiki page（`page.source` 继承笔记 source）；来源类型在 notes 与 pages 两层都可见。本 ADR 不新建表、不重建索引、不动 `embed-drainer`/向量检索（同步产生的新笔记自然经 `note_embed_dirty` 队列进入检索）。

## 理由

- **复用 > 新建**：发现（Probe）= 每个源一个只读函数；同步 = 一个 ticker + 一组配置；来源 = 一个派生函数 + badge 泛化。全部落在 ADR-0011 管线与服务层 ticker 惯例内，不碰 schema、不碰分层冻结面（ADR-0006）。
- **隐私默认关延续既有纪律**：定时同步是「周期读取 agent 私人记忆目录」的高敏感能力，逐源显式开启与 ADR-0011 决策 4 一致，避免「打开应用就自动把我所有 agent 的对话导进来」。
- **来源分类用派生映射而非 schema 迁移**：`source` 字符串已是版本化事实（v23 及更早），分类是展示语义；单一派生函数保证服务端与前端一致，未来确有过滤/统计需求时再物化为列（见待决）。
- **CodeBuddy 先行顺理成章**：代码已写完，只差接线；其双源形状（会话转录 + curated 记忆）正好覆盖「发现 + 定时同步」两种形态的试点，也直接解除仓库当前唯一未完成操作。

## 后果

- **正面**：设置页从「我知道有哪些源」变成「应用告诉我本机装了什么」，新装 agent 后无需手写路径；定时同步让 agent 记忆增量持续进入知识库并随现有 embed-drainer 进入语义检索；来源类型在笔记、页面、设置三处用户可见，满足「这条知识哪来的」的一等追问。
- **负面**：注册时探活增加一次只读 IO（每个源一次 stat + 少量文件列举，可接受）；定时同步周期性占用 IO/CPU，与开启源数量成正比，靠间隔配置与逐源开关控制；来源分类是人工枚举，新源上线需同步登记映射，否则落入 `manual`/原始字符串兜底。
- **待决**：① 是否提供「全局一键开启所有源同步」——倾向否，逐源开关更符合隐私默认关；② 是否在搜索/列表加「来源类型」筛选与统计——等真实需求信号；③ `source_type` 是否物化为列（schema v24）——当按类型过滤成为高频路径时再考虑；④ 同步失败是否加 backoff 重试——先沿用 `TriggerImport` 单次语义 + `lastErr` 展示，观察真机表现。

## 落地安排（分阶段，供评审）

- **P1 接线（先行）**：`git add internal/importers/codebuddy/codebuddy.go`；`plugin.go` 注册双源 + 注释更新；`runtime.go` 注释更新；`PluginsTab` 目标项目组加 `codebuddy_project`。验证：`go build ./...`、`go test ./internal/importers/codebuddy/ ./internal/core/plugin/runtime/`；真机跑一次 CodeBuddy 双源导入（需本机存在 CodeBuddy 数据）。
- **P2 发现（Probe）**：runtime 增加 `Probe` 契约与 `SourceStatus.detected/detail`；七源（claude/codex/opencode/openclaw/hermes/cursor/codebuddy×2）各实现只读探活（多数是检查根目录 + 至少一个样本文件，复用各源既有 golden-file 样本）；设置页展示发现状态。
- **P3 定时同步**：`knowledge_sync.go` ticker + `knowledge_sync_interval_minutes` + 逐源 `sync_source_<name>` + 设置页开关与 lastRun 展示。
- **P4 来源类型与 UI**：`SourceType` 派生函数 + 测试（钉住上表映射 + 未知兜底）；NoteSection 来源 badge 泛化；设置页每源类型/开关信息增强；i18n 文案。
- **P5 验证与收口**：golden-file 回归保持（ADR-0011 决策 5）；`go test ./internal/...`；GUI 真机冒烟（源列表发现 → 开启某源定时同步 → 笔记 badge → 设置页 lastRun）；本 ADR 的晋升与实施修正记录按项目惯例在落地后回填。