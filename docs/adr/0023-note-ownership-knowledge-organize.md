# ADR-0023: 笔记归属与知识库整理——导入不再静默丢知识，漂着的知识有落点、可归口

- 状态：Proposed（**2026-10-16 设计经用户批准；S1–S5 已落码**：schema v24、导入落点、清理豁免、点笔记直达、知识库整理 UI + 3 个桌面专属 RPC 全部实现，`go test ./...` / `-race` / web build/test/lint / build-docs 全绿。晋升 Accepted 仍只差**条件 5 的真库副本演练**（由用户亲自跑）。三个开放点已定：新列 + Go 层枚举校验（不内联 CHECK）、删除只做「手动可选」不自动级联、整理以项目级归口为主 note 级为辅。codebuddy-memory 经用户要求并入统一落点。落码得到的与设计不同的事实见文末「实现修正」节）
- 日期：2026-10-16
- 关联：[ADR-0011](0011-multi-agent-memory-importers.md)（多 agent 记忆源导入——本 ADR 处理它「匹配不到项目」那条未定义支路）、[ADR-0007](0007-session-memory-protocol.md)（会话记忆/handoff——同一批外部知识的另一入口）、[ADR-0015](0015-w3-minimal-compile-loop.md) / [ADR-0019](0019-entity-resolution-merge.md)（「合并/归口永远由人触发」的先例——整理与「删占位连带删笔记」都继承这条）、[ADR-0006](0006-scope-freeze.md)（范围冻结——本需求属核心「记录→检索」闭环，非扩界；但属大功能，按 CONTRIBUTING 先讨论并经用户批准）、[ADR-0021](0021-temporal-graph-page-versions.md)（「副本先行」迁移纪律对本 ADR 的 v24 继续有效）；起点 schema **v23**
- 先例：`SplitProjectDown`（`internal/db/projects.go:209`）已用 `filepath.Base(repo.Path)` 建项目——「用仓库名建落点项目」的形状现成；`MergeProjectUp`（`projects.go:293`）与 `MoveNote` 已有「把笔记换到另一个项目」的 UPDATE——「整理/归口」不发明新搬运机制，复用同款

## 背景

用户的原始痛点是「笔记跟项目关联性不直接」。把这句话拆到代码上，暴露出四个各自独立的问题：

1. **导入匹配不到项目时，知识被静默丢弃，而不是变成「无头笔记」**。`project_notes.project_id` 是 `INTEGER NOT NULL` + 外键（`internal/db/db.go:193-205`），根本没有「无归属笔记」这种状态。importer 匹配不到项目返回 `ProjectID: 0`（`internal/importers/memsrc/memsrc.go:25`），runtime 在 `internal/core/plugin/runtime/runtime.go:227-229` 直接 `if d.ProjectID <= 0 { run.Skipped++ }` 丢掉。所谓「无头笔记」在库里从未存在——2026-10-14 事故里 AI1 丢的 41 条就是这类被丢弃知识的一部分。
2. **知识库点笔记只跳到项目、看不到那条笔记本身**（`web/src/pages/knowledge/KnowledgeCard.tsx:48` 只带 `project_id`）。
3. **项目无法表达「它来自哪个仓库 / 是不是导入占位」**：`projects` 表没有任何 kind 或 repo 绑定列（`internal/domain/types.go:9-19`，硬编码列清单 `projectColumns` 在 `projects.go:13`）；「全部项目」是前端筛选哨兵 `value=0`（`web/src/pages/Review.tsx:227`），不是真实行；库里也**没有「默认项目」行**——`insertDefaults`（`internal/db/migrate.go:544`）只种 `app_config`，不种项目。
4. **清理逻辑两头不讨好**：`CleanupStaleDataTx`（`internal/db/cleanup.go:50-54`）只删「无 repo **且**无 notes **且**无 todos」的项目——占位项目一旦仓库从扫描里消失却还挂着笔记，会**永久滞留**；反过来，一个还没被用过的**空「默认项目」会被这段逻辑误删**。

「导入找不到项目也该有落点、点笔记要直达笔记、漂着的知识要能归口」是一组耦合需求：它们共享同一处数据模型缺口（项目缺少「来源与角色」的表达）。本 ADR 定这个缺口的形状与落点策略，不落码。

## 参照物（抄机制、不抄栈）

- **自己的 `wiki_pages` 全局档**：页面层早有 `project_id` 可空 + `COALESCE(project_id,0)` 读出的「GLOBAL tier」（`internal/db/wiki.go:46,241`），且 `internal/app/bindings.go:528` 注明「projectID 0 是 GLOBAL tier，不是 every project」——这正是笔记层若走「可空/哨兵」路线的对标物；本 ADR 恰恰**不**选这条，理由见候选矩阵。
- **自己的 `SplitProjectDown`**：以仓库基名建项目、以仓库路径作 `root_path` 的先例，占位项目直接复用。
- **自己的 `note_versions` 人审门 / ADR-0019 合并**：破坏性收敛（合并、删笔记）永远由人触发，不做自动。

## 决策

**1. `projects` 加一列 `project_kind`，取值 `scanned` / `import_placeholder` / `default`。** 迁移用 `ALTER TABLE projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'scanned'`（常量默认满足 SQLite 对 ADD COLUMN 带 NOT NULL 的要求）。**取值集合在 Go 层校验，刻意不给 `projects` 加内联 CHECK / 表级约束**——给已有表改/加 CHECK 会触发整表重建（本仓既有教训，见 MEMORY / ADR-0021 实现修正），一个纯分类列用 Go 校验足够。加列的**硬失败点**（漏一处即编译或运行炸）：`projectColumns` 常量（`projects.go:13`，被 `GetAllProjects`/`GetStarredProjects`/`GetProjectByID`/`SearchProjects` 四处 Scan 共用）+ `domain.Project` struct（`types.go:9-19`）必须一并扩列，且 `internal/integrity/checks.go:49` 的 `ExpectedSchemaVersion` 从 23 推进到 24、可逆迁移测试跟上，否则全量 `go test -race` 红。

**2. 迁移里幂等种子一行 kind=`default` 的「未分类」项目** 作为兜底落点与整理目标。其 `root_path` 取一个真实文件系统路径永不可能产生的合成哨兵（推荐 `repotory://memory/default`），既满足 `root_path NOT NULL UNIQUE`、又不与任何扫描仓库碰撞。展示名「未分类」由前端按 `kind='default'` 本地化渲染，**不把 locale 键写进数据列**（数据层存一个内部固定名即可，双语只发生在界面）。种子用 `INSERT ... WHERE NOT EXISTS (SELECT 1 FROM projects WHERE project_kind='default')` 保证幂等。

**3. 导入落点改为「混合」模型，消灭静默丢弃。** 给 `plugin.ImportDoc`（`internal/core/plugin/plugin.go:45`）增加 `RepoPath` / `RepoName` 字段（各源从 `cwd` / `folder` / 目录变形名等派生，`memsrc` 已有 `LastPathSegment`）。把 `runtime.go:227` 那个 `ProjectID<=0 → skip` 分支改为落点判定（判定放在 runtime，因为那里持有 `r.db`，而 `memsrc.MatchProject` 只吃切片、无 `*sql.DB`）：
- **有仓库路径**的源（codex / cursor / claude / opencode / codebuddy）→ 调新 db 助手 `EnsureImportPlaceholderTx`，按 `root_path` upsert 一个 `kind='import_placeholder'` 项目并回填 `doc.ProjectID`；
- **无仓库路径**的全局记忆源（openclaw / hermes / codebuddy-memory，本就只有可配置的目标项目、无 per-project 路径）→ 回填默认项目 id。**codebuddy-memory 由此并入统一落点**：它此前在「未配置目标」时会 `return docs, nil` 有意早退丢弃（其 importer 注释 + `memsrc.TargetProject` 的「未配置→0→skip」旧语义），现改为落「未分类」——那条护栏的本意是别猜错项目，而中性默认桶不是猜测。

从此「匹配不到」不再等于「丢」，而等于「落到占位/默认、等待整理」。历史上被丢弃的那批，用户重跑「设置→立即导入」会以占位项目重新入库（`upsertDoc` 的 source+title 幂等键使重导是更新而非重复）。

**4. 占位项目的「自动转正」语义，以及它的天然例外。** 占位项目的 `root_path` = 真实仓库路径，而 `root_path` 上有 UNIQUE——若该仓库日后被扫描发现，`SyncProjectTx`（`projects.go:121`）会命中同一行。给 `SyncProjectTx` 增加一条：命中既有 `kind='import_placeholder'` 行时，把 `kind` 提升为 `scanned`（占位无缝转成真实项目，笔记原地留存）。**例外**：claude 的标识是目录变形名（`-Users-…-Proj`，`internal/importers/claude/claude.go:115`）未必等于真实仓库路径，转正不了，只能靠整理人工合并——这一点在文档里写明，别当缺陷去修。

**5. 新增「知识库整理」，主粒度是项目级归口。** 一个界面列出挂在 `kind IN ('import_placeholder','default')` 项目下的笔记分组。**主操作**：一键把某个占位/默认项目的**全部笔记**并入用户选定的目标真项目（复用 `MergeProjectUp` 里 `UPDATE project_notes SET project_id=? WHERE project_id=?` 这条语句与 `MoveNote` 契约）；单条例外可降级到 note 级移动。清空后的**空占位项目**允许删除。整理交互复用 `Review.tsx` 批量 approve 的骨架，不新造队列。

**6. 删除/清理语义：默认项目受保护，连带删除只做「手动可选」。** `CleanupStaleDataTx` 增加「`kind='default'` 永不删」豁免（堵住决策 2 的种子被自己清空后误删）。占位项目：仍按现规则（空了就删），但当它「有笔记、无对应仓库行」时——即源仓库从扫描中消失——在整理界面标记为「源仓库已消失」，由用户**手动**选择「连同其笔记一并删除」。**不做自动级联删除**（撞用户「先确认再删」的一贯偏好，也违反文件/数据保护：破坏性动作必须是本人拍板的可选项）。

**7. 迁移形态：单开 v24。** 与 ADR-0021 倾向「合批」不同，本 ADR 有**即时的 schema 需求方**——`project_kind` 列与默认项目种子就是这次功能的地基本身，等不起一次「未来的 CHECK 重建」。因此新开 v24，纯增量（1 列 + 1 种子行 + 触发器/清理谓词调整）。

## 候选矩阵（无头落点怎么表达）

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| 真·可空 `project_id`（笔记可不属于任何项目） | 远 | 概念上最「无头」 | 破 `NOT NULL`+FK 后，FTS / embedding 队列 / `ListNotes` join / 级联 / 所有 `project_id>0` 假设全要改；改动面大且没有对应产品收益 |
| `project_id=0` 全局档哨兵（对齐 `wiki_pages`） | 中 | 与页面层一致 | AUTOINCREMENT 从 1 起、FK 无 `id=0` 目标 → 要么插伪行要么拆外键；且前端「全部项目」已占 `value=0`、与 `bindings.go:528` 的「GLOBAL tier ≠ every project」语义已经打架，再把笔记 `0` 塞进同一空间会更糊 |
| **本决策：占位项目 + 默认项目（`project_id` 恒合法非空）** | 近（复用 `SyncProjectTx`/`SplitProjectDown`/`MoveNote`/`MergeProjectUp`） | 不破 schema 约束、笔记恒有归属、给漂着的知识一个明确落点与整理出口、「项目从哪来」第一次在数据里可见 | 会「长」出一批占位项目行，需 kind 区分与整理收敛；claude 类源不自动转正 |
| 维持现状（静默丢弃） | 0 | 省事 | 持续丢用户知识（已实证丢 41 条），正是本需求要消灭的对象 |

## 后果

**正面**：导入零静默丢失（匹配不到 → 占位/默认落点）；点笔记从「跳了个空项目」变「直达那条笔记并高亮」；漂着的知识第一次有明确落点与归口出口；`project_kind` 让「这个项目是扫来的 / 导入占位的 / 系统默认」在数据层可判，为后续「编辑项目关联仓库」预留了表达位（本轮不做编辑，见待决）。

**负面（不粉饰）**：
- `projects` 多一列 + 种子一行默认项目：触 `ExpectedSchemaVersion` 与可逆迁移测试；v24 碰真库必须按「副本先行」纪律（ADR-0021 修订记录强调它对**每一次**后续迁移继续有效），演练由用户亲自跑。
- 会凭空多出一批占位项目，整理不及时仪表盘会「变脏」——靠 `kind` 区分 + 整理界面收敛，但视觉噪声是新成本。
- 占位项目与扫描真项目可能「同名不同路径」并存（claude 变形名、或仓库被移动），需整理人工合并，非全自动。
- 「源仓库消失 → 连带删笔记」是手动项：残留数据要用户主动清，不自动。这是刻意用「整洁」换「安全」，符合本仓对破坏性动作的一贯姿态。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | 占位项目去重（同仓库多次导入、同名不同路径） | 按 `root_path` upsert 天然去重——`root_path` 相同才算同一个；不同路径视为不同来源 |
| 2 | 默认项目「未分类」可否改名 / 可否被整理掉 | 允许改显示名；锁定**不可删、不可作为归口源被清空后消失**（它是兜底落点，`CleanupStaleDataTx` 已豁免） |
| 3 | 整理界面挂载点 | 并入知识库页一个「整理」子视图，复用 `Review.tsx` 批量交互骨架，不新开顶级页 |
| 4 | 「源仓库已消失」的判定信号 | 以「占位项目有笔记但 `repositories` 无对应 `project_id` 行」为准——纯查询、无需新表、幂等 |
| 5 | 本轮是否顺带做「编辑项目关联仓库」UI | **不做**（用户已选仅「展示 + 占位区分」）；`project_kind` + `root_path` 已为其预留表达位，另立需求时再落 |
| 6 | 占位/默认项目的笔记是否照常进 FTS 与语义检索 | 是，无任何特殊——`kind` 只影响项目分组与展示，不改变笔记的可检索性 |

## 晋升条件（Proposed → Accepted）

1. **迁移测试**：`project_kind` ADD COLUMN 成功；默认项目行幂等种子（连跑两次仍恰一行）；可逆（重建/回退路径）；`ExpectedSchemaVersion=24` 与迁移计数对齐。
2. **导入落点测试**：构造匹配不到项目、但携带 `RepoPath` 的 `ImportDoc` → 断言生成 `kind='import_placeholder'` 项目、笔记 `project_id` 指向它、`Skipped` 不再增长；构造无 `RepoPath` 的 doc → 断言落 `kind='default'` 项目。
3. **清理/转正测试**：空的默认项目**不被** `CleanupStaleDataTx` 删除（豁免生效）；挂笔记的占位项目不被自动删；扫描命中同 `root_path` 后占位 kind 提升为 `scanned`（`SyncProjectTx`）。
4. **归口/整理测试**：项目级归口后，目标项目笔记数增加、源占位项目清空；「源仓库已消失 → 手动删除」路径正确连带删笔记，且默认项目**永不**进入该删除路径。
5. **真库副本演练一次**（只读源 + 一次性副本，沿用 ADR-0021「副本先行」纪律）：迁移与种子在副本上对账通过后，再由用户亲自把 v24 推上真库。
6. **双语手册同步**：`docs/**` 与 `docs/en/**` 同批改（导入落点、知识库整理、点笔记直达、占位/默认项目含义），`CHANGELOG.md` 记 `[Unreleased]`；`docs/**/*.html` 为构建产物不手改。

## 实现修正（2026-10-16 落码时得到的事实）

设计阶段与本 ADR 设想不同、在实现/测试时被现实纠正之处，记录在此以免下一个人重新踩：

1. **落点助手是 `EnsureImportPlaceholder(db *sql.DB, repoPath, repoName)`，不是决策 3 写的 `EnsureImportPlaceholderTx`。** 因为 runtime 的 `upsertDoc` 走的是非事务的 `*sql.DB` 直写路径（`GetNoteBySourceTitle` / `CreateNoteEx` 都拿 `r.db`），落点判定放在 `TriggerImport` 的循环里最自然，故助手收 `*sql.DB` 而非 `*sql.Tx`。另补了决策未点名的 `GetDefaultProjectID(db)`（供无仓库路径的落点与整理取默认项目 id）。`repoName` 为 claude 这类「有解码标签、无真实路径」的源预留（占位名用它、`root_path` 用合成 URI `import://claude/<mangled>`，因此 claude 占位天然不自动转正——与决策 4 的例外一致）。

2. **落码时新增了决策未写的一条：消费面按 `project_kind` 分层。** 「default 行恒在」是有副作用的——若不区分，(a) `ResolveProject` 的「唯一项目自动解析」永不定格（多一个未分类就多一个候选）、消歧目录被污染；(b) agent 从 `repotory_projects_list` 挑中占位/未分类的 id 再调 `repotory_context` 会落空；(c) integrity 的 `scan_coverage` 把非扫描产物的 default/占位算成「已发现未采集」，变永久 warn。故实现里：AI 上下文解析器与 `ListProjects`（MCP 面）**只认 `scanned`**，`scan_coverage` 的三个计数**加 `project_kind='scanned'` 谓词**；桌面仪表盘的 `GetProjects` **仍展示** default/占位（带徽标）。`ListProjects` 经全仓核验只服务 MCP 面，过滤它不动 UI。

3. **恒种子行打破了一批旧测试的隐含假设。** default 抢占 `id=1`，且「表里只有用户项目」「新装 0 项目」不再成立：`projects_level_test.go`（split/merge 计数 + 「任意非 pid 即分出来的项目」循环会误认 default）、`service/project_level_test.go`（按 id 区间扫活项目）、`service_test.go`（GetProjects 计数）、`cmd/mcp` list/context 测试、`context_test.go`（`ResolveProject("1")` 与写死 `| 1 | alpha |` 的目录断言）、`toolcopy_test.go`（钉「当前版本=v23」）都要改。**修法是按 kind 过滤或按返回的 pid 定位，而不是把 default 藏起来**（UI 要展示它才算真落地）。

4. **迁移条目是 `{id: 24, fn: EnsureProjectKind}`，且 `EnsureProjectKind` 兼作 InitDB 无条件自愈**（与 v22/v23 同套路）：ALTER 撞「duplicate column name」由 `isAlreadyExistsErr` 吞掉，种子 `WHERE NOT EXISTS` 幂等。加列走 `ADD COLUMN ... NOT NULL DEFAULT 'scanned'`（常量默认满足 SQLite 约束），未内联 CHECK。

5. **codebuddy-memory 经用户拍板并入统一落点**：它此前「未配置目标即 `return docs, nil` 早退丢弃」，现改为落「未分类」，取代该 importer 自身的旧有意行为（决策 3 原文只点名 openclaw/hermes）。

6. **整理是 3 个桌面专属 RPC**（`ListOrganizeGroups` / `ReassignLandingNotes` / `DeleteLandingProject`），已登记进 `internal/app/surface.go` 的 `bindingSurfaces`（`SurfaceDesktopOnly` + Reason，无 MCP 孪生；`TestBindingSurfaceRegistryIsComplete` 钉住）。`ReassignLandingNotes` 是**单事务、只移笔记**（不移待办）；目标允许 `scanned | default`、拒占位与自身；`DeleteLandingProject` 只接受 `import_placeholder`（default/scanned 一律拒），非空须 `deleteNotes=true` 显式确认。前端 `OrganizeGroup.project_kind` 定为**可选**（Go 恒发、兼容旧 mock）；点笔记高亮用 `requestAnimationFrame` 把 setState 移出 effect 同步体以过 `react-hooks/set-state-in-effect`。
