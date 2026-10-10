# ADR-0020: 社区摘要——用 Label Propagation 让 synthesis 层自己长出来

- 状态：Proposed（**仅设计，不落代码**。晋升前置：ADR-0018 lane 1 Accepted（社区检测要沿受控 relation 白名单传播，词表是它的地基）。触发方式（同步 vs job）见决策 6 与待决 4，是本文最需要被拍的一条）
- 日期：2026-10-10
- 关联：[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（决策 1 把 synthesis 列为五类之一、决策 5 的 lint 有 `no-synthesis` 缺口检查）、[ADR-0015](0015-w3-minimal-compile-loop.md)（"MVP 不做 synthesis 自动重写"、只新建 pending 不改批准页、审核隔离）、[ADR-0016](0016-async-batch-jobs.md)（长任务 job 化，但 `compile_jobs.kind` 加值=整表重建）、[ADR-0018](0018-knowledge-graph-foundation.md)（lane 1 受控关系词表 = 本 ADR 传播所沿的边；lane 2 PPR 是"第一次读图"，本 ADR 是"第二次"）、[ADR-0019](0019-entity-resolution-merge.md)（合并让社区更干净，但本 ADR 不阻塞在其后：分裂也能摘要，只是质量打折）；TODO **M7** 后续；起点 schema v21
- 配套：`RepoNest-知识图谱实施计划-2026-10-09.md` 的 Phase 4a

## 背景

ADR-0014 决策 1 给页面层定了五类：`entity / concept / source / synthesis / query`。但**生成侧只补齐了四类**：

1. `entity/concept/source/query`——编译器逐条笔记产出（W3，pending→人审），问答回档产 `query`（W2）。**只有 `synthesis` 没有任何系统性生成机制**。编译器 prompt 允许模型偶尔把某页标成 `synthesis`（`wiki_compile.go:225` 的 kind 枚举里就有它），但那是逐条笔记视角的偶发标签，不是"跨来源把一个主题合成一篇观点"——synthesis 的语义是综述，编译器一次只看一条笔记，天生做不了综述。
2. 于是 W4 的 `no-synthesis` 检查（`wiki_lint.go:271`：≥5 张叶子页却零综述）**永远是红的**——它如实报告了"知识仍是散片"，但系统没有任何东西去把它变绿。这是一个**防腐层对着一个空转生成层的死循环**：lint 喊缺综述，没人能补，因为根本没有生成综述的那一步。

ADR-0015 当时明确"MVP 不做 synthesis 自动重写"，ADR-0018 又把它推给"各自单开 ADR"。它就是这条链上被两次显式让位、却始终没被填的那个空。本 ADR 来填。

## 参照物（抄机制、不抄栈）

- **Microsoft GraphRAG 的 global search**：对图做社区检测（Leiden）→ 逐社区让 LLM 摘要 → 逐层向上综合。这是"跨来源长出观点"的标准做法。但它的栈绑 Leiden 多层聚合 + map-reduce 全局摘要，本仓 local-first / 零新依赖 / 单 SQLite 用不起也不必（几百~几千页规模）。**只借"社区→摘要"这条机制。**
- **Label Propagation（LP）**：Raghavan 2007 的经典社区检测，每个节点把邻居里占多数的 label 传给自己，几轮收敛。纯 Go 几十行、无权重、无外部库。质量不如 Leiden，但在本仓规模够用，且**简单 = 可审计**（契合本仓 lint/可逆的整风）。
- Karpathy / nashsu：synthesis 是人手写的，工具不生成。本仓折中——**机器起草成 pending、人批准后成立**，既补上生成侧又不越人在环。

## 决策

**1. 用 Label Propagation，不用 Louvain/Leiden。** LP 纯 Go 数十行、零依赖、零 CGO。Leiden 社区质量更高，但多层图聚合 + 模块度优化的代码量和参数面在几百页规模是过度工程，且更难审计。**明确接受 LP 两个已知弱点**（边界模糊、可能塌成巨无霸），用尺寸阈值兜（决策 2）。

**2. 社区尺寸阈值。** 丢弃 `< minCommunitySize`（默认 3）的碎片（它们不构成"主题"，摘要出来是硬凑），截断 `> maxCommunitySize`（默认 30）的巨无霸（LP 退化产物，且超模型上下文预算）。两者都可配。

**3. 只在受控 relation 白名单上传播。** 沿 `ref / part-of / depends / implements / documents`（"讲同一件事"的信号），**不沿 `contradicts`**（对立不是同主题）、**不沿 `mentions`**（正文顺带提一句不代表同社区）。这是 ADR-0018 lane 1 词表第一次被"拿来跑算法"——词表若还是自由串，这一步根本无从定义白名单。破对称：label 初始按成员 slug 字典序分配，收敛中并列取字典序最小——**保证同图同输入必得同划分**（LP 本征的随机性用确定性 tie-break 摁死，这样幂等测试才成立）。

**4. 输入只含 approved 页；产物只落 pending。** pending/rejected 不进社区检测（隔离区，不参与"当前知识长什么样"的判断）。每社区产出一张 `synthesis` 页，`status=pending`、`source='wiki-community'`（区别于编译器的 `'wiki-compile'`，审核 UI 一眼看出来源）。沿用 ADR-0015 全套：新建不改批准页、只有人批才进检索。

**5. synthesis 页 ↔ 成员用 `documents` 边连。** 每个成员页 → 该 synthesis 页加一条 `documents` 边（词表里"描述"的正解：综述描述它所概括的页）。边由**代码**建，不给模型 `add_link` 权力。synthesis 正文 ≤1500B（复用编译器的每页字节预算）。

**6. 模型只写正文，无结构权。** 给模型看：社区成员页的 excerpt（每页截断，总量预算封顶）+ 它们之间的白名单边。让它**只输出一段纯文本综述**（不是 op JSON），页/边/来源挂接全由代码完成。这比 W3 编译器**权力更小**——编译器还能选 kind/slug，社区摘要的模型只能填一段正文，编不出页也连不出边。

**7. 幂等 + 不重写已批准 synthesis。** synthesis 页 slug 稳定派生：`synthesis-` + sha256(排序后的成员 slug 列表) 前 8 位——同一社区重算得同一 slug。撞已有 `pending`（自己上轮的产物）→ 更新；撞**已批准** synthesis → **不改**，降级为一条 `synthesis-revision` 待办（完全复用 W3 编译器 `fileRevisionTodo` 的处置）。守 ADR-0015。

**8. 触发：同步跑 + 硬预算，MVP 不 job 化。** 单次上限 `≤10 个社区 / 每社区 ≤24 页 / 每页 ≤1500B / 总墙钟 ≤DefaultBatchChatTimeout`，触顶即停在报告里写明停在哪（复用 ADR-0015/0016 的"预算即止"）。**为什么先不 job**：给 `compile_jobs.kind` 加 `'community'` 值要整表重建（SQLite CHECK 不可 ALTER，红线），为一个人手动触发、量又不大的动作付那次重建的代价不值；等真出现"整库社区摘要要跑十几分钟/要排队"的信号，再与 ADR-0019 迟早要做的那次 rebuild 合并做。**这是待决 4，最需要你拍。**

**9. 门控 `wiki_community_summary`（默认关），绝不进 ticker。** lint 有 6h 定时是因为它只读+便宜；社区摘要每社区一次 LLM 调用、会写 pending 页，是**花钱的生成动作**，必须人显式点。不进 `startWikiLintTicker`、不进 `auto_import`、不做 MCP 工具（与 ADR-0015 决策 6 同理：agent 能自己往库里灌综述=绕开人在环）。

**明确不做**：Leiden 多层分层摘要（过度工程）、synthesis 之间再合成更高层（先跑通单层）、社区摘要 job 化（待决 4）、跨项目社区（MVP 社区按项目分别跑，全局页单列一档）、自动合并进 synthesis 的修订（走待办，见决策 7）。

## 候选矩阵

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| 抄 GraphRAG 全套（Leiden + 分层 + map-reduce 全局摘要） | 远 | 工业级主题摘要 | 违背 local-first/零依赖；几百页规模用不上；不可审计 |
| Louvain 单层 | 中 | 社区质量优于 LP | 模块度优化代码量 & 参数面显著大于 LP，本规模收益不明显 |
| **本决策：LP + relation 白名单 + pending 产物 + 人审 + 同步预算** | 中 | 补上 synthesis 生成侧、绿灯 no-synthesis、第二次读图 | LP 边界不完美；缝合怪风险 |
| 不做，synthesis 永远靠人 | 0 | 省事 | lint 的 no-synthesis 成永久红灯的死循环；五类缺一类是残缺 |
| 不用图、直接叫模型"给整个库写综述" | 看似更简单 | 一步到位 | 主题边界飘忽、无社区结构、超上下文、产物不可幂等/不可审计 |

## 后果

**正面**：五类页面第一次全部有生成路径；`no-synthesis` 从"空喊"变成"能被填绿"；这是 ADR-0018 lane 2 之后**第二处真正读图**（把词表当算法输入而不只是 lint 判据）；模型权力比编译器更小（只填正文）；产物默认 pending，隔离/审核/回滚基建全是现成的。

**负面（不粉饰）**：
- LP 社区质量看图结构吃饭，边界会糊、偶尔塌大块——**不承诺社区"正确"，只承诺幂等 + 阈值不失控**。
- 摘要幻觉写进 `synthesis` 页，比一条编译页**看起来更权威**（kind 就叫"综述"）——错误更隐蔽。唯一屏障仍是人在环 + excerpt 预算 + `wiki-community` 来源标记让人警惕。
- 社区把不相关页凑一块时会产出"缝合怪"综述；现有 lint 只查单页/单边，不查一个集合是否内聚。建议**落码时顺带**加一条 `community-incoherent` 检查（走 `wiki_lint_llm`、低置信、只产待办），但本 ADR 不阻塞于它。
- 同步 vs job 是真实未决（决策 8/待决 4）；同步在小库没问题，几百社区的大库会顶到超时。
- 又一次三层动（新算法包 + service 生成 + 触发绑定/UI），非单点。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | 社区检测放哪 | 新建 `internal/graph/lpa.go`（纯函数、图进/社区出、零 db 依赖、独立单测）；service 层只做"读边→喂 lpa→写页" |
| 2 | LP 轮数 / tie-break | 默认 ≤15 轮；按 slug 字典序破对称保幂等；不收敛即取当前 |
| 3 | synthesis 来源标记 | `source='wiki-community'`（新常量），不复用 `wiki-compile`，让审核与 lint 可区分 |
| 4 | **触发：同步预算 vs job** | **MVP 同步 + 硬预算**；把"要不要 job 化"与 ADR-0019 的 `compile_jobs` 整表重建绑成同一件事，出现真实长耗时信号时一并做 |
| 5 | 社区粒度阈值是否可配 | min=3/max=30 作代码默认，暴露两个配置键可覆盖（数值键，走 allowedConfigKeys） |
| 6 | 全局页（project_id=0）怎么处理 | 单独跑一次社区检测，不与某项目混；其 synthesis 落 project_id=0（跨项目综述），恰好喂 W5 的 L3 |

## 晋升条件（Proposed → Accepted）

1. `internal/graph/lpa` 纯单测：链/树/两社区一桥/孤立三角/巨块——期望社区正确，min/max 阈值生效，**同图重跑划分逐字节一致**（tie-break 幂等）。
2. 生成契约测试（`service` 层，仿 `wiki_compile_test`）：非纯文本/空回复 → 不写正文但页与 `documents` 边与来源挂接照常建；重跑幂等（同 slug 不产生第二张）；**撞已批准 synthesis 不重写、降级为 `synthesis-revision` 待办**；预算触顶即停在报告。
3. `wiki_community_summary` 关时零外发、零写库；开时报告里带外发字节数。
4. 端到端真库副本演练一次，产出被人审一轮（沿用 W3 的门禁教训：机制正确 ≠ 收益真实）。
5. 落码后 `no-synthesis` 这条 lint 能在"跑了社区摘要并批准若干 synthesis"后转绿——用它自己的检查反证生成侧通了。
