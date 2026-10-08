# ADR-0014: LLM Wiki 知识编译层——摄入时编译、消费时取证

- 状态：Proposed（本文只定方向与排序，未落任何代码；晋升条件见末尾）
- 日期：2026-10-07
- 关联：[ADR-0003](0003-fts5-search.md)（FTS5）、[ADR-0007](0007-session-memory-protocol.md)（context / handoff）、[ADR-0011](0011-multi-agent-memory-importers.md)（多 agent 记忆导入）、[ADR-0012](0012-semantic-search.md)（语义检索）、[ADR-0013](0013-vector-database-selection.md)（向量存储）、[ADR-0010](0010-session-auto-capture.md)（隐私默认关的纪律）；TODO **M6**；起点版本 v1.15.1

## 背景

v1.15.1 的知识层是一个**写入通道丰富、结构很薄、检索局部融合、消费端被动**的笔记库。四个失衡都有明确证据：

1. **写入零加工**。人写 Markdown、5 个外部 agent 记忆源幂等导入（`service/plugin.go:114-120`，仅 claude 自动）、agent 经 MCP `notes_create` / `handoff` 回写——三条通道都是「原文搬运」，没有任何一步是 LLM 抽取、整合、标注矛盾。
2. **结构扁平**。`project_notes` 只有 title / tags / kind / pinned / source（`internal/db/db.go:131-144`），全部挂在 project 一层：笔记**不关联具体 repository**，笔记之间**零引用**（全库无 backlink / wiki-link / 链接表，grep 零命中）。它是「打了标签的列表」，不是知识网。
3. **检索局部融合**。FTS5 trigram 覆盖 notes + todos（`db/migrate.go:27-38`、`db/search.go:21-79`），但语义 RRF 只作用于 notes 且默认关（`service/search_semantic.go:15-30`、`service/search.go:27`，`SearchAll` 不做语义融合 `search.go:30-44`）；`repo_meta`（tech stack / README / 依赖 / 贡献者，`db/migrate.go:79-88`）与提交记录**根本不在同一个可检索面上**。
4. **消费端不取证**。桌面 `AskAI` 的上下文是「10 条笔记，每条截 500 字节」（`service/ai.go:380-436`，尤其 `ai.go:431`），无检索排序、无流式、无工具调用、无引用——**它既不是 RAG，也不是编译，只是把库的一小段塞进 prompt**。W2 实现时还纠正了本文原稿的一处措辞：那 10 条不是「最近」写的，而是**最早**写的——`db.ListNotes` 的排序是 `pinned DESC, sort_order ASC, created_at ASC, id ASC`（oldest first），`aiProjectContext` 直接取前 10 条。也就是说笔记越多，最新沉淀越不可能进 prompt；这比"按时间取前 10"还要更反直觉。讽刺的是消费面早就 agent-ready：MCP 13 工具 + `/api/rpc` 反射全部 Wails 绑定（`cmd/mcp/main.go:72`、`internal/httpapi/rpc.go:12-45`）。

这个反差就是机会：**取证能力已经对外卖出去了，对内却还没用上。**

## 参照物（抄什么、刻意不抄什么）

- **Karpathy 的 LLM Wiki 原始 gist**（`gist.github.com/karpathy/442a6bf555914893e9891c11519de94f`）。核心不是检索算法而是**编译时机**：RAG 在查询时从零找碎片、理解不累积；LLM Wiki 在摄入时把新文档**合并进一份持久的、互链的 Markdown 库**——更新实体页、修订主题综述、标注新数据与旧结论的矛盾。三层：raw sources 只读、wiki 由 LLM 独占读写、`schema.md` 是这个 agent 的纪律来源。三操作：ingest / query / **lint**（查矛盾、过时声明、孤页、缺交叉引用、数据缺口）。两条导航约定：`index.md`（内容目录，每次 ingest 必更新，查询先读它再下钻）与 `log.md`（追加式、前缀可 grep 的操作时间线）。**它自己承认边界**：index 导航在 ~100 来源 / 几百页面内表现好，更大规模才需要真正的检索。
- **`nashsu/llm_wiki`**（桌面实现，~2 万星）。给出可抄的目录与工程细节：`purpose.md` / `schema.md` / `raw/sources/` / `wiki/{index,log,overview}.md` + `entities/` `concepts/` `sources/` `queries/` `synthesis/`；页面 = Markdown + YAML frontmatter，**没有数据库**；一次 ingest 可触碰 **10-15 个页面**；SHA256 跳过未变文件、两步 CoT 摄入、带崩溃恢复与重试的**串行**队列；好答案归档回 `wiki/queries/`（探索也复利）。
- **`TencentCloud/TencentDB-Agent-Memory`**（~2.8 万星，WorkBuddy / CodeBuddy / OpenClaw / Hermes / DeepSeek Harness 共用的记忆中枢，且明确致谢 Karpathy 那篇）。它在 wiki 之上多做的两件事正是 RepoNest 该抄的：**分层蒸馏** L0 原始转录 → L1 原子事实 → L2 场景 → L3 画像，检索时「L2/L3 引导上下文，要具体事实才用 BM25 + 向量 + RRF 回落到 L1/L0」，并用**条数 / 字符预算 / 超时三重封顶**防止记忆吃掉整个上下文窗口；**资产化治理**——Chat Memory / Skill / LLM-Wiki / CodeGraph 统一注册为 Memory Asset，带 ownership / version / status / visibility(private|team|restricted) / 使用计数 / agent 绑定。

## 决策

### 1. 先补结构，再让 LLM 写

第一步不含任何 LLM 调用：新增页面层与链接层——`wiki_pages`（四类：`entity` 实体 / `concept` 概念 / `source` 来源摘要 / `synthesis` 综述）+ `page_links`（双向可查的出链/入链）+ 笔记与 repository 的关联。**没有这张图，「一次 ingest 触碰 10-15 页」在数据结构上就不成立**，先上生成只会得到一堆没人能导航的文本。

现成的地基：`GenerateLLMsTxt` 已经是 index 的雏形（项目目录 + 最近笔记 + 代码库摘要，`internal/service/llm.go:16-58`，扫描上限 200 条），缺的是它背后真正的页面层，而不是再造一个目录。

### 2. SQLite 仍是唯一 SSOT，文件树只读导出，不做双向同步

这是与三个参照物最大的分歧，必须写死。它们都建立在「wiki 就是一堆 md 文件 + git」上，靠 Obsidian 提供阅读与 graph view。RepoNest 反过来：**库内结构 + 单向导出旁路**（`reponest wiki export` 渲染成 md 文件树 + frontmatter + wikilink，附导出清单）。

理由：笔记一旦有两个写入者（文件编辑器 + 应用内编辑器），冲突、去重与 `note_versions`（现每笔记至多留 50 版快照，`db/migrate.go:155-182`）都要重做一套。**单向导出已经白嫖了 Obsidian graph view 与「git 里 review LLM 改了什么」这两个真实价值，而不必接受双写代价。** 待双向同步出现真实需求信号（用户拿 Obsidian 改回来）时，另开一篇 ADR 取代本决策。

### 3. 消费端先于生成端

`AskAI` 改为真正的取证：先读 index 找相关页 → FTS5 + 向量 RRF 选页 → 读页 → **带引用**作答 → 好答案可一键回档成新页（Karpathy 的 query 闭环）。同时补流式，并把上下文预算改成条数 / 字符 / 超时三重封顶，替掉现在「固定 10 条 × 500 字节」这种既不省 token 也不保真的写法。

排序理由：这一步不改数据模型就能立刻改善已有功能；而且它是 lane 1 产物的验证场——页面层有没有用，看取证质量最准。评测用现成的 `internal/search/abeval`（Recall@k / NDCG@k + `Compare` delta + 阈值门）。

### 4. 摄入时编译：opt-in、逐源、先审后入索引

importer 从「把外部 md 原文 upsert 成 note」升级为「读源 → 抽取 → 新建/更新页面 → 更新 index + 追加 log → 标注与既有笔记的矛盾」。约束三条：LLM 写入一律打 `source='llm-wiki'`（该字段现成）+ 默认**待审**，人工批准后才进 index；一次一个源、人保持在环（Karpathy 本人如此）；整条链路默认关，延续 ADR-0010/0012「敏感能力默认关 + 文档化」的纪律。

**前置项（阻塞）**：`auto_import` 当时默认 `"1"`（`db/migrate.go:452`），启动即 `ImportAll` 所有自动源。编译层会把这个既有的隐私隐患放大（原文入库 → 派生出一堆 LLM 综述页），所以先收紧默认值与逐源门，再谈编译。

> **已收口（schema v15，2026-10-07）**：落地时发现问题比设想更深——`db.GetConfig` 把 `sql.ErrNoRows` 映射成 `("", nil)`，而启动判据是 `v != "0"`，于是**"从未设置"本身就等于放行**，不是默认值选错。修法是把判据翻成 `== "1"`（安全态成为默认态）、种子改 `"0"`、并加一条一次性迁移把旧库归零。W3 仍需另立逐源门：v15 只关掉了"启动即导入"。

### 5. Lint 是必配项，不是可选优化

lint 定时跑五查（矛盾 / 过时声明 / 孤页 / 缺交叉引用 / 数据缺口），产出**落 `project_todos`**（复用现有表，不新造反馈面）。硬约束：LLM 只能建议，不得自动改写页面——这是防「幻觉被编译成看似已确立的知识」的唯一有效手段。人类侧的裁决就是勾掉或保留待办。

### 6. 分层记忆映射到既有表，不自建远端 hub

L0 = 会话转录（已有 capture 链路）/ L1 = 原子笔记（现在的 `project_notes`）/ L2 = 项目场景（`BuildProjectContext` 已是雏形，含 handoff 优先排序，`internal/service/context.go:94-120`）/ L3 = 跨项目稳定画像（待立项）。检索按 L2/L3 引导、L1/L0 取证分层，而不是一股脑塞。

对外仍然只走已有两个面：MCP 工具 + `/api/rpc`。**不去自建或对接远端 Memory Hub**——RepoNest 的定位是 local-first（scoop 清单里的原话就是 "Local-first code project context base"）。若将来要互通，方向是 RepoNest 作为那类 Hub 的**一个本地 asset 源/审计面板**，且要等 OMP 稳定（自家导出至今标着 PROVISIONAL、导入侧刻意未写，`internal/service/omp_export.go:13-19`）。

## 候选矩阵

| 方案 | 与现状距离 | 价值 | 主要风险 |
|---|---|---|---|
| 纯加强 RAG（把 commits / repo_meta / todos 拉进统一检索面 + AskAI 用上向量） | 最近，lane 3 的子集 | 快、无迁移 | 理解仍不累积：每次查询从零拼，矛盾无人标注 |
| 文件树 wiki（照抄 nashsu / Obsidian 生态） | 需推翻现有 SSOT | 白嫖 graph view、diff 友好、git 背书 | **两个写入者**；`note_versions` / FTS5 / 向量索引全部重做；桌面应用体验倒退 |
| **本决策：库内结构 + 单向导出 + 分层 + 编译 opt-in** | 中 | 累积理解、可导航、可审计、零 SSOT 迁移 | 页面分类粒度定错日后合并痛；token 成本高（见后果） |
| 直接接腾讯 Hub 当后端 | 远 | 团队共享 / ACL / Skill 资产现成 | 违背 local-first；引入外部进程与协议 churn；隐私面陡增 |

## 后果

**正面**：知识第一次有了「谁写的、从哪来、和谁矛盾」的结构；`AskAI` 从「塞库头部」变成「取证 + 引用」；MCP 消费面获得比 13 个工具更强的导航入口；`note_versions` + `source` 字段天然构成 LLM 写入的审计与回滚底座（这两个字段就是为今天准备的）。

**负面（不粉饰）**：
- token 成本远高于朴素索引——一次 ingest 是多次 LLM 调用，且可能触碰十几个页面；本地 Ollama 跑不动时这条路就退化。
- 幻觉与遗漏会被「编译」成看起来已确立的结论，而且**比 RAG 更难发现**（错误进了 index 与综述，不在原文里）。lint + 先审后入是唯一屏障。
- 串行摄入让大库首次导入很慢；几百页之上 index 导航必然退化，要立刻让向量上位（好在 sqlite-vec 已在库内，零 CGO，这条反而是优势）。
- 隐私面放大：见决策 4 的前置项。
- 新迁移 + 新配置键 + 新前端面，是三层同时动的活儿，不是一个 sprint 的量。

**待决**：~~页面四类够不够~~（→ **已由 W1 裁决为五类**）；`schema.md` 等价物放全局还是按项目（个人倾向全局默认 + 项目覆盖）；与 `dsh-plugin-reponest` 的关系（导出旁路给 dsh 的会话读？）；向量层在页面量上来后是否要把融合范围从 notes 扩到 `wiki_pages`（受 ADR-0012「只对 notes 融合」的既有约束限制）。

> **W1 落地时的两处修正（2026-10-08，schema v16）**
>
> 1. **四类改五类**。决策 1 原列 entity / concept / source / synthesis，但决策 3 承诺的 query 闭环（「好答案回档成新页」）需要一个归宿，而 Karpathy 模式与 `nashsu/llm_wiki` 的目录里都确有 `queries/` 一类。缺了它，答案只能写回普通笔记，页面层就缺一类真实存在的产物。实现因此是五类，多出的 `query` 由 SQLite 的 `CHECK` 与 Go 侧枚举双重守住。
> 2. **「可逆」必须配自愈**。迁移带版本戳、v16 只跑一次，而派生层按定义允许被丢弃重建（决策 5 把 SSOT 留在 `project_notes`）。只在迁移里建表的话，一次 `DropWikiSchema` 之后重启就是 `schema_version=16` 却没有表的死库。故 `InitDB` 现在无条件 `EnsureWikiSchema`，沿用 `EnsureFTSIndex`「可对任何库安全调用」的同一契约。这一条是**被可逆性测试抓出来的**，不是设计时想到的——测试先失败，才看见漏洞。
>
> 3. **W1b 也已落地，并更正本文的命令名**：原文写的 `reponest wiki export` 并不存在——根 `reponest` 是 Wails 桌面应用、没有任何子命令派发。实际交付的是独立二进制 `reponest-wiki-export`（`cmd/wiki-export`），且比原计划多做两件：陈旧文件只报告不删除（删除等于宣称树归导出所有，正是决策 2 拒绝的立场），以及统计页正文里指向不存在页的 `[[链接]]`（页→页的边由 FK CASCADE 保证不悬空，正文链接是唯一会烂的地方，这份计数即 W4 lint 的输入）。
>
> 4. **W4 落地时把"五查"拆成了两层**。原文把 lint 当作一类动作，实际其中三查（孤页 / 缺交叉引用 / 数据缺口）纯 SQL 即可判定，另两查（矛盾 / 过时声明）必须问模型且会出错、会花 token。因此结构层随时可跑，模型层单独设 `wiki_lint_llm` 开关且默认关，`llm_note` 负责把"没发现问题"与"没开"区分开。另加了两条原文没有的采信规则：模型给的 `page_id` 必须能在库里查到才成为 finding，回复解析不出 JSON 就一条不提。

> 另修一处测试脆断言：v15 迁移测试把版本号绝对断言为 15，v16 一落地即红；不变量应是「v15 跑过了」（`>= 15`），否则每次加迁移都得回改老测试。

## 晋升条件（Proposed → Accepted）

1. lane 1 的迁移在真实库上跑通且**可逆**（迁移测试，参照 `migrate_fts_repair_test` 那一档严格度）；
2. lane 3 用 `cmd/abeval` 交出真实标注 query 集上的 delta，证明「取证 > 塞入」不是感觉；
3. 决策 4 的前置隐私项（`auto_import` 默认值 + 逐源门）已单独收口；
4. 页面四类与 schema 作用域有结论。

三条路线的落地拆解见 `TODO.md` 的 **M6**。
