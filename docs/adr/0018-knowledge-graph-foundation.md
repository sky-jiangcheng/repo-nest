# ADR-0018: 知识图谱底座——受控关系词表让图可查，图感知取证让图被读

- 状态：Proposed（**lane 1 代码与契约测试已落地**：schema v21 关系词表（CHECK 由 `wikiRelations` 派生，Go/DDL 单一事实源）+ 编译器选词 + approve 抽取 `mentions` + `lintRelationShape` 三查，`go test ./internal/db ./internal/service ./internal/integrity` 全绿。**唯一未勾的晋升条件是 lane 1 条件 1「真库副本演练 + 旧边只读导出」**——它要在你的实际 `dashboard.db` 副本上跑，agent 不代跑，故 lane 1 暂不晋升为 Accepted。（`cmd/wiki-rehearse` 已把该演练做成一键只读工具：`mode=ro` 打开源库 → `VACUUM INTO` 一次性副本 → 在副本上跑真实 `InitDB`(含 v21) → 校验不丢边/归一正确 → 退出码判定；由你亲自跑。）lane 2（页面向量 + 图感知取证，schema v22）前置 = lane 1 Accepted + Phase 0 delta 达标，尚未整体开工；其中**图感知取证机制（一跳扩召回 + PPR 定序，零 schema 改动）已按 `wiki_graph_search` 默认关落地**——关时行为与从前逐字一致，开时的收益仍待 Phase 0 `abeval` 度量背书才算兑现，故 lane 2 不据此晋升）
- 日期：2026-10-09
- 关联：[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（本文兑现其"先补结构"承诺的下半段——结构有了但要能查询；并兑现其决策 5 lint"数据缺口"一查的具体形状、并收口其待决"语义融合是否扩到 wiki_pages"）、[ADR-0015](0015-w3-minimal-compile-loop.md)（本文不违反其"编译器永不修改已批准页"——新增只发生在边/待审面）、[ADR-0016](0016-async-batch-jobs.md)（lane 2 若慢，复用一表两内核）、[ADR-0012](0012-semantic-search.md)（RRF 语义融合）、[ADR-0013](0013-vector-database-selection.md)（零 CGO 向量存储）、[ADR-0010](0010-session-auto-capture.md)（敏感能力默认关的纪律）；TODO **M6** 的收口 + 新增 **M7**；起点 schema v20
- 配套实施计划：`RepoNest-知识图谱实施计划-2026-10-09.md`（本 ADR = 其 Phase 1 + Phase 2；Phase 3 实体消歧、Phase 4 社区摘要与时态图各自单开 ADR-0019/0020/0021）

## 背景

ADR-0014 的第一条决策是"先补结构，再让 LLM 写"，理由是"没有这张图，一次 ingest 触碰 10-15 页在数据结构上就不成立"。这张图现在建成了（`wiki_pages` + `page_links` + `note_pages`，schema v16 起），但**建成 ≠ 能用**。三处代码级证据说明它目前是"写而不读"的：

1. **关系是野字符串**。`page_links.relation` 是自由文本，默认 `'ref'`（`internal/db/wiki.go:61-70`，无 `CHECK`）。编译器写边时把模型给的字符串原样落库，为空则填 `"compiled-from"`（`internal/service/wiki_compile.go:377`），问答存页写 `"cites"`（`wiki_evidence.go:292`）。**没有任何一处代码约束 relation 的取值**，所以这张"图"没法按语义查询——你无法问"所有 `depends` 关系"，因为没人保证这个词被统一拼写。
2. **取证根本不读图**。`GatherEvidence`（`wiki_evidence.go:91`）只调 `db.SearchWikiPages`（FTS）和 `s.SearchNotes`，**从不调 `WikiEdgesFrom`/`WikiEdgesTo`**。边目前只被三处一跳读取：导出渲染反链、lint 结构检查、编译 prompt 附带上下文。用户在 Obsidian graph view 里能看到图，但 AskAI 回答时**把图当不存在**。
3. **页面没有向量**。`internal/db/vecindex.go` 的 vec0 索引 `note_embeddings` 是 **notes-only**（`KnnNoteIDs`、`PutNoteEmbedding` 全部 keyed by note rowid）。`wiki_pages` 没有对等的向量面，所以 ADR-0014 待决里"语义融合扩到 wiki_pages"那半条至今未做。全仓 grep `WITH RECURSIVE|pagerank|leiden|louvain|community|betweenness|transitive`——**零命中**。

结论：Wiki 的"知识网"目前只有**节点**能用（FTS 检索页面），**边**和**向量**两条腿都没接上。这与 ADR-0014 的愿景（"可导航、可取证"）之间，差的不是数据模型，是把模型用起来的那层。本 ADR 就是那层，且大部分零新依赖、纯 SQLite + Go 可做。

## 参照物（抄机制、不抄实现）

- **Karpathy LLM Wiki**：`index.md` 是人肉导航，`[[wikilink]]` 是边。他自承"index 导航在几百页内好，更大才需真检索"——RepoNest 的规模终会越过那条线，所以"图可查询"是为"图可遍历检索"铺路。
- **Microsoft GraphRAG**：两条核心机制值得抄思想——**local search**（以命中实体为种子在图上做带个性化的随机游走选证据，≈本 ADR 的 PPR）与 **community summaries**（对图做社区检测、逐社区摘要，填 synthesis 空层——本 ADR **不做**，见 Phase 4）。它的实现绑 Neo4j + Leiden + 全局摘要 map-reduce，违背本仓 local-first / 零 CGO / 单 SQLite，**只借判据不借栈**。
- **TencentDB-Agent-Memory**：分层蒸馏里"要具体事实才回落到 BM25 + 向量 + RRF"，正是本 ADR lane 2"向量粗筛 → 图精排"的分层思路。

## 决策（两半，各自晋升）

### Lane 1（MVP，schema v21）：把边从"字符串"变"可查询的关系"

**1. 受控关系词表，8 型。** 钉死为 `ref`（默认泛引用）/ `part-of` / `depends` / `implements` / `documents` / `supersedes` / `contradicts` / `mentions`。太少（如只 3 型）lint 判不出环/倒挂；保持自由串等于没有图。词表住在 Go 枚举 `internal/db/wiki.go`（新增 `Relation*` 常量 + `ValidRelation`），与 SQLite `CHECK` 双重守（沿用 kind 五类"CHECK + Go 枚举双守"的既有手法）。

**2. `page_links` 加 `CHECK` 需要整表重建。** SQLite 不能 `ALTER` 列级 `CHECK`（红线），故 v21 = 建带 `CHECK` 的新表 → `INSERT … SELECT` 时把**既有非法 relation 规范化进 `ref`**（不丢边、只改标签）→ drop 旧表 → rename。全程幂等，且**可逆测试要求** drop/rebuild 前后 `sqlite_master` 逐字节 + 边多重集相等。迁移前须能只读导出一份旧边供回看（见"后果·负面"）。

**3. 编译器产出边时从枚举里选词。** 改 `wiki_compile.go:227-228` 的 JSON 契约 literal，把 `"relation":"…"` 写成封闭枚举 + 附定义与示例；默认串 `:377` 从 `"compiled-from"` 改 `"ref"`；`LinkWikiPages`（`wiki.go:488`）对未知 relation **降级为 `ref` 并记日志而非报错**（脏值不得打断写入）。既有守卫全不动：`add_link` 至少一端为本轮产物（`ownedByCompile :403`）、预算、只写 pending、永不改批准页。

**4. 正文 `[[wikilink]]` 在 approve 时抽取为 `mentions` 边。** 导出已在解析正文链接并统计 `dangling`（`wiki_export.go:50,243`）。反向补一刀：对**能解析到已存在页**的正文链接，`LinkWikiPages(…, 'mentions')`——`UNIQUE(from,to,relation)` + `ON CONFLICT DO NOTHING` 天然幂等。解析不到的维持"只报不建"（dangling 语义不变）。**触发点在 approve**（人批准的页才进图），复用 `ApproveWikiPage` 路径，不单开 job、不进编译器的自动写面。

**5. 关系词表的结构体检进 lint。** `wiki_lint.go` `RunWikiLint`（:125）加 `lintRelationShape`，**照 `lintMissingRefs` 的"预取边集避免 O(n²) 查询"写法**（:192-202），并按被审项目作用域过滤边。检查项（落地时定稿的三条，均无歧义可判）：**`supersedes` 时间倒挂**（取代方 `updated_at` 早于被取代方）、**`part-of`/`depends` 成环**（组成与依赖应无环，着色 DFS 找回边）、**`contradicts` 单向**（冲突本应对称，缺反向即提示）。**产出仍只进 `project_todos`**（`fileFindings :389`），不破"lint 不改页"红线。

> **实现时对本条做了一处替换**：草案原列第三查为「`documents` 指向非 source」，落地时发现它要么恒真、要么依赖一套没敲定的 kind↔relation 全矩阵（ADR 待决 #1 已明确词表先不定矩阵），是个判不准的伪检查。换成 **`contradicts` 对称性**——它不依赖任何跨字段约定、纯图内可判，且真能抓出"只标了一半的冲突"。三条检查都不引入 kind↔relation 约束，保持"先只定边型"的承诺。

**Lane 1 明确不做**：锁死 kind↔relation 全矩阵（先只定边型，矩阵留待有真实数据再收）、批量重写既有边的 relation（迁移只归一非法值，合法旧边不动）、给边加权重/属性（属 lane 2）。

### Lane 2（schema v22，晋升前置见末尾）：让取证读图

**6. 页面独立向量空间。** 仿 `note_embeddings`，新增 `page_embeddings`（vec0，keyed by `wiki_pages.id`，**只嵌 approved 页**）+ `page_embeddings_meta`。**必须独立表**——`wiki_evidence.go:104-108` 已记 `FuseRRF` 对 note/page 混合 id 会串号（`hybrid.FuseRRF` 返回的是裸 int64 列表，page#7 与 note#7 无法区分）。增量仿 v14：`page_embed_dirty` 队列 + 触发器（正文真变 / status 迁入 approved 置脏、迁出删向量）。**门控挂在既有 `semantic_search` 之后**（那是用户同意把文本发端的授权），不新设嵌入开关，避免"主开关开、页嵌关"留陈旧索引。store 走 `vectordb.Store` 注册表（local=sqlite-vec 默认，远程不可达退 local）。⚠️ `vectorStore()` 有 memo（`search_semantic.go:71`），改 `vector_store*`/`embedding_*` 或报错必须 `invalidateVectorStore()`，否则页嵌静默用旧端点。

**7. 取证读图 = 一跳/二跳扩召回 + PPR 排序。** 注入点是唯一且已预留的：`wiki_evidence.go` `evidencePages`（:163-182），其注释（:104-108）明说"position interleaving 用到页有自己的向量空间为止"——正是此处。
- **扩召回**：以 FTS/页向量命中的 seed 页为起点，`WITH RECURSIVE` 沿 **relation 白名单**（`ref`/`part-of`/`documents`；**不沿 `contradicts`**，矛盾是给用户看的、不是喂进同一份证据的）取 1-2 跳邻居入候选。
- **排序**：纯 Go 在有向 `page_links` 子图上做带 reset 的随机游走（PPR）。reset 向量 = 检索命中页 +（笔记命中经 `note_pages` 投影到其所属页）。转移按 relation 白名单与边权。几百~几千页毫秒级收敛（~15 轮），远在 `EvidenceBudget.Deadline = 3s` 内。用它替掉现在的 bm25 顺序。
- **融合**：notes 侧沿用 `SearchNotes`→语义 RRF；页面向量命中与页 FTS 命中在**页专属 id 空间**做 `hybrid.FuseRRF`，再与笔记结果**按类型分列交织**（不跨类型塞进同一次 RRF，破串号）。
- **门控** `wiki_graph_search`（新布尔键，默认关）：关时 `evidencePages` 走现有 FTS 顺序，**零行为回退风险**；开时邻居与 PPR 生效。

**Lane 2 明确不做**：外部图库（Neo4j/Memgraph/KuzuDB）、RDF/OWL/SPARQL、GNN——全部违背 local-first + 单 SQLite + 零 CGO（ADR-0014 决策 2、ADR-0017）。纯 SQL 递归 + Go 侧 PPR 在几万页以内足够；真要上外部图库，已有 `vectordb` 那套"可插拔 + 不可达退回 local"的接缝可套用，属另立 ADR 的决策。

### 前置（阻塞 lane 2 度量，不阻塞 lane 1 编码）

Phase 0——真实标注 query 集 + `abeval` delta（`cmd/queryset`/`cmd/abeval`，脚手架已就绪，标注须人判断）。lane 1 的价值是**结构性的**（边可查询、lint 能判环），不需要召回度量背书；但 lane 2 的核心主张"图感知取证 > 纯 FTS 取证"**必须**在同一份 `queries.jsonl` 上量出 evidence→evidence+graph 的正向 delta 才算兑现，否则只是"多了会算的机器"，不是"更准的证据"。

## 候选矩阵

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| 只做 lane 1（词表+wikilink+lint），不动取证 | 最近，一个 v21 | 图立刻可查询、lint 有结构判据、零外发新面 | 边仍不参与回答，Obsidian 图与 AskAI 各看各的 |
| **本决策：lane 1 结构 + lane 2 消费，均默认关** | 中 | 图既能查又能在回答时被读；补上 ADR-0014 收口项 | lane 2 三层同动非单 sprint；PPR 可能引入低质邻居 |
| 跳 lane 1 直接给页做向量 | 看似省一步 | 快 | relation 仍是野字符串，PPR 沿什么边扩散都说不清，白名单无从定义；串号风险裸奔 |
| 照抄 GraphRAG 上 Neo4j | 远 | 团队共享、成熟算法 | 破 local-first/单库/零 CGO；几百页规模用不上；隐私面陡增 |
| 顺手做社区摘要（Phase 4） | 中 | 填 synthesis 空层 | 触及 `compile_jobs.kind` 的 CHECK 加值 = 整表重建；应单独排期与 ADR |

## 后果

**正面**：边从噪声变成可查询的语义（`WHERE relation='depends'` 第一次有意义）；AskAI 从"平铺文本取证"升级成"图上取证"；lint 的"数据缺口"一查从启发式变成结构事实；ADR-0014 待决"融合扩到 wiki_pages"就此收口；页面层与 notes 层共用同一向量后端与门控，不新增第二套基建。

**负面（不粉饰）**：
- **v21 会改写既有边的 relation**（非法值→`ref`），这是动用户库的不可逆操作——虽不删边，但标签变了。缓解：迁移只"归一非法、保留合法"，配可逆测试逐条对账，且**上线前在真库副本演练并导出旧边清单**。
- 词表选窄了会漏语义、选宽了没人用；8 型是起点不是终态，日后扩词表又是一次 rebuild。
- lane 2 有**外发面变宽**：开 `semantic_search` 后，页面正文（可能含代码片段、会话摘录）经 embedding 端点。文档必须像 ADR-0015 决策 7 那样写清"开了会把什么发出去"。
- PPR 邻居有噪声：三重收口（只 approved 页 + relation 白名单 + 三重预算硬拦）能压住但不根治；最终仍以 abeval delta 论成败。
- 两半合一篇 ADR 但**分两次晋升**，需读者分清"已批准的边界"——故状态行把 lane 1/lane 2 的 schema 与前置写得明明白白。

## 待决（附推荐）

| # | 问题 | 推荐 |
|---|------|------|
| 1 | 词表是否含 `mentions`（正文 wikilink 自动抽取） | **含**——它是唯一不依赖模型、可确定性回填的边来源，先攒图密度 |
| 2 | 迁移遇到旧库里五花八门的 relation 值 | **归一入 `ref` 并只读导出旧值清单**，不丢弃、不猜映射 |
| 3 | lane 2 的 PPR 与现有 bm25 谁定序 | **PPR 定序，bm25 退为 seed 召回**；`wiki_graph_search` 关时维持 bm25 现状 |
| 4 | 页面向量是否复用 notes 的 embedding 模型/维度 | **复用同一 provider 与 dim**，但索引物理独立（串号红线），meta 各存各的 |
| 5 | 扩召回跳数 | **默认 1 跳**，2 跳留给 abeval 证明有增益再放开 |

## 晋升条件

**Lane 1（→ lane 1 Accepted，可先于 lane 2 单独晋升）**：
1. v21 迁移可逆——rebuild 前后 `sqlite_master` 快照 + 边多重集相等，且合法 relation 不被改写、非法值确定性归一入 `ref`；真库副本演练一次并留存旧边清单；
2. 契约测试：`LinkWikiPages` 对未知 relation 降级不报错；编译器产物 100% 落在 8 型枚举内；
3. approve 抽取 `mentions` 边幂等（重跑不增边），且**只碰已批准页**（pending 页正文链接不进图）；
4. `lintRelationShape` 三查（倒挂/环/单向冲突）各一例触发且**只写 `project_todos`、逐字段不改页**（并入 `TestWikiLint_NeverMutatesPages` 的不变量）；
5. 词表定稿 + 双语文档（`docs/features/knowledge.md` 中英"边类型"节）+ 本 ADR 状态行更新。

**Lane 2（→ lane 2 Accepted，前置 = lane 1 已 Accepted）**：
6. **Phase 0 的 `queries.jsonl` 上，evidence+graph 相对 evidence 的 Recall@k/NDCG@k delta 为正且过 `abeval` 门**——机制正确不等于收益真实，这一条不过关 lane 2 不得 Accept；
7. 页向量独立空间：`page#N` 与 `note#N` 同时存在时 RRF 不串号（构造专项回归）；`wiki_graph_search` 关时 `evidencePages` 结果与今天**逐条一致**（零行为回退）；
8. PPR 纯函数单测（收敛、边权、reset 投影）+ 递归 CTE 环/深度上界测试；`vectorStore()` memo 在配置变更后对页面路径同样失效（防用旧端点）；
9. 三重预算仍兜得住邻居扩召回（deadline 触发即停在已收集集合，不超发）。
