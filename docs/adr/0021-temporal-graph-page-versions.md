# ADR-0021: 时态图——`page_versions` 快照让「修订已批准结论」可以安全发生

- 状态：Proposed（**地基已落码（schema v22），待真库回填对账后升 Accepted**。`page_versions` 快照表、作为唯一写入者的 `AFTER UPDATE` 触发器、两条 append-only 守卫、逐页回填与 `DropWikiSchema` 可逆性均有测试；晋升条件 1、2 已满足；条件 4 已于 2026-10-13 改写为「真库落地后的回填对账」（真库当天在无演练前提下推进到 v22，原措辞失去对象，见该条修订记录），其完成需先存在 approved 页，而批准是人独有的动作；条件 3 随 `revise_page` 一起等待——决策 4 与待决 4 明确不在本 ADR 落地修订流。实现期得到的事实见末尾「实现修正」节，其中「外键 CASCADE 会触发行触发器，因此快照表不能带级联外键」与「迁移实际取 v22 而非 v23」两条与设计不同）
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
4. **真库落地后的回填对账**：一旦真库里存在 approved 页，核对「每张 approved 页恰有一条快照、内容与活页逐字节相等、无重复条目」，并确认两条 append-only 守卫在真数据上照样拒绝改写。

   > **修订记录（2026-10-13，本条被改写）**
   >
   > 本条原文要求「真库副本演练一次（`cmd/wiki-rehearse` 同款纪律：只读源库 + 一次性副本），回填对账通过」，
   > 也就是**先让 v22 碰副本，再让它碰真库**。这个顺序在同一天被打破了：真库在**没有演练**的前提下从 v21
   > 推进到了 v22。起因是把工作树构建的 `cmd/mcp` 当作 `--help` 冒烟探针跑了一次——`cmd/mcp` 的 `main`
   > 第一件事就是 `db.InitDB(platform.GetDbPath())`，而它不解析任何 flag，所以那次「冒烟」在效果上等于一次
   > 真实的带迁移开库。后果已逐项对账：4 项目 / 5 笔记 / 7 页（全部 pending）/ 2 条边 / 7 条 note_pages /
   > 6 条 todo，slug 与边内容一字未变；`page_versions` 为 0 行（正确，当时没有 approved 页）；v22 只新增
   > 1 张表 + 3 个触发器，纯增量、无破坏。
   >
   > 原文保留而不删，是因为要区分两件事：**「副本先行」这条纪律没有被撤销**，它对后续每一次迁移继续有效
   > （lane 2 的 v23、以及任何将来的 DDL 变更），ADR-0018 lane 1 那次演练仍是本仓的标准先例。被改写的只是
   > 本条的可完成性——真库已经在 v22 上，再要求一次「v22 上真库之前的副本演练」永远写不出完成态，
   > 而一条永远关不上的条件正是本仓在 ADR-0009 分发评估门上纠正过的同类错误。
   >
   > 因此本条改写为今天仍然可完成、且真正承载剩余风险的那个检查：有 approved 页之后的回填对账。
   > 在此之前，v22 的验收只由 `internal/db/wiki_versions_test.go` 的临时库测试承担（晋升条件 1、2）。

## 实现修正（2026-10-13 落码时得到的事实）

设计阶段有四处与本 ADR 的设想不同，都在实现或测试时被现实纠正，记录在此以免下一个人重新踩：

1. **决策 2 的「两个写入点」落成一个 `wiki_pages` 上的 `AFTER UPDATE` 触发器，不是两处 Go 调用。** 理由与迁移 v14 给 embedding 队列用触发器是同一条：`internal/db` 是所有写入者收敛的地方——importer 直写、编译器、将来的 `revise_page`、以及今天还不存在的路径都经过它，而 Go 侧 helper 是「会被忘记调用」的那种东西。触发器的 `WHEN` 子句同时承担了两件事：只在 `new.status = 'approved'` 时写（pending 草稿与 rejected 墓碑不进快照），以及内容/标题/kind 未变时不写（审核按钮重复点击、`SetWikiPageStatus` 重放都只是 bump `updated_at`，不产生版本）。
2. **决策 3 的 append-only 与决策 1 的表结构硬冲突，本 ADR 未预见：SQLite 会为外键 CASCADE 动作触发行触发器。** 若 `page_versions.page_id` 写成 `REFERENCES wiki_pages(id) ON DELETE CASCADE`，那么 `BEFORE DELETE` 守卫会让 `DELETE FROM wiki_pages` 失败——连带让**删除一个项目**失败（项目删除级联穿过 wiki_pages）。取舍只有两个：「删项目坏掉」或「快照比它所记录的页面活得更久」。审计层选后者：`page_id` 是普通 INTEGER，孤儿快照是预期状态，已删页面的历史仍可读，由 `TestPageVersions_ProjectDeleteIsNotBlockedByAppendOnly` 钉住（含「先证明守卫在位、再证明级联删得掉」两半）。
3. **迁移编号取 v22，不是决策 5 预测的 v23；ADR-0018 lane 2 改占 v23。** 决策 5 倾向「与那次迟早要来的 CHECK 重建合批」，但落码时看清了：0019 待决 3 与 0020 决策 8 都明确**不新开 `compile_jobs.kind` 值**，所以那次重建眼下没有任何需求方；而 lane 2 还卡在 Phase 0（真实标注 query 集）。为一个可能几个月不落地的迁移预留号码，只会留下一个空洞。
4. **回填不是「迁移跑一次就完」，它挂在 InitDB 每次开库重跑。** 触发器看不见 `INSERT`，所以「出生即 approved」的页面（人写的页、W2 把答案回档成的页）永远不会自己产生快照；`EnsurePageVersions` 里那条带 `NOT EXISTS` 门槛的回填才是它们的基线来源。门槛是逐页的，因此每次开库重跑既安全又必要。
5. **决策 4 的 `revise_page` 未落地**，与本 ADR 原文一致（「只定型，不在本 ADR 落地」+ 待决 4「不单独立项，等第一个真实『已批准页过时』案例由用户触发」）。但决策 2 的第②个写入点今天确实被用上了：ADR-0019 的合并把一张曾批准的页转成墓碑时，它的批准时点内容已经在 `page_versions` 里可读——「合并销毁信息」这件事因此有了反悔的凭据，尽管没有一键撤销。

**晋升状态**：条件 1（迁移与回填对账、幂等重放、`DropWikiSchema` 可逆）与条件 2（append-only 拒绝 UPDATE/DELETE、审核重入不产生重复快照）已由 `internal/db/wiki_versions_test.go` 满足；条件 3 随 `revise_page` 一起等待。条件 4 已于 2026-10-13 改写为「真库落地后的回填对账」——因为真库当天在**无演练**的前提下已被推进到 v22，原措辞失去对象（详见该条修订记录）；改写后的对账需要先存在 approved 页，而批准动作按设计只能由人做出。故本 ADR 维持 Proposed。
