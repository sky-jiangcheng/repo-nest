# ADR-0021: 时态图——给页面一个改错了能回去的底座

- 状态：Proposed（**仅设计，不落代码**。晋升前置：ADR-0018 lane 1 Accepted（版本表的迁移纪律要被真库演练验证过，同 ADR-0019/0020）。本 ADR 是实施计划 Phase 4b 战略项，明确暂缓；它**不放松** ADR-0015「编译器永不改批准页」，只造将来安全放松所需的底座）
- 日期：2026-10-10
- 关联：[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（五类页面；`note_versions` 自 v8 起就是笔记的审计底座，本文把同款底座补到页面）、[ADR-0015](0015-w3-minimal-compile-loop.md)（「永不改批准页」红线 + `fileRevisionTodo` 的 `compile-revision` 待办——本文给这条红线提供将来安全放松的唯一路径）、[ADR-0018](0018-knowledge-graph-foundation.md)（lane 1 八型词表里的 `supersedes` 是时态语义的现成落点；`lintRelationShape` 的 `supersedes` 时间倒挂检查目前只能看 `updated_at` 墙钟）、[ADR-0019](0019-entity-resolution-merge.md)（合并执行重指向边，同样受益于「改错了能回去」）、[ADR-0020](0020-community-synthesis.md)（其 `synthesis-revision` 待办与本文的「应用修订」面是同一个 UI 槽）；TODO **M7** 后续；起点 schema v21
- 配套：`RepoNest-知识图谱实施计划-2026-10-09.md` 的 Phase 4b

## 背景

页面侧有一个笔记侧早在 v8 就补上了的缺口：**没有版本史**。

- 笔记：`note_versions` 表 + `note_versions_snap` 触发器（`internal/db/migrate.go:152-181`）——每次改动自动快照、每页保留 50 条、FK 级联；v11 还把触发器收窄到「只有 title/content/tags/kind 真变才拍」。UI 有版本列表 + LCS diff + 一键恢复（`docs/features/knowledge.md`「版本历史」节）。
- 页面：`wiki_pages` 只有 `created_at`/`updated_at` 两枚墙钟（`internal/db/wiki.go:41-58`），`UpdateWikiPage`（:698）直接覆盖写——**改错了没有任何回去的路**。编译器想修订已批准页时，`fileRevisionTodo`（`wiki_compile.go:416-423`）只能产一条 `compile-revision` 待办，意思等于「人，你自己去改」：人照旧手工改，照旧没有快照。

于是实施计划 Phase 4b 把时态图定为「**允许编译器安全修订既有结论的前置**」——没有快照与可回滚，「改批准页」这条红线永远不敢松，编译器只能永远停留在「只新建」的半自动状态。本 ADR 就是这个前置于设计层的落槌：先定版本粒度、保留条数、与 lint 的关系，再谈放松红线。

另有一处时态债：八型词表里的 `supersedes`（取代）语义上就是时态边，但 `lintRelationShape` 的时间倒挂检查只能比 `updated_at` 墙钟（`wiki_lint.go`），而墙钟会被任何无关 touch 污染——「A 取代 B」到底以哪个时间点为准，目前没有可依据的结构。本 ADR v1 不修这个（见决策 6），但把它的方向定下来。

## 参照物（抄机制、不抄栈）

- **本仓自己的 `note_versions`**：最该抄的就是它——同库、同触发器套路、同 50 条清理、同 FK 级联，LCS diff 与版本 UI 全是现成的。页面与笔记在本仓是平级的两种知识载体，审计底座没有理由分两套。
- **bitemporal 建模（valid_time × transaction_time）**：数据库课上的标准答案，「这个事实从何时起为真」与「我们何时知道它为真」分开记。方向正确，但本仓**没有一个消费方**需要按时间段提问——现在上它是为假想需求把写路径翻倍。v1 不做，方向留在待决 5。
- GraphRAG / Neo4j 的时态扩展：同样绑外部栈，违背 local-first / 单 SQLite / 零 CGO（ADR-0014 决策 2、ADR-0017），只确认「时态是知识图的标配维度」这一判断，不借实现。

## 决策

**1. `page_versions` 镜像 `note_versions`，不发明第二套。** 表结构逐列对齐（`page_id / title / content / kind / created_at`，FK `ON DELETE CASCADE`，`(page_id, created_at DESC)` 索引）；`AFTER UPDATE ON wiki_pages` 快照触发器 + 每页保留 50 条的清理触发器，两条都 `IF NOT EXISTS` 保幂等。schema 号取 **v23**，把 v22 留给 lane 2 的 `page_embeddings`（见待决 1）——「v22=页向量」已写进多份文档，不抢占。v23 与 [ADR-0019](0019-entity-resolution-merge.md) 决策 6 的可选 `page_merges` 共抢，按「先落库者认领」裁决（待决 1）。

**2. 快照触发面：title/content/kind 真变才拍。** 照抄 v11 的教训（`migrate.go:200-210`）：`status` 迁移（pending→approved、reject）、`source`、墙钟类 touch **不拍**——审核状态有审核队列可查，混进内容史只会让版本列表充满无意义快照。判定用 `WHEN` 子句逐列比较，不靠应用层自觉。

**3. 回滚 = 先拍当前、再应用旧版，永不删版本。** 「恢复到这个版本」不是 DELETE 历史，而是把当前内容先存成一个新版本（它也是一段真实发生过的状态），再把旧版内容写回当前。这与本仓 `reject≠delete`、合并重指向不删页是同一条纪律：**历史只增不减**。删页时版本随 FK 级联消失——那是页整体消失，不是挑选历史删除。

**4. 读出三件套对齐笔记侧，只进桌面 + UI。** `ListPageVersions` / `GetPageVersion` / 页级 LCS diff（`NoteDiff` 的同款算法现成），经 `internal/app/bindings.go` 薄包装 + Review/Knowledge 页的版本侧栏。**不新增 MCP 工具**（ADR-0015 决策 6：agent 能改历史=绕开人在环）；回滚是破坏性写，只能由人在 UI 点。

**5. 本 ADR 不放松「编译器永不改批准页」。** 它只造底座。修订闭环分两步：v1 人照旧手工改（但现在可回滚、改前有 diff 可看）；「机器起草修订 → 人一键应用 → 可回滚」是 v2，届时单独拍板，与 ADR-0019 的合并执行、ADR-0020 的 `synthesis-revision` 共用同一个「应用修订」UI 槽。**把红线松绑与造底座分成两个决策点**，是本 ADR 最重要的自我约束。

**6. bitemporal 与 `supersedes` 锚版本：v1 都不做。** 不给 `wiki_pages` 加 `valid_from/valid_to`（无消费方）；不给 `page_links` 加属性列锚定版本号（守 lane 1「先只定边型」的承诺，边加属性是另一次 rebuild）；`lintRelationShape` 的 `updated_at` 时间倒挂检查原样不动。时态债登记在案，等第一个真实消费方（最可能是「按时间点重放取证」或 ADR-0019 合并时的事实裁剪）再收。

**7. 不需要新门控键。** 版本史是纯本地审计数据、零外发、失败不影响主路径（触发器在事务内，写页失败即什么都不留）；但「回滚」按钮必须留在显式的人的操作里，不进任何自动路径（lint ticker、auto_import、MCP 一律不碰）。

**明确不做**：bitemporal 有效性区间、事件溯源（把 `wiki_pages` 改 append-only——现有读路径全得重写）、修订自动应用、`supersedes` 边加属性列、lint 过时检查（staleness：邻居都在演进而某页长期不动——另立低优先 lint 项）、乐观锁/并发合并（v1 用 UI 提示兜，见待决 6）。

## 候选矩阵

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| **本决策：`page_versions` 镜像 `note_versions`** | 近，一个 v23 | 页面第一次「改错了能回去」；diff/UI/纪律全部现成；修订闭环的唯一安全底座 | 全量快照的存储放大（页正文有 ≤1500B 预算，可接受） |
| 完整 bitemporal（valid_from/valid_to + supersedes 锚版本） | 远 | 可回答「何时起算」 | 无消费方；写路径翻倍；边表加属性又一次 rebuild |
| 事件溯源（append-only 页 + 投影读） | 远 | 最纯的时态模型 | 全部读路径重写；回滚语义要重新定义；规模过度 |
| 不做，继续 `compile-revision` 待办让人手工改 | 0 | 省一个迁移 | `UpdateWikiPage` 永无回滚；「编译器修订既有结论」免谈；`supersedes` 时态债继续欠 |

## 后果

**正面**：页面侧补齐与笔记侧对等的审计/回滚底座（用户心智不用分两套）；ADR-0015 红线第一次有了「将来安全放松」的具体路径而非永久冻结；版本列表让「这页被改过什么」在审核面可见，间接制衡手工误改；`supersedes`/`contradicts` 的时态语义有了明确的演进方向。

**负面（不粉饰）**：
- 全量快照有存储放大——每页最多 50 份正文。页正文受编译器 ≤1500B 预算约束，量级可控，但大库上是实打实的字节数。
- 触发器与 lane 2 的 `page_embeddings` dirty 队列有交互：回滚走 `UpdateWikiPage` 同一写入口，向量置脏必须照常发生，否则回滚后向量索引指向旧文本。迁移时核清，若 lane 2 先落地需构造回归（晋升条件 5）。
- 「版本历史」是又一块前端面（列表 + diff + 恢复确认），虽可仿笔记侧，仍是实打实的工作量。
- 回滚与并发编辑可能打架（人 A 改着、人 B 回滚了旧版）：v1 不做乐观锁，只用「回滚将覆盖当前内容，当前内容会存为新版本」的确认提示兜——覆盖不发生静默丢失，但冲突提示是 v2 的事。
- 造好底座不等于闭环成立：修订 v2（机器起草 → 人应用）仍是独立决策，本 ADR  deliberately 不替它拍板。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | schema 号：v22 已被 lane 2 页向量占用（冻结中） | **本 ADR 取 v23**，把 22 留给页向量；若 lane 2 被永久放弃，再议是否把 23 改名 22（迁移已发布则不改）。[ADR-0019](0019-entity-resolution-merge.md) 决策 6 的可选 `page_merges` 也曾铅笔记为 v23：按**先落库者认领**裁决——若那张可选表先落，它取 v23、本 ADR 顺延 v24；若本 ADR 先落，它顺延取下一个空号。两边都不把号码当承诺，发布时以 `ExpectedSchemaVersion` 实值为准 |
| 2 | 版本粒度 | **全量快照**——notes 先例 + LCS diff 现成；差量存储在页规模没有收益 |
| 3 | 保留条数 | **每页 50**，与 `note_versions` 逐字一致；不做可配键，等出现「不够」的真实信号 |
| 4 | 快照触发面 | **title/content/kind**（照 v11 教训）；status/source/墙钟不拍 |
| 5 | bitemporal 有效性区间 | **v1 不做**，等第一个消费方；`supersedes` 边锚版本同理排队 |
| 6 | 回滚的并发语义 | **先不做乐观锁**：确认提示写明「当前内容将存为新版本」；要锁是 v2 |
| 7 | 与 lint 过时检查的关系 | **v1 不动** `lintRelationShape`；staleness（长期不动页）另立低优先 lint 项，不混进本文 |

## 晋升条件（Proposed → Accepted）

1. 迁移可逆：rebuild 前后 `sqlite_master` 快照 + 版本多重集对账，合法内容不被改写；幂等重放（沿用 lane 1 纪律）。
2. 触发器精度回归：title/content/kind 任一真变必拍；approve/reject/置顶类 status 迁移**不拍**（照 v11 先例钉住）。
3. 回滚不删版本：回滚动作先拍当前为新版本再应用旧版；对同一版本重复回滚得到同样结果（幂等）。
4. 删页级联带走其版本（FK CASCADE）；`reject≠delete` 红线不受影响（拒绝 pending 页仍不产生版本残留）。
5. 与 `page_embeddings` dirty 队列不打架：回滚经 `UpdateWikiPage` 同一写入口，dirty 照常置脏（若 lane 2 已落地，构造专项回归）。
6. 真库副本演练一次：沿用 `cmd/wiki-rehearse` 的只读副本纪律，确认迁移对既有页零丢内容。
7. 双语文档（`docs/features/knowledge.md` 「版本历史」节补页面侧）+ 本 ADR 状态行更新；落码时才入 CHANGELOG。
