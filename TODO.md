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
- [ ] 保留全局 CSS 仅用于 reset、design tokens、跨组件基础样式——**2026-10-09 复核：目标已大幅接近但未收口**。`design-system` 全局 CSS 降至 3,556 行（2026-10-09 第三批后为 **2,562 行**）；`dashboard.css`/`project-detail.css` 已只剩共享语义色、共享操作按钮和 `.detail-section` 等跨组件基座，`features/knowledge.css`（29）与 `heatmap.css`（已删）本轮收口完毕，剩下的是组件基础层（`components/*`）与两屏基座。
- [x] **P35 路线已定「集中收三大文件」并落地第一块（外观 tab）**：`theme-*` 六类迁进 `AppearanceTab.module.css`，settings.css 305→160 行。**过程中推翻了"按文件整收"的做法**——`settings.css` 的 `.settings-section`/`.section-desc`/`.form-hint` 被 7 个组件共享、`.empty` 全站 15 处在用，整文件搬会把共享规则私有化；真正的迁移单位是**组件独占簇**。另挖出 `layouts/main.css` 反向伸进组件类的冗余规则（同值 560px，已删）与 `plugin-err`/`plugin-error` 两条死规则（零引用，已删；TSX 在用的 `plugin-ok` 反而无定义，留待 PluginsTab 那轮定性）。**第二块也已落地**：`ScanRootsTab.module.css` + `PluginsTab.module.css`，settings.css 164→47 行，至此该文件只剩多子页共用基座。三个撞上的坑记在这：PluginsTab 里 `sources.map((s) => ...)` 会遮蔽本仓惯例导入名 `s`（该文件改用 `css`）；`.root-item.empty` 的 `empty` 只有本 tab 会加，收成本地 `rootItemEmpty` 以避开在 `:hover:not()`/`:has()` 里嵌 `:global()`；`plugin-ok` 定性完成——markup 在用而 CSS 从无定义，已随迁移去掉。剩余按簇清单：AiTab/AuthorsTab/StandardsTab/ActionsTab 的表单类（多半仍是共用基座，逐处核对），之后才是 `project-detail.css`(761) 与 `dashboard.css`(641) 两块大石。**每块都需要人工实机看一遍**，CI 对 CSS 改名无感
- [ ] 逐组件迁移，每轮 sprint 处理 1-2 个组件——**2026-10-09 完成 Dashboard、ProjectDetail 两块大石与 Knowledge/Heatmap 两个小簇**：
  - 已落：`AppearanceTab.module.css`、`ScanRootsTab.module.css`、`PluginsTab.module.css`、`ProjectCard.module.css`（2026-10-08）、`Knowledge.module.css` + `Heatmap.module.css`（2026-10-09），`settings.css` 305 → **47 行且剩下的确认为真共享基座**（`settings-group*` 被 ActionsTab 与 PluginsTab 共用），`knowledge.css` 320 → **29 行**、`heatmap.css` **整文件删除**（全部类仅 `Heatmap.tsx` 引用，另删 23 条零引用死规则）。全局 CSS 3,556 → **2,994**、module 1,993 → **2,479**。规则体逐条对账丢失 0，`src/` 旧类残留 0，tsc/vite/93 用例全过。
  - **两块大文件的阻碍已核查，结论与上一版记录不同（2026-10-08 复核，务必以此为准）**：上一版写的「`stat-value` 被 2 个文件共用、属 Heatmap 与 ProjectDetail，`.card-stat .stat-value.green` 一条规则横穿三个组件」**是错的**——逐条词边界匹配后：裸 `.stat-value` 在**全部 tsx 里零引用**（Heatmap 用的是 `.heatmap-stat-value`，前缀版，同名不同类）；`.card-stat` 唯一引用在 `Dashboard.tsx:107` 的骨架屏里且内无 `.stat-value` 子元素；`.progress-value`/`.card-hero-value`/`.detail-stat`/`.stat-label` 同样零引用。**这些规则组永不匹配，删掉不改变任何渲染。**
  - **真正在用的语义色入口只有两个**：`SummaryBar.tsx:48/57/63`（`summary-value green|red|zero`）与 `ProjectDetail.tsx:205/206`（`head-stat-value`，其 css 已自带 doubled 类）。所以承重墙只有一处，不是三处。
  - **本轮修掉的是一处活 bug**：`.flat-num`（dashboard.css:637）设置 `color` 且写在 `.green`/`.red`（:96/97）**之后**，同特异性（0,1,0）→ 项目卡扁平行的增删数字与 muted 零值**全部渲染成灰色**。已用真实 Chrome 量过 computed style 确认（修前 `--text-primary`，修后 `--success`/`--danger`/`--text-muted`），并把配对写成显式 doubled 规则（`.flat-num.green` 等），从此不再依赖源码顺序。`web/src/styles/semanticInk.test.ts` 钉住这个不变量，回退 CSS 即失败。
  - **已删死规则 169 行**（dashboard.css 641→524、project-detail.css 761→603，均为逐条零引用校验后删，规则体无丢失，postcss 解析 + 浏览器实测双证）。**这改变了迁移的风险评估**：跨屏真共享的只剩 `tab-btn`（tabs.css + project-detail.css）与 `card-star`/`starred`（ProjectCard + ProjectSearchDropdown）。
  - 下一步因此更简单：先补 semanticInk 测试守住顺序不变量，再按 SummaryBar → ProjectCard → Dashboard → ProjectDetail 逐屏迁，每屏人工实机看一次——**CSS 改名 CI 完全无感，这两屏是最高曝光面**。不配「新组件一律 module.css」的冻结约定，这条路追不上新增。
  - **ProjectCard 已迁（2026-10-08）**：`ProjectCard.module.css` 新建，`dashboard.css` 524 → 446 行。**边界判据落地成文**（写在 module 文件头）：`.card-star`/`.card-refresh-btn`（与 ProjectSearchDropdown 共享）、`.green/.red/.muted-num` 语义色、`.badge*` 集合（与 NoteSection 共享）、基础 `.project-card` 面（Dashboard 骨架屏在用）全部留全局；组件独占的 shell/flat/name/stats/pair/label/num/badges 全部下沉。`.flat-num` 全局基类与 `.flat-num.green/.red/.muted-num` 三条 doubled 规则随之退役——`.num` 与 `.numSuccess/.numDanger/.numMuted` 同在一个 module 里被同一套 hash 作用域，全局级联顺序风险从结构上消失；`semanticInk.test.ts` 的 pairs 列表同步移除 `flat-num` 并写明缘由。计数不含本轮：累计全局 CSS 5101 → 4758、module 464 → 580。
  - **Dashboard / ProjectDetail 两屏迁移前仍需先做**：这两屏的骨架屏与真组件共用全局类名（`.project-card`、`.card-header`、`.card-grid`、`.card-stat`），迁移时必须先把骨架屏改用同一 module 或独立占位类，否则会出现"骨架与真卡各有一个类、样式漂移"——本轮已在 ProjectCard 的 module 注释里标记此依赖。
  - **2026-10-09：Dashboard / SummaryBar / GoalRing / ProjectCard 基座 / ProjectDetail / Overview / CommitLog / TrendChart 全部私有化**。Dashboard 骨架屏改用页面自己的 skeleton surface，ProjectCard 用自己的 `.card` 基座；共享 `.card-star`/`.card-refresh-btn` 与语义色留全局。ProjectDetail 只保留 `.detail-section`、`.empty-section` 与 heatmap empty-state 共享。全局 CSS 4,758 → 3,556、module CSS 580 → 1,993；tsc/vite/eslint/108 用例全绿。**剩余风险**：这两屏的视觉细节仍需人工实机复核，CI 对 CSS 类名迁移无感。

  - **2026-10-09 第三批（v1.16.1）：`main.css` 残留清理 + BlockEditor/StatusBar + fab.css 拆分**。① `main.css` 149 → 116 行：`.action-row` → 新建 `ActionsTab.module.css`；`.sort-control` → 已有的 `Dashboard.module.css`（上一轮漏下，它定义在 layouts 文件里）；`.date-picker` 与 `.date-input` 的 150px 尺寸 → 新建 `DatePicker.module.css`（`.date-input` 属 inputs.css 共享族，markup 不动、只迁尺寸）。② `block-editor.css`（16 类）与 `status-bar.css`（6 类）整文件删除，分别新建 `BlockEditor.module.css` / `StatusBar.module.css`，`index.css` 里的 `.status-dot-error` 收进后者；`semanticInk.test.ts` 清单同步删两条（必须与 index.css 导入顺序一致）。③ `fab.css` 22 → 1 类：12 个 `fab-*` 归新建的 `QuickCaptureFab.module.css`，7 个 `ai-*` + `.link-btn` 追加进已有的 `AIAskPanel.module.css`，`.fab-content` 两组件共用留全局。跨组件类一律 `:global()` 包裹（`.active` 全站 17 文件、`.muted` 与 SummaryBar 共用、`.btn` 23 文件）。累计全局 CSS 2,994 → **2,562 行**（22 文件）、module 2,479 → **2,997 行**（22 文件）。
  - **全站归属审计已跑（2026-10-09）**：`web/src/styles/**` 24 个 CSS × 80 个 TS/TSX 的引用文件数 —— **170 个类全站仅 1 个组件引用、32 个类零引用（死代码）**。下一轮候选：`features/notes.css`（22 独占，拆 NoteEditor 与 VersionHistoryPanel，注意 `note-textarea`/`note-title-input`/`note-tags-input` 与 inputs.css 共享族重名）、`components/search.css`（22 独占，`hit-*` → Knowledge、`search-*` → ProjectSearchDropdown）、`command-palette.css`（14 独占）、`toast.css`（6 独占）、`todos.css`（6 独占）；零散残留（`.skeleton-card`/`.tech-chip`/`.badge-*`/`.toggle-*`/`.skeleton-value`/`.skip-link`/`.card-refresh-btn`+`.spin`/`.empty-section`/`.heatmap-empty-state`）可并成一轮；32 个死类建议单独清一轮并写进 CHANGELOG。审计脚本对「字符串里恰好含类名」会误报（如 `.status-bar` 被 `api/endpoints.ts` 命中），需人工过一遍。
  - **空壳类累计 4 个**（markup 在用、全站无 CSS 定义）：`knowledge`、`knowledge-section`、`ai-url-row`、`status-warn`，待按 `plugin-ok` 先例定性后清理。

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
- [x] **chromem-go 已接入（第 3 个可选后端，纯 Go 嵌入式）**：`internal/search/vectordb/chromem.go` `Store` 实现 + registry 注册（`"chromem"`，`vector_store_url` 存持久化目录，空目录拒绝并退回本地——内存模式会在重启时丢光向量）。映射：note id ↔ `"note:<id>"` doc id、Upsert=delete-first（chromem 拒重复 id，这样才能幂等重写）、Search 对 `nResults > 库内总数` 做钳制而非报错（调用方本就接受少命中）、embeddingFunc 恒为“必须永不运行”的哨兵——向量一律由调用方预算好。持久化模式经 reopen 往返验证（同一个目录第二个 DB 实例能看到向量）。顺带修掉一个被新后端暴露的既有缺陷：`Kinds()` 把 `"local"` 一起排字典序，`chromem` 排到它前面就把默认项挤出首位——现在 local 恒在首、registry 键单独排序。回归 6 组（`chromem_test.go`，含持久化 reopen 与 registry 退回）。剩余：Bleve（需先评估“是否替代 FTS5”的架构决策）、Pinecone/Milvus（需凭据/集群）按同 registry 流程待接；OMP 导入向等 v1 稳定
- [ ] OMP 导入向 + 字段映射对齐（等 OMP v1 稳定）；Bleve（需 `go get` + 架构决策）、Pinecone/Milvus（需凭据/集群）按同 registry 流程待接
- [x] **增量 embed 已落地（原「现全量重建」缺口的收口）**：schema v14 加 `note_embed_dirty` 队列 + `project_notes` 三个触发器（INSERT/UPDATE-WHEN-文本真变/DELETE），后台 `embed-drainer`（`internal/service/embed_drain.go`，5s tick、批量 64）排空：note 还在→embed+Upsert，note 已删→`Store.Delete`（`Store` 接口为此新增 Delete，local/qdrant/weaviate 各一实现）。选触发器而非服务层 hook 的理由是硬的：`internal/db` 才是所有写入者收敛的地方，5 个 agent 记忆 importer 走插件运行时直写 db、**根本不经 `service.*Note`**，服务层 hook 会漏掉最大的一路——这与 FTS5 当年用同样三个触发器解决的是同一个问题。三条刻意的保守：① 一切仍在 `semantic_search` 之后（那是用户同意把笔记文本发往端点的显式授权，不另开第二个开关，否则"主开关开、增量关"会静默留下陈旧索引）；② 维度未知或索引尚未建立时**直接跳过且不消费队列**——猜维度意味着用错的 width 调 `Ensure`，那会 drop 并重建整个索引；③ 端点挂掉时队列原地保留，下一 tick 重试，所以离线笔记本合上盖子也不会丢写。`RebuildEmbeddings` 顺带持久化它学到的 `embedding_dim`（这是增量得以解锁的前置），且**只有完整跑完才清空队列**。回归：`internal/db/embed_dirty_test.go` 5 例（含"只改标签/置顶/移动不触发 embedding，改标题触发"）+ `internal/service/embed_drain_test.go` 6 例（ importer 直写路径端到端可召回、删除后不再被召回且零 embedding 请求、开关关时零外发、两道门、端点挂掉不消费队列、store 拒删不消费）。
- [ ] A 面向普通用户默认上线前：一份真实标注 query 集。设置 AI tab 已提供知情开关、embedding/向量存储配置与重建索引入口（默认仍关；`cmd/abeval` 仍是默认变更门）

### M4: Agent 集成即插即用

- [x] Claude Code hook 示例：SessionEnd hook 自动触发 reponest_handoff（v1.9.4 交付于 docs/features/ai-integration.md「会话结束自动交接」节）
- [x] `npx reponest-init` 类一键注册脚本（写 .mcp.json + 提示 hook 配置；ADR-0009 第一步，该 ADR Proposed→Accepted 的门槛项）——已交付 `scripts/reponest-init/`（零依赖 Node ≥18，幂等 + dry-run + hook 安装，2026-10-02）

### M5: IDE 存在感（ADR-0009 薄客户端分发）

- [x] VS Code 扩展（唯一 IDE 扩展，一份 VSIX 覆盖 VS Code / Cursor / Windsurf 全 fork 家族）——命令面板 context / handoff / search + 状态栏入口与「上次交接时间」+ 侧边栏笔记检索（MCP stdio 薄客户端）。VSIX 打包、artifact 与 release 已由 `.github/workflows/release.yml` 落地；状态栏经 `/api/rpc` 的 `LatestHandoff` 只取时间戳/标题，不拉正文。Marketplace 发布仍受 `VSCE_PAT` 配置影响
- [ ] JetBrains 插件：缓议——独立 Kotlin/Gradle 代码库双倍维护面，待真实需求信号（issue/star）并补充 ADR 后再立项
- [x] 分发评估门：**本条是 ADR-0009 决策 5「传播原则」的复述，早已是正式条款**（中英两版均在：「今后每个分发资产立项时必须回答『人在哪个界面上看见它』；只有 agent 能消费、人不可见的资产，需说明其服务的是存量用户的深度而非获客」）。挂在 TODO 里永远不会被「完成」——它是约束不是待办，故不再占未勾位，判定依据指向 ADR-0009 本身

### M6: LLM Wiki 知识编译层 → [ADR-0014](docs/adr/0014-llm-wiki-knowledge-compiler.md)

- [x] **W0 前置隐私门已落地（选 (b) 旧库一次性归零，schema v15）**：三条出路里选了姿态最硬的一条，代价明示——对从未主动开启过的老用户，表现为"自动导入停了"，需重新显式开启一次。**实现时挖出真正的根因，比"无法区分默认与显式"更糟**：`db.GetConfig` 把 `sql.ErrNoRows` 映射成 `("", nil)`，而启动判据是 `err == nil && v != "0"` → **空串不等于 "0"，行不存在＝自动导入照跑**。所以这不是"默认值选错了"，而是"未设置"这个状态本身就等于放行。修法三处配套：① 判据翻成 `== "1"` 并抽成 `service.autoImportEnabled()`（缺失 / 空串 / 乱值一律关，安全态成为默认态）；② `insertDefaults` 种子改 `"0"`（保留行只为设置页显示真值，不再承载隐私语义）；③ v15 迁移 `UPDATE app_config SET value='0' WHERE key='auto_import' AND value <> '0'`——只改写"曾经为开"的行、显式 '0' 不动、幂等可重放、不新增行。`ExpectedSchemaVersion` 同步 14→15（同上一轮，仓库的 `TestExpectedSchemaVersionMatchesMigrations` 是防漂移的守卫）。回归：`internal/db/migrate_v15_test.go` 3 例（旧库归零 + 不碰无关键 + 版本推进 / 幂等重放且永不反向打开 / 无行时不得凭空造出一个"开"）+ `internal/service/auto_import_test.go` 6 状态表驱动（含"行被删除"这个原 bug 用例）。文档与升级告知：`features/ai-integration`（中英配置表）、`features/settings`（中英开关说明）、`plugins/overview`（中英）全部改写，CHANGELOG 以**未发布的破坏性变更**明示。注：M6-W3 的编译层仍需在 `semantic_search` 之外另立逐源门，本项只解决"启动即导入"
- [x] **W1 结构成图已落地（schema v16，无任何 LLM 调用、无界面）**：`internal/db/wiki.go` 加四张派生表——`wiki_pages`（**五类** kind，`CHECK` + Go 枚举双重守；`slug` **全局唯一**，理由是按项目唯一会因 `project_id` 可空留下 NULL 洞，两条全局页同名让 wikilink 解析歧义）、`page_links`（有向边，`to_page_id`/`from_page_id` 双向索引，`CHECK(from<>to)` 禁自环，FK 全 CASCADE，`UNIQUE(from,to,relation)` 使重复连线幂等）、`note_pages`、以及关闭另一个历史缺口的 `note_repositories`（笔记此前只能挂项目，从来不能指向项目内的某个仓库）。`DropWikiSchema` 整层可弃，`InitDB` 每次开库无条件 `EnsureWikiSchema` 自愈。回归 9 例含**可逆性构造证明**（drop 前后 `sqlite_master` 对象快照逐字节相等，且先断言"加了东西"再断言"能撤回去"，否则比较是空的）。两处被测试抓出的真问题已记进 ADR：可逆性原本缺自愈（v16 只跑一次戳，drop 后重启得到有版本无表的死库）；以及 v15 测试把版本号绝对断言成 15 的脆断言。**刻意不做**：service 层门面与前端绑定——W1 的消费者是 W2 取证改造，此刻加无人调用的绑定正违反本仓 P32 的"无死绑定"审计；W1 的验收物是结构 + 可逆 + 双向查询，不是界面
- [x] **W1b 单向导出旁路已落地（`reponest-wiki-export`）**：`internal/service/wiki_export.go` + `cmd/wiki-export`。命令名更正——ADR 原文写的 `reponest wiki export` 并不存在（根 `reponest` 是 Wails 应用、没有任何子命令派发）。渲染 `wiki/<entities|concepts|sources|synthesis|queries>/<slug>.md`，frontmatter 带 `reponest_page_id`/slug/title/kind/project/`source: reponest-export`；出链与反向链接用 `[[slug|标题]]`（Obsidian 图视图直接可吃），另有 `wiki/index.md`（按 kind 分组、每页一行，就是 ADR 说的 index 物化）与 `wiki/EXPORT-MANIFEST.json`。四条保守取舍：① 目标目录含外来文件即拒写（`ErrWikiExportNotEmpty`），自己的 `wiki/` 子树允许幂等重写；② **陈旧文件只报告不删除**（`stale_files`）——删除等于宣称树归导出所有，正是决策 2 拒绝的立场，且它分不清我们的残留与用户的笔记；③ slug 源自用户标题而此处是文件系统写入方，路径穿越一律拒绝；④ manifest 最后写且**不含绝对路径**，中途崩溃会留下与磁盘不符的清单（同打包清单 sha256 门的思路），换目录导出必须逐字节相同。另为 W4 备好输入：正文手写 `[[链接]]` 指向不存在页计入 `dangling_links`（页→页的边由 FK CASCADE 保证不悬空，测试已直接断言）。回归 6 例 + 真二进制在临时库端到端跑过（不触碰用户 `dashboard.db`）
- [x] **W2 AskAI 取证改造——核心、门禁、前端与流式均已落地，仅剩真实标注集**：`internal/service/wiki_evidence.go` 实现 `GatherEvidence`（页面 FTS5 v17 + 既有笔记检索两路召回、round-robin 交织）+ 条数/字符/超时三重预算 + `[P#]/[N#]` 引用块 + `FileAnswerAsPage`（答案回档成 `query` 页、链到被引页、挂上被引笔记）。`AskAIWithEvidence` / `FileAnswerAsPage` 已加桌面绑定并计入 bindings 的绑定↔MCP 审计块（按设计无 MCP 孪生：消费者是界面）。fixture 门禁用现成 `abeval` 度量：legacy recall@8=0.000 / evidence=1.000——**只证明机制正确，不证明真实收益**（语料刻意把答案笔记放在 legacy 窗口之外）。实现期抓到的两个真 bug 已修并留注释：`added = added || tryAdd(...)` 的 Go 短路让笔记在任何页命中时被静默丢弃（即常态）；引用 `P1` 曾被当作「页 id=1」解析，而 `Render` 的 ref 是位置编号——回档会链到无关页，改为必须由那次检索的 Evidence 集解析，编造的 ref 解析为空。**剩余两件已变一件**：① 流式已落地（`e669d01`→`3ac0942` 五个提交：service 层 `streamChatWithTimeout` + SSE 解析纯函数、`/api/ai/ask-stream` SSE 端点与 4 个 handler 测试、`StartAskStream` 桌面绑定与 Wails 事件推送、前端 transport SSE/Wails 双通道与 AIAskPanel 逐字输出，`docs/features/ai-integration` 双语同步去掉「非流式」措辞）；② 真实标注 query 集上的 delta 仍缺——脚手架 `cmd/queryset` 已就绪，标注本身需人判断（见 M6-W3 前置项）。（原列第三件「AIAskPanel 切换」已在 `bbc1863` 完成：面板现走 `askAIWithEvidence`，渲染依据清单、零证据显式提示、「存为页面」只取答案真正引用的编号。）
- [x] **W3 最小编译闭环已实现（schema v18 + `internal/service/wiki_compile.go` + 5 个桌面绑定）**：按 ADR-0015 批准的参数落地——编译单元=一条笔记、一次模型调用、只接受 `create_page|add_link|attach_note` 三种 op（**类型里没有"改页/删页/approve"，契约就问不出这些动作**）；产物一律 `status=pending` + `source=wiki-compile`；**永不修改已批准页**——撞上同名 slug 时降级为 `compile-revision` 待办，页面 content/title/updated_at 逐字段不变由测试钉住；每条产出行自动挂来源笔记（不给模型选，W4 的 `no-source` 才兜得住）；预算 20 条/每条 6 页/每页 1500 字节/单次 60 页，触顶即**停下并在报告里写明停在哪**；`wiki_compile` 独立开关默认关，不接定时器、不进 `auto_import`、不做 MCP 工具（agent 能自己写待审知识就等于绕过人在环）。`add_link` 还加了一条 ADR 里没写、实现时才发现必要的规则：**两端至少一端是本轮产物**，否则模型可以把已有批准页互相连成没人审过的图。审核侧新增 `ListPendingWikiPages`（带来源笔记与出入链，便于对照）/`ApproveWikiPage`/`RejectWikiPage`，后者**拒绝删已批准页**。导出按批准的开放问题 2 实现：pending 页照常导出并带 `status`/`source`，而 `index.md` 只列 approved——它是检索物，不能把人引到未审内容。契约测试 9 例：门关闭时零外发、pending 对检索与 AskAI 证据不可见而审核可见、批准后转可见、非 JSON 回复零写入、不改已批准页且降级出待办、重跑幂等（1 更新 0 新建）、预算触顶并报告、拒绝不能删已批准、v18 默认与 CHECK。剩余（不属本项）：真实库上跑一轮并由人审（审核 UI 现已存在，见 M6-W3-UI 项）
- [x] **M6-W3-UI 独立「审核」tab 落地；并把「拒绝=删除」改成「不合格→错误页」**（用户定的两条）：`web/src/pages/Review.tsx` + `Review.module.css` + `/review` 路由 + 顶部导航项，i18n 双语 key 全对齐。**后端语义变更（用户可见）**：`RejectWikiPage` 不再删行，而是 `status=rejected` + 正文替换为错误占位，**保留入链**——删除会级联带走 `page_links`，让每个引用过它的页面静默多出死链，比留一个标错节点更糟。配套 `ListRejectedWikiPages`（错误页可见可清理）与 `DeleteCompiledPage`（彻底删除逃生阀，只对未发布页开放，防错误页无限堆积；已批准页拒绝）。无新迁移——`status` 的 CHECK 早已允许 `rejected`，此前只是没人写入。都进了前端 API 层并有端点契约测试（null 一律降级为空，防评审页崩）。前端 99 用例 / 后端 `go test -race` 24 包 / eslint / tsc / vite build 全绿。诚实边界：tab 的「开始编译」仍需先配 `wiki_compile=1`（设置页尚无该开关）；lint 模型侧与交互问答未异步化。
- [x] **W4 Lint 五查已落地**（`internal/service/wiki_lint.go` + `App.RunWikiLint` 绑定 + `startWikiLintTicker`）：五查按"是否需要模型"拆成两层——**孤页 / 缺交叉引用 / 数据缺口**三查纯 SQL 可判（无成本、无外发、随时可跑），**矛盾 / 过时声明**两查走模型且单独由 `wiki_lint_llm` 显式开启（默认关）。安全边界不是口头承诺而是结构事实：**findings 只写 `project_todos`，代码里没有任何一条路径能让 lint 改页面**，由 `TestWikiLint_NeverMutatesPages` 逐字段（content/title/kind/updated_at）钉住——因为"让 LLM 去修 LLM 写错的东西"是更深一层的腐蚀，而建议错了只值用户看一眼复选框。细节上的几个决定：① 缺交叉引用是**标注过的启发式**（正文出现他页标题却无链接），只出建议不自动连线，且边集预取避免 O(n²) 次查询；② 模型的 `page_id` 必须能在库里查到才采信（它编一个 999999 就丢一条），未知 `type` 一律不认；③ **回复解析不出来就一条不提**，并把原因写进 `llm_note`——幻觉出来的"矛盾"是要用户手动清的噪音；④ `llm_note` 让"没发现问题"和"功能没开"在报告里长得不一样；⑤ 定时**第一跳在 6 小时后**而非启动时，免得打开应用就往用户待办里写东西；⑥ 全局页（project_id=0）的发现照常上报但计入 `todos_unfiled`，因为 todo 必须有项目可挂，宁可少挂也不给它编一个家。去重只对**未完成**的同名 todo：勾掉之后页面又漂移，是新事实不是骚扰。另为 W1b 的 `WikiPage.UpdatedAt` 补了字段（过时判定绕不开"谁更新"，放结构体比再查一次强）。回归 7 例（六类检查各触发一次 + 幂等重跑 + 全局页不入库 + 归属项目正确 + LLM 门与不采信 + 空库静默 + parseLintReply 表驱动）
- [x] **W5 分层管道已落地（`internal/service/context_layers.go` + `LayeredProjectContext` 绑定；AskAI 的 L3/L2 前缀改走装配器）**：L3 全局画像（不属于任何项目的**已批准**页面）→ L2 项目场景（复用 `aiProjectBaseContext`，不另造第二套"项目是什么"）→ L1 整理型笔记 → L0 会话逐字稿；**空 query 即只做定向、完全不触发检索**，L0 需显式 `includeTranscripts` 才取。分类是按来源的启发式（`ClassifyNoteLayer`：codex/opencode/cursor 与 kind=log → L0，其余 → L1），代码里写明"来源不等于可信度"，避免后来人把它当质量分级。预算**逐层独立**（全局 1200 / 项目 1600 / 笔记 8 条 2400 / 逐字稿 3 条 900），报告里的 `items` 与实际渲染同源——`Items=len(items)` 那种"报 3 条给 0 字"的自相矛盾已修。**本项落地的是管道，不是蒸馏**：L3/L1 的自动生成本质上是 W3 编译器的工作（产物待审），这里不自动产出任何页面，否则就成了 ADR-0015 拒绝的"没有闸门就让模型写库"。测试 7 组：分类表 13 例、空 query 不回落、L0 需显式开、层序稳定、逐层预算、pending 与别项目页面不进 L3、空库干净。两个同构 bug 由测试抓出并写进注释作规则：预算小于一行时必须裁剪而非消音整层（与 W2 证据层的 40 字预算同一教训），以及计数必须与实际渲染同源。语义融合扩到 `wiki_pages` 那半条仍**未做**——它需要页面语料与页面向量索引，属 W3 真实演练之后的事
- [x] **M6 已落地能力的功能文档已补齐（双语）**：`docs/features/knowledge.md` 与英文镜像新增「知识页面与审核队列」一节（五类页面、pending/approved 的可见性差别、谁在写页面、lint 五查与产出、导出到 Obsidian 的两个刻意性质：只出不进 + 不删除）；`ai-integration.md` 双语**重写 AI 问答一节**（取证链路、三重预算、存为页面、检索为空时回退旧路径），并保留仍成立的那些限制（非流式、无工具调用、页面尚无自己的向量索引、真实库上的收益未测）；配置表补 `wiki_lint_llm` 与 `wiki_compile` 两行并标注各自会把什么发出去；可执行清单补 `cmd/wiki-export`；`settings.md` 双语加一条「本页还没有的开关」，把「待审页没有审核界面」与两个门刻意不放开关的理由写在用户会去找它们的地方。挂这条的原因本身值得记：边实现边写文档时，`ai-integration` 里「10 条笔记、无引用」那段已被我自己的 W2 改动作废——文档与代码同批改才不欠债。（同时更正：M6-W2 条目里「AIAskPanel 尚未切换」已过期于 `bbc1863`，见该条。）
- [x] **`finish_reason` 已透传，截断与格式错在报告里不再同形**：`chatResponse.Choices[].FinishReason` 透传到 `AITestResult`，`truncatedHint()` 把它翻成可操作的一句话（`length`/`max_tokens`/`model_length` → 换模型或提高 max_tokens；`content_filter` → 换问法；未知值也说明）。三条消费点各自落地：compile 与 lint 的 `llm_note` 在解析失败时**先看 finish_reason**——被服务端截断时说"被截断"并附补救办法，真正格式错才说"无法解析"；交互问答走 `EvidenceAnswer.Truncated`，面板弹 info toast 并给出同样的补救提示（不把建议写进答案正文，因为用户可能把答案存成页面）。刻意保留"字段缺失当作 stop"：多数服务端成功时不带这个字段，缺失不是截断的证据。测试 8 例，其中一条用 400ms 慢端点 + 真 finish_reason 走通 HTTP 全程。`aiChatTimeout` 顺带被拆成三档（见下条）。
- [x] **ADR-0016 异步化已实现（schema v19）**：`compile_jobs` 表（queued/running/succeeded/failed/canceled，CHECK 守）+ `bgGo` 单 worker 轮询认领 + `StartCompileJob`/`GetCompileJob`/`ListCompileJobs`/`CancelCompileJob` 四个绑定。**编译内核零改动被复用**（job 循环直接调既有的单条编译入口），满足 ADR 验收 ①；同步入口保留并**与 job 共用同一个笔记选择函数** `compileNoteTargets`，顺带修掉一个方向性错误：原先按 `ListNotes` 顺序取，而它是 `created_at ASC`，等于"编译本项目"先烧你最旧的笔记而不是最近的。三条不写就会咬人的规则各有测试：worker 收到 `ctx.Done()` 落 `canceled`；重启时 stuck 的 `running` 显式判 `failed` 且不被重新认领（没人记录它断在哪条笔记，假装续跑就是编造进度）；**迟到的进度写不能复活已取消的 job**（`AND status <> 'canceled'`——ADR 原文没列，实现时才发现）。提交路径实测毫秒级返回，并用 400ms 慢端点做反证：提交期间模型调用计数必须为 0。**待决 ① 已落地（schema v20）**：加 `kind` 判别列（CHECK 守）+ `findings` 计数，`EnsureCompileJobQueue` 用 ALTER 保住既有编译历史；`runNextCompileJob` 按 kind 分派到既有同步入口，**无第二套实现**（本 ADR 的边界判据）。两条与原设想不同：lint job **必须带 project**（`project_id` 是 NOT NULL + 外键，"全部页面"在库里没有表示，造哨兵项目会污染用户项目列表，故全量 lint 仍走定时同步路径）；结构化三查不进 job（毫秒级 SQL，让用户轮询是演戏）。测试 8 例 + 3 例 v19→v20 升级（含"旧行仍可读"）。测试 7 例；`ExpectedSchemaVersion` 18→19→20。原验收里的"断连演练"在异步形态下换了对象：杀客户端已无关紧要，被钉住的是 worker 取消与重启恢复两条。
- [x] **交互问答的"异步化"做成了修正超时上限，而不是改成 job**（结论与原设想相反，理由是载荷不是偏好）：取证问答发送分层上下文（L3 1200 + L2 1600 字符）+ 最多 12 条检索项（约 4000 字符），与一次编译请求同量级，而编译在本地 27B 上**实测 173 秒**。用 90 秒的 `aiChatTimeout` 兜住它，等于把"模型还在算"报成"失败"——与当初批处理上限太短是同一类错误、反方向。故新增 `aiAskTimeout = 5 分钟`（不超过 `DefaultBatchChatTimeout`，因为 `cmd/server` 的写超时由后者推导并有断言守着），`AskAIWithEvidence` 改走 `TestAIChatWithTimeout`。**没有把它变成 job**：调用方只有一个、面板已有 pending 态，改成 job 只多一个轮询循环而等待时长一分不变；流式（能改变等待本身）另算。`aiChatTimeout` 保留给纯聊天路径（短问题、无检索），并在注释里写清它现在只管这一条。测试 2 例：上限必须 > 纯聊天且 <= 批处理；150ms 慢端点在真上限下必须拿到答案。
- [x] **W2 真实标注 query 集的脚手架已就绪，标注本身仍需用户判断**：`cmd/queryset`（`-seed` 导出带行号的工作表 / `-emit` 把填好的行号解析成 `queries.jsonl`）。**为什么不能由工具生成标注**：「哪条笔记能回答这个问题」是关于内容的判断，编出来的标注会让门禁在与检索质量无关的原因上通过或失败——所以工具只做机械的那一半：用户填行号，id 解析由工具做（手抄 id 才是标注不准的主因）。三个刻意的设计：行号取自**被编辑的那份文件**而不是重查数据库（否则库一变编号就错位）；`relevant` 为空的行直接报错而不是静默跳过（否则报告里的 n= 与工作表对不上，读起来像工具坏了）；不足 10 条时提示"这是起点不是证据"——这个量级上一条 query 就能把 recall@10 推过门限。测试 5 例，其中一条用 `cmd/abeval` 同样的 struct 反解输出，钉住 `abeval.Case` 无 json tag 这个坑（直接编码会写成 `Query`/`Relevant`，被 abeval 读成 0 条）。
- [x] **W3 真实演练的剩余一半已补齐——质量判断已做出：bonsai-27b 的产出值得进入审核流（结论绑定该模型）**——真库副本上完整走完两段。**编译重跑**（5 条真实笔记、约 168 秒/条）：结构保证全数兑现——7 张 pending 页 7/7 自动带来源、`rejected=0`（无一 op 触发守卫）、trivial 笔记（"hello"）正确产出 0 页、全部页在字节预算内；报告 `attachments=14` 而表里只有 7 行不是 bug：自动挂源与模型显式 attach_note 双计数（`note_pages` 主键去重），计数口径是"成功次数"不是"行数"。**逐页保真审计**（agent 对照来源逐页复核，用户委托）：4/7 忠实——cbipay-project、customer-tag-synchronization（主客户开户/高价值客户/补偿任务三条全对）、cbipay-3-0-6-feature（分支名逐字命中）、call-chain-verification（含 SQL 误引案例的忠实压缩）；3/7 局部幻觉——customer-tier-api-optimization 编造"提升同步带宽与数据一致性"的动机（笔记只说接口优化）、fab-panel-quick-collection 把悬浮球测试噪声抬成概念页并补"优化信息录入效率"、repo-nest 无中生有"自动化测试任务生成"（笔记只说笔记与待办创建）。共性：全部是"读页对照来源即可识破"的局部错误，无一是结构性逃逸——这正是审核闸门存在的理由，也正是它工作正常的样子。**审核流端到端演练**（同副本）：approve/reject 迁移正确（4 批准/3 拒绝/0 pending）；FTS 判别实测——"不合格"一词只存在于被拒页占位文本，approved-only 检索 0 命中、去掉过滤 3 命中（被拒页留在索引但检索不可见）；拒绝保留入链（指向被拒页的 1 条边仍在，删行才会级联带走）；`DeleteCompiledPage` 拒绝已批准页。**建议**：本地 27B 约 168-173 秒/条，20 条一批约 1 小时，走 ADR-0016 异步 job；按此样本预期约四成页面会被拒——审核 UI 每页侧挂来源笔记，对照成本就是读两段短文。真库开启 `wiki_compile` 是用户动作（W0 隐私门默认关，agent 不代开）。模型差异警示不变：同一 prompt 下 minicpm5-1b 吐 `{"pages":[]}` 外加散文、gemma-4-e2b 输出无法解析的 op、只有 bonsai-27b 产出合法 op，所以"编译可用"这个结论必须绑定模型说，不能泛化。
- [x] **schema 作用域已裁决：全局默认 + 项目覆盖（2026-10-09）**——采纳 ADR-0014 待决项的倾向，但把「覆盖」的边界写死三条不可谈判线：① **kind 词表不可覆盖**：`wiki_pages.kind` 的 CHECK（W1 五类）是 DDL 级不变量，审核 UI 的 kind 徽章着色、导出目录、W5 L3 跨项目全局画像装配全部假设统一词表，项目自造 kind 三处同破；② **安全契约不可覆盖**：三种 op、预算（20 笔记/6 页/1500B/60 页）、pending→approved 流转、永不触碰已批准页——这些住在 Go 代码里；schema.md 等价物（Karpathy 语境：agent 的纪律来源）只描述「页面长什么样」（命名约定、何谓一条好页、交叉引用习惯），永远无权描述「编译器被允许做什么」——能被项目覆盖松绑守卫的 schema 等于给模型开后门；③ **SSOT 落库不落导出树**：等价物形态是代码内默认 + 项目级覆盖行（ADR-0014 决策 2 的直接推论——导出树是只读派生物，配置放那里改不回来）。**只定结论、不落实现**：真库 job #1 用硬编码 prompt 编译成功（7/7 带来源、rejected_ops=0），尚无项目提出不同约定的真实需求，外置化等第一个真实分歧出现再做——届时覆盖取显式整段替换（可 diff、可审计），不做静默合并。原条目「定了才动 W3」的前提已被事件超越：W3 真库运行早已用硬编码 prompt 完成，本决策实际约束的是将来的外置化形态。附带效应：ADR-0014 晋升条件 4（页面四类 + schema 作用域有结论）就此满足——条件 2（abeval 真实标注 delta）仍开着，ADR 维持 Proposed；待决清单剩余 dsh-plugin 关系与向量融合范围两项，均不阻塞

> **M6 刻意不做**：自建或对接远端 Memory Hub（违背 local-first 定位，且外部协议仍在 churn——自家 OMP 导出至今标 PROVISIONAL、导入侧刻意未写）；文件树双向同步；为 wiki 引入新向量后端（sqlite-vec 零 CGO 已在库内，ADR-0013）。

---

## 🟢 知识图谱路线（ADR-0018 后续）

> ADR-0014 把「结构」补齐了（wiki_pages / page_links），但那张图**写而不读**：relation 是自由串、`GatherEvidence` 从不走边、页面没有向量。M7 分两半把它用起来。实施计划见 `RepoNest-知识图谱实施计划-2026-10-09.md`（Phase 1..4），设计依据 [ADR-0018](docs/adr/0018-knowledge-graph-foundation.md)。

### M7: 知识图谱底座 → [ADR-0018](docs/adr/0018-knowledge-graph-foundation.md)

- [x] **Lane 1 代码 + 契约测试已落地（schema v21）**：受控关系词表八型（`ref/part-of/depends/implements/documents/supersedes/contradicts/mentions`），`page_links.relation` 的 `CHECK` 由 Go 侧 `wikiRelations` 派生（DDL 与枚举单一事实源，`TestRelationVocabulary_CheckDDLMatchesGoSet` 钉住不漂移）；统一入口 `db.NormalizeRelation`（未知→`ref`，旧拼写 `compiled-from`/`cites`→`ref`、`depends-on`→`depends` 走别名表）；`LinkWikiPages`/`UnlinkWikiPages` 入库前一律归一，脏值降级不报错。编译器 `add_link` prompt 从枚举选词、产物默认关系 `compiled-from`→`ref`；回档问答证据边 `cites`→`ref`。`ApproveWikiPage` 抽取正文 `[[wikilink]]` 为幂等 `mentions` 边（只连真实存在页 / 不连自己 / 仅批准时触发，模型无关故可自动跑）。`lintRelationShape` 三查（`supersedes` 时间倒挂 / `part-of`·`depends` 成环 / `contradicts` 缺反向）按项目作用域预取边集、只写 `[lint]` 待办、逐字段不改页。迁移 v21 重建 `page_links`（SQLite 不能给已有列加 CHECK）：归一非法 relation、合并归一后重复边、只改标签不删边、幂等重放。**测试**：`internal/db/wiki_test.go`（CHECK 强制 / NormalizeRelation 表驱动 / DDL-Go 集对齐 / v21 旧库迁移与幂等 / 边双向性改用枚举值）+ `internal/service/wiki_graph_test.go`（approve 抽取幂等且仅已批准、三查各触发）；`go test ./internal/db ./internal/service ./internal/integrity` 全绿，`ExpectedSchemaVersion` 20→21。
- [ ] **Lane 1 晋升条件 1（用户动作）：真库副本演练 + 旧边只读导出**——v21 会动 `page_links`，须在真实 `dashboard.db` 副本上跑一遍确认「不丢边 + 归一结果符合预期」，之后 ADR-0018 lane 1 才升 Accepted。agent 不代跑用户的库。
- [ ] **Lane 2（schema v22，ADR-0018 第二半）：页面独立向量空间 + 图感知取证**——`page_embeddings`（vec0，仅 approved 页，物理独立于 `note_embeddings` 以避 `FuseRRF` 串号）+ `WITH RECURSIVE` 一跳扩召回 + 纯 Go PPR 排序，注入点 `internal/service/wiki_evidence.go:evidencePages`，新门 `wiki_graph_search` 默认关。**前置 = lane 1 Accepted + Phase 0（真实标注 query 集 + `abeval` delta，见 M6-W2 与本文件遗留项）达标**——机制正确不等于收益真实，这条不过关不 Accept。
- [ ] **后续（各自单开 ADR，暂不实现）**：ADR-0019 实体消歧合并（治「概念分裂」，建议只产 `[lint] merge` 待办、人点采纳、合并时重指向边不删页以守 `reject≠delete`）；ADR-0020 社区摘要（Label Propagation 填 synthesis 空层，触及 `compile_jobs.kind` 的 CHECK 需整表重建，与未来批量任务一并做那次 rebuild）；ADR-0021 时态图（`page_versions` 快照，是「允许编译器安全修订既有结论」的前置）。→ **2026-10-10：ADR-0019 草案已成文**（`docs/adr/0019-entity-resolution-merge.md`，双语，Proposed，待批准；仅设计未落码，其晋升排在 ADR-0018 lane 1 Accepted 之后）。0020 / 0021 仍待起草。

> **M7 刻意不做**：外部图库（Neo4j/Memgraph/KuzuDB）、RDF/OWL/SPARQL、GNN——违背 local-first + 单 SQLite + 零 CGO（ADR-0014 决策 2、ADR-0017）。纯 SQL 递归 + Go 侧 PPR 在几万页以内足够；真要上外部图库，套用 `vectordb` 那套「可插拔 + 不可达退回 local」的接缝即可，属另立 ADR。

---

## 📋 遗留项

- [x] **性能（M3-A 审核发现）已落地**：`service.vectorStore()` 改为 memo。原实现每次解析都要 4 次 `db.GetConfig`，而远程后端还要在 `vectordb.Open` 的工厂里做一次 HTTP 可达性探测——配了 Qdrant/Weaviate 时**每次语义检索都多一个网络往返**，这才是真正贵的部分（本地默认只贵那几次 SELECT）。失效点两处：`UpdateConfig` 命中 `vector_store*` 前缀即丢 memo（否则在设置页改了端点要到重启才生效），以及任何一次 store 报错即丢 memo（远程中途挂掉时下一轮重新解析并退回 local，而不是抱着死句柄不放——缓存之前这个是"意外自愈"的，加了缓存必须显式做）。查询路径顺带补了此前静默的向量检索失败日志。回归：`internal/service/vector_store_cache_test.go` 4 例（memo 生效 / 四个 vector_store* 键各自失效且 `embedding_model` 不误伤 / 死 store 在 fuse 与 rebuild 两条路径都被丢弃且错误如实上抛）
- [x] **文档同步已做（原「尚未提及捕捉/语义检索/向量库/CLI/OMP」）**：`docs/features/ai-integration.md` + `docs/features/knowledge.md` 各补中英两版——语义检索（两级 RRF、默认关、失败退回词法、首次全量重建 + 之后触发器队列增量、`vector-init` 引导、`abeval` 评测门在前开关在后）、AI 问答的真实现状（静态打包 10 条 × 500 字节、非流式无引用，明说这是「塞入」不是「取证」，改造路径指向 ADR-0014 M6-W2）、无头 HTTP 与可执行清单（`cmd/server` 的 `/api/rpc` 反射、`dev.sh` 双服务、**发布资产只含桌面包 + `reponest-mcp` + VSIX，其余需自行 `go build`**）、OMP 导出（只有导出、字段 provisional）、会话捕捉键 `claude_session_capture`。过程中修掉三处文档债：① `ai-integration` 的价值定位段原断言「RepoNest 不调用任何大语言模型——代码里没有任何 API key 配置、没有 endpoint 设置」，而配置白名单实测 17 键、v1.15.0 的 AI 问答就在打 `/chat/completions`——改为「不托管模型 / 不代持密钥 / 可选能力默认关」并把隐私责任写清；② 配置表原文「白名单只含 4 个配置键，没有任何一项涉及 LLM」同样过期，已拆成核心配置与 AI 侧配置两张表并逐键标注「开了会把什么发出去」；③ `cmd/server/main.go` 的 Usage 注释写 `reponest server [--port ...]`，但根 `reponest` 是 Wails 应用、**没有任何子命令派发**，已改为 `go build -o reponest-server ./cmd/server`。另发现并修复双语漂移：英文站导入章节还停在「一键导 Claude 记忆」，中文站早已是 5 源表格——英文侧补齐 5 源 + 三条易踩规则。核验过「短 CJK 查询自动降级 LIKE」这句旧文档仍然成立（`internal/db/search.go` 确有 LIKE 路径），未误删

- [ ] 桌面 GUI 回归测试：建议在真机跑一轮冒烟（扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答）
- [ ] **D25 首屏界面对齐（2.0 候选，非现在）**：首屏改为「记忆环入口（最近 handoff + 全局检索）+ 项目状态一览」并排呈现；Dashboard、目标环、热力图、工作日告警作为「项目理解」核心能力的人侧表达**完整保留**——不删除、不默认隐藏、不沉入插件（修正原「仪表盘退化为项目列表 + 最近活动、目标环沉入插件或删除」方向，依据 [ADR-0017](docs/adr/0017-one-kernel-two-entry-points.md)）
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
| **2.0 规划** | D25 首屏界面对齐（记忆环 + 项目状态并排）+ C11 插件系统评估 + P35 全量 CSS Modules | 按版本 |
| **按需** | P29, P31, P32, P33, P36 | 随重构穿插 |
