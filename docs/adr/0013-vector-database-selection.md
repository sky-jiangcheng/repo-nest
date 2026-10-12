# ADR-0013: 本地向量存储选型（sqlite-vec）与安装引导

- 状态：Accepted（本地 sqlite-vec 默认 + `VectorStore` 接缝已落地；`cmd/vector-init` 引导 + 远程 Qdrant 可选、不可达自动退回本地，均已实现）
- 日期：2026-10-03
- 关联：[ADR-0003](0003-fts5-search.md)（FTS5）、[ADR-0012](0012-semantic-search.md)（语义检索：Embedder + RRF 融合）、TODO M3

## 背景

M3 语义检索需要「把向量存起来 + 做相似度检索」的存储层，与 FTS5 结果经 RRF 融合。本 ADR 由用户选型文档引入，但据本仓 2026-10-03 的实际进展做了两点**事实校正**（原选型文档的过时/错误处见下）。

**先厘清两个独立的「本地 vs 远程」轴**（原稿把二者混在一起）：
- **轴 A · Embedding provider**（文字→向量怎么算）：本地（自托管 Ollama）/ 远程（OpenAI 兼容 API）。→ 已由 `hybrid.Embedder` + `hybrid.RemoteEmbedder` 支持（ADR-0012），base_url 指向 Ollama 即本地、指向云 API 即远程。
- **轴 B · 向量存储**（向量存哪、怎么 KNN）：本地嵌入库 / 远程向量数据库服务。→ 本 ADR 决策 = **本地**。

## 事实校正（相对用户选型原稿）

1. **sqlite-vec 不需要 CGO。** 本仓用 `modernc.org/sqlite/vec`——sqlite-vec 的**纯 Go 移植**，经 `sqlite3_auto_extension` 装载。已在 `CGO_ENABLED=0` 下 `go build ./...` 与 `internal/vecprobe` 的 vec0 建表/KNN/`vec_distance_l2` 实测通过。因此**不需要** `//go:build vec` 标签、不需要静态链接 C 扩展。
2. **向量存储层已落地、非「尚未」。** `internal/db/vecindex.go` 已实现 vec0 派生索引（`EnsureVectorIndex(dim)` 按 dim 自管重建、`Put/Delete/KnnNoteIDs/Clear/Drop`），并已接进默认关的 A 检索融合（ADR-0012）。
3. 因第 1 点，**`chromem-go`（原为「避免 CGO」的纯 Go 备选）的动机消失**——modernc sqlite-vec 已是纯 Go；且它能与结构化数据同库同事务，优于另起一套 chromem 存储。故降为不采用。

## 决策

**轴 B 选：sqlite-vec（`modernc.org/sqlite/vec`，纯 Go）作为唯一默认向量存储。**

1. **同库同文件**：向量存进既有 `repotory.db` 的 `vec0` 虚拟表 `note_embeddings`（rowid=note id），与 notes / FTS5 同库、同事务、同备份。
2. **派生缓存**：向量是可从 `project_notes` 全量重算的缓存（`RebuildEmbeddings`），换 embedding 模型/维度即 drop+重建；SQLite 主库仍是唯一事实源。dim 记在自管 `note_embeddings_meta`，不读 sqlite-vec `_info` 影子表列名（跨版本不稳）。
3. **默认关**：语义检索仅 `semantic_search=1` 且向量索引就绪且配好 embedding provider 才生效；任何未配/失败一律优雅退回纯 FTS5（ADR-0012）。
4. **远程向量库经 `VectorStore` 接缝以 Qdrant 实现、默认本地 + 自动退回**：`internal/search/vectordb` 定义 `Store` 接口（`Ensure/Clear/Upsert/Search`），两实现——`Local`（sqlite-vec，默认）与 `Qdrant`（REST：建集合 / 写点 / `points/search`，`api-key` 可选）。`vectordb.Open` 按 `vector_store`(local|qdrant)/`vector_store_url`/`vector_store_api_key`/`vector_store_collection` 选择；**选了 qdrant 但未配/不可达则静默退回本地**。默认仍本地（个人库够用），远程面向 >百万向量/多端共享的 B 端；**Qdrant 与 Weaviate 均已实现**，再换 Pinecone/Milvus 只需再加一个 `Store` 实现。REST 契约带 build-tag 门控的**真实服务冒烟测试**（`ollamalive`/`qdrantlive`/`aelive`，CI 默认不跑），已在本地真 Ollama + 真 Qdrant 容器跑通（含全链路语义召回）。云端 Qdrant / 其它厂商未测。

## 安装引导（新增，落地「引导用户到设置」）

`cmd/vector-init`（对外的 `repotory vector-init`）——**默认本地、引导式、远程为显式可选**：

1. 环境自检：SQLite 版本 + `vec_version()`（确认 vec0 扩展已装载）。
2. 存储层健康检查：建 vec0 → 写测试向量 → KNN 往返 → 清理，逐条打勾（就是本 ADR 决策 1/2 的最小验证）。
3. Embedding provider 选择（轴 A）：
   - `[1]` **本地 Ollama（默认推荐，离线）**：base_url `http://localhost:11434/v1`，`nomic-embed-text`/`bge-m3`；
   - `[2]` **远程 OpenAI 兼容 API**（联网、内容出机器，ADR-0012 风险项）；
   - `[3]` 跳过，稍后在设置里配。
   选定后写入 `embedding_*` 配置（**api key 走脱敏存储，不回传前端**）。
4. 收尾**指向设置页**：打印「去 设置 → 插件 复核/调整 provider、打开 `语义检索` 开关、点『重建索引』」——即用户要的「引导到向量库设置」。`semantic_search` **保持默认关**，由用户在设置里显式开。
5. 远程向量库：`cmd/vector-init -store qdrant -store-url <url> [-store-api-key <k>]` 写入 `vector_store*` 配置并探测；不可达自动退回本地并如实打印。契约正确性由 `qdrantlive`/`aelive` 冒烟测试背书（本地真服务已跑）。

## 理由

- 单二进制 / 同库事务 / 已有 FTS5 / 零 CGO / 规模够用——sqlite-vec 是唯一同时满足全部产品约束的选项；原稿列的独立服务全都要额外进程与网络，违背本地优先定位。
- 「默认本地 + 引导到设置 + 远程确认后可选」把易用性（开箱即用）与灵活性（B 端确有远程诉求时可控切换）分开，且把语义检索的**风险项（内容出机器）放在显式开关之后**，符合 ADR-0010/0012 一贯的「敏感能力默认关、文档化」纪律。

## 后果

- 正面：向量能力零新依赖、零 CGO、与主库同生命周期；安装引导让本地默认开箱可用、设置可发现。
- 负面：单库向量规模有上限（个人库远未触及）；真实服务冒烟测试靠 build-tag 门控、CI 默认不跑（云端 Qdrant 未覆盖，接云端时补验）。
- 待决：更多远程后端（Weaviate/Pinecone）按同一 `Store` 接缝再加；embedding 与 key 在纯云场景的更严格管理（如系统 keychain）视需求。

## 候选矩阵与扩展（2026-10-03 复核，回应用户"候选多一点/配置灵活"）

后端经 `vectordb.Register(kind, factory)` 注册为**可插拔 registry**（`Kinds()` 供 vector-init / 设置枚举；未知 kind 或远程不可达一律退回本地）。加一个后端＝一个 `Store` 实现 + 一行 Register，调用方零改动。按是否**守零 CGO** + 嵌入/远程分两类：

| 后端 | 形态 | 零 CGO? | 定位 | 接入成本/备注 |
|---|---|---|---|---|
| **sqlite-vec (modernc)** | 本地·同库 | ✅（本仓已验） | 默认 | 已实现；个人库规模绰绰；2026 评测称"pragmatic winner"，未过时 |
| **chromem-go** | 本地·纯 Go 库 | ✅ | 轻量纯 Go 备选 | 纯 Go、向量入内存，适合小数据；未加（无强需求） |
| **Bleve** | 本地·纯 Go 引擎 | ✅ | 文本+向量一体 | 纯 Go、久经考验；**可连 FTS5 一起替代**（大改动），1M 规模边际 |
| **Qdrant** | 远程/自托管 | ✅（HTTP，客户端零 CGO） | 首个远程 | **已实现** + `qdrantlive`/`aelive` 真服务冒烟 |
| **Weaviate** | 远程/自托管 | ✅（HTTP，客户端零 CGO） | 第二远程 | **已实现**（REST 建类+`note_id`属性/GraphQL nearVector/UUID 幂等 upsert）+ `weavialive` 真服务冒烟（容器跑通） |
| Pinecone / Milvus | 远程 | ✅（HTTP 客户端） | 大规模 | 各需一个 `Store` 适配器（按真实 API 核验后再写，勿凭记忆） |
| **LanceDB** | 本地/远程 | ❌ **需 CGO** | 高速 ANN | **与零 CGO 约束冲突**；要用须重开 ADR 讨论，默认不接 |
| go-libsql | 本地 | ❌ CGO | — | 无 Windows 支持，跨平台出局 |

**2026 复核要点**：sqlite-vec 仍是活跃、被推荐的默认，未过时；上面"CGO"一列是硬筛——LanceDB/go-libsql 破我们零 CGO 前提，故不列默认。

## 展望：对齐开放记忆标准（回应用户"OpenAI/各 IDE 都在公开存储格式"）

- **现状风险**：codex/opencode/openclaw/hermes importer 都是**对各家未公开、随版本漂移的落盘格式**做逆向（已用宽松解析 + golden-file + allowlist 兜底，但仍脆）。
- **可对齐标准 = OMP（Open Memory Protocol）**：厂商中立的开放 AI 记忆规范（Memory Object：id/content/type∈{episodic,semantic,procedural}/source/tags/created_at；`/v1/handoff`、`/v1/conversations`；可移植 JSON 导入导出；已有 Claude Code/Cursor/Copilot/Codex CLI 等适配宣称）。**若成熟，Repotory 可把它作为导入/导出契约，取代逐工具逆向**。
- **但**：OMP 现 **v0.4、pre-1.0、社区早期（~85 star）**，兼容多靠各家适配器。**不现在硬依赖**。
- **导出接缝已实现（provisional）**：`service.ExportMemoryJSON` 把知识库导出为 OMP 风格 Memory Object 数组（id=`urn:repotory:note:<id>`、content=标题+正文、type 由 kind/handoff 投影为 episodic/procedural/semantic、source.tool=repotory、tags、created_at/updated_at），desktop binding `App.ExportMemoryJSON` + 前端 `exportMemoryJSON`。**仅导出向**，OMP 导入向 + 字段映射待其 v1 稳定再对齐（type 映射是尽力投影、非规范）。

## 后端接入现状（截至本次）

- 已实现并验证：`local`(sqlite-vec) · `qdrant` · `weaviate`（后两者对真容器 `qdrantlive`/`weavialive` 冒烟通过）。
- 接缝就绪、**待依赖可拉取**：`chromem-go`、`bleve` 均需 `go get` 新模块——本会话环境**离线**（proxy 不可达），故未加；联网后 `vectordb.Register` 一处即接入（Bleve 另需评估"是否替代 FTS5"的架构决策）。
- 接缝就绪、**待凭据/集群**：`pinecone`（需云账号 key）、`milvus`（需起集群）——按"先核真 API 再写、不凭记忆"原则未盲写客户端；有可用环境时同 `weaviate` 流程补。
