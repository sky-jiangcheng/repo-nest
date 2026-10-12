# RepoNest：LLM Wiki 完成度审核 + 知识图谱机会评估

- 审核日期：2026-10-09
- 代码基线：`github/master` 已 fast-forward 同步（本地 0/0），HEAD = `214facd`（v1.16.2 packaging sha256），前一个功能提交 `3a7e04a`（P35 全局样式迁移 CSS Modules 收口）
- 方法：逐项对照 ADR-0014/0015/0016/0017、TODO.md M6、CHANGELOG 与 `internal/db/wiki*.go` / `internal/service/wiki_*.go` / `internal/db/vecindex.go` 的真实实现，并对纯 Go 包做了离线 `go build` + `go vet`（均 rc=0）
- schema 版本：`ExpectedSchemaVersion = 20`（v16 页面层 / v18 status+source / v19 compile_jobs / v20 kind 判别列）

---

## 一、结论先行

1. **代码已更新到最新**，且同步是安全的 fast-forward（上游仅新增一个打包 sha256 提交），工作区只有一批未跟踪的 `.md` 与 `outputs/`，无被跟踪文件改动。
2. **LLM Wiki（ADR-0014）的功能主干已全部落地**——W0 隐私门 → W1 页面/链接结构 → W1b 单向导出 → W2 AskAI 取证（含流式）→ W3 摄入编译（含异步 job + 审核 tab）→ W4 lint 五查 → W5 分层上下文，全部有实现、有契约测试，W3 还在真库副本上跑通并逐页人审。**这不是"还差很多"，而是"机制已成、证据未补"。**
3. **真正的差距是三类**：(a) 晋升所需的**真实收益证据**（标注 query 集 + `abeval` delta）没做，ADR-0014 因此仍停在 Proposed；(b) **消费端的"图"还没被用起来**——检索完全不走 `page_links`、`wiki_pages` 也没有向量索引；(c) **生产端只会新建不会收敛**——概念分裂被当作 MVP 代价，缺实体消歧与页面版本快照。
4. **知识图谱技术非常契合，且大部分零新依赖、纯 SQLite + Go 就能做**。当前数据结构本身已经是一张有向属性图（wiki_pages=带 kind 的节点，page_links=带 relation 的边，note_pages / note_repositories=二部边），只是被"写而不读"。这是最高性价比的下一步方向。

---

## 二、离 LLM Wiki 实现还差多少（按 ADR-0014 四条 lane + 晋升门逐项）

| 能力 | 状态 | 证据 | 还差什么 |
|---|---|---|---|
| W0 隐私前置门（auto_import 默认关） | ✅ | schema v15，判据翻 `=="1"`，旧库一次性归零 + 回归 | 无 |
| W1 页面/链接结构 | ✅ | v16：`wiki_pages`(五 kind) + `page_links`(有向边) + `note_pages` + `note_repositories`，整层可弃 + 自愈 | 无 |
| W1b 单向导出旁路 | ✅ | `internal/service/wiki_export.go` + `cmd/wiki-export`（Obsidian graph view 可吃，stale 只报不删，dangling 计数） | 与 dsh-plugin 关系仍未决 |
| W2 AskAI 取证 + 三重预算 + 引用 + 存页 + 流式 | ✅ 机制 / ❌ 真实收益 | `wiki_evidence.go`：GatherEvidence + FileAnswerAsPage；流式五提交已落；fixture 门禁 recall 0→1.0 | **真实标注 query 集缺失**（脚手架 `cmd/queryset` 已就绪，标注要人判断）——这是晋升门 #2 |
| W3 摄入时编译（opt-in、只写 pending、永不改批准页、硬预算） | ✅ | v18 status/source 列 + `wiki_compile.go` 三 op；真库跑通 5 条→7 pending 页，4/7 忠实、3/7 局部幻觉（绑定 bonsai-27b） | 生产端**只能新建**；synthesis/query 靠人；schema.md 仍硬编码 prompt（决策已定未外置） |
| W3 异步 job | ✅ | v19/v20 `compile_jobs`（一个表 compile+lint 两内核），取消/崩溃恢复有测试 | 无 |
| W3 审核 UI | ✅ | `web/src/pages/Review.tsx` + reject 不删行保留入链 + `DeleteCompiledPage` | 批量/合并/修订建议仍薄；审核疲劳（真库约四成会拒）无根治 |
| W4 lint 五查 | ✅ | `wiki_lint.go`：三查纯 SQL 随时跑、两查走模型默认关；findings 只进 `project_todos`，代码路径无法改页 | 三查全是结构启发式，未用图度量（见下） |
| W5 分层上下文 L3/L2/L1/L0 | ✅ 管道 | `context_layers.go` + `LayeredProjectContext`；逐层独立预算 | **管道≠蒸馏**：L3/L1 自动生成还没做（依赖编译器产物） |

**一句话**：功能面 ≈ 95% 完成，**但"这是不是一次成功的产品升级"尚未被数据证明**——缺的正是 ADR-0014 晋升条件 2（真实标注 delta）。补上这一条之前，"LLM Wiki 已实现"只能算"已建好、未验证"。

---

## 三、消费端的图是"死"的（KG 机会的技术根因）

代码级事实（已 grep 核验）：

- **检索不看图**：`GatherEvidence`（`wiki_evidence.go`）只做 `SearchWikiPages`(FTS) + 笔记检索 round-robin 交织，**从不调用 `WikiEdgesFrom/WikiEdgesTo`**。`page_links` 目前只被三处"读"：导出渲染反链、lint 结构检查、编译 prompt 附带上下文——**没有一处用于回答时扩召回/排序**。
- **页面没有向量**：`internal/db/vecindex.go` 只有 `note_embeddings`(vec0) 与 `KnnNoteIDs`。**`wiki_pages` 无 vec0 索引**，语义融合仍锁在 notes（ADR-0012 约束 + TODO 明确写"语义融合扩到 wiki_pages 那半条仍未做"）。
- **边类型是野字符串**：`page_links.relation` 默认 `'ref'`；编译器落边时 `firstNonEmpty(op.Relation, "compiled-from")`，取证存页时写 `"cites"`。即模型可以自造任意 relation，**没有受控词表**，图因此不可按语义查询。
- **没有任何图算法**：全仓 grep `pagerank|leiden|louvain|communit|WITH RECURSIVE|transitive|betweenness` **零命中**。

这三条恰好是知识图谱技术能直接命中的地方。

---

## 四、知识图谱技术：哪些可用、怎么落（按性价比分档）

> 硬约束：本项目立身之本是 local-first + 单一 SQLite SSOT + 零 CGO（ADR-0014 决策 2、ADR-0017）。所以推荐项一律"纯 SQL / 纯 Go、不加第二个存储、不引 CGO"。

### Tier A｜零新依赖、当前规模立刻见效（强烈建议先做）

1. **受控 relation 词表（轻量本体）**
   定 5–8 个稳定边型（如 `ref` / `depends` / `implements` / `documents` / `contradicts` / `supersedes` / `part-of` / `mentions`），用 `CHECK` 或触发器钉住 kind↔relation 合法矩阵。把编译器 prompt 的 `relation` 收成枚举、lint 可按边型校验。**这是把"噪声边"变"语义图"的最低成本一步。**

2. **1–2 跳图扩召回（`WITH RECURSIVE`）**
   在 GatherEvidence 里，以 FTS/向量命中为种子，沿白名单 relation（如 `ref`/`part-of`，不沿 `contradicts`）做递归 CTE 邻居扩展进上下文。SQLite 原生支持，零依赖。让"取证"第一次具备图结构感知。

3. **Wikilink 抽取入库（mention→边）**
   导出已在统计正文 `[[链接]]` 的 `dangling_links`；反过来把**能解析到已存在页**的正文 `[[slug]]` 自动补成 `relation='mentions'` 的边，让人手写的 wiki-link 真正进图。

4. **中心性度量喂 lint**
   度数/介数（小规模纯 Go 可算）替换现在"缺交叉引用/孤页"的启发式：可判"高入度零出度（吸收态）""零入度有出度（孤立生产者）""桥页"。lint 从文本建议升级为结构体检。

### Tier B｜明显价值、成本中等（A 做完后顺势推进）

5. **Personalized PageRank 选证据（GraphRAG 局部检索）**
   检索命中作种子，在 `page_links` 上做带 reset 的随机游走（纯 Go 向量迭代，~15 轮收敛），按 PPR 分给页面排序。几百~几千页规模下毫秒级、内存可忽略。**这是把"检索"升级成"图上取证"的核心增益。**

6. **`wiki_pages` 向量索引 + 融合扩到页面（补上那半条 TODO）**
   复用已在库内的 sqlite-vec，给页建 vec0；`FuseRRF` 把 kind 从 note 扩到 page。直接改善"孤页召回"和"相似概念"。也是下面实体消歧的前置。

7. **实体消歧 / 概念合并（治"概念分裂"这个已知的 MVP 痛点）**
   blocking（slug 前缀 + 共享来源笔记 + 页向量余弦）找候选对 → LLM 判"是否同一" → **产出 merge 建议进 `project_todos`（人批才合，绝不自动合）**，正好绕开"编译器不得改批准页"的红线。真库演练里四成被拒就是这个问题，这是它的结构性解法。

8. **社区检测 → synthesis 主题页（GraphRAG 全局检索）**
   Label Propagation（~20 行 Go、零依赖）或 Louvain（质量更高，纯 Go 可实现）把页面聚成主题社区，逐社区摘要 → 自动填 `synthesis` 层与 W5 的 L3 画像。当前 synthesis/query 全靠人写，这一步让"综述"能生长。

### Tier C｜战略项 / 暂缓

9. **时态图（bitemporal 边 + `page_versions` 快照）**
   仿 `note_versions`（已是现成审计底座）给页面加版本，边加 `valid_from/valid_to` 或 `supersedes`。**这是"允许编译器安全修订既有结论"的前置**，把 ADR-0015 推迟的能力补回来。价值高，但要单独 ADR。

10. **不建议现在引入的**：Neo4j/Memgraph/KuzuDB、RDF/OWL/SPARQL、GNN。全部违背 local-first + 单库 + 零 CGO。纯 SQL 递归 + Go 侧算法在几万页以内足够；真要上外部图库，也已有 `vectordb` registry 那套"可插拔 + 不可达退回本地"的接缝模式可套用（KuzuDB 是嵌入式里最不坏的候选，但属另立 ADR 的决策）。

---

## 五、建议执行顺序（把 KG 与"补证据"排进同一条线）

1. **先补 ADR-0014 晋升门 #2**：`cmd/queryset` 导出工作表 → 人工标注 → `cmd/abeval` 跑真实 delta。没有它，Wiki 功能再全也只是"未验证"。（人工判断，不可代劳）
2. **Tier A 一次性小迁移**：受控 relation 词表 + CHECK 约束 + Wikilink 抽取入库。低成本，立刻让图"可查询"。
3. **Tier B-5/6**：页面向量索引 + 融合扩页 + PPR 扩召回，把消费端从"平铺文本取证"升级成"图上取证"（回到 1 的评测里能量化收益）。
4. **Tier B-7 实体消歧 merge 建议**：直接压审核疲劳与概念分裂。
5. **Tier B-8 社区摘要** 与 **Tier C-9 时态图**：分别解决"综述靠人"和"编译器能否改页"，各自单开 ADR。

---

## 六、审核附带提醒

- 工作区有 **11 个未跟踪文件**（9 份 `P35-*迁移说明与复核清单.md` + `RepoNest-待办复核与重排-2026-10-09.md` + `outputs/`），本次更新未受影响，但建议尽快定性提交或清理。
- **`RepoNest-待办复核与重排-2026-10-09.md` 已过时**：它把"Knowledge/Heatmap 的 `*.module.css` 未建"列为待办，但二者（`web/src/pages/Knowledge.module.css`、`web/src/components/Heatmap.module.css`）实际已在 `3a7e04a` 落地；它的"9 项全开放"清单也未反映 M6 已基本完成。别拿它当现状依据。
- 纯 Go 包 `internal/db`/`internal/search`/`internal/importers` 离线 `go build` + `internal/service` `go vet` 全绿；完整 `go test -race`（需 wails 依赖与 tags）与真桌面冒烟未在本次运行，仍属 TODO「桌面 GUI 回归冒烟」开放项。
