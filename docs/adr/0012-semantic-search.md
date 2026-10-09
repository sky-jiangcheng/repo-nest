# ADR-0012: 语义检索评估——向量召回补 FTS5 字面盲区，守住零 CGO（M3）

- 状态：Accepted-in-principle（向量侧零 CGO 已实测、hybrid RRF 核心已落地；**C 已默认落地**；A 定为 OpenAI 兼容协议，嵌入器 + 向量存储 + 配置/密钥脱敏 + **后端全库重算 & FTS×vec RRF 融合均已落地、端到端测试通过、仍默认关**；设置 UI 已提供开关与重建，真实标注 A/B 门仍未过；放弃 B）
- 日期：2026-10-02
- 关联：[ADR-0003](0003-fts5-search.md)（FTS5 trigram 全文检索）、[ADR-0006](0006-scope-freeze.md)、[ADR-0007](0007-session-memory-protocol.md)、TODO 会话记忆路线 M3

## 背景

现有检索是 **FTS5 trigram + bm25**（`internal/db/search.go`、`internal/db/migrate.go` 建 `project_notes_fts`/`project_todos_fts`），补 LIKE 兜底。trigram 对**子串/拼写/部分词**很稳，但结构性地缺**语义**：搜「内存泄漏」召回不到只写了「heap 持续增长、GC 压力大」的笔记——词面不重叠。M3 想补这层召回。

关键约束来自产品形态：RepoNest 是本地优先的 Wails 桌面 App，跨 mac/win 分发，**数据库驱动是 `modernc.org/sqlite`（纯 Go、零 CGO）**（`go.mod`、`internal/db/db.go`）。任何检索增强都不得把 CGO 重新引入构建（会直接炸掉跨平台打包与体积假设）。

## 决策 / 评估结论

1. **向量存储与检索：零 CGO 这条路现在已成立（2026-09 更新的事实，且本仓已实测验证）**。`modernc.org/sqlite` 现已提供 **`modernc.org/sqlite/vec`**——纯 Go 移植、sqlite-vec 兼容的向量虚拟表/索引（KNN、`vec_distance_*`）。即「ANN 索引 + 相似度查询」这一半**不需要**换驱动、不需要 CGO。结论：向量侧技术可行性从「存疑」上调为「可行」，且与现有 FTS5 同库共存（一张 `vec0` 虚拟表 + 外键回连 `project_notes.id`）。
   - **实测（`internal/vecprobe`，`CGO_ENABLED=0 go test`）**：bundled sqlite-vec = **v0.1.9**，`vec0` 建表 + `MATCH … AND k=?` KNN + `vec_distance_l2` 全部在纯 Go 驱动下跑通（3-4-5 距离 = 5.0，KNN 命中正确最近邻）。注意其副作用：导入该包会使 `sqlite3_auto_extension` 初始化 SQLite，从而**禁用 `RegisterPageCache`**（本仓未用页面缓存自定义，无冲突）；vecprobe 用「只有 _test.go、不被 app 引用」的包隔离这一副作用。
   - **已落地中间件（`internal/search/hybrid`）**：`Embedder` 接口（把「向量怎么来」与「怎么融合」解耦，embedding 路线未定不影响此层）+ `FuseRRF`（Reciprocal Rank Fusion，k=60，确定性 tie-break，无外部依赖）+ 单测。**尚未**接进 `db/search.go` / 建 `vec0` 生产表 / 加配置开关——那些要等下面决策 2 定了 embedding 路线、且过 A/B 门。
2. **真正的硬门是本地 embedding 生成，且它才是默认关的理由**。要把笔记/query 变成向量，需要一个推理运行时；而主流本地方案（ONNX Runtime、llama.cpp、sentence-transformers 后端）普遍要 CGO 或附带大体积模型——这与「零 CGO + 桌面轻分发」正面冲突。M3 是否值得做，**几乎完全取决于能否找到零 CGO 的 embedding 路径**，需在下列选项中拍板：
   - A：**可选远程 embedding API**（零 CGO、零模型捆绑，但违背「纯本地」叙事且引入网络依赖/隐私面）；
   - B：**纯 Go 小模型推理**（若有满足质量阈的小 embedding 模型可在纯 Go 下跑；需先做技术验证，未知数最大）；
   - C：**放弃向量、只增强 FTS**（如 synonym 词表 / 查询改写，零新依赖，但天花板低）。
3. **落地形态（若通过）：混合检索 + RRF 融合 + 默认关**。FTS5 与向量各出一路排序，用 Reciprocal Rank Fusion 合并，向量路默认不启用、由设置开关控制。理由：字面匹配（trigram）在多数日常查询里已够且更快，向量只在「换词搜」场景增益；混合保证不劣化基线。
4. **先评测后开关（A/B 门，实现前置）**：建一个小型标注评测集（真实笔记 + 人工写的「应命中」query 对），脚本化比较 FTS-only vs FTS+vec 的 recall@k / nDCG；**达不到明确质量增益阈值就不改默认**、不引入 embedding 依赖。评测产物与结论回写本 ADR。
5. **数据可重建、非真相源**：embedding 与向量索引视为派生缓存，SQLite 主库仍是唯一事实源；向量表可随时 drop + 依据笔记重算，允许换模型/升版本，不参与备份真相。

## 理由

- 向量侧的可行性问题已被 `modernc.org/sqlite/vec` 这一外部事实解决，值得如实记录，避免团队继续按「sqlite-vec 需要 CGO」的旧认知否决它。
- 但把「能不能存向量」和「能不能零 CGO 地**产出**向量」分开看，才是 M3 的真问题：后者才是与产品定位（本地、轻量、跨平台）冲突的点，因此默认关、先 A/B 是诚实且低风险的推进方式。
- 混合 + RRF + 派生缓存：对现有 FTS5 只增不改、可随时回退，符合 ADR-0006 的克制。

## 后果

- 正面：语义召回可补上 trigram 的换词盲区，且**不**以牺牲零 CGO 为代价换取向量存储。
- 负面/风险：本地 embedding 的模型体积/推理时延/质量三者权衡尚无定论；引入向量表带来重算与版本管理成本。
- **待决（实现前需回答）**：① embedding 生成走 A/B/C 中哪条路（决定 M3 成败，先做 B 的可行性 spike）；② A/B 评测的质量增益阈值与评测集来源；③ 默认关下的首个目标用户场景；④ 模型/维度升级时向量表的重算策略与迁移成本。

## 决策落地（2026-10-02 产品拍板）

1. **放弃 B（纯 Go 本地小模型）**：能在零 CGO 下跑出可用质量的 embedding 运行时基本不存在（GGUF/ONNX 普遍要 CGO），风险过高、可能死路，不投入。
2. **默认走 C（本地、零依赖、零外传）**：以 FTS5 增强（同义词表 / query 改写 / 词面近义扩展）为**默认语义补强手段**，与 M3 前立场一致，纯本地、即时可交付、不破本地优先叙事。
3. **A（远程 embedding API）作面向 B端的可选进阶、默认关**：
   - 面向**B端**（自有风控、明确愿意使用远程 embedding API 的用户）；**C端默认完全不开**。与 ADR-0010 的 C/B 分层同源。
   - 开启即走「远程 embedding + `modernc.org/sqlite/vec` 向量存储 + `internal/search/hybrid` 的 RRF 与 FTS5 融合」——向量存储侧零 CGO 已实测可行，缺的就是 embedding 由远程产出。
   - **风险文档是硬性交付项**：设置页 + 文档必须显著写明「开启 A 会把笔记内容（query 与/或正文）发送到第三方 embedding API，数据离开本机、依赖网络与密钥、可能涉及合规」，并要求用户显式确认；Provider 具体选型待产品给定（`Embedder` 接口已就绪，实现一个远程 client 即可插）。
4. **A/B 评测仍保留为开关门**：C 先行上线；A 上线前用标注 query 集测 recall@k，确认语义确有增益再作为 B端默认推荐，否则维持只开 C。

## C 落地进度（2026-10-03）

**C 的第一版已实现、默认生效、零依赖**：`internal/db/search.go` 的笔记检索在严格 FTS5 AND **命中为零**时，做一次**停用词感知的 OR 查询松弛**（丢小写英文停用词、把剩余词 OR 合并）；仍**在 FTS 内**完成，不新增 LIKE 兜底，因此「索引坏/空→返回空」的既有语义与 `migrate_fts_repair_test` 不破。AND 有结果时**不触发松弛**（不会误加只含部分词的文档）——纯召回增量，不改变当前好结果集。单测 `TestEscapeFTSOR` / `TestSearchNotes_ORRelaxationWhenANDEmpty` / `TestSearchNotes_NoRelaxationWhenANDMatches`。这是「纯 query 改写、零外部词库」的最小可用形态；更进一步的同义/近义（需词库或 embedding）留待 A 路线过 A/B 门后再评估。

## A 落地进度（2026-10-03）—— provider 选型 + 嵌入器基座

**选型结论：不锁单一厂商，标准化到 OpenAI 兼容 `/v1/embeddings` 协议**（OpenAI / Voyage / Jina / 自托管 Ollama·llama.cpp·LM Studio 都实现它），配置化 `base_url + model + api_key + dim`：
- **质量默认推荐**：OpenAI `text-embedding-3-small`（便宜、多语、CJK 尚可）；更高档 `text-embedding-3-large`；代码/中英混排更强可看 Voyage-3 / Jina-embeddings-v3。
- **关键洞察（解掉「本地 + 零 CGO」两难）**：既然 A 本就是「B端自愿走网络端点」，把 `base_url` 指向**本机自托管的 Ollama**（`nomic-embed-text`/`bge-m3`）即可：模型在**独立进程**跑，RepoNest 二进制**仍是零 CGO**、且**数据不出机**——对隐私敏感的 B端比云 API 更契合本地优先。即：一个 `RemoteEmbedder`（OpenAI 兼容）同时覆盖云端与本地 Ollama 两条路。
- **风险 posture 不变**：A 默认关、显式开；开启即在文档/设置显著披露「笔记文本会送往所配置的端点（云→出机；本地 Ollama→不出机）」，由用户自选端点自负其责。

**已落地基座（未接搜索热路径，待门）**：`internal/search/hybrid.RemoteEmbedder` 实现 `Embedder`（OpenAI 兼容 POST、按 `index` 乱序回填、HTTP/维度错误宽松失败、bearer 可选），带 httptest 单测。**向量存储层已落地**：`internal/db/vecindex.go`——`EnsureVectorIndex(dim)`（建/按 dim 变更重建 `note_embeddings` vec0，dim 记在自管的 `note_embeddings_meta`，不依赖 sqlite-vec 内部 schema）、`PutNoteEmbedding/DeleteNoteEmbedding/KnnNoteIDs/ClearVectorIndex/DropVectorIndex`；vec 扩展经 `_ modernc.org/sqlite/vec` 链入 `db`（零 CGO、已全仓 `CGO_ENABLED=0` 构建验证；不用 RegisterPageCache 故无冲突）。**配置 + 密钥安全已落地**：新增 `semantic_search`/`embedding_base_url`/`embedding_model`/`embedding_dim`/`embedding_api_key` 配置键，**`embedding_api_key` 在 `GetConfig` 中掩码为 `********`**（不回传前端，后端经 `db.GetConfig` 读真值），有单测。`FuseRRF`/`vecprobe` 亦就绪。

**A 后端接线已完成并端到端验证（仍默认关）**：`internal/service/search_semantic.go`——`RebuildEmbeddings()`（全库分批重算、dim 可自探、读真值 api-key）+ `fuseSemantic()`（FTS 命中 + `KnnNoteIDs` 命中经 `FuseRRF` 融合；`semantic_search=1` 且 vec 索引就绪且端点配好才走，任一失败/关闭**优雅退回纯词法**，绝不减结果）；`App.RebuildEmbeddings` desktop binding。httptest 桩端点端到端测试证明：词法零命中的查询经向量召回补出笔记、关掉即回到纯词法。**A/B 评测门已就绪**：`internal/search/abeval`（Recall@k / NDCG@k，binary relevance，`Compare` 出 hybrid−lexical delta + 单测）+ `cmd/abeval`（对活库跑：每条标注 query 各跑 lexical(semantic off)/hybrid(semantic on)，打印指标与 `GATE PASS/FAIL`，`-min-recall` 定阈值；无 embedding 端点时 hybrid 退化为 lexical、门自然不过）。**仍缺（A 面向普通用户默认上线前）**：一份真实标注 query 集以及它证明的检索增益；增量 embed 已落地。设置页已提供开关与重建，用于知情进阶用户，但真实标注门未过前不改变默认关。
