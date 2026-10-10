# ADR-0019: 实体消歧与概念合并——治「概念分裂」，合并永远由人触发

- 状态：Proposed（**仅设计，不落代码**。合并执行 + 结构候选发现可在 ADR-0018 lane 1 Accepted 后开工；其中「向量相似度」这一路 blocking 依赖 lane 2 的 `page_embeddings`，未落地时该信号缺席即可、不阻塞其余。晋升条件见末尾）
- 日期：2026-10-10
- 关联：[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（决策 4「编译只新建」）、[ADR-0015](0015-w3-minimal-compile-loop.md)（决策 3 白纸黑字把「概念分裂 → 人合并」列为接受的代价；`reject≠delete`；编译器永不改批准页）、[ADR-0016](0016-async-batch-jobs.md)（判等若耗时要并入 lint 的 job 内核，不新开 kind）、[ADR-0018](0018-knowledge-graph-foundation.md)（lane 1 的受控 relation 词表是合并要重指向的边；lane 2 的页向量是本 ADR 的可选信号）；TODO **M7** 后续首条；起点 schema v21
- 配套：`RepoNest-知识图谱实施计划-2026-10-09.md` 的 Phase 3

## 背景

ADR-0015 决策 3 做了一个明确的取舍：**编译器只新建 pending 页，永远不改已批准页**。它如实写了代价——「概念会分裂（同一主题出现两页待批），需要人在审核时合并」。问题在于：**这笔合并债到期了，却没有还款工具。**

1. **没有任何合并原语**。审核界面（`web/src/pages/Review.tsx`）只有 `approve` / `reject` / `delete` 三个动作（:117-121），全仓 grep `Merge` 只命中 `MergeProjectUp`（项目合并，无关）和路径/标签/待办的去重——**没有一处能把两张页面并成一张**。ADR-0015 让人背的债，系统没打算让人还。
2. **Lane 1 真库演练直接撞上它**。5 条笔记编译出 7 张 pending 页，逐页保真复核 4/7 忠实、3/7 局部幻觉；其中「把悬浮球测试噪声抬成概念页」正是分裂 + 错标，只能靠人处置。按样本外推约四成产页会被拒——**拒绝留下一堆语义重叠的残页，图随时间变脏**。
3. **预防不彻底**。编译器 prompt 其实已经在发「已有 slug 词表」（`wiki_compile.go:213-220` 把 `ListWikiPages` 的 slug/title/kind/status 全量喂给模型），但模型照样会造 `payment-gateway` 与 `gateway-payment` 这种近似页。光靠 prompt 挡不住，需要**检测 + 合并**做兜底。

## 参照物（抄机制、不抄自动）

- **经典实体消歧 / record linkage**：`blocking`（廉价找候选，把 O(n²) 压到可行）→ `matching`（判等）→ `merge`（合并）。工程命门是**分块**，不是每对都问模型。
- **Microsoft GraphRAG 的实体消歧**：用 embedding + 文本证据让 LLM 判「是否同一实体」再合节点——但它**让模型自动合并**。本仓**只借「判等」，不借「自动合并」**：合并是信息销毁，必须人在环。
- Karpathy / nashsu 都不做自动合并，保持一致。

## 决策

**1. 合并是只有人能触发的状态迁移**（继承 ADR-0015）。编译器、模型、lint、job 任何自动路径都**不得**调用合并；只有审核界面由人点才走。这是本 ADR 最需要被批准的一条，也是它与 GraphRAG 的根本分歧。

**2. 候选发现纯结构、零模型、零向量，现在就能跑。** blocking 用四类廉价信号（都不依赖 lane 2）：
- **slug 词形**：`NormalizeWikiSlug` 后互为前缀，或编辑距离小；
- **共享来源笔记**：`note_pages` 里两页挂的 `note_id` 集合有重叠（同源极可能同概念）；
- **标题词重叠**：token 集合 Jaccard 高；
- **已互链像一件事**：彼此有 `ref`/`mentions` 边却各讲一遍（这里第一次**读** lane 1 的关系边）。

这一步**只产候选对、不产结论**，并用 SQL/内存把候选从 n² 压到 top-K。

**3. 判等分两层，向量可选。** 第一层廉价打分，超阈值的对才进第二层；第二层**可选地**叠 lane 2 的 `page_embeddings` 余弦相似度——lane 2 未落地时该信号恒 0，**不阻塞**决策 2/4/5。本 ADR 因此在 lane 2 之前就能独立成立。

**4. 模型只做二判，不改库。** 对候选对问一次：「这两页是否同一概念？若是，建议保留哪张作 winner（理由）？」输出 `{"same":bool,"loser":"slug","winner":"slug","reason":"…"}`。新键 `wiki_merge_suggest`（默认关，复用 `ai_chat_*` 端点，每轮 pair 预算封顶如 ≤200 对）。结果**只转成 `[lint] merge <loser> → <winner>` 待办**——复用 `fileFindings` 只进 `project_todos` 的同一纪律（`wiki_lint.go`），**模型无权合并**。

**5. 合并执行 = 一个事务，只有人点采纳才走：**
- 把 loser 的所有出入边重指向 winner（`LinkWikiPages(…, 'ref')` 幂等，winner 已有同边则被 `UNIQUE` 丢弃）；
- `note_pages` 里 loser 的来源挂接改到 winner（provenance 不丢）；
- loser 置 `status=rejected` + 正文替换为「已并入 [[winner]]」——**沿用 `reject≠delete`，不删行**（删行会级联带走尚未移净的边并毁审计）；因边已全部重指向，winner 承接所有引用，loser 不留悬空入链；
- winner 的 `id/slug/content/updated_at` **一字不改**——合并是「loser 被吸收」，不是「编辑 winner」，因此仍守住 ADR-0015「不修改已批准页内容」。人改的是边与 loser，不是 winner 正文。

**6. 可选审计表 `page_merges(loser_id, winner_id, kept_at)`（若需要则 v23）。** 普通 `CREATE TABLE`，**不碰任何 CHECK 家族**（避开 `compile_jobs.kind` / `wiki_pages` 那种「加值=整表重建」的坑）。用于「看合并史」和将来「撤销一次合并」（把 loser 从 rejected 恢复、边回指）。**默认不建**，等出现真实撤回需求再加——延续 ADR-0018「覆盖取显式整段替换、不做静默合并」的克制。

**7. 预防收紧（把合并债前移）。** 在编译器 prompt 加一条硬规则：「若本条笔记讲的是词表里已存在的概念，用 `add_link`/`attach_note` 指过去，不要再 `create_page` 造近似页」。改 prompt 即可、风险低，降低候选对产生速率。

## 候选矩阵

| 方案 | 价值 | 主要风险 |
|---|---|---|
| 全自动合并（GraphRAG 式） | 省事 | 合并是**信息销毁**，比分裂（冗余）更难发现且**不可逆**；让模型销毁人已批准知识正是 ADR-0014/0015 一路拒绝的 |
| 只靠 slug 精确匹配 | 零成本 | 漏掉 `payment-gateway` vs `gateway-payment` 这类词序/近义分裂 |
| 不做 blocking、每对都问模型 | 召回最高 | O(n²) 次 LLM 调用，token 成本失控（ADR-0015 已警告成本随页面平方增长） |
| 死等 lane 2 向量再消歧 | 信号最全 | 把可用的结构消歧（决策 2/5）绑死在还没建的向量上，因噎废食 |
| **本决策：结构 blocking + 廉价判等 + 向量可选增强 + 人工合并** | 现在可建、成本可控、销毁在人手里 | 阈值调不好会漏判或刷屏；判等仍会错 |

## 后果

**正面**：给 ADR-0015 的「接受代价」补上真正的还款工具；近似页能收拢、图变干净；lane 1 的关系边第一次被读（候选发现用边）；合并走人在环，销毁可控可审计；结构 blocking 不依赖尚未落地的向量，可独立推进。

**负面（不粉饰）**：
- 合并仍是**销毁性**操作，误并两张其实不同的页 = 丢知识。缓释有三重（winner 内容不动 / 审计表 / 人在环），但**不根治**。
- 候选阈值是新的调参面：太松则每对都建议 → 审核疲劳（Lane 1 已现的失败模式），太紧则分裂照旧。
- 判等仍要模型、有成本且会错，所以只到「给人看的建议」，不到「替你定」。
- 依赖 lane 2 的余弦信号在其落地前缺席，早期消歧精度打折（但功能不缺，见决策 3）。
- 又是一次 db（合并事务）+ service（判等/blocking）+ web（合并建议区）三层同动，非单点改动。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | winner 由谁定 | 模型给建议（多来源 / 更创建早 / 入度高），界面允许人翻转，最终以人点为准 |
| 2 | 合并可否撤销 | MVP 先只留 `rejected` 墓碑可硬删；撤销（决策 6 的 `page_merges`）等有真实撤回需求再开 |
| 3 | 判等跑在哪 | 并入 lint 模型侧（复用 `wiki_lint_llm` + job + 预算），**不新开 job kind**（守 ADR-0016 一表两内核、避免 `compile_jobs.kind` 整表重建）；但用独立键 `wiki_merge_suggest` 与「矛盾/过时」分开授权 |
| 4 | 是否跨项目合并 | MVP 不跨：合并要求两页同项目、或至少一张是全局页（`project_id=0`），否则边重指向会让 loser 项目的人被动丢知识 |

## 晋升条件（Proposed → Accepted）

1. **lane 1 已 Accepted**（v21 真库演练通过）——合并要重指向受词表约束的边，故排在 lane 1 之后。
2. 合并事务契约测试：(a) winner 的 content/updated_at 一字不变；(b) 合并后 loser 出入边为空且 `status=rejected`；(c) `note_pages` 来源改挂 winner 不丢；(d) 对同一对重放幂等（边被 `UNIQUE` 吸收）。
3. 候选发现测试：四类信号各造一正例 + 一不该报的负例；top-K 截断生效；断言不做全乘遍历。
4. 模型侧：非 JSON / 编造 winner/loser slug → 不采信任何建议（沿用 lint「`page_id`/slug 必须能在库里查到才采信」的同一规则）；`wiki_merge_suggest` 关时零外发。
5. 审核界面有「合并建议」区 + 采纳/忽略，并在**真库副本上端到端演练一次**——不是只靠 fixture（Lane 1 的门禁教训：机制正确 ≠ 收益真实）。
