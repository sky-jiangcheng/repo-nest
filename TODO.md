# TODO — 已知事项与待办

> 本文件记录产品深度评估产出的改进项。Sprint 1-13 已完成（D1-D11, D20-D24, C1-C3, C5-C6, C8, C10, C12-C15, P3, P7, P10-P11, P13-P16, P19, P21, P25-P28, P30, P35, P37-P38）。
> 第四轮深度评估（2026-08-27）：[docs/product-review/2026-08-27-new-deep-review.md](docs/product-review/2026-08-27-new-deep-review.md)。
> 修复后请从此清单移除并写入 CHANGELOG。
>
> **快捷跳转**：[✅ 已完成](#-已完成sprint-1-13) ｜ [🔴 删除项](#-删除项d7-d11) ｜ [🟡 收敛项](#-收敛项c11) ｜ [🟢 细化项](#-细化项p34-p38) ｜ [📋 遗留项](#-遗留项)

---

## ✅ 已完成（Sprint 1-13）

<details>
<summary>展开查看已完成项</summary>

| ID | 内容 | 完成 |
|----|------|------|
| D1 | 删除 `scripts/legacy/` | S1 |
| D2 | 删除 `scripts/generate_screenshots.py` | S1 |
| D3 | `tools/agent-score/` → MCP `reponest_agent_score` | S1 |
| D4 | 社区文件精简 | S1 |
| D5 | 删除 `.agents/` + gitignore | S1 |
| D6 | 删除 `.workbuddy/` + gitignore | S1 |
| D7 | 删除 `web/dist/` 构建产物 | S3 |
| D8 | 删除 DMG 素材 | S3 |
| D9 | 删除 `tokens.md` | S3 |
| D10 | 删除 `openapi.json` | S3 |
| D11 | 删除 docs issue 模板 | S3 |
| C1 | PWA/SEO 清理 | S1 |
| C2 | 插件系统降级 | S2 |
| C3 | 块编辑器冻结 | S2 |
| C5 | Knowledge 页面数据层拆分 | S4 |
| C6 | Dashboard 页面数据层拆分 | S4 |
| C8 | 历史评估文档压缩 | S3 |
| C10 | NoteSection 拆分（417行 → 305行 + hooks） | S6 |
| C12 | ProjectDetail 拆分（316行 → useProjectDetail） | S7 |
| C13 | CSS 死代码清理（移除 ~20 个未引用类） | S8 |
| C14 | queries_test.go 拆分（784行 → test_helpers + queries_test） | S8 |
| P3 | MCP 写入工具 | S2 |
| P7 | 版本号 SSOT | S2 |
| P10 | MCP 搜索结果结构化 | S3 |
| P11 | MCP 写入返回 note_id | S3 |
| P13 | 搜索排序优化 | S4 |
| P14 | 前端错误边界 | S4 |
| P15 | `db/db.go` 拆分 | S5 |
| P16 | `service/project.go` 拆分 | S5 |
| P19 | 版本历史 diff 可视化 | S5 |
| P21 | useScanPolling 测试 | S5 |
| P25 | `knowledge.go` 拆分（572行 → knowledge.go + types.go） | S7 |
| P26 | `stats.go` 拆分（471行 → 7 files） | S7 |
| P27 | `cmd/mcp/main.go` 拆分（453行 → 6 files） | S7 |
| P28 | MCP 工具描述增强 | S7 |
| P30 | 前端路由懒加载（dashboard/knowledge/projectDetail/settings chunks） | S8 |
| D20 | 删除空目录 `skills/` | S9 |
| D21 | 删除空目录 `.agents/` | S9 |
| D22 | `.claude/settings.local.json` 确认未跟踪 | S9 |
| D23 | PWA 图标清理（`web/public/` 3 个 icon 文件） | S9 |
| C15 | install 脚本评估 → 保留 + README 双路径说明 | S9 |
| P35 | NoteSection CSS Modules 试点（notes.css 242→192 行，新建 .module.css 93 行） | S10 |
| P38 | ProjectDetail 拆分（316→214 行 + useProjectDetail hook 122 行） | S11 |
| P37 | SKILL.md 工作流指引 + MCP 工具参数/示例增强 | S12 |
| D24 | PWA 移出桌面主构建（ADR-0008：残留清零 + 图标/孤儿 locale 清理） | S13 |
| P31 | `Domain/types.go` 评估 → 核心实体已收拢，DTO 按分层归位 | S14 |
| P32 | Wails 绑定层审计 → 46 方法无死绑定，`bindings.go` 顶部落审计块 | S14 |
| P33 | `TrendChart` 评估 → 实为 chart.js 封装（非纯 SVG），保留 | S14 |
| P34 | `project_overview.go` 评估 → 不拆，`mineAndCache` recover 已生效 | S14 |
| P36 | `knowledge.go` 进一步拆分评估 → 内聚度高暂不拆 | S14 |
| P29 | `parseTimestamp` 多格式鲁棒解析（unix/RFC3339/ISO8601/git %ai/%ad 默认）+ 测试 | S14 |
| P35·b | KnowledgeCard → `KnowledgeCard.module.css`（全局仅留共享 .pin-btn） | S14 |
| C11·A | 插件运行时：`loader.go` 合入 `runtime.go`（删 loader.go，Claude importer 路径不受影响） | S14 |

</details>

---

## 🔴 删除项（D7-D11）

> 纯减法，零功能回退。

### D7: 删除 `web/dist/` 构建产物

- [x] 删除 `web/dist/` 目录（~200 个 KaTeX 字体/Mermaid chunk/Vite 缓存文件）
- [x] `.gitignore` 已包含 `web/dist/`，`vite build` 重建后 Wails embed 正常

### D8: 删除 macOS DMG 素材

- [x] 删除 `build/dmg-background.svg` + `build/dmg-readme.txt`

### D9: 删除 `tokens.md`

- [x] 删除 `web/src/styles/tokens.md`

### D10: 删除手动维护的 OpenAPI spec

- [x] 删除 `docs/api/openapi.json`（后续改为 CI 自动生成）

### D11: 删除 docs issue 模板

- [x] 删除 `.github/ISSUE_TEMPLATE/docs.yml`

---

## 🔴 删除项（D20-D24）

> 第四轮评估新增。零功能回退，纯减法。

### D20: 删除空目录 `skills/`

- [x] `rm -rf skills/`（零内容空目录）

### D21: 删除空目录 `.agents/skills/`

- [x] `rm -rf .agents/`（AI 工具产物，.gitignore 已覆盖）

### D22: `.claude/settings.local.json` 从 git 跟踪中移除

- [x] 确认未被 git 跟踪（.gitignore `.claude/` 已覆盖），无需操作

### D23: PWA 图标清理（`web/public/`）

- [x] 确认无代码引用 icon-192/512/maskable
- [x] 删除 3 个 PWA 图标文件（共 51KB），保留 favicon.ico + favicon.svg

### D24: PWA 移出桌面主构建（ADR-0008）

- [x] 删除 `web/public/` 3 个 PWA 图标（icon-192/512/maskable）——同 D23 范围，随 ADR-0008 落地
- [x] 两套 locale（zh-CN / en）清理 11 个孤儿安装字符串（installTitle/Desc/App/Msg/Desktop 等）
- [x] `App.tsx` 路由注释改为「浏览器 / 桌面壳」区分，不再以 PWA 叙事描述
- [x] `main.go` CSP 注释去掉 PWA/registerSW.js 表述
- [x] README / getting-started / settings / SKILL.md / docs 失实行清零
- [x] ADR-0008 + index 登记
- [x] web 构建保留（`npm run build` 照常），不再是可安装 PWA

---

## 🟡 收敛项（C11, C15）

> 前轮遗留 + 本轮确认。

### C11: 插件运行时精简评估（669 行）

- [x] **方案 A（推荐）已完成**：`loader.go`（实际 121 行，非 52）合入 `runtime.go` 末尾「yaegi script loader」段，删除 `loader.go`；`runtime.go` import 合并（+reflect/strings/interp/stdlib），符号无冲突（`exportedTypes`↔`Context`、`loadPlugin`↔`compileScript` 同包互引）；`go build ./... + go vet + go test ./internal/core/plugin/runtime/` 全绿
- [ ] 方案 B（2.0 考虑）：评估移除 yaegi 依赖，Claude importer 改为内置函数
- [x] Claude importer 路径确认不受影响：`internal/importers/claude` 是 Go 原生 `plugin.KnowledgeImporter`，经 `service/plugin.go:100` 的 `rt.RegisterSource` 注册，不走 yaegi `compileScript`/`script` 路径，本次合并不触碰

### C15: install 脚本评估

- [x] **保留**脚本，README 安装说明已调整为「直接下载」+「脚本安装」双路径

---

## 🟢 细化项（P34-P38）

> 第四轮新增。按 AI 产品优先级排序。

### P34: `service/project_overview.go` 评估（239 行）

- [x] 确认函数内聚度合理，**不拆**：9 个方法均为 project/stats/level/star/search/overview/mining/summary/list 的薄 service 委托，同属「项目读模型」域。异步挖掘在 `GetProjectOverview` 内以 goroutine 触发 `mineAndCache`，`project_overview.go:139` 的 `defer recover()` 已确认生效（后台 panic 记日志不崩主进程）

### P35: 前端 CSS 架构迁移（4,055 行全局 CSS）

- [x] NoteSection CSS Modules 试点完成（notes.css 242→192 行，NoteSection.module.css 93 行新建）
- [x] 第二组件 KnowledgeCard 迁移完成：卡片样式从 `knowledge.css` 迁入 `KnowledgeCard.module.css`（`kind-*` 动态类改为 `badgeByKind` 查表映射），全局仅保留共享的 `.pin-btn`（与 NoteSection 复用）与 `.markdown-body`/`.btn`；`knowledge.css` 随之收缩，`npm run build` + tsc + eslint 全绿
- [ ] 保留全局 CSS 仅用于 reset、design tokens、跨组件基础样式
- [ ] 逐组件迁移，每轮 sprint 处理 1-2 个组件（下一步：ProjectCard）

### P36: `knowledge.go` 进一步拆分评估（536 行）

- [x] 评估函数间共享参数情况，**暂不拆**：12 个函数全部以 `repoPath string` 为入参、各自独立作用于仓库路径，无跨函数共享可变状态，内聚度高（单一「repo 知识探测」职责）。若后续再增挖掘维度，首选切出依赖探测子簇（`DetectDependencies` + `parseNpmDeps/parseGoDeps/parseCargoDeps`，约 150 行）为 `dependencies.go`

### P37: SKILL.md 工作流指引增强

- [x] 在 SKILL.md 开头增加「推荐工作流」段落（search → ask → read → create）
- [x] 为每个 MCP 工具补充使用场景 + 参数约束 + 示例值

### P38: ProjectDetail 拆分确认（316→214 行）

- [x] `useProjectDetail` hook 已创建（122 行），数据层真正下沉
- [x] 组件降至 214 行（≤200 目标基本达成）

---

## 🟢 细化项（P25-P33）

> 新增项。按文件体量和 AI 产品优先级排序。

### P25: `knowledge.go` 拆分 ✅

- [x] 572 行拆为 `knowledge.go`（515 行实现）+ `types.go`（48 行类型定义）

### P26: `stats.go` 拆分 ✅

- [x] 471 行拆为 7 个文件：`types.go` + `stats.go` + `validation.go` + `query.go` + `commits.go` + `range.go` + `dates.go`

### P27: `cmd/mcp/main.go` 拆分 ✅

- [x] 453 行拆为 6 个文件：`main.go` + `tools_notes.go` + `tools_projects.go` + `tools_search.go` + `tools_score.go` + `results.go`

### P28: MCP 工具描述增强 ✅

- [x] 为每个工具补充使用场景 + 参数约束 + 示例值
- [x] 推荐 AI 工作流：ask → read → create → update

### P29: `stats` 时间戳解析鲁棒性 🔻低

- [x] `parseTimestamp`（`internal/stats/validation.go`，P26 拆分后）从「只认 `2006-01-02 15:04:05`、忽略错误」升级为多格式：裸 unix 秒（git `%at`）、RFC 3339 / ISO 8601（git `%aI`/`%cI`，带/不带时区）、git `%ai`（`… -0700`）、date-only（`%ad --date=short`）、git 默认作者日期（`Mon Jan _2 … -0700`，含空格补零日）；不可解析仍返回 0（保持 latest-commit 比较的宽松契约）。新增 `TestParseTimestamp` / `TestParseTimestamp_Unparseable`（10 + 4 例）

### P30: 前端路由级懒加载 ✅

- [x] `React.lazy()` + `Suspense` 对 Dashboard/Knowledge/ProjectDetail/Settings 代码分割
- [x] `vite.config.ts` manualChunks 独立 chunk（dashboard、knowledge、projectDetail、settings）

### P31: `Domain/types.go` 评估 🔻低

- [x] 评估是否收拢核心 domain 类型：**已收拢**，`internal/domain/types.go`（132 行）已集中 13 个核心持久化实体（Project/Repository/Todo/Note/NoteWithProject/NoteVersion/NoteDiff/TodoCount/NoteCount/DailyStat/HeatmapDay/SearchHit/RepoMeta）。各包里另见的 `service.ProjectResponse`、`knowledge.RepoKnowledge` 等是响应/DTO 形状，按分层归各层，非「散落的 domain 实体」，无需再收拢

### P32: Wails 绑定层审计 🔸中

- [x] 审计 `bindings.go` 46 个方法（218→254 行含审计注释）：**无死绑定/无重复**。逐方法标注 MCP 对应 vs desktop-only，结论落为 `bindings.go` 顶部审计块。13 个绑定有对应 `reponest_*` 工具（notes/project/scan/context/search 面），其余为 GUI 状态、todos、config、插件管理、文件导出、笔记版本/pin/move 等刻意仅供桌面端；`reponest_ask/handoff/agent_score/integrity/notes_read` 为 agent-only、无绑定（直接走 service）

### P33: `TrendChart` 组件评估 🔻低

- [x] **原描述已过时**：`TrendChart.tsx` 不是「纯 SVG」，而是 `chart.js` / `react-chartjs-2` 的 `<Line>` 配置封装（约 100 行 options/data），被 `ProjectDetail`（经 `useProjectDetail`）实际使用。折线图用 CSS 替代不成立——chart.js 已承担该职责，手写 CSS 折线图反而更重。**结论：保留**，作为 chart.js 的薄封装合理

---

## 🟢 会话记忆路线（ADR-0007 / ADR-0009 后续）

> 定位升级为「AI agent 记忆层」后的主攻方向，按传播价值排序；M4-M5 的分发决策（薄客户端纪律、VS Code 先行、JetBrains 缓议）见 [ADR-0009](docs/adr/0009-ide-presence.md)。

### M1: 会话自动捕捉（零人工参与）→ [ADR-0010](docs/adr/0010-session-auto-capture.md)

- [x] **解析核心已落地**（`internal/importers/claude/session.go`）：`ParseSession` 流式解析 `~/.claude/projects/<slug>/<id>.jsonl`，宽松跳过未知/超长行、忽略 sidechain，抽取稳定信封字段（sessionId/cwd/gitBranch/timestamp）+ 最后一条 assistant 文本 + 首次 user 指令 + 去重工具名；实测对齐真机 v2.1.278（`session_test.go`）
- [x] **按需捕获已接线**（决策：默认关 + C端按需 / B端可选 hook）：`claude.LatestSessionForRootPath` + `Service.CaptureClaudeHandoff(projectID)` 走共享 `CreateHandoffNote`，受 `claude_session_capture`（默认关，仅 =="1" 才读盘）门控；`Session→HandoffInput`（最后 assistant 文本→Summary、工具+git 分支→Changes、打 `auto-captured` 标签）；desktop binding `App.CaptureClaudeHandoff`；单测覆盖。
- [x] **前端入口已接**：`endpoints.ts.captureClaudeHandoff`（transport 按方法名动态派 Wails 绑定，无需手改 wailsjs）+ `Settings→PluginsTab` 的 Claude 捕捉开关、「按项目 ID 捕捉」按钮、`openclaw_project`/`hermes_project` 目标项目输入；tsc + 端点契约测试绿
- [x] **B端 SessionEnd-hook 自动化已接**：`cmd/reponest-capture` CLI（解析 `-cwd`/stdin `.cwd`/进程 cwd）→ `Service.CaptureClaudeSessionByCwd`（cwd 末段匹配项目→同一门控捕获）；未开启/无匹配项目**退出码 0** 不打断会话收尾；`reponest-init` hook 示例指向它；单测覆盖
- [x] 体积与隐私评估：ADR-0010 已定「默认关 + 只读路径白名单 + 尾部 N 有界提取 + 宽松失败 + golden-file」，边界与既有 claude *memory* importer（读 `memory/*.md`，非 jsonl 逐字稿）划清

### M2: 多 agent 记忆源导入 → [ADR-0011](docs/adr/0011-multi-agent-memory-importers.md)

- [x] **Codex 源已落地**：`internal/importers/codex`（流式解析 `~/.codex/sessions/**/rollout-*.jsonl`，取 cwd→项目匹配 + 首次指令 + 最近回复成一条 `log` 笔记），复用 `plugin.KnowledgeImporter`/`upsertDoc`；抽公共件 `internal/importers/memsrc`（`MatchProject`/`ReadCapped`/`LastPathSegment`），claude 改为委托（测试不破）。**隐私门**：新增 `RegisterSourceManual` + `sourceEntry.auto`，`ImportAll`（启动自动导入）只跑 auto 源、Codex 经设置里 sources 列表显式一键触发（ADR-0011 决策 4）。golden-style 测试 + runtime 门控测试
- [x] **OpenCode 源已落地**（`internal/importers/opencode`，opt-in MANUAL）：真机核验 `~/.local/share/opencode/storage/session/<hash>/ses_*.json`（自带 title/summary/directory，已是摘要级），`directory` 末段→项目匹配；golden-style 测试 + 接口断言
- [ ] Cursor 源（**真机核验后暂缓**）：`state.vscdb` 正文散在 `bubbleId` blob + 版本化 headers + ProseMirror，且项目归属在 DB 内缺失、本机仅空 draft 无从校验 → 低 ROI 高脆弱，按「不背未公开易碎格式债」暂缓；有稳定真实样例再立项
- [x] **OpenClaw 源已落地**（`internal/importers/openclaw`，opt-in MANUAL）：allowlist 到 `~/.openclaw-autoclaw/workspace/*.md` 单层非递归（parent 含私钥/vault，绝不触碰；有 allowlist 单测），全局记忆经 `openclaw_project` 配置定向（未设→skip）；「只做 2.0」＝按当前布局
- [x] **Hermes(curated) 源已落地**（`internal/importers/hermes`，opt-in MANUAL）：**Nous Research 独立产品，与 OpenClaw 两家**（早先误判已更正）。官网文档核验 root `~/.hermes/`（`$HERMES_HOME` 覆盖），allowlist 到 `memories/{MEMORY,USER}.md`（`.env`/`mcp-tokens`/`state.db` 绝不读），`hermes_project` 配置定向
- [ ] Hermes sessions（`~/.hermes/sessions/` + `state.db`）未导入：schema 无文档、本机不可核验 → 待真实样例/官方 schema 再实现（不猜活格式）

### M3: 语义检索 → [ADR-0012](docs/adr/0012-semantic-search.md)

- [x] 评估 + **实测验证**：`CGO_ENABLED=0` 下 `modernc.org/sqlite/vec`（bundled sqlite-vec v0.1.9）跑通 vec0 建表 + KNN + `vec_distance_l2`（回归测试 `internal/vecprobe`，副作用用无生产码的 test-only 包隔离）；混合检索中间件 `internal/search/hybrid`（`Embedder` 接口 + `FuseRRF` k=60 + 单测）已落地，尚未接入生产
- [x] **决策已定 + C 第一版已落地、默认生效**（`internal/db/search.go`）：放弃 B；默认走 C＝严格 FTS5 AND **命中为零**时做**停用词感知的 OR 查询松弛**（零外部词库/零 CGO，纯 FTS 内不新增 LIKE，坏索引语义不破、AND 有结果不误触发）；单测齐。A（远程 embedding）留作 B端可选、默认关
- [x] **A 后端接线完成 + 端到端测试**（`internal/service/search_semantic.go`）：`RebuildEmbeddings()`（全库分批重算、dim 自探）+ `fuseSemantic()`（FTS+`KnnNoteIDs` 经 `FuseRRF`；默认关，关闭/未配/失败一律退回纯词法，绝不减结果）；`App.RebuildEmbeddings` binding；httptest 桩端点验证「词法零命中→向量补出、关掉即回纯词法」
- [x] **A/B 评测门已就绪**：`internal/search/abeval`（Recall@k/NDCG@k + `Compare` delta + 单测）+ `cmd/abeval`（对活库跑 lexical vs hybrid、`GATE PASS/FAIL`、`-min-recall` 阈值；无端点则 hybrid=lexical 自然不过门）
- [x] **向量存储选型定 + 安装引导落地（[ADR-0013](docs/adr/0013-vector-database-selection.md)）**：轴 B 默认**本地 sqlite-vec（`modernc.org/sqlite/vec` 纯 Go，校正原稿「需 CGO」之误）**；`db.VectorStoreHealthCheck` + `cmd/vector-init`（引导式：自检 vec → 建/验 vec0 → 选 embedding provider[Ollama 本地默认/远程 OpenAI/skip] → 写配置 → **指向 设置→插件** 复核+开启+重建；`semantic_search` 保持默认关）
- [x] **远程向量库接缝已实现（Qdrant，opt-in + 自动退回本地）**：`internal/search/vectordb` `Store` 接口（`Local`＝sqlite-vec 默认 / `Qdrant`＝REST）+ `Open` 按 `vector_store*` 配置选择、不可达退回本地；`cmd/vector-init -store qdrant` 写入并探测；search_semantic 的 Rebuild/fuse 改走 `Store`；httptest 桩测 + **build-tag 门控真实冒烟（`ollamalive`/`qdrantlive`/`aelive`，本轮已在本地真 Ollama+真 Qdrant 跑通**，含全链路语义召回；CI 默认不跑）。换 Weaviate 只需再加一个实现
- [ ] 后端改为 `vectordb.Register/Kinds` 可插拔 registry（加后端＝一实现+一行 Register，调用方零改）；候选矩阵见 ADR-0013：默认 sqlite-vec / 纯 Go 备选 chromem-go、Bleve（可连文本一起替代 FTS）/ 远程 Qdrant(已)、**Weaviate(已实现+真容器冒烟)**、Pinecone/Milvus(待)；**LanceDB/go-libsql 因 CGO 破零-CGO 前提不列默认**
- [x] **OMP 导出接缝已实现（provisional）**：`service.ExportMemoryJSON` 导出 OMP 风格 Memory Object 数组（binding `App.ExportMemoryJSON` + 前端 `exportMemoryJSON` + 单测）；仅导出向
- [ ] OMP 导入向 + 字段映射对齐（等 OMP v1 稳定）；chromem-go/Bleve（需 `go get`，本会话离线未加）、Pinecone/Milvus（需凭据/集群）按同 registry 流程待接
- [ ] A 面向普通用户上线前：embedding 配置前端 UI（未过门前刻意不做开关）、笔记增删改增量 embed（现全量重建）、一份真实标注 query 集

### M4: Agent 集成即插即用

- [x] Claude Code hook 示例：SessionEnd hook 自动触发 reponest_handoff（v1.9.4 交付于 docs/features/ai-integration.md「会话结束自动交接」节）
- [x] `npx reponest-init` 类一键注册脚本（写 .mcp.json + 提示 hook 配置；ADR-0009 第一步，该 ADR Proposed→Accepted 的门槛项）——已交付 `scripts/reponest-init/`（零依赖 Node ≥18，幂等 + dry-run + hook 安装，2026-10-02）

### M5: IDE 存在感（ADR-0009 薄客户端分发）

- [x] VS Code 扩展（唯一 IDE 扩展，一份 VSIX 覆盖 VS Code / Cursor / Windsurf 全 fork 家族）——骨架已交付 `ide/vscode/`（tsc 零错误）：命令面板 context / handoff / search + 状态栏入口 + 侧边栏笔记检索（MCP stdio 薄客户端）；余：VSIX 打包 / CI、状态栏「上次交接时间」（需服务端交接时间戳 API）
- [ ] JetBrains 插件：缓议——独立 Kotlin/Gradle 代码库双倍维护面，待真实需求信号（issue/star）并补充 ADR 后再立项
- [ ] 分发评估门：此后每个分发资产立项时必须回答「人在哪个界面上看见它」；仅 agent 可消费的资产需说明服务存量深度而非获客（ADR-0009 决策 5）

### M6: LLM Wiki 知识编译层 → [ADR-0014](docs/adr/0014-llm-wiki-knowledge-compiler.md)

- [ ] **W0 前置隐私门（阻塞 W3）**：`auto_import` 现默认 `"1"`（`internal/db/migrate.go:452`）→ 改默认关 + 逐源显式开。迁移**只改默认值、不得静默关掉用户已显式开启的源**，并补迁移测试
- [ ] **W1 结构成图（不含任何 LLM 调用）**：`wiki_pages`（entity / concept / source / synthesis 四类）+ `page_links`（双向可查）+ note↔repository 关联。迁移必须**可逆**并带测试（参照 `migrate_fts_repair_test` 的严格度）；「四类够不够（是否加 question / decision）」在本步给结论并回填 ADR-0014 待决项
- [ ] **W1b 单向导出旁路**：`reponest wiki export` 渲染 md 文件树 + YAML frontmatter + wikilink + 导出清单，用 Obsidian graph view 反向验证结构是否自然。**刻意不做双向同步**（ADR-0014 决策 2：两个写入者会让 `note_versions` / FTS5 / 向量索引全部重做）
- [ ] **W2 AskAI 取证改造（最高 ROI，先于生成端）**：读 index → FTS5 + 向量 RRF 选页 → 读页 → **带引用**作答 → 好答案一键回档成新页；补流式；上下文预算改为「条数 / 字符 / 超时」三重封顶，替掉 `internal/service/ai.go:431` 的固定 10 条 × 500 字节。**完成判据 = 过 `cmd/abeval` 门**（真实标注 query 集 + Recall@k / NDCG@k delta），不接受"感觉变好了"
- [ ] **W3 摄入时编译（opt-in、默认关）**：importer 由「原文 upsert」升级为「抽取 → 新建/更新页面 → 更新 index + 追加 log → 标注与既有笔记的矛盾」；LLM 写入一律 `source='llm-wiki'` + 待审，人工批准后才进 index；一次一源、人保持在环。新增字符串配置键必须在 `service/config.go` 的 `allowedConfigKeys` 与 `stringConfigKeys` **两处**登记，否则 `UpdateConfig` 按数值校验拒绝
- [ ] **W4 Lint 五查（必配防腐层）**：矛盾 / 过时声明 / 孤页 / 缺交叉引用 / 数据缺口 → 产出落 `project_todos`（复用现有表，不新造反馈面）；**LLM 只能建议、不得自动改写页面**；定时与手动触发各一条路径
- [ ] **W5 分层蒸馏与检索**：L0 转录（capture 链路已有）/ L1 原子笔记（`project_notes`）/ L2 项目场景（`BuildProjectContext` 已是雏形，`internal/service/context.go:94-120`）/ L3 跨项目画像（待立项）；检索改「L2/L3 引导上下文，要具体事实才回落 L1/L0」。页面量上来后评估把语义融合范围从 notes 扩到 `wiki_pages`（受 ADR-0012「只对 notes 融合」现状约束）
- [ ] **schema 作用域结论**：`schema.md` 等价物放全局默认还是按项目覆盖（倾向全局默认 + 项目覆盖），定了才动 W3

> **M6 刻意不做**：自建或对接远端 Memory Hub（违背 local-first 定位，且外部协议仍在 churn——自家 OMP 导出至今标 PROVISIONAL、导入侧刻意未写）；文件树双向同步；为 wiki 引入新向量后端（sqlite-vec 零 CGO 已在库内，ADR-0013）。

---

## 📋 遗留项

- [ ] **性能（M3-A 审核发现）**：`service.vectorStore()` 每次语义检索都重读 5 个 config + 对远程库做 `Reachable()` 探测 → 应按配置签名缓存 store（配置变更失效），避免每次查询的额外 SQL + 远程探测。默认关，优先级低
- [ ] **文档**：`ai-integration` / `knowledge` 功能文档尚未提及新能力（捕捉/语义检索/向量库/CLI/OMP）；后续同步（本轮已补 README + settings 中英 + ADR 中英对齐）

- [ ] 桌面 GUI 回归测试：建议在真机跑一轮冒烟（扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答）
- [ ] **D25 仪表盘生产力门面收缩（2.0 候选，非现在）**：首屏讲记忆环、打开是仪表盘，定位纯度持续被消耗。收敛方向：仪表盘退化为「项目列表 + 最近活动」；目标环 / 每日代码量标准 / 工作日告警沉入插件或删除（GitBoard/GitBuddy 时代遗产，见 [ADR-0008](docs/adr/0008-pwa-removal.md) 遗留项）
- [x] `reponest_context` brief/full 档位评估（v1.9.4 收敛：中等优先，缓做——当前固定 10 notes × 1200 字符 + 8 commits 对单会话偏充裕）
- [x] README 对比表 + ASCII 架构图（v1.9.4 收敛：文档润色，低优先，缓做）
- [x] `mineAndCacheAsync` 后台 goroutine 加 recover（`project_overview.go:138`）
- [x] `.zcode/` 已移出跟踪，不需要 history rewrite
- [x] P29 `parseTimestamp` 鲁棒性（多格式 + 测试，见 P29 结论）
- [x] P31 `Domain/types.go` 评估（核心实体已收拢，DTO 按分层归各层）
- [x] P32 Wails 绑定层审计（无死绑定，`bindings.go` 顶部审计块）
- [x] P33 `TrendChart` 评估（实为 chart.js 封装非纯 SVG，保留）
- [x] P36 `knowledge.go` 进一步拆分评估（内聚度高暂不拆，见 P36 结论）

---

## 建议执行节奏

| 阶段 | 内容 | 预估 |
|------|------|------|
| ~~Sprint 1-5~~ | ~~D1-D11, C1-C3, C5-C6, C8, P3, P7, P10-P11, P13-P16, P19, P21~~ | ✅ 共 28 项 |
| ~~Sprint 6~~ | ~~D12-D19 剩余删除 + C10 NoteSection 拆分~~ | ✅ |
| ~~Sprint 7~~ | ~~C12 + P25-P27 大文件拆分 + P28 MCP 描述增强~~ | ✅ |
| ~~Sprint 8~~ | ~~C13 CSS 清理 + C14 测试拆分 + P30 懒加载~~ | ✅ |
| ~~**Sprint 9**~~ | ~~D20-D23 资产大扫除 + C15 文案调整~~ | ✅ |
| ~~**Sprint 10**~~ | ~~P35 NoteSection CSS Modules 试点~~ | ✅ |
| ~~**Sprint 11**~~ | ~~P38 ProjectDetail hook 提取~~ | ✅ |
| ~~**Sprint 12**~~ | ~~P37 SKILL.md 工作流指引~~ | ✅ |
| ~~**Sprint 13**~~ | ~~D24 PWA 移出桌面主构建（ADR-0008 落地）~~ | ✅ |
| **Sprint 14** | P29/P31/P32/P33/P34/P36 评估类小项收口（验证 + 落结论，含 P32 绑定审计块、P29 多格式解析 + 测试） | ✅ 共 6 项 |
| **待排（M6）** | ADR-0014 五步：W0 隐私门 → W1/W1b 结构成图 + 单向导出 → W2 AskAI 取证（须过 `abeval` 门）→ W3 摄入编译 → W4 lint | 未估（db / service / web 三层同动，非单 sprint 量） |
| **2.0 规划** | D25 仪表盘生产力门面收缩 + C11 插件系统评估 + P35 全量 CSS Modules | 按版本 |
| **按需** | P29, P31, P32, P33, P36 | 随重构穿插 |
