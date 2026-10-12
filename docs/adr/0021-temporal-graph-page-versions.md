# ADR-0021: 时态图——`page_versions` 快照让「修订已批准结论」可以安全发生

- 状态：Proposed（**仅设计，不落代码**。前置：ADR-0018 lane 1 Accepted；且只在出现第一个真实的「已批准页过时需要修订」需求时才值得落码——提前实现是给没有用户的机制付维护费。触发时机见待决 4）
- 日期：2026-10-12
- 关联：[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（页面 SSOT 落库不落导出树——快照也只能住库里）、[ADR-0015](0015-w3-minimal-compile-loop.md)（「永不修改已批准页」——本 ADR 是它的第一道受控松动通道，不动那条不变量本身）、[ADR-0016](0016-async-batch-jobs.md)（`compile_jobs.kind` 加值 = 整表重建的红线）、[ADR-0018](0018-knowledge-graph-foundation.md)（lane 1 的 `supersedes` 时间倒挂 lint 是时态语义的第一个消费者，但它只有 `updated_at` 一个可变时间戳可依）、[ADR-0019](0019-entity-resolution-merge.md) / [ADR-0020](0020-community-synthesis.md)（共享同一次 CHECK 整表重建的合批先例）；TODO **M7** 后续；起点 schema v21
- 先例：笔记层已有 `note_versions`（P19 版本历史 diff 可视化）——页面层补齐对等的审计能力

## 背景

ADR-0015 定死了一条铁律：编译器**永不修改已批准页**——撞上同名 slug 时降级为一条 `compile-revision` 待办（`internal/service/wiki_compile.go:423`），修订完全靠人手动改页。这条不变量在「没有人提出修订」时是完美的安全设计，但它同时意味着：

1. **修订没有可审计的路径**。知识会过时——笔记说接口升到 v2，页里还是 v1 的结论。今天唯一的出路是「另写一页」，那正是 ADR-0019 要治的「概念分裂」；或者人手改页，而 `wiki_pages` 是可变行、`content` 就地覆盖——**改错了无法回看「批准当时批的是什么」**。
2. **`supersedes` 的时态语义没有数据支撑**。lane 1 的 lint 能查「supersedes 时间倒挂」，但两页谁旧谁新只有 `updated_at` 一个可变值，页一被改它就丢。
3. **笔记有版本历史（`note_versions` + P19 diff），页面反而没有**。AI 生成的内容比手写笔记更需要「批准时点可复现」的审计。

「允许编译器安全修订既有结论」是 M7 后续里明确挂着的方向，而它的前置就是：**每一次变更先留快照**。本 ADR 只定这个地基的形状，不落码。

## 参照物（抄机制、不抄栈）

- **自己的 `note_versions`**：append-only 版本行 + 当前行为活页 + diff 视图。页面层照抄这个已经跑通的模式，不发明新东西。
- **MediaWiki revision**：每次改动一条 revision，「当前」是查询出来的最新版。**只借「append-only + 可回看」的机制，不抄它的架构**——「当前 = 最新 revision 查询」会把 FTS、导出、证据、向量全部改成 join 读，代价不成比例（见候选矩阵）。
- **时态数据库的 transaction time / valid time 二分**：本仓只需要 transaction time（这个版本何时被写/被批），不需要 valid time（事实何时为真）——后者要求人标注生效时间，本仓没有这个输入源。

## 决策

**1. 新表 `page_versions`，只存 approved 内容的快照，活页行保持可变。** `wiki_pages` 的读路径（FTS v17、检索、导出、证据、AskAI）**零改动**——当前视图永远是活页行，快照表是纯审计层。最小列集：`id / page_id / title / content / kind / source / created_at`（快照记录「批准时点的页面长相」）。pending 与 rejected 页**不进快照**：它们本来就是未审草稿或错误占位，没有「结论历史」可审计。

**2. 写入点只有两个。** ① `ApproveWikiPage` 冻结一份快照——「被批准的正是这份内容」从此可复现；② 未来任何修改已批准内容的路径，生效前先冻结旧版。首版迁移把现存每张 approved 页回填一条快照（`created_at` 取页面 `updated_at`，语义 =「该内容自何时成立」）。

**3. append-only 用触发器做硬约束，不靠自觉。** `BEFORE UPDATE/DELETE → RAISE(ABORT)` 触发器，把「快照不可改写」从纪律变成结构事实——与「lint 改不了页面」用 `TestWikiLint_NeverMutatesPages` 钉住是同一仓风：能结构化的不变量绝不留在注释里。

**4. 修订流（只定型，不在本 ADR 落地）：`revise_page` 提议版本人审。** 编译器新增一种 op（契约类型从 3 op 变 4 op，是 Go 类型改动、不触任何 CHECK）：目标必须是已批准页，产物不是改页，而是一条 `status='pending'` 的提议版本 + 一条 `[compile] revise` 待办；审核 UI diff 当前内容 vs 提议版本，采纳才「先冻结旧快照、再写活页」，拒绝则丢弃提议、活页逐字节不变——`reject≠delete` 的精神原样适用。**`compile_jobs.kind` 不加新值**（复用 `'compile'`，op 层面区分），不为一个待决能力付整表重建的代价。

**5. 迁移编号 v23，且倾向与那次迟早要来的 CHECK 重建合批。** lane 2 已占 v22；`compile_jobs.kind` 若最终要加值（ADR-0020 决策 8 的合批思路）、或 wiki 层再有 DDL 级变更，快照表随那次 rebuild 一同落。单开 v23 也可行，二选一是待决 1。

**6. 时态语义最小化。** 只做 transaction time；不做「历史时点的图重放」（图查询永远只看当前边集）；不做 valid time；不设版本保留上限（本仓规模先全保留，见待决 3）。

## 候选矩阵

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| MediaWiki 模式（当前 = 最新 revision 查询） | 远 | 单一事实形态 | FTS/导出/证据/向量全改 join 读，现有全部消费方都要动 |
| Git 式分支/合并历史 | 远 | 可分叉知识 | 过度工程；「知识分支」没有用户输入源 |
| **本决策：活页 + 批准/变更前快照 + 提议版本人审** | 中 | 修订可审计、批准时点可复现、为 revise_page 铺路 | 快照与活页一致性测试面；表随编辑线性增长 |
| 不做（维持 compile-revision 待办 + 人手改页） | 0 | 省事 | 历史永远丢失；「另写一页」加剧概念分裂 |
| 只在导出树里留副本 | 小 | 导出侧可回看 | 导出树是只读派生物，SSOT 不在导出（ADR-0014 决策 2）；库里照样无历史 |

## 后果

**正面**：「批准时点」第一次可复现（审计/回看都有锚点）；修订从「人肉 + 覆盖」变成「提议 → diff → 人审 → 旧版自动留档」；`supersedes` lint 的时间语义有真实数据可依；ADR-0020 的 `synthesis-revision` 与未来任何自动重写讨论有了落点。

**负面（不粉饰）**：
- 每次批准多一次写（KB 级，可控），快照表随编辑单调增长——没有保留策略就是「永不清理」，先接受再观察。
- append-only 触发器会让「想顺手修一下历史数据」的合法运维变成迁移工程（先 drop 触发器、改完再重建）——这是刻意代价，买的是不可抵赖。
- 提议版本是新的审核面：审核 UI 要多一种「diff 两段内容」的视图（可复用 P19 NoteDiff 的渲染，但仍是新代码）。
- 又是「只定地基不定功能」：没有 revise_page 时，快照表的唯一写入点是批准动作——完整价值要等修订流落地才兑现。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | 迁移形态：单开 v23 vs 与 `compile_jobs.kind` 的 CHECK 重建合批 | **合批**（等 ADR-0019/0020 或 lane 2 出现真实 DDL 需求时一并做；在那之前本 ADR 维持纯设计） |
| 2 | append-only 执行机制 | `BEFORE UPDATE/DELETE RAISE(ABORT)` 触发器（硬约束），配一条「触发器在位」的 schema 断言测试 |
| 3 | 版本保留策略 | 全保留、无清理；真库演练若显示增速失控再回来定上限 |
| 4 | `revise_page` 何时落地 | **不单独立项**：等第一个真实的「已批准页过时」案例出现，由用户触发；在那之前 `compile-revision` 待办照旧 |
| 5 | 审核 UI 的 diff 呈现 | 复用 P19 NoteDiff 的渲染路径，不新写 diff 算法 |
| 6 | 快照是否记录操作者 | MVP 不记（单机单用户，`source` 字段足够）；B 端多用户需求出现时再加列 |

## 晋升条件（Proposed → Accepted）

1. 迁移测试：快照表创建、approved 页回填对账（每张 approved 页恰有一条快照且内容逐字节相等）、幂等重放、`DropWikiSchema` 可逆性（沿 W1 的构造证明套路）。
2. append-only 测试：对 `page_versions` 的 UPDATE / DELETE 必须被触发器拒绝；`ApproveWikiPage` 幂等重入不产生重复快照。
3. 契约测试（若 revise_page 落地）：提议版本对检索 / AskAI 证据不可见；采纳后旧内容可从快照找回；拒绝后活页逐字节不变。
4. 真库副本演练一次（`cmd/wiki-rehearse` 同款纪律：只读源库 + 一次性副本），回填对账通过。
