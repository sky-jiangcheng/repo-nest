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
- [ ] 保留全局 CSS 仅用于 reset、design tokens、跨组件基础样式——**本轮实测：这条目前是愿望不是现状**。非 module 全局 CSS 共 5,101 行，其中真正符合该描述的只有 `reset.css` + `tokens.css` + `typography.css` + `index.css` = 355 行（约 7%）；其余 4,746 行按归属仍可继续下沉：`features/` 3,071（`project-detail.css` 761、`dashboard.css` 641、`settings.css` 305、`knowledge.css` 280、`heatmap.css` 271…）、`components/` 1,276、`layouts/` 399。判据待定义清楚再收口：`components/buttons|inputs|cards|tabs` 这一层算「跨组件基础样式」（留下）还是算「可被各组件覆写的壳」（迁走）
- [x] **P35 路线已定「集中收三大文件」并落地第一块（外观 tab）**：`theme-*` 六类迁进 `AppearanceTab.module.css`，settings.css 305→160 行。**过程中推翻了"按文件整收"的做法**——`settings.css` 的 `.settings-section`/`.section-desc`/`.form-hint` 被 7 个组件共享、`.empty` 全站 15 处在用，整文件搬会把共享规则私有化；真正的迁移单位是**组件独占簇**。另挖出 `layouts/main.css` 反向伸进组件类的冗余规则（同值 560px，已删）与 `plugin-err`/`plugin-error` 两条死规则（零引用，已删；TSX 在用的 `plugin-ok` 反而无定义，留待 PluginsTab 那轮定性）。**第二块也已落地**：`ScanRootsTab.module.css` + `PluginsTab.module.css`，settings.css 164→47 行，至此该文件只剩多子页共用基座。三个撞上的坑记在这：PluginsTab 里 `sources.map((s) => ...)` 会遮蔽本仓惯例导入名 `s`（该文件改用 `css`）；`.root-item.empty` 的 `empty` 只有本 tab 会加，收成本地 `rootItemEmpty` 以避开在 `:hover:not()`/`:has()` 里嵌 `:global()`；`plugin-ok` 定性完成——markup 在用而 CSS 从无定义，已随迁移去掉。剩余按簇清单：AiTab/AuthorsTab/StandardsTab/ActionsTab 的表单类（多半仍是共用基座，逐处核对），之后才是 `project-detail.css`(761) 与 `dashboard.css`(641) 两块大石。**每块都需要人工实机看一遍**，CI 对 CSS 改名无感
- [ ] 逐组件迁移，每轮 sprint 处理 1-2 个组件（下一步：ProjectCard）——**已改按簇推进，两块落地；`dashboard.css` / `project-detail.css` 经核查不可机械搬运，判据见下**：
  - 已落：`AppearanceTab.module.css`、`ScanRootsTab.module.css`、`PluginsTab.module.css`，`settings.css` 305 → **47 行且剩下的确认为真共享基座**（`settings-group*` 被 ActionsTab 与 PluginsTab 共用）。累计全局 CSS 5101 → 4847、module 214 → 464。规则体逐条对账丢失 0，`src/` 旧类残留 0，tsc/vite/93 用例全过。
  - **两块大文件的真实阻碍（下一轮别重新发现）**：① `dashboard.css` 里的 `.green`/`.red` 不是工具类而是**承重墙**——文件里就写着注释：裸 `.green`(0,1,0) 曾输给 `.card-stat .stat-value`(0,2,0) 导致项目卡增减数字全灰，靠抬平特异性才修好；紧随其后的 `.card-stat .stat-value.green` / `.progress-value.green` / `.card-hero-value.green` 一条规则**同时穿过三个组件所有者的类**（card-stat 属 Dashboard、stat-value 属 Heatmap 与 ProjectDetail、progress-value 属 SummaryBar）。② `stat-label`/`stat-value`/`tab-btn`/`detail-section`/`project-card`/`card-star`+`starred` 各自被 2 个文件共用，"整文件搬"与"整组件搬"都会把共享类私有化。
  - 因此正确顺序是先做一次**归属裁决**（`stat-value` 这类跨屏复用的到底是谁的：抽进 `components/` 基座还是保留全局），再按 SummaryBar → ProjectCard → Dashboard → ProjectDetail 逐屏迁，每屏需人工实机看一次——CSS 改名 CI 完全无感，这两屏是最高曝光面，不适合盲改。节奏前提（两轮迁出 214 行 vs 同期 +1,046）依然成立：不配「新组件一律 module.css」的冻结约定，这条路追不上新增
两轮试点共迁出 214 行，而同期全局 CSS 从 4,055 涨到 5,101（**+1,046**）。按 1-2 组件/轮的速率追不上新增，这条路不会自然收敛。可选：改为「按文件冻结 + 新组件一律 module.css」的增量策略，存量只在触碰时迁；或按最大三块（project-detail / dashboard / settings）集中收。定了再动，别照旧速率继续走

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
- [x] Cursor 源已落地（**原判据被本机实测推翻后，决策为"导正文 + 标 best-effort"，已实现**）：原记录称「正文散在 `bubbleId` blob、库内无项目路径、本机仅空 draft 无从校验」。2026-10-07 只读复核：真实库在 `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`（7.1 MB），**此前查的 `~/.cursor/state.vscdb` 不是 macOS 的落点**——"仅空 draft"是找错路径得出的结论。实测有 71 条 `bubbleId:<composerId>:<bubbleId>` 行共 380 KB，且 `json_valid=1` **明文可解析**，顶层含 `text` / `type` / `createdAt` / `workspaceUris`（即正文不必去逆 `composerData` 里那个 6/6 非空的 `blobEncryptionKey`）；项目归属另有 `composerHeaders(composerId, workspaceId, createdAt, lastUpdatedAt, isSubagent, isArchived)` 表可 join。所以「无正文 / 无项目路径 / 无样例」三条判据全部不成立。仍然成立的两条：① 格式未公开且带 `_v` 版本位、随 Cursor 版本漂移，属「不背未公开易碎格式债」的原政策范围；② 本机有效语料只有 1 个真会话（148 KB）+ 5 个短 composer，验证面极窄。→ 需要决策（做 / 不做 / 只导 headers 不碰正文），不再是技术阻碍。
  **落地结果**：`internal/importers/cursor/`（opt-in MANUAL 源，注册在 `service.registerBuiltinImporters`）。读路径全 `mode=ro`（先试 `mode=ro`，WAL sidecar 不可用时退回 `immutable=1`，永不写别人的库）；归属链 `composerHeaders.workspaceId → workspaceStorage/<id>/workspace.json 的 folder → memsrc.MatchProject`，查不到即跳过、绝不猜；每场会话只出「首条提问 + 末条回复」两个截断摘要（bubble 的 `createdAt` 是 ISO-8601 串，键内嵌随机 UUID 不可排序，故按时间戳排序且容忍不可解析值）。三条 best-effort 纪律：① 单行 JSON 坏掉只丢该行（真库里实测就有 1 条 `json_valid=0` 的 bubble）；② 表或列不认识 → **整源返回 error**，因为"导入 0 条"与"你没有会话"必须可区分；③ Cursor 未安装 → `(nil, nil)` 成功空操作，与漂移明确两种语义。
  **真机验证**：`TestImportAgainstRealInstall`（`REPONEST_REAL_CURSOR_DB=1` 门控，沿用本仓 `*live` 冒烟惯例，只打印计数与长度、绝不打印正文）跑你本机真库通过：1 个可归属会话、1 个解析出可用正文、0 条入库（内存库无项目，正是设计行为）。
  **一个用血换来的约束**：只读 handle 的连接池限成 1 条，所以 composerHeaders 必须先收完并 Close，才能逐会话查 bubble——嵌套查询会**死锁**（真实安装冒烟测试第一版就是这么挂 120s 的；`Import()` 本身顺序正确，已在两处写死注释防回归）。
- [x] **文档双语漂移已修**：`docs/en/plugins/overview.md` 整页重对齐中文同名页——它的源清单表里**只有 `claude` 一行**，且仍写着「built-in claude importer 经 yaegi 解释」这套已被 ADR-0011 之后的实现推翻的说法；中文页则已是 5→6 源全表 + 匹配规则 + 配置键。英文页实际错在三处（不止缺行）：开篇写「通过 yaegi 解释执行的 Go 脚本向知识库幂等导入」——而 6 个内置源都是 Go 原生 `plugin.KnowledgeImporter`、**完全不经过 yaegi**（yaegi 只承载用户脚本插件）；mermaid 把入口画成「5 个内置源 / yaegi 解释」；源表只有 `claude` 一行，等于 codex/opencode/openclaw/hermes/cursor 五个源在英文站不存在，且整页缺「项目归属三级规则」「openclaw/hermes 未配置即静默全 skip」「导入统计语义」「headless 模式」四节。现已按中文页逐节补齐并同步 cursor 的 best-effort 说明。**中文页自身两处也已修**：同一句 yaegi 开篇、mermaid 里的「5 个内置源」。教训：双语漂移不只是"少几行"，它会讲出一个不存在的产品架构——而英文站是对外门面。
  - [x] **OpenClaw 源已落地**（`internal/importers/openclaw`，opt-in MANUAL）：allowlist 到 `~/.openclaw-autoclaw/workspace/*.md` 单层非递归（parent 含私钥/vault，绝不触碰；有 allowlist 单测），全局记忆经 `openclaw_project` 配置定向（未设→skip）；「只做 2.0」＝按当前布局
- [x] **Hermes(curated) 源已落地**（`internal/importers/hermes`，opt-in MANUAL）：**Nous Research 独立产品，与 OpenClaw 两家**（早先误判已更正）。官网文档核验 root `~/.hermes/`（`$HERMES_HOME` 覆盖），allowlist 到 `memories/{MEMORY,USER}.md`（`.env`/`mcp-tokens`/`state.db` 绝不读），`hermes_project` 配置定向
- [ ] Hermes sessions（`~/.hermes/sessions/` + `state.db`）未导入：2026-10-07 复核，本机**完全没有** `~/.hermes`，无从校验；上游 `NousResearch/hermes-agent` 源码里有 `hermes_cli/foreign_sessions.py`（读 codex 等**别人**的会话），但 Hermes 自身会话落盘格式无公开文档。判据从「schema 无文档」更正为「无文档**且**无本机样例」——保持待真实样例，不猜活格式

### M3: 语义检索 → [ADR-0012](docs/adr/0012-semantic-search.md)

- [x] 评估 + **实测验证**：`CGO_ENABLED=0` 下 `modernc.org/sqlite/vec`（bundled sqlite-vec v0.1.9）跑通 vec0 建表 + KNN + `vec_distance_l2`（回归测试 `internal/vecprobe`，副作用用无生产码的 test-only 包隔离）；混合检索中间件 `internal/search/hybrid`（`Embedder` 接口 + `FuseRRF` k=60 + 单测）已落地，尚未接入生产
- [x] **决策已定 + C 第一版已落地、默认生效**（`internal/db/search.go`）：放弃 B；默认走 C＝严格 FTS5 AND **命中为零**时做**停用词感知的 OR 查询松弛**（零外部词库/零 CGO，纯 FTS 内不新增 LIKE，坏索引语义不破、AND 有结果不误触发）；单测齐。A（远程 embedding）留作 B端可选、默认关
- [x] **A 后端接线完成 + 端到端测试**（`internal/service/search_semantic.go`）：`RebuildEmbeddings()`（全库分批重算、dim 自探）+ `fuseSemantic()`（FTS+`KnnNoteIDs` 经 `FuseRRF`；默认关，关闭/未配/失败一律退回纯词法，绝不减结果）；`App.RebuildEmbeddings` binding；httptest 桩端点验证「词法零命中→向量补出、关掉即回纯词法」
- [x] **A/B 评测门已就绪**：`internal/search/abeval`（Recall@k/NDCG@k + `Compare` delta + 单测）+ `cmd/abeval`（对活库跑 lexical vs hybrid、`GATE PASS/FAIL`、`-min-recall` 阈值；无端点则 hybrid=lexical 自然不过门）
- [x] **向量存储选型定 + 安装引导落地（[ADR-0013](docs/adr/0013-vector-database-selection.md)）**：轴 B 默认**本地 sqlite-vec（`modernc.org/sqlite/vec` 纯 Go，校正原稿「需 CGO」之误）**；`db.VectorStoreHealthCheck` + `cmd/vector-init`（引导式：自检 vec → 建/验 vec0 → 选 embedding provider[Ollama 本地默认/远程 OpenAI/skip] → 写配置 → **指向 设置→插件** 复核+开启+重建；`semantic_search` 保持默认关）
- [x] **远程向量库接缝已实现（Qdrant，opt-in + 自动退回本地）**：`internal/search/vectordb` `Store` 接口（`Local`＝sqlite-vec 默认 / `Qdrant`＝REST）+ `Open` 按 `vector_store*` 配置选择、不可达退回本地；`cmd/vector-init -store qdrant` 写入并探测；search_semantic 的 Rebuild/fuse 改走 `Store`；httptest 桩测 + **build-tag 门控真实冒烟（`ollamalive`/`qdrantlive`/`aelive`，本轮已在本地真 Ollama+真 Qdrant 跑通**，含全链路语义召回；CI 默认不跑）。换 Weaviate 只需再加一个实现
- [x] **registry 已落地（核验收口，非待办）**：`internal/search/vectordb/store.go` 已有 `registry` map + `Register(kind, factory)` + `Kinds()`（local 恒隐式）+ `Open` 三重退回（未配 / 未知 kind / 远程工厂报错或不可探测一律 local）；Weaviate 已经在 registry 里，调用方零改动。原条目描述的「改为可插拔 registry」已完成，**剩余未接后端（Pinecone/Milvus/chromem-go/Bleve）统一记在下面一条**，不重复挂账。附带修正：`vectordb.go` 包注释原写「Two implementations: local / qdrant」漏了 Weaviate，已改为三个 + registry 指路
- [x] **OMP 导出接缝已实现（provisional）**：`service.ExportMemoryJSON` 导出 OMP 风格 Memory Object 数组（binding `App.ExportMemoryJSON` + 前端 `exportMemoryJSON` + 单测）；仅导出向
- [ ] OMP 导入向 + 字段映射对齐（等 OMP v1 稳定）；chromem-go/Bleve（需 `go get`，本会话离线未加）、Pinecone/Milvus（需凭据/集群）按同 registry 流程待接
- [x] **增量 embed 已落地（原「现全量重建」缺口的收口）**：schema v14 加 `note_embed_dirty` 队列 + `project_notes` 三个触发器（INSERT/UPDATE-WHEN-文本真变/DELETE），后台 `embed-drainer`（`internal/service/embed_drain.go`，5s tick、批量 64）排空：note 还在→embed+Upsert，note 已删→`Store.Delete`（`Store` 接口为此新增 Delete，local/qdrant/weaviate 各一实现）。选触发器而非服务层 hook 的理由是硬的：`internal/db` 才是所有写入者收敛的地方，5 个 agent 记忆 importer 走插件运行时直写 db、**根本不经 `service.*Note`**，服务层 hook 会漏掉最大的一路——这与 FTS5 当年用同样三个触发器解决的是同一个问题。三条刻意的保守：① 一切仍在 `semantic_search` 之后（那是用户同意把笔记文本发往端点的显式授权，不另开第二个开关，否则"主开关开、增量关"会静默留下陈旧索引）；② 维度未知或索引尚未建立时**直接跳过且不消费队列**——猜维度意味着用错的 width 调 `Ensure`，那会 drop 并重建整个索引；③ 端点挂掉时队列原地保留，下一 tick 重试，所以离线笔记本合上盖子也不会丢写。`RebuildEmbeddings` 顺带持久化它学到的 `embedding_dim`（这是增量得以解锁的前置），且**只有完整跑完才清空队列**。回归：`internal/db/embed_dirty_test.go` 5 例（含"只改标签/置顶/移动不触发 embedding，改标题触发"）+ `internal/service/embed_drain_test.go` 6 例（ importer 直写路径端到端可召回、删除后不再被召回且零 embedding 请求、开关关时零外发、两道门、端点挂掉不消费队列、store 拒删不消费）。
- [ ] A 面向普通用户上线前：embedding 配置前端 UI（未过门前刻意不做开关）、一份真实标注 query 集

### M4: Agent 集成即插即用

- [x] Claude Code hook 示例：SessionEnd hook 自动触发 reponest_handoff（v1.9.4 交付于 docs/features/ai-integration.md「会话结束自动交接」节）
- [x] `npx reponest-init` 类一键注册脚本（写 .mcp.json + 提示 hook 配置；ADR-0009 第一步，该 ADR Proposed→Accepted 的门槛项）——已交付 `scripts/reponest-init/`（零依赖 Node ≥18，幂等 + dry-run + hook 安装，2026-10-02）

### M5: IDE 存在感（ADR-0009 薄客户端分发）

- [x] VS Code 扩展（唯一 IDE 扩展，一份 VSIX 覆盖 VS Code / Cursor / Windsurf 全 fork 家族）——骨架已交付 `ide/vscode/`（tsc 零错误）：命令面板 context / handoff / search + 状态栏入口 + 侧边栏笔记检索（MCP stdio 薄客户端）；余：VSIX 打包 / CI、状态栏「上次交接时间」（需服务端交接时间戳 API）
- [ ] JetBrains 插件：缓议——独立 Kotlin/Gradle 代码库双倍维护面，待真实需求信号（issue/star）并补充 ADR 后再立项
- [x] 分发评估门：**本条是 ADR-0009 决策 5「传播原则」的复述，早已是正式条款**（中英两版均在：「今后每个分发资产立项时必须回答『人在哪个界面上看见它』；只有 agent 能消费、人不可见的资产，需说明其服务的是存量用户的深度而非获客」）。挂在 TODO 里永远不会被「完成」——它是约束不是待办，故不再占未勾位，判定依据指向 ADR-0009 本身

### M6: LLM Wiki 知识编译层 → [ADR-0014](docs/adr/0014-llm-wiki-knowledge-compiler.md)

- [x] **W0 前置隐私门已落地（选 (b) 旧库一次性归零，schema v15）**：三条出路里选了姿态最硬的一条，代价明示——对从未主动开启过的老用户，表现为"自动导入停了"，需重新显式开启一次。**实现时挖出真正的根因，比"无法区分默认与显式"更糟**：`db.GetConfig` 把 `sql.ErrNoRows` 映射成 `("", nil)`，而启动判据是 `err == nil && v != "0"` → **空串不等于 "0"，行不存在＝自动导入照跑**。所以这不是"默认值选错了"，而是"未设置"这个状态本身就等于放行。修法三处配套：① 判据翻成 `== "1"` 并抽成 `service.autoImportEnabled()`（缺失 / 空串 / 乱值一律关，安全态成为默认态）；② `insertDefaults` 种子改 `"0"`（保留行只为设置页显示真值，不再承载隐私语义）；③ v15 迁移 `UPDATE app_config SET value='0' WHERE key='auto_import' AND value <> '0'`——只改写"曾经为开"的行、显式 '0' 不动、幂等可重放、不新增行。`ExpectedSchemaVersion` 同步 14→15（同上一轮，仓库的 `TestExpectedSchemaVersionMatchesMigrations` 是防漂移的守卫）。回归：`internal/db/migrate_v15_test.go` 3 例（旧库归零 + 不碰无关键 + 版本推进 / 幂等重放且永不反向打开 / 无行时不得凭空造出一个"开"）+ `internal/service/auto_import_test.go` 6 状态表驱动（含"行被删除"这个原 bug 用例）。文档与升级告知：`features/ai-integration`（中英配置表）、`features/settings`（中英开关说明）、`plugins/overview`（中英）全部改写，CHANGELOG 以**未发布的破坏性变更**明示。注：M6-W3 的编译层仍需在 `semantic_search` 之外另立逐源门，本项只解决"启动即导入"
- [x] **W1 结构成图已落地（schema v16，无任何 LLM 调用、无界面）**：`internal/db/wiki.go` 加四张派生表——`wiki_pages`（**五类** kind，`CHECK` + Go 枚举双重守；`slug` **全局唯一**，理由是按项目唯一会因 `project_id` 可空留下 NULL 洞，两条全局页同名让 wikilink 解析歧义）、`page_links`（有向边，`to_page_id`/`from_page_id` 双向索引，`CHECK(from<>to)` 禁自环，FK 全 CASCADE，`UNIQUE(from,to,relation)` 使重复连线幂等）、`note_pages`、以及关闭另一个历史缺口的 `note_repositories`（笔记此前只能挂项目，从来不能指向项目内的某个仓库）。`DropWikiSchema` 整层可弃，`InitDB` 每次开库无条件 `EnsureWikiSchema` 自愈。回归 9 例含**可逆性构造证明**（drop 前后 `sqlite_master` 对象快照逐字节相等，且先断言"加了东西"再断言"能撤回去"，否则比较是空的）。两处被测试抓出的真问题已记进 ADR：可逆性原本缺自愈（v16 只跑一次戳，drop 后重启得到有版本无表的死库）；以及 v15 测试把版本号绝对断言成 15 的脆断言。**刻意不做**：service 层门面与前端绑定——W1 的消费者是 W2 取证改造，此刻加无人调用的绑定正违反本仓 P32 的"无死绑定"审计；W1 的验收物是结构 + 可逆 + 双向查询，不是界面
- [ ] **W1b 单向导出旁路**：`reponest wiki export` 渲染 md 文件树 + YAML frontmatter + wikilink + 导出清单，用 Obsidian graph view 反向验证结构是否自然。**刻意不做双向同步**（ADR-0014 决策 2：两个写入者会让 `note_versions` / FTS5 / 向量索引全部重做）
- [ ] **W2 AskAI 取证改造——核心、门禁与前端均已落地，剩流式与真实标注集**：`internal/service/wiki_evidence.go` 实现 `GatherEvidence`（页面 FTS5 v17 + 既有笔记检索两路召回、round-robin 交织）+ 条数/字符/超时三重预算 + `[P#]/[N#]` 引用块 + `FileAnswerAsPage`（答案回档成 `query` 页、链到被引页、挂上被引笔记）。`AskAIWithEvidence` / `FileAnswerAsPage` 已加桌面绑定并计入 bindings 的绑定↔MCP 审计块（按设计无 MCP 孪生：消费者是界面）。fixture 门禁用现成 `abeval` 度量：legacy recall@8=0.000 / evidence=1.000——**只证明机制正确，不证明真实收益**（语料刻意把答案笔记放在 legacy 窗口之外）。实现期抓到的两个真 bug 已修并留注释：`added = added || tryAdd(...)` 的 Go 短路让笔记在任何页命中时被静默丢弃（即常态）；引用 `P1` 曾被当作「页 id=1」解析，而 `Render` 的 ref 是位置编号——回档会链到无关页，改为必须由那次检索的 Evidence 集解析，编造的 ref 解析为空。**剩余三件**：① AIAskPanel 切到 `askAIWithEvidence` 并渲染引用 + 「存为页面」按钮（需肉眼验收，故未混进本提交）；② 流式（传输层，与答案质量无关，单独一步）；③ 真实标注 query 集上的 delta（原判据的另一半）。**顺带更正本文档旧断言**：那 10 条笔记不是「最近」写的而是**最早**写的（`db.ListNotes` 为 `pinned DESC, sort_order ASC, created_at ASC, id ASC`，取前 10）——笔记越多，新沉淀越进不了 prompt，比原先描述的更反直觉；ADR-0014 与 ai-integration 双语页已同步更正。**面板已切换**：`AIAskPanel` 改走 `askAIWithEvidence`，回复下方渲染引用清单（`[P#]` 类型/标题，预算外省略数一目了然），零证据时明确标黄提示「本次仅用项目上下文作答」而不是留个空列表让人猜；新增「存为页面」按钮，**回档链接取答案正文里真正出现的 `[P#]/[N#]`**（正则抽取并与该次检索的 ref 集求交，模型编造的 `[P9]` 不入库），答案一个字都没引时才退回整套证据。跨项目防护用「给证据打上所属项目」实现而非在 effect 里清 state（本仓 react-hooks 规则禁后者，且这样陈旧答案的 `P1` 结构上就不可能解析到新项目的页）。顺带修掉 `ProjectCommitLog` 里同规则的两处违规（选择态与结果态都改为按 key 打标，副产品是一次慢响应再也画不到新视图上）。回归 +3 例（`endpoints.test.ts` 的路由契约，含"绑定返回 null 时证据必须降级为空集"——否则面板是空白+死按钮）；前端 13 文件 96 用例、tsc、eslint（全 src 零错）、vite build 通过。**剩余两件**：① 流式（传输层，与答案质量无关）；② 真实标注 query 集上的 delta。另需一次肉眼验收：证据清单与「存为页面」的实际排版
- [ ] **W3 摄入时编译（opt-in、默认关）**：importer 由「原文 upsert」升级为「抽取 → 新建/更新页面 → 更新 index + 追加 log → 标注与既有笔记的矛盾」；LLM 写入一律 `source='llm-wiki'` + 待审，人工批准后才进 index；一次一源、人保持在环。新增字符串配置键必须在 `service/config.go` 的 `allowedConfigKeys` 与 `stringConfigKeys` **两处**登记，否则 `UpdateConfig` 按数值校验拒绝
- [ ] **W4 Lint 五查（必配防腐层）**：矛盾 / 过时声明 / 孤页 / 缺交叉引用 / 数据缺口 → 产出落 `project_todos`（复用现有表，不新造反馈面）；**LLM 只能建议、不得自动改写页面**；定时与手动触发各一条路径
- [ ] **W5 分层蒸馏与检索**：L0 转录（capture 链路已有）/ L1 原子笔记（`project_notes`）/ L2 项目场景（`BuildProjectContext` 已是雏形，`internal/service/context.go:94-120`）/ L3 跨项目画像（待立项）；检索改「L2/L3 引导上下文，要具体事实才回落 L1/L0」。页面量上来后评估把语义融合范围从 notes 扩到 `wiki_pages`（受 ADR-0012「只对 notes 融合」现状约束）
- [ ] **schema 作用域结论**：`schema.md` 等价物放全局默认还是按项目覆盖（倾向全局默认 + 项目覆盖），定了才动 W3

> **M6 刻意不做**：自建或对接远端 Memory Hub（违背 local-first 定位，且外部协议仍在 churn——自家 OMP 导出至今标 PROVISIONAL、导入侧刻意未写）；文件树双向同步；为 wiki 引入新向量后端（sqlite-vec 零 CGO 已在库内，ADR-0013）。

---

## 📋 遗留项

- [x] **性能（M3-A 审核发现）已落地**：`service.vectorStore()` 改为 memo。原实现每次解析都要 4 次 `db.GetConfig`，而远程后端还要在 `vectordb.Open` 的工厂里做一次 HTTP 可达性探测——配了 Qdrant/Weaviate 时**每次语义检索都多一个网络往返**，这才是真正贵的部分（本地默认只贵那几次 SELECT）。失效点两处：`UpdateConfig` 命中 `vector_store*` 前缀即丢 memo（否则在设置页改了端点要到重启才生效），以及任何一次 store 报错即丢 memo（远程中途挂掉时下一轮重新解析并退回 local，而不是抱着死句柄不放——缓存之前这个是"意外自愈"的，加了缓存必须显式做）。查询路径顺带补了此前静默的向量检索失败日志。回归：`internal/service/vector_store_cache_test.go` 4 例（memo 生效 / 四个 vector_store* 键各自失效且 `embedding_model` 不误伤 / 死 store 在 fuse 与 rebuild 两条路径都被丢弃且错误如实上抛）
- [x] **文档同步已做（原「尚未提及捕捉/语义检索/向量库/CLI/OMP」）**：`docs/features/ai-integration.md` + `docs/features/knowledge.md` 各补中英两版——语义检索（两级 RRF、默认关、失败退回词法、首次全量重建 + 之后触发器队列增量、`vector-init` 引导、`abeval` 评测门在前开关在后）、AI 问答的真实现状（静态打包 10 条 × 500 字节、非流式无引用，明说这是「塞入」不是「取证」，改造路径指向 ADR-0014 M6-W2）、无头 HTTP 与可执行清单（`cmd/server` 的 `/api/rpc` 反射、`dev.sh` 双服务、**发布资产只含桌面包 + `reponest-mcp` + VSIX，其余需自行 `go build`**）、OMP 导出（只有导出、字段 provisional）、会话捕捉键 `claude_session_capture`。过程中修掉三处文档债：① `ai-integration` 的价值定位段原断言「RepoNest 不调用任何大语言模型——代码里没有任何 API key 配置、没有 endpoint 设置」，而配置白名单实测 17 键、v1.15.0 的 AI 问答就在打 `/chat/completions`——改为「不托管模型 / 不代持密钥 / 可选能力默认关」并把隐私责任写清；② 配置表原文「白名单只含 4 个配置键，没有任何一项涉及 LLM」同样过期，已拆成核心配置与 AI 侧配置两张表并逐键标注「开了会把什么发出去」；③ `cmd/server/main.go` 的 Usage 注释写 `reponest server [--port ...]`，但根 `reponest` 是 Wails 应用、**没有任何子命令派发**，已改为 `go build -o reponest-server ./cmd/server`。另发现并修复双语漂移：英文站导入章节还停在「一键导 Claude 记忆」，中文站早已是 5 源表格——英文侧补齐 5 源 + 三条易踩规则。核验过「短 CJK 查询自动降级 LIKE」这句旧文档仍然成立（`internal/db/search.go` 确有 LIKE 路径），未误删

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
