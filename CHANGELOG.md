# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 格式，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

版本号 SSOT 为 `wails.json` 的 `info.productVersion`，由 `scripts/bump-version.sh` 同步至 `web/package.json`、`internal/version/version.go` 与文档站徽章。

## [Unreleased]

### 新增

- **知识图谱底座 · lane 1（[ADR-0018](docs/adr/0018-knowledge-graph-foundation.md)，schema v21）**：给页面之间的链接加了一层受控关系词表——`ref / part-of / depends / implements / documents / supersedes / contradicts / mentions` 八个枚举值，数据库层用 `CHECK` 硬约束（`page_links.relation` 不再是自由文本）。该 `CHECK` 由 Go 侧的关系清单派生生成，两者不会各说各话。目的：让这张图第一次可以按语义查询（"所有 depends 边"），并为 ADR-0018 lane 2 的图感知取证铺路。
  - **批准页面时自动抽取正文 `[[wikilink]]` 为 `mentions` 边**：只连到真实存在的页、不连自己、幂等；不依赖模型，故可自动执行而不破坏人在环——"批准"这个动作本身就是"这些链接可信"的信号。
  - **lint 新增三条结构体检**（纯 SQL/内存判定，不开模型、不改页面）：`supersedes` 时间倒挂、`part-of`/`depends` 成环、`contradicts` 缺反向。与其余检查一样只写 `[lint]` 项目待办。
- **`db.ListPageLinks`**：一次查询返回每条边连同两端的 kind / updated_at / project，供 lint 的关系体检免 N+1 地遍历，按被审项目作用域过滤。

### 变更

- **编译器的 `add_link` 关系改为从八型枚举里选**（prompt 契约同步更新），产物默认关系由 `compiled-from` 改为 `ref`；回档问答指向证据页的链接由 `cites` 归一为 `ref`。任何未知值或旧拼写在入库前经 `db.NormalizeRelation` 折叠到词表（如 `depends-on`→`depends`、`compiled-from` / `cites`→`ref`），未知值降级为 `ref` 而非报错打断一次本来合格的写入。
- **迁移 v21 重建 `page_links` 表**（SQLite 不能给已有列补 `CHECK`）：既有边的 relation 被规范化进词表，同一对页面因归一而产生的重复边被合并——**只改标签、不删边**。**破坏性提示**：升级前建议在真库副本上演练并留一份旧边只读导出；ADR-0018 lane 1 的晋升条件「真库副本演练」未完成前，该 ADR 仍为 Proposed。

## [1.16.2] - 2026-10-09

### 变更

- **P35 全局 CSS 迁移到 CSS Modules（收尾）**：把全局样式里仍由单个组件独占的规则继续下沉到组件自己的 `.module.css`——笔记与搜索（`notes.css` / `search.css`）、命令面板与 toast 与待办（`command-palette.css` / `toast.css` / `todos.css`）、散落在各文件里的 20 个零散类（新建 `App.module.css`）、`navbar.css` 整文件并入、`messages.css` 分发到 Dashboard / ErrorBanner / ErrorBoundary / NotFound 四个新 module、`tabs.css` 的三个单组件类（新建 `Settings.module.css` / `NoteFilterBar.module.css` / `ScopeToggle.module.css`）。跨组件共享的类一律留全局，module 内引用它们时用 `:global()` 包裹。
- **共享族收口到 `shared.css`**：`buttons.css` / `cards.css` / `inputs.css` / `tabs.css` 四个文件的跨组件共享族，以及 `messages.css` 的 `.message-banner`、`search.css` 的动态类，合并为显式的 `styles/design-system/shared.css`，文件头标明这是设计系统 API。全局样式因此只剩「设计基座 + 共享族 + 工具生成类」三类，原文件删除。
- **空壳钩子类加注释**：10 个 markup 在用但全站无样式定义的类（`ai-url-row` / `chart-simple` / `commit-log` / `commits-trend` / `knowledge-section` / `large` / `overview-empty` / `status-warn` / `knowledge` / `settings`）在对应组件里标注为预留钩子，保留不动。
- **导入顺序调整**：`main.tsx` 中 `import './styles/index.css'` 提到 `import App` 之前，dev 模式下的样式注入顺序与设计系统优先级一致。

### 修复

- 清理全站已无引用的死类（`btn-active` / `badge-info` / `meta-pill` / `stat-tag` / `team` / `settings-head` / `page-sub` / `section-header-row`）。
- 笔记编辑区与搜索框靠 doubled 选择器（`.noteTextarea.noteTextarea` 等）提到 (0,2,0)，修复构建产物中设计系统 CSS 排在最后导致的同特异性覆盖失效。

## [1.16.1] - 2026-10-09

### 变更

- **P35 第六块：`main.css` 残留的组件独占规则清理**：`layouts/main.css` 里三条只被单个组件引用的规则迁出（149 → 116 行）——`.action-row`（含 `.action-row code`）→ 新建 `settings/ActionsTab.module.css`；`.sort-control` → 已有的 `Dashboard.module.css`（上一轮 Dashboard 迁移漏下的，因为它定义在 layouts 文件里而不是 feature 文件里）；`.date-picker` 与 `.date-input` 的 150px 尺寸 → 新建 `components/DatePicker.module.css`。`.date-input` 属 `inputs.css` 共享输入族（与 `.form-input` 等列在同一个选择器组），所以 markup 里 `form-input date-input` 一字未动，只把尺寸收进组件 module，避免用 module 类替换掉它与基元的关联。留全局的判据逐类核过：`.settings-section`/`.section-desc` 被 5-7 个设置页 tab 共用、`.settings-group*` 是 ActionsTab 与 PluginsTab 共用、`.empty-actions` 被 5 个组件共用、`.form-*`/`.btn*` 是基础样式。
- **P35 第七块：BlockEditor 与 StatusBar 迁入 CSS Modules**：`features/block-editor.css` 整文件（16 类）迁入新建的 `BlockEditor.module.css`，`features/status-bar.css`（6 类）迁入新建的 `StatusBar.module.css`，两个文件删除；原先住在 `styles/index.css` 的 `.status-dot-error`（只有 StatusBar 用）一并收进 module。两处跨组件类按既有判据用 `:global()` 包裹：`.block-palette-item.active` → `.blockPaletteItem:global(.active)`（`.active` 是裸工具类，全站 17 个文件在用），`.status-item.muted` → `.statusItem:global(.muted)`（`SummaryBar` 也在用）。`semanticInk.test.ts` 的文件清单同步删掉这两条——该清单必须与 `index.css` 的 `@import` 顺序一致，否则测试读到的级联顺序与真实顺序不符。
- **P35 第八块：`fab.css` 拆分（QuickCaptureFab + AIAskPanel）**：22 类拆成三份——12 个 `fab-*` 类迁入新建的 `QuickCaptureFab.module.css`，7 个 `ai-*` 类与 `.link-btn` 追加进已有的 `AIAskPanel.module.css`，`.fab-content` 留全局（两个组件都用它给各自的文本域：捕获框与 AI 问答框）。`.fab-tab.active` 写 `:global(.active)`、`.ai-actions .btn` 写 `.aiActions :global(.btn)`，保留对全局 `.btn` 基元的引用。`fab.css` 22 → 1 类。
- **顺带做了全站 CSS 归属审计**（脚本化：`web/src/styles/**` 下 24 个 CSS 文件定义的每个类 × 80 个 `.ts`/`.tsx` 的引用文件数）：**170 个类全站只有 1 个组件引用、32 个类零引用（死代码）**，后续轮次按这份清单推进。累计全局 CSS 2,994 → **2,562 行**（22 文件），module CSS 2,479 → **2,997 行**（22 文件）。审计同时暴露 4 个空壳类（markup 在用、全站无任何 CSS 定义）：`knowledge`、`knowledge-section`、`ai-url-row`、`status-warn`，按 `plugin-ok` 先例待单独定性后清理。
- 校验：`npm run build`（tsc + vite）、`eslint src` 与 vitest 15 文件 108 用例全过；每个新 module 的 `s.*` 引用与 module 内类名逐一比对（无缺失定义、无定义未引用——CSS module 的类型不会因拼错报错，这步必须手工核）；`web/src/styles` 下已无被迁走的类定义。**视觉面仍需人工实机复核**：ActionsTab 两个按钮行、Dashboard 排序控件与日期选择器、笔记编辑器块区与 `/` 块面板、底部状态栏、悬浮球与面板。

## [1.16.0] - 2026-10-09

### 新增

- **语义检索设置 UI**：设置 → AI 新增默认关的语义检索开关、embedding endpoint/model/key、向量存储（本地 SQLite-vec / Qdrant / Weaviate / chromem）与重建索引入口。真实标注 A/B 门未过，因此默认值不变；密钥继续脱敏回显，仅输入新值时覆盖。
- **VS Code 状态栏显示最近交接时间**：新增 `LatestHandoff` 只读绑定（仅 note id / project id / title / updated_at，不返回正文），扩展启动、写入交接后与每分钟轮询一次；服务不可达时状态栏保持基础文案。
- **chromem-go 向量存储后端（第 3 个可选后端，纯 Go 嵌入式）**：`internal/search/vectordb/chromem.go` 实现 `Store` 接口并注册进 registry（`"chromem"`），ADR-0013 候选矩阵里「纯 Go 本地备选」的接缝终于接上——它是**嵌入式**存储而非服务器：`vector_store_url` 存的是持久化目录而非 URL，无需可达性探测；空目录配置会被工厂拒绝并退回本地 sqlite-vec，因为内存模式会在重启时把用户重建过的向量全部丢光，假装可用比直接说不行更糟。映射细节：note id ↔ `"note:<id>"` doc id；Upsert 采用 delete-first（chromem 拒绝重复 id，这样才能幂等重写同一条笔记）；Search 对「要前 10 条但库里只有 3 条」做钳制而非报错（chromem 原生会拒绝 nResults > 库内总数，而调用方本就接受少命中）；collection 的 embeddingFunc 恒为一个「必须永不运行」的哨兵函数——向量一律由 RebuildEmbeddings / embed-drainer 预算好，若哨兵被触发说明有代码路径忘了带向量，宁可报错也不静默存零向量。持久化模式经 reopen 往返验证。顺带修掉一个被新后端暴露的既有缺陷：`Kinds()` 对含 `"local"` 在内的全列表排字典序，`chromem` 恰好排在 `local` 前面把默认项挤出首位（既有测试断言 `ks[0]=="local"`，此前只因字母序侥幸成立）——现在 local 恒在首、registry 键单独排序。回归 6 组（`chromem_test.go`：往返/幂等/删除可召回/持久化 reopen/Clear 后可用/超限钳制/校验/registry 退回）。Bleve（需先做「是否替代 FTS5」的架构决策）与 Pinecone/Milvus（需凭据/集群）仍待接
- **5 个零测试 cmd 补齐冒烟测试**：`cmd/abeval`（JSONL 解析：合法/坏行整文件拒绝而非静默跳过/空文件与不可读文件区分开 + `labeledCase` ↔ `abeval.Case` 形状钉死 + `ids` 保序）、`cmd/reponest-capture`（`resolveCwd` 三级优先级：flag > stdin 的 `{"cwd":...}` hook 对象 > 进程工作目录，含截断 JSON 不崩——hook 在用户每次会话结束时跑，panic 会打进终端）、`cmd/server`（WriteTimeout > 批处理上限的启动期守卫独立复核 + 本地回环绑定 + slowloris/keep-alive 超时都在 + `envOr` 端口覆盖优先级）、`cmd/vector-init`（provider 默认表钉住——写错的默认值会被直接写进用户配置、`firstNonEmpty` 覆盖优先级、provider 契约）、`cmd/wiki-export`（真 main() 全流程：导出树 + source 戳 + 回执行、非空目标拒写、缺 `--out` 退田 2；flag 重定义与 log.Fatalf 的 os.Exit 经子进程隔离，一套 inline-first-then-subprocess 机制保证同一二进制可重入）。此前这 5 个二进制在 CI 里只有「能编译」的保证，行为回归无任何网
- **ProjectCard 迁入 CSS Modules（P35 第四块）**：`ProjectCard.module.css` 新建，`dashboard.css` 524 → 446 行。边界判据落地成文（写在 module 文件头）：`.card-star`/`.card-refresh-btn`（与 ProjectSearchDropdown 共享）、`.green/.red/.muted-num` 语义色、`.badge*` 集合（与 NoteSection 共享）、基础 `.project-card` 面（Dashboard 骨架屏在用）全部留全局；组件独占的 shell/flat/name/stats/pair/label/num/badges 全部下沉。`.flat-num` 全局基类与三条 doubled 规则随之退役——`.num` 与 `.numSuccess/.numDanger/.numMuted` 同在一个 module 里被同一套 hash 作用域，全局级联顺序风险从结构上消失；`semanticInk.test.ts` 的 pairs 列表同步移除 `flat-num` 并写明缘由。累计全局 CSS 5101 → 4758、module 464 → 580

### 变更

- **Dashboard 与 ProjectDetail 迁入 CSS Modules（P35）**：Dashboard、SummaryBar、GoalRing、ProjectCard 基座、ProjectDetail、Overview、CommitLog 与 TrendChart 的组件独占样式全部 hash 作用域；跨组件共享的语义色、card action button、`.detail-section` 与 `.section-header` 留在全局设计系统。全局 CSS 从 4,758 行降至 3,556 行，module CSS 增至 1,993 行；旧 dashboard/project-detail 类在 `src/` 零引用。tsc、eslint、vite 与 108 个前端用例通过；两屏仍建议人工实机复核。
- **Knowledge 页与 Heatmap 组件迁入 CSS Modules（P35 第五块）**：新增 `Knowledge.module.css`、`Heatmap.module.css`；`knowledge.css` 320 → 29 行，`heatmap.css` 271 行整文件删除——它在 `src/` 下的每一个类都只被 `Heatmap.tsx` 引用。全局 CSS 3,556 → 2,994 行，module 1,993 → 2,479 行。三处只有实际动手才会撞上的东西：① **跨组件类在 module 里必须 `:global()` 包裹**——`.knowledge-filters .filter-btn.pinned-active`、`.knowledge-toolbar .page-head-actions` 及其 `.btn` 这三处若照搬选择器，`.filter-btn`/`.page-head-actions`/`.btn` 会被一起 hash 掉，筛选按钮的置顶态与工具栏动作排布会静默失效；② **动态类 `level-${n}` 改为 `LEVEL_CLASS` 查表**——热力图五级色阶原先靠全局 `.level-N`，hash 之后模板串拼不出类名，顺带把 `.level-N` 这种泛用名锁进组件作用域；③ **`heatmap.css` 的「Full variant」整段是死代码**——`.heatmap-container`/`-content`/`-grid-wrapper`/`-day-labels`/`-grid`/`-week`/`-cell`（含 `.level-*`、暗色变体与 `[role=button]`）/`-legend` 共 23 条规则在 `src/` 下零引用，连同被 `.heatmap-simple .heatmap-loading` 在四个属性上完全覆盖的基座 `.heatmap-loading` 一并删除，删而不迁。边界判据写在两个 module 文件头：`.pin-btn`（与 NoteSection 共享）留全局，`.heatmap-empty-state` 定义在 `project-detail.css`、本次不动。验证仍是机械对账：把 module 选择器反解回原类名后与原文件逐块比对，选择器 + 声明体全等（31/31 与 31/31，丢失 0 / 新增 0），`src/` 旧类残留 0，tsc / eslint / vite build 与用例全过。**顺带发现两个空壳类**：Knowledge 页外壳的 `knowledge` 与 `knowledge-section` 在 `src/` 下检索不到任何样式定义（markup 在用、CSS 从来无定义），本次保留原样，待按 `plugin-ok` 先例单独定性。**视觉面仍需人工实机看一遍**：CSS 改名 CI 完全无感
- **审核页 UI 重做——对齐设计系统（用户可见）**：上一轮可读性修复只解决了"读得懂"，没解决"看着不糙"。本轮按知识页已验证的视觉语言整体重排：顶部改知识页同款 **sticky 工具栏**（同一套负 margin 算术：`top:-32px` + 对称负 margin/padding 补偿，静态布局零变化；断点跟 main.css 的 768px 缩到 -20px；不透明背景防内容透出），项目下拉自绘而非复用 `.form-select`——那个控件是 34px 文本输入高度，而工具栏按钮排是 28px 的 `.btn-sm`，module 里做高度覆盖赢不赢要看 import 顺序而非意图；可见大标题退役（导航已命名本 tab，h1 留 `visually-hidden` 给读屏，与知识页同一判例），标题的语义职责压缩成一行 lead——"拒绝=替换为错误页而非删除、链接不断"这条规则从界面猜不出来，值得占一行。job 行升级为卡片：soft 状态徽章（running 带呼吸圆点，全页唯一动效，`prefers-reduced-motion` 下关闭）、进度独立成行、计数改 chips、补上此前没显示的项目名。审核卡重排信息层级：kind 徽章按类别着色（entity/concept/synthesis 三色，模型自造的未知 kind 退中性灰——着色是扫读辅助不是语义保证）、标题 15px 提为主角、slug 降为 mono 次行、来源笔记与出入链全部 chips 化（链改显 title 而非 slug）、动作按钮次左主右、≤768px 拉满整行宽度。后台任务与错误页两个区块**空时整体不渲染**——审核者每次进页不必滚过两个空盒子。i18n +6 键（空态标题/正文、笔记引用、kind 双语名）−3 死键（`emptyJobs`/`emptyRejected` 随区块隐藏退役，`noProjects` 早已无人引用）
- **审核 tab 的可读性修复（用户可见）**：待审/错误页卡片此前用裸 `<pre>` 渲染页面正文——编译产物本身是 markdown（标题、列表、`[[wikilink]]`），原文裸奔让每张卡片读起来像内存转储，且卡片头不显示页面标题（只有机器 slug）。现在正文走与知识库同一条 `renderMarkdown` 管线（strip frontmatter + DOMPurify 清洗，与 NoteSection 同源），标题优先渲染、slug 退为次要元数据；错误页占位文本刻意保持纯文本（它是错误说明不是文档）。补上后端早已返回却从未渲染的 `in_links`（反向链接——审一条模型写的页时最需要的是"谁引用了它"），以及此前漏掉的 `pages_updated`/`links_created` 计数；job 行的进度文本在 `notes_total` 与 `requested_notes` 都是 0 时（worker 被杀、快照缺失）不再渲染出可疑的 "0 / 0"，只显示状态徽章。新增 i18n key 双语对齐（`pagesUpdated`/`linksCreated`）
- **首页（知识库）工具栏冻结**：搜索框 + 新建/导入按钮 sticky 化，长列表滚动时始终可见。复用 `.settings-tabs` 验证过的负 margin 算术（sticky 的 `top` 以 scrollport 为原点，而工具栏的静态位置在 `.main-content` 32px padding 之下，`top: 0` 会让内容从 32px 空带里穿过去；`top: -32px` + 对称负 margin + padding 补偿使静态布局零变化）。全出血边距让分隔线贯穿整个内容区，不透明背景防止滚动内容透出。断点刻意跟随 main.css 的 768px（padding 缩窄点）而非本文件其他规则的 640px——sticky 偏移必须跟着 padding 变，否则钉住的栏会偏离边缘；settings-tabs 存在同样的 640/768 错位，本轮不动它
- **README Go 版本徽章 1.25+ → 1.26+（中英双语）**：go.mod 已声明 `go 1.26.0`，徽章滞后一版。`.gitignore` 补 `.w3-rehearsal/`（W3 真库演练工作区，纯本地产物）

- **长时批任务改为异步 job（ADR-0016，schema v19）**：新增 `compile_jobs` 表与四个绑定（`StartCompileJob` / `GetCompileJob` / `ListCompileJobs` / `CancelCompileJob`），**提交立刻返回 job id，不再等模型**。依据是实测数字：本地 27B 模型单条笔记 173 秒——同步调用即使命题不被超时掐断也不合理（界面必须挂等、看不见进度、切走就丢结果）。编译内核**零改动被复用**（job 循环直接调既有的单条编译入口），同步入口保留且与 job **共用同一个笔记选择函数**，顺带修掉一个方向性错误：原先按 `ListNotes` 顺序取笔记，而它是 `created_at ASC`，等于"编译本项目"会先烧掉你最旧的笔记而不是最近的。取消语义定为"停在下一条"而非抢占在途请求（抢占只省下一次已花掉的请求，代价是整条调用链多一套取消语义）；重启时 stuck 的 `running` 显式判 `failed` 且不重新认领（没有任何地方记录它断在哪条笔记上，假装续跑就是编造进度）；另补一条 ADR 没写、不写就会咬人的规则——**迟到的进度写不能把已取消的 job 复活**。未接入界面：编译目前只有绑定与 `/api/rpc` 入口，交互问答仍是同步 90 秒（异步化今天只覆盖批量编译，别把两者混谈）。回归 7 例，含"提交期间模型调用计数必须为 0"的反证：`wiki_compile` 打开后，可对单条笔记或某项目前 N 条跑一次编译——模型读笔记、提出"新建哪些页 / 加哪些链接 / 声明哪些来源"，系统把产物写成 `status=pending`、`source=wiki-compile` 的页面。**这一步是 RepoNest 第一次让模型往知识库写东西**，所以可撤销性是设计前提而不是事后补救：编译器**只能新建、永不修改已批准页**（撞上同名 slug 就降级成一条 `compile-revision` 待办交人判断），操作类型里根本没有"改页/删页/批准"，拒绝一个 pending 页等于删一行并级联带走它的链接与来源挂接。**"批准"是只有人能做的状态迁移**：v18 加的 `status`/`source` 两列与检索侧的 `p.status='approved'` 过滤是这条规则的物理保证——未审核的模型输出既进不了知识库搜索，也进不了 AskAI 的证据块。其余落地细节：产出行强制挂来源笔记（不给模型选，W4 的 `no-source` 检查因此能反向给生成层当守卫）；预算硬顶（20 条笔记 / 每条 6 页 / 每页 1500 字节 / 单次 60 页）触顶即停并在报告里写明停在哪，因为模型不知道什么时候该停；重跑同一条笔记是刷新自己上次的 pending 产物而非堆副本；`add_link` 要求两端至少一端是本轮产物，防止模型把已有批准页互相连成没人审过的图；不接任何定时器、不进 `auto_import`、不做 MCP 工具（agent 可自写待审知识会绕过整个在环设计）。审核面提供 `ListPendingWikiPages`（带来源与出入链）/`ApproveWikiPage`/`RejectWikiPage`（拒绝不能删已批准页）。导出按 `status` 标注 pending 供在 Obsidian 里审，而 `index.md` 只列 approved。`status` 默认取 `approved` 是刻意的：现存页面全部由人发起，默认 pending 会在升级时把用户既有知识整体撤出检索
- **wiki 结构层落地（M6-W1，schema v16）——纯结构、零 LLM 调用、暂无界面**：ADR-0014 决策 1 的地基。新增四张**派生表**：`wiki_pages`（页面，五类 kind：`entity`/`concept`/`source`/`synthesis`/`query`，由 SQLite `CHECK` 与 Go 枚举双重守）、`page_links`（有向边 + `from`/`to` 双向索引，反向可查是这个层存在的全部理由：裸 project_notes 里笔记之间零引用，「一次 ingest 触碰 10-15 页」在数据模型上根本无法表达）、`note_pages`（页面↔来源笔记，供日后"改这条笔记会影响哪些页"）、`note_repositories`（关闭另一个历史缺口：笔记能指向项目，却从来不能指向项目内的某个仓库，而这整个产品的对象就是仓库）。三条设计判断都写进了代码注释并各有理由：`slug` **全局唯一**而非按项目唯一（按项目会因可空 `project_id` 在 SQLite 唯一索引里留下 NULL 洞，两条全局同名页让 wikilink 解析歧义）；kind **从 ADR 的四类改成五类**（ADR 决策 3 承诺的 query 闭环需要一个归宿，Karpathy 模式与参照实现都确有 `queries/` 这一类，缺了它答案只能写回普通笔记）；`CHECK(from<>to)` + FK CASCADE 让自环与悬空边在库层就不可能出现，否则 W4 的 lint 检查建立在脏数据上。**可逆性是被测试逼出来的**：v16 带版本戳只跑一次，若只在迁移里建表，一次 `DropWikiSchema` 之后重启就得到「有版本号、没表」的死库；现在 `InitDB` 每次开库无条件 `EnsureWikiSchema` 自愈（沿用 `EnsureFTSIndex` 的同一契约），可逆性测试则用 `sqlite_master` 对象快照证明 drop 前后逐字节相同——并先断言"确实加了东西"，否则那次比较是空的。回归 9 例（`internal/db/wiki_test.go`）。**本步刻意不含 service 门面与界面绑定**：W1 的第一个消费者是 W2 的 AskAI 取证改造，此刻加无人调用的绑定正违反本仓 P32 的「无死绑定」审计
- **第 6 个内置知识源：Cursor 会话（`cursor`，opt-in 手动、best-effort）**：从 Cursor 的 `globalStorage/state.vscdb` 读会话，落 `kind=log` 笔记。判据全部来自对本机真库的实测而非猜测——bubble 键形如 `bubbleId:<composer>:<uuid>`、`json_valid=1` 明文、`type` 是数值 sender 枚举（真库 71 条里只有 1/2）、`createdAt` 是 ISO-8601 串、`composerHeaders.workspaceId` 恰为 `workspaceStorage/<32hex>` 目录名，故归属链是 `workspaceId → workspace.json 的 folder → 项目匹配`。每场会话只取**首条提问 + 末条回复**两个截断摘要，不复制整段转录；查不到项目就跳过不猜；全程 `mode=ro` 打开别人的数据库（WAL sidecar 不可读时退回 `immutable=1`）。**"best-effort" 是硬约束不是免责声明**：表或列不是它认识的形状时整个源**报错**，因为"导入 0 条"必须能和"这台机器没有 Cursor 会话"区分开；单条 bubble 的 JSON 坏掉则只丢那一条（真库里确实存在这样一条）。默认仍不随启动自动导入跑，需在 设置 → 插件 手动触发
- **笔记增删改即时进向量索引，不必再跑全量重建**（schema v14 + 后台 `embed-drainer`）：ADR-0012 把向量索引和查询侧融合都做完了，唯独索引侧是全有全无——开启语义检索后新建/修改的笔记要等下一次「重建索引」才可见，而那是 O(全部笔记) 的端点遍历。顺带说明今天的实际形态：语义检索仍只有配置键、设置页刻意没有开关与按钮（等 A/B 评测门通过，见 TODO M3），所以这条改动目前服务的是手动配置的那部分人，但它也是那个开关上线前必须存在的一半——否则 UI 一开，用户会立刻撞上"我写的笔记搜不到"。现在 `note_embed_dirty` 队列由 `project_notes` 上的三个触发器维护，drainer 每 5 秒批量排空：note 还在就 embed + Upsert，note 已删就 `Store.Delete`（`Store` 接口为此新增 `Delete`，local / Qdrant / Weaviate 各一实现——**删除不处理的话，用户删掉的文本会永久留在检索结果里**）。
  - 为什么是触发器而不是服务层 hook：`internal/db` 才是所有笔记写入者收敛的地方，5 个 agent 记忆 importer 经插件运行时直写 db、**完全不经 `service.*Note`**。服务层 hook 会精准漏掉最大的一路写入，而这正是 FTS5 当年用同样三个触发器解决过的同一个问题。测试也刻意走直写路径来证明这点。
  - 三个刻意的保守：整条路径仍在 `semantic_search` 之后（那是用户同意把笔记文本发往端点的显式授权，不另开第二个开关，否则会出现"主开关开着、索引却静默不更新"）；维度未知或索引尚未建立时直接跳过且**不消费队列**（猜维度意味着用错的 width 调 `Ensure`，那会 drop 并重建整个索引）；端点挂掉时队列原地保留、下一 tick 重试，所以离线合盖不会丢写。
  - 只改标题/内容才排队：置顶、拖排序、改标签、移动项目都不改变送给端点的文本，不该花钱。这里踩到一个 SQLite 语义坑——`AFTER UPDATE OF title, content` 是**语法判定**，只要 SET 里提到 title 就触发，而 `UpdateNoteMeta` 每次保存都会重发 title，等于每次改标签白付一次 embedding；已改为 `WHEN OLD.title IS NOT NEW.title OR OLD.content IS NOT NEW.content` 的值判定（`IS NOT` 同时正确处理 NULL）
  - `RebuildEmbeddings` 现在会持久化它学到的 `embedding_dim`（增量工作的解锁条件），并且**只有完整跑完才清空队列**——中途 bail 时那些没 embed 的笔记全靠这条队列兜着。顺带暴露一个既有契约：rebuild 中途失败时返回的是计数而不是 error，所以真正防止半截重建丢笔记的是这个持久队列，不是返回值
  - 回归 11 例：`internal/db/embed_dirty_test.go`（触发器只在内容写时排队、元数据写不排队、live/gone 分流、limit、DDL 重放安全）+ `internal/service/embed_drain_test.go`（importer 直写端到端可召回、删除后不再召回且零 embedding 请求、开关关时零外发、两道门前置条件、端点挂掉与 store 拒删都不消费队列、半截重建不清空队列）。`internal/integrity` 的 `ExpectedSchemaVersion` 同步升到 14——该常量与迁移列表漂移会让自检说谎，仓库里的 `TestExpectedSchemaVersionMatchesMigrations` 正是为此而设，本次就是它抓出来的

### 变更

- **⚠️ 审核流的「拒绝」从删除改为错误页（用户可见的行为变更）**：`RejectWikiPage` 过去删除待审行，会级联带走它的所有链接，于是每个引用过它的页面都静默多出死链。现在改为 `status=rejected` 并把正文替换成错误占位，**保留节点与入链**——标记不合格不该破坏图谱，删除是可选的后续动作。配套 `ListRejectedWikiPages` 让错误页可见可清理，`DeleteCompiledPage` 提供只对未发布页开放的彻底删除逃生阀（防错误页无限堆积，且明确拒绝删除已批准页）。无新增迁移：`status` 的 CHECK 早已允许 `rejected`，此前只是没人写入。
- **新增独立「审核」tab（`/review`）**：待审页面列表（含来源笔记与出入链）可批准 / 标记不合格 / 彻底删除；编译任务列表显示进度条与 `notes_done/notes_total`、产出计数、停止原因与错误，运行中每 5 秒轮询、无活动即停；「开始编译」提交 job（未设 `wiki_compile=1` 时按钮可用但会报错，门槛本身仍未前置到设置页）。前端 API 层新增 9 个端点与契约测试（绑定返回 null 时一律降级为空数组，避免列表页崩溃）。
### 修复

- **两处超时把长任务判成失败，真库演练才暴露**：① 批量 LLM 调用（摄入编译、lint 的模型侧）此前与交互问答共用 90 秒客户端超时，而本地 27B 模型单条笔记就要 >90s，结果编译在任何真实配置下都不可能成功——批量路径改用 10 分钟天花板（`DefaultBatchChatTimeout`），交互问答仍为 90 秒（人不该对着输入框挂十分钟）；② 无头服务 `WriteTimeout=60s` 会在 handler 早已提交数据之后掐断响应，演练中 curl 收到 `http=000` 而库里确实多出了待审页——一次成功操作被报成失败，是能让用户不再信任工具的那类 bug。现在写超时对齐到批量天花板之上，并在启动期用断言守住该不等式（不靠注释，注释会腐烂）。异步化是正式解法（见新增 ADR-0016），本次是止血。

- **记忆分层管道落地（M6-W5，L3→L0）**：`LayeredProjectContext` 按稳定度装配上下文——L3 跨项目画像（不属任何项目的**已批准**页面）→ L2 项目场景（复用 `aiProjectBaseContext`，不另造第二套"这个项目是什么"）→ L1 整理型笔记 → L0 会话逐字稿；空 query 时**完全不触发检索**（只要定向就不要为一句话跑一遍搜索），L0 需显式 `includeTranscripts` 才纳入。分类是按来源的启发式（codex/opencode/cursor 与 `kind=log` 归 L0），代码与文档都写明**来源不等于可信度**，免得后来人把它当质量分级。预算**逐层独立**，因为一个共享上限会让话多的 L0 挤掉给人指路的 L3/L2。AskAI 的上下文前缀改走同一装配器，两条路径不再各有一套"项目上下文"。**本步交付的是管道而非蒸馏**：不自动生成任何页面——那是 W3 编译器的活且产物必须待审，否则就成了 ADR-0015 拒绝的无闸门写入。测试抓出两个同构 bug 并升格为注释里的通用规则：预算小于一行时必须裁剪而非**消音整层**（与 W2 证据层那次 40 字预算同一教训），以及块内 `items` 必须与实际渲染同源（原先会"报 3 条给 0 字"）。回归 7 组

- **M6 已落地能力补齐双语功能文档**（`knowledge` / `ai-integration` / `settings` 各中英两版）：新增「知识页面与审核队列」一节讲清五类页面、**pending 与 approved 的可见性差别**（未批准的页面搜不到、但导出/审核/lint 看得见——这条最容易被当成 bug，所以写在用户会先看到的地方）、谁有权写页面、lint 五查与其产出、导出到 Obsidian 的两个刻意性质（只出不进、不删除）。AI 问答一节按 W2 的实际行为**重写**：从「静态打包 10 条笔记」改为「两路召回 + 三重预算 + 可引用编号 + 检索为空时回退旧路径」，并保留仍成立的能力边界（非流式、无工具调用、页面尚无自己的向量索引、**真实库上的收益尚未测量**）；配置表补 `wiki_lint_llm` 与 `wiki_compile` 并各自标注会把什么发出去（编译是**唯一让模型往知识库里写东西**的开关）；可执行清单补 `cmd/wiki-export`。设置页新增「本页还没有的开关」一条，把「待审核页面还没有审核界面」与两个门刻意不放 UI 开关的理由写在用户会去找它们的地方。这一轮是**文档与代码同批改**：上一版 `ai-integration` 里「无引用、静态打包」那段已被我自己的 W2 改动作废——欠债正是那样产生的

- **知识检索与 AskAI 证据现在只读已批准页面**（ADR-0015 决策 1 的前半）：`wiki_pages` 新增 `status`/`source` 两列，两处 FTS 检索路径加 `p.status='approved'` 过滤。清点路径（导出旁路、lint、审核队列）刻意**保留全部状态**——实现时发现原方案要求 `ListWikiPages` 一并过滤会同时废掉 W1b 的 pending 导出与 W4 对 pending 的检查，那两处的职责恰恰是"把人没审的东西摆出来看"，故收窄为"检索过滤、清点全量"，并已在 ADR-0015 内记为对原计划的偏离
- **wiki lint 五查落地（M6-W4）——防腐层，且"只建议不改页"是结构事实不是口号**：`wiki_lint.go` + `App.RunWikiLint` + 6 小时定时。五查按是否需要模型分两层：孤页、缺交叉引用、数据缺口（thin 页 / 无来源笔记 / 正文 `[[链接]]` 指向不存在页 / 有叶子页却零综述）三查纯 SQL，无成本无外发；矛盾与过时声明两查走模型且单独由 `wiki_lint_llm` 显式开启（默认关，`llm_ran=false` 时 `llm_note` 会说明为什么没跑——"没查出问题"和"功能没开"不能长得一样）。所有 finding 一律只落 `project_todos`，代码里不存在让 lint 改页面的路径，并有测试逐字段钉住 content/title/kind/updated_at 不变：让 LLM 去修 LLM 写错的东西是更深一层的腐蚀，而一条错建议只值用户看一眼复选框。采信规则同样从严——模型给的 `page_id` 必须库里查得到、未知 `type` 不认、回复解析失败则一条不提；全局页（无项目可挂）如实计入 `todos_unfiled` 而不是随便找个家。去重只对未完成的同名 todo，勾掉后再漂移属新事实。定时首跳推迟一个周期，避免"打开应用就写了我的待办"。顺带给 `WikiPage` 加了 `UpdatedAt`（过时判定必须知道谁更新得更晚）。回归 7 例

- **wiki 单向导出旁路落地（M6-W1b）**：`reponest-wiki-export`（`cmd/wiki-export`，独立二进制——根 `reponest` 是 Wails 应用、没有子命令派发，ADR 原文写的 `reponest wiki export` 是个不存在的命令，已更正）把派生层渲染成 `wiki/<按 kind 分目录>/<slug>.md` + YAML frontmatter + `[[wikilink]]` 出链与反向链接 + `wiki/index.md` 目录页 + `EXPORT-MANIFEST.json`，让 Obsidian 的 graph view 能回答数据库答不了的问题：**这套页面分类是不是知识本来的形状**——这正是 ADR-0014 决策 2 用"只读导出"换生态、拒绝双向同步之后必须交付的一半。四条保守取舍：① 目标目录含外来文件即拒写（自己的 `wiki/` 子树允许幂等重写），因为只读契约不该变成"顺手覆盖用户在 Obsidian 里批注过的文件"；② **删掉页留下的陈旧文件只列出不删除**——删除等于宣称这棵树归导出所有，也正是它分不清"我们的残留"和"用户自己的笔记"；③ slug 源自用户标题而这里是文件系统写入方，故一律拒绝路径穿越，不指望归一化函数替它守门；④ manifest 最后写且**不含绝对路径**：中途崩溃会留下与磁盘不符的清单（与打包清单 sha256 门同一思路），而换目录导出必须逐字节相同才有意义。顺带产出 W4 的输入：正文里手写的 `[[链接]]` 指向不存在页时计入 `dangling_links`——页→页的边有 FK CASCADE 兜着不可能悬空，正文链接是唯一会烂的地方。回归 6 例，并用真二进制在临时库上端到端跑过（不触碰用户的 `dashboard.db`）

- **AI 问答面板接入取证链路（M6-W2 前端）**：悬浮球的 AI tab 不再走"无检索的直问"，改用 `AskAIWithEvidence`，并在回复下方列出本次依据（`[P1] entity · Payment Gateway retry policy` 一行一条），预算挤掉的条数与"什么都没检索到"都**显式说出来**——零证据时答案照样给，但界面标黄提示"仅用项目上下文作答"，不留一个空列表让人猜功能坏没坏。新增「存为页面」按钮：回档成 `query` 页并链到依据。**回档链接只取答案正文里真正出现的 `[P#]/[N#]`**（正则抽取后与该次检索的 ref 集求交），模型编一个 `[P9]` 不会凭空生出链接；一个字都没引时才退回整套证据并在计数上如实显示。跨项目复用面板实例的防护从"effect 里清 state"改成"给证据打上所属项目"——本仓 react-hooks 规则禁前者，且打标之后陈旧答案的 `P1` 结构上就不可能解析到新项目的页。顺手用同一手法清掉 `ProjectCommitLog` 里两处同源 lint 违规（副作用是慢响应再也画不到新视图上）。验证：前端 13 文件 **96** 用例（+3 例路由契约，含"绑定返回 null 时证据必须降级为空集"这条防空白面板的断言）、`tsc` 零错、`eslint src/` 零错、`vite build` 通过。界面排版仍需一次肉眼验收。剩余：流式（传输层，与答案质量无关）、真实标注 query 集上的 delta

- **W2 取证层落地（核心），并更正一条被写进 ADR 的事实错误**：新增 `service.GatherEvidence`——页面 FTS5（schema v17，与 notes/todos 同构的 external-content + trigram + 三触发器，故任何写入路径建的页都立即可检）与既有笔记检索两路召回、按位置交织，在「条数 / 字符 / 超时」三重预算下渲染成带 `[P#]/[N#]` 引用的证据块；`FileAnswerAsPage` 把好答案回档成 `query` 页并链到被引页、挂上被引笔记，闭合 ADR-0014 决策 3 的 query 环。`AskAIWithEvidence` / `FileAnswerAsPage` 已加为桌面绑定。两个实现期抓到的真 bug：① `added = added || tryAdd(...)` 的 **Go 短路求值**——页分支置真后笔记分支根本不执行，等于**只要有任何页命中，笔记就静默不进证据**，而这正是常态；② 回档时把引用 `P1` 当作「页 id 为 1」解析，而 `Render` 的 ref 是**位置编号**，于是答案会链到一张无关页——改为引用只能由生成它的那次检索解析，编造的 ref 解析为空。门禁跑在 fixture 上并用现成 `abeval` 度量（legacy recall@8 = 0.000、evidence = 1.000），但**这组数字只证明机制正确、不证明真实收益**：语料是刻意构造的（答案笔记全在 legacy 窗口之外），真实标注 query 集仍缺，判据的另一半照旧开着；流式属传输层、与答案质量无关，单独一步。另更正一处被我写进 ADR-0014 与 `ai-integration` 双语页的断言：pre-W2 上下文取的那 10 条笔记**不是最近写的，而是最早写的**——`db.ListNotes` 排序是 `pinned DESC, sort_order ASC, created_at ASC, id ASC`，`aiProjectContext` 直接取前 10。这意味着项目笔记越多、新沉淀越不可能进入 prompt，比"按最近取"更反直觉，也让 W2 的必要性比原稿说得更强

- **英文站知识源页整页重对齐**（`docs/en/plugins/overview.md`，顺带修中文同名页两处）：双语漂移的实际后果不是"少几行"，而是**讲出一个不存在的架构**——英文页开篇写着「通过 yaegi 解释执行的 Go 脚本向知识库幂等导入文档」，而 6 个内置源全是 Go 原生 `plugin.KnowledgeImporter`、根本不经过 yaegi（yaegi 只承载用户自己写的脚本插件）；mermaid 把入口画成「5 个内置源 / yaegi 解释执行」；源表只有 `claude` 一行，codex / opencode / openclaw / hermes / cursor 五个源在英文站等于不存在，并整页缺「项目归属三级规则」「openclaw/hermes 未配置即静默全 skip」「导入统计语义」「headless 模式」四节。现按中文页逐节补齐。中文页同样两句已修（yaegi 开篇、mermaid 源数 5→6）

- **P35 第二块：扫描根列表与知识源列表各归其主**（新增 `ScanRootsTab.module.css` + `PluginsTab.module.css`，`settings.css` 164 → 47 行，至此该文件只剩多个子页共用的基座）。三处只有实际动手才会撞上的东西：① **`s` 遮蔽陷阱**——本仓 CSS Modules 惯例导入名是 `s`，而 PluginsTab 里写着 `sources.map((s) => ...)`，照抄惯例会让回调内的 `s` 变成模块对象、`s.name` 当场失效；该文件的导入名因此改用 `css`，并在文件里写明原因，免得下一个人"顺手改回一致"。② **`.root-item.empty` 的 `:global()` 诱惑**——`empty` 看着像全站共用类，但这个修饰符只有扫描根 tab 会往行上加，所以收成本地 `rootItemEmpty`，避开在 `:hover:not()` 与 `:has()` 里嵌 `:global()` 这种 Vite/postcss-modules 边角行为。③ 上轮留的 `plugin-ok` 定性完成：**markup 在用、CSS 从来无定义**，随迁移一并去掉这个空壳类（若日后真要给 ok/err 上状态色，那是一次有意的改动，不是留个不生效的类）。脚本另撞过一次：按「最后一行以 import 开头」定位插入点，结果那条是跨行 `import {`，插入掉进括号里造成语法错——`tsc` 当场抓到，教训是插入点要落在语句结束处
- **P35 第一块落地：外观 tab 的 CSS 归组件所有**（新增 `AppearanceTab.module.css`，`settings.css` 305 → 160 行）。这一步先推翻了一个致败前提：**「按文件整收」不可行**——`settings.css` 里 `.settings-section` / `.section-desc` / `.form-hint` 被 7 个组件共享、`.empty` 全站 15 处在用（这里只是 `.root-item.empty` 复合修饰），整文件搬进某个 module 会把共享规则变成某组件的私有类。真正的迁移单位是**组件独占簇**，所以本轮只搬 `theme-*` 六类（AppearanceTab 独占），共享基座原样留全局。过程中挖出两处"只按文件搬必漏"的问题：① `layouts/main.css` 有条 `.settings-section > .theme-options { max-width: 560px }` 从布局层反向伸进组件类，而组件自己的规则里**已经写了同一个 560px**——冗余耦合，已删并写明理由；② `.plugin-item.plugin-err` 与 `.plugin-error` 两条规则零引用，而 TSX 在用的 `plugin-ok` 反过来在 CSS 里没有定义，ok/err 状态样式两边都指向不存在的东西：死规则已删，`plugin-ok` 留待 PluginsTab 那轮定性。验证靠机械对账而非眼睛：规则体逐条比对（丢失 0 / 新增 0）、`src/` 全站旧类残留 0、tsc 零错、`vite build` 通过、93 用例通过、新产物 `settings-*.css` 里旧类 0 次而新类 1 次（`web/dist` 会累积历史构建产物，需用 mtime 区分，否则会把旧文件误判成漏改）。**视觉面仍需人工实机看一次设置页**：CSS 改名这类重构 CI 完全无感
- **⚠️ 破坏性变更：启动自动导入改为默认关，旧库会被一次性归零**（schema v15）。读别人工具的记忆文件并把内容写进本库，此前不需要任何人点头。根因比"默认值选错"更深：`db.GetConfig` 把 `sql.ErrNoRows` 映射成 `("", nil)`，而启动判据是 `err == nil && v != "0"`——**空串不等于 `"0"`，所以"从未设置过"这个状态本身就被判成开**。换句话说，一个被文档描述为"用户可以关掉"的开关，对那些从来没碰过它的人其实是常开的，而这正是绝大多数人。三处配套修：① 判据翻成 `== "1"` 并抽成 `service.autoImportEnabled()`，让缺失行 / 空串 / 乱值一律为关，**安全态成为默认态**；② `insertDefaults` 的种子从 `"1"` 改 `"0"`（保留这一行只为设置页显示真值，不再承载隐私语义）；③ 迁移 `UPDATE app_config SET value='0' WHERE key='auto_import' AND value <> '0'`——只改写曾经为开的行，显式 `"0"` 不动，幂等可重放，不会凭空造出一个"开"。代价如实说明：老用户会看到自动导入停止，需在 **设置 → 插件** 显式重新开启一次；之所以不做"只改新库默认"，是因为旧版本无法区分"用户主动开启"与"程序种下的默认值"，任何试图自动保留的做法都等于继续替用户做这个决定。`ExpectedSchemaVersion` 同步 14→15。文档四处已改（`features/ai-integration` 与 `features/settings`、`plugins/overview` 中英各一）。回归 9 例：`internal/db/migrate_v15_test.go` 3 例 + `internal/service/auto_import_test.go` 6 状态表驱动（含"行被删除"这一原 bug 用例）
- **功能文档补齐并修掉三处过期断言**（`docs/features/ai-integration.md` + `knowledge.md`，中英各一版）：新增语义检索（RRF 两级融合、默认关、失败退回词法、首次全量重建 + 之后触发器队列增量、`vector-init` 引导、`abeval` 评测门在前开关在后）、AI 问答的真实现状、无头 HTTP 与可执行清单（含「哪些二进制根本没随版本发布」）、OMP 导出、`claude_session_capture`。三处修错：① 价值定位段原断言「RepoNest 不调用任何大语言模型——代码里没有任何 API key 配置、没有 endpoint 设置」，但配置白名单实测 17 个键、v1.15.0 的 AI 问答就在打 `/chat/completions`——改为「不托管模型 / 不代持密钥 / 可选能力默认关」，并说清这个区别决定隐私责任落点；② 配置表原文「白名单只含 4 个配置键，没有任何一项涉及 LLM」同样过期，拆成核心与 AI 侧两张表、逐键标注「开了会把什么发出去」；③ `cmd/server/main.go` 的 Usage 注释写着 `reponest server`，而根 `reponest` 是 Wails 应用、没有任何子命令派发，改为 `go build -o reponest-server ./cmd/server`。另修一处**双语漂移**：英文站导入章节还停在「一键导 Claude 记忆」，中文站早已是 5 个源的表格——4 个 importer 在英文站是隐形的，现已补齐（含三条易踩规则）。同时核验「短 CJK 查询自动降级 LIKE」这句旧文档仍然成立，未误删
- **语义检索不再为每次查询探测远程向量库**（`service.vectorStore()` 改为 memo）：原实现在每条查询路径上重读 4 次 `db.GetConfig`，而远程后端还要在 `vectordb.Open` 的工厂里做一次 HTTP 可达性探测——配了 Qdrant / Weaviate 的用户**每次语义检索都白付一个网络往返**，「向量比词法还慢」成为默认。失效点两处：`UpdateConfig` 命中 `vector_store*` 前缀即丢缓存（否则在设置页改端点要重启才生效——缓存会把配置写入变成静默无效输入），以及任何一次 store 报错即丢（远程中途挂掉时下一轮重新解析并退回 local；这件事在加缓存之前是「意外自愈」的，缓存后必须显式做，否则一个死句柄会被永久抱着）。顺带补上向量检索失败此前**完全静默**的日志：`fuseSemantic` 只在 embed 失败时打日志，store 失败直接返回词法结果，用户开着语义检索却因远程库挂掉而长期只拿到词法命中，界面上看不出任何区别。回归 `internal/service/vector_store_cache_test.go` 4 例（缓存生效 / 四个 `vector_store*` 键各自失效且 `embedding_model` 不误伤 / 死 store 在 fuse 与 rebuild 两条路径都被丢弃且错误如实上抛）
- **TODO 收口 + 一处包注释修正（不改任何行为）**：核对 M3 时发现 `vectordb.Register/Kinds` 可插拔 registry **早已落地**——`internal/search/vectordb/store.go` 有 `registry` map + `Register` + `Kinds`（local 恒隐式）+ `Open` 的三重退回（未配 / 未知 kind / 远程工厂报错或探测不通一律 local），Weaviate 已在表内，调用方零改动。该待办属重复挂账，改写为已落地结论并把未接后端合并进相邻等待项，不重复计数。「分发评估门」一条则复述了 [ADR-0009](docs/adr/0009-ide-presence.md) 决策 5 里既有的正式条款（约束不是待办，挂在清单上永远不会「完成」），不再占未勾位。附带修正 `internal/search/vectordb/vectordb.go` 包注释：原文写「Two implementations: local / qdrant」，漏了已实现的 Weaviate，现为三个后端 + registry 指路
- **P35 的推进前提被实测推翻（记录，未改代码）**：非 module 全局 CSS 现有 5,101 行，而符合「全局只保留 reset / design tokens / 跨组件基础样式」这一目标的仅 355 行（约 7%），其余 4,746 行按归属仍可下沉（`features/` 3,071、`components/` 1,276、`layouts/` 399）。两轮试点共迁出 214 行，同期全局 CSS 却从 4,055 涨到 5,101（+1,046）——「每轮 sprint 迁 1-2 个组件」的速率追不上新增，这条路不会自然收敛。两条待办已改写为带判据的版本：先定义 `components/buttons|inputs|cards|tabs` 那层算不算基础样式，再在「按文件冻结 + 新组件一律 module.css」与「集中收 `project-detail`/`dashboard`/`settings` 三大文件」之间选路

## [1.15.1] - 2026-10-07

提交 tab 的一次返工：中间两版（指向 forge 的外链 → 仓库行内就地展开）都不好用，最终收敛成「趋势卡 + 一条跨仓库提交时间线」。本版验证：Go 23 包测试通过（含 `-race`）、前端 13 文件 93 用例、`tsc && vite build` 通过。

### 变更

- **提交 tab 收敛为两张卡、两个问题**：趋势卡回答「多少、什么时候」，提交记录卡回答「具体发生了什么」。
  - **热力图卡整个移除**：它与 dashboard 的热力图是同一份数据的第二次渲染，还把两张卡的正文挤到首屏之下。活动指标（总提交 / 近30天速率 / 活跃天数 / 活跃月份）折进趋势卡头部，`ScopeToggle` 留在原位。上一版为「上下两带分栏线对齐」做的容器改造随这次删卡一并作废——分栏线错位的诉求不复存在。
  - **提交记录改为一条按时间合并的时间线**，上方一排仓库 chip 做筛选（「全部」即合并视图）；合并视图里每行的仓库名本身可点，点一下收窄到该仓库，不必回到筛选行。单仓库项目没有可筛选项，直接出正文而不是一排只有一个 chip 的筛选行。删除 `ProjectCommitsSection` / `RepoBreakdown` / `RepoCommitList`（297 行）及其样式（`project-detail.css` -329 行），新增 `ProjectCommitLog`。
- **提交记录改由后端按需读 git，两条新绑定同批落地**：`daily_stats` 只有「谁在哪天改了多少行」——没有 message、没有 SHA，库里拼不出提交列表，而指向 forge 的外链对没有 remote 的内网仓库就是死链。
  - `GetRepoCommits(repoID, limit)`：单个仓库的日志。`RecentCommit` 补 `Hash` 字段（cherry-pick、forge 搜索都要用），前端 SHA 点击即复制。入参用 repo id 而非 path，不能被指向任意目录。
  - `GetProjectCommits(projectID, limit)`：项目内所有仓库按时间合并的日志。与上一条同一条读 git 路径，作者不过滤，保证合并视图与单仓库视图口径一致。仓库列表为空返回空日志，只有 project id 不存在才报 `ErrProjectNotFound`——`GetRepositoriesByProjectID` 对缺失项目返回空切片而不报错，存在性得单独判一次。
  - 两条的 limit 都可由客户端传，但服务端钳在 200 以内（否则 `git log -999999` 会走完整个历史）。`internal/service/repo_commits_test.go` 覆盖合并排序、空仓库列表返回空切片而非 nil / 非 error、未知 id 报错、limit 钳制。
- **forge 外链降级为次要出口**：`stats.RemoteWebURL`（读 `remote.origin.url`，剥 `.git` 后拼 `/-/commits`；SSH / file:// / 无路径主机一律返回空而非猜测）仍在，但渲染成仓库工具条上的地球图标，只在有 browsable remote 时出现。`internal/stats/remote_test.go` 覆盖 8 种 remote 形态 + 无 remote / 非仓库目录。
- **AI 问答并入悬浮球面板**：AI 问答原本是项目详情页头部的独立按钮，与右下角悬浮球是同一个意图（「就这个项目记点东西」）的两处入口，且离开项目页就够不到。现改为悬浮球面板的两个 tab（记录 / AI 问答），共享外壳与项目选择器；提问包上下文改由面板按所选项目自行拉取（`getProjectDetail` + `getProjectOverview`），因此对下拉里任意项目都可用，而非仅当前页面的项目。详情页头部只留「复制 AI 上下文」
- **悬浮球面板 320 → 380px**：AI tab 要同时容纳提问框、回答框与提示段，320px 下提示折三行、回答 placeholder 被裁
- **toast 宽度 340 → 400px 且长路径可断行**：toast 最常见的内容就是复制的仓库路径，60+ 字符在 340px 下折三行
- **热力图格子 20px → 11px + 趋势图细线**：格子取周历的最小可读单位，图例同步；趋势图 `borderWidth` 3（Chart.js 默认）→ 1.25、`pointRadius` 2 → 0，300+ 采样点时原本糊成一片实心色块，填充降到 8% alpha，X 轴刻度 8 → 6 且 `autoSkip`（`maxTicksLimit: 8` 配 300 个日期会挤成一坨）

### 修复

- **dashboard 热力图在宽卡下仍整行溢出**：列 `flex-shrink: 0` 把格子钉死在自然宽 11px，卡片再宽也不会利用空间。单行 53 列改走 `.heatmap-grid-year`（`flex: 1 1 0` + `min-width: 14px` 一个格子加列间距），宽卡下均分填满，窄于 ~53×14px 才横向滚动。上一版的「按日历月分块」是绕开这个问题而非修掉它，随本次布局收敛一起回退；等高占位块（stats/legend spacer）也随热力图卡移除而删除
- **仓库路径 copy 点击后毫无反馈**：成功只改了按钮的 `title` 属性，而 tooltip 仅在 hover 时渲染——指针此刻就在按钮上，用户看不到任何东西；图标也不变。现在原地换成 ✓ 图标 + `--success` 着色，并弹 toast 显示完整路径；失败也弹 error 而非静默吞掉
- **`viewCommits` 在语言文件里重复定义**：同一个 JSON 对象里出现两次同名键，后者静默覆盖前者（实际生效的是「提交记录」）。随失去引用的 `showLess` / `authorCount` 一起清理，新增 `commitLog` / `filterRepoHint`

## [1.15.0] - 2026-10-07

AI 问答闭环 + 项目详情页重构 + 悬浮球快捷收录的一次大版本。全量验证：Go 21 包测试、前端 13 文件 93 用例、5 个页面 GUI 实测（0 console 错误 / 0 失败请求 / 0 横向溢出）。

### 修复

- **`?date=` 链接打开详情页即崩溃**（P0）：趋势图统计映射的键是 `stat_date` 原串（部分行带 DATETIME 后缀），而轴标签键被截断为纯日期——`stats.get(d)!` 取到 undefined 即抛 TypeError，整页被 ErrorBoundary 接住。统计映射在源头按日归一（截断幂等，标签处理保持不变）。此回归由趋势轴格式修复引入，暴露于星标卡带日期跳转路径
- **热力图格子全部空白**（P1）：`GetHeatmapData` 把 `stat_date` 的 DATETIME 存储格式（`2025-10-11T00:00:00Z`）原样返回，而前端网格按本地 `YYYY-MM-DD` 作为单元格键——每个格子都 miss，渲染成一片空格子，卡片头部数字（直接对这些行求和）却是对的。API 契约修正为纯 `YYYY-MM-DD`（超长截断），前端网格键同步做防御性截断
- **项目详情操作无反馈**（P1）：Group level 的合并/拆分失败时 `setError` 写入的错误横幅不在正常视图的渲染路径里，"没有可合并的相邻项目"这类失败在界面上与按钮失灵无异；成功时也无任何确认。操作结果现在以页首横幅反馈（成功/无相邻项目/原始错误三种文案），横幅此前被误插进 loading 骨架分支（`replace` 命中第一处同名牌），本次修正到真实渲染分支
- **「分组层级」数字点 +/- 纹丝不动**（P1）：`MergeProjectUp` / `SplitProjectDown` 只把新level 作为返回值计算，三条 UPDATE 全部没写 `level_override`——而项目详情页正是渲染该字段作为「分组层级」。用户点击后界面毫无变化，且后续每次操作都从同一个陈旧值重算。三个 UPDATE 分支（含单仓库 down 那支）全部补上持久化，实测连续 up 三次返回值与落库值同步递增 1/2/3
- **项目分组拆分后合不回去**（P1）：`projects.root_path` 是 UNIQUE。split 出来的项目要合并回父目录时，父目录那个项目仍占着这条 root_path，而兄弟匹配只比 `filepath.Dir(sroot) == parentDir`——占位者的 Dir 是祖父目录，永远匹配不上，最后那条 UPDATE 撞唯一约束使整个事务回滚。用户看到「合并失败」但数据完好（因回滚），实为单向门：split 一次就再也重组不回去。匹配条件增加 `|| sroot == parentDir`，把占位者也当作合并源；实测 14 项目 → 4 项目，11 repos + 4 notes 全部归位
- **AI 端点探测拼出非法 URL**（P2）：localhost→127.0.0.1 回退用 `strings.Replace(url, "://localhost:", ...)` 实现，手填 `127.0.0.1:1234` 会被二次替换成无法解析的 `127.0.0.1:127.0.0.1:1234`（`invalid port`），且 path/query 中的 "localhost" 会被误改。改为新增 `aiLocalhostFallback()` 走 `url.Parse` 只换 `u.Host`：已填IP 原样使用、无端口不拼尾随冒号、主机名大小写不敏感、fallback 不适用时不重试

### 变更

- **新增 AI 问答配置（设置 → AI）**：支持任何 OpenAI 兼容的 `/chat/completions` 端点——本地 LM Studio、vLLM、Ollama 或远程大模型。Base URL 可直接探测拉取可用模型供下拉选择；探测对常见误填具备自愈能力：`…/api/v1`（LM Studio 正确地址是 `…/v1`）等近miss路径按候选序列自动尝试并以实际生效的 URL 回填表单，`localhost` 连接被拒时自动回退 `127.0.0.1`（IPv6/IPv4 监听差异），错误按连接拒绝/超时/域名错/认证失败(401/403)/路径未找到分类给出可行动提示；模型可用 mini completion 实测（免保存即可测试表单当前值）；API Key 可选（本地服务通常无需），密钥本地存储且 GetConfig 打码返回。配置后 AI 问答面板出现「直接发送」：后端新增 `AskAI` RPC，把问题与自动生成的项目上下文发给端点并返回回答，回答仍可编辑后存为笔记；未配置时回落到「复制提问包」路径并引导去设置
- **AI 问答讨论入口（详情页头部）**：RepoNest 不内置模型，问答闭环做成产品可验证的三步——「复制提问包」（问题 + 完整项目上下文，约 2KB）粘贴到任意 AI 对话 → 回答粘贴回面板并可自由纠正 → 一键存为知识笔记（`source=ai`，问题作标题、问答结构保留）。上下文打包与 Copy AI Context 共享同一构建器
- **子仓库行可进入**：按仓库分布的每行新增「在 VS Code 中打开」（`vscode://file/` 深链，代码编辑交接给用户编辑器——产品本体保持只读知识库边界）与「复制路径」；"+N more" 从死文本改为展开/收起按钮，展开显示全部作者统计
- **提交 tab 改为 T 型布局**：横条 = 热力图与趋势图并排（各自限高、共享一个 scope 切换，消除双 toggle 的错位），竖条 = 双列可变长度区（按仓库分布 | 活动指标+提交流）；趋势图 X 轴从 DATETIME 原串修为纯日期
- **提交 tab 合并仓库分布 + 空窗口一键放宽**：详情页的「提交」与「仓库」都是提交统计的展开（按时间 / 按仓库），分列两个 tab 且默认 7 天窗口在无近期提交的项目上全空白，读起来像坏了。合并为单一「提交」tab（热力图 → 趋势 → 活动指标与提交流 → 按仓库分布），独立仓库 tab 移除（旧 `?tab=repos` 链接自动落到提交）；热力图/趋势的空态在更大窗口有数据时提供「查看 30 天 / 查看全部」一键放宽，不再是一句死提示
- **概览与提交 tab 去重**：概览原来内嵌「活动指标」与「最近提交」两个时间维度块，与提交 tab 的热力图/趋势大面积重复。重新划分——概览只回答「项目是什么」（技术栈/语言/README/依赖/贡献者），提交 tab 聚合全部时间维度（热力图/趋势/活动指标/最近提交流，新增 ProjectCommitsSection 承载后两者）
- **项目详情页重构为紧凑头部 + 顶部 tab**：原固定头部堆叠标题卡、汇总 chips、4 列统计网格与 meta 行，占据绝大部分视口，热力图/趋势/仓库/笔记等动态内容全部沉到折叠线以下。重构为三行紧凑头部（身份行 + 单行统计带 + tab 栏），内容区按「概览 / 提交 / 仓库 / 笔记 / 待办」五个满高 tab 分层；激活 tab 同步到 `?tab=` 可链接、`?newNote=1` 跳转自动落在笔记表单；tablist 带方向键漫游焦点
- **笔记按类型分类**：笔记过滤从「全部/知识/其他」三档升级为逐类五档——全部 / 知识 / 日志 / 想法 / 其他，快速收录的内容按其类型归入对应分类（悬浮球收录时可选类型）；`useFilteredNotes` 的 "其他" 语义从 "非知识" 收紧为 "类型=其他"，与 Knowledge 页三档口径不同属有意拆分
- **仪表盘统计标签语义**：汇总卡的 "Repos" 实为「当日有统计数据的仓库数」，静默日恒为 0，读起来像总数出错；标签改为 "Active repos / 活跃仓库"
- **星标仓库卡片扁平化**：星标项目从多行高卡（hero 数字 + 目标进度条 + 2×2 统计 + 贡献条）压缩为单行（名称 + 今日/增/删/仓库数 + 徽章），单个星标项目不再渲染成整宽"下拉框"式的庞然大物
- **新增悬浮球快捷收录**：全页面右下角固定悬浮球，点开即收录——项目下拉（自动定位当前详情页项目）、一段文本、笔记类型（知识/日志/想法/其他），可存为笔记或待办；Esc/遮罩关闭，成功走全局 toast。笔记、待办、快速记录从此收口到一个入口

## [1.14.4] - 2026-10-06

CBiPay 真实数据样例测试(11 仓库聚合工作区)暴露的修复批次,共 4 个 P1、1 个 P3,另含测试中振出的 2 个隐藏老 bug。全部在真实数据上 GUI 实测验证。

### 修复

- **标签整串入库导致标签过滤永远空结果**（P1）：写入口把 tags 输入原文透传，`"cbipay, 样例测试, 支付"` 整串成为一条标签——标签列表提供的是「整串 chip」，而前端过滤按逗号拆分后匹配，任何 chip 都命中不了自己的笔记。规范化收敛到 db 写入收口（`NormalizeTags`：按半角/全角逗号拆分、trim、去空、去重、`", "` 连接，与前端 `parseTags`/`joinTags` 契约一致），`CreateNoteEx`/`UpdateNoteFull`/`UpdateNoteMeta` 三入口全覆盖；`ListAllTags` 同步改为拆分聚合 + 排序（原 `SELECT DISTINCT tags` 返回的是每条笔记一串）；迁移 v13 清洗存量并在 Go 侧执行（逻辑须与 `NormalizeTags` 完全一致）
- **项目详情 recent commits 恒为空**（P1）：`GetRecentCommits` 把字面 NUL 字节拼进 `--pretty=format:` 参数，而 POSIX argv 不允许参数含 NUL，execve 对每个仓库都失败且错误被逐仓库吞掉——所有项目、所有仓库一律空列表。分隔符改用 `%x00` 格式说明符（提交标题可含换行，不能用 `%n`）；补真实 git 仓库回归测试
- **多仓库项目的挖掘 activity / 贡献者恒为零**（P1）：挖掘在项目根目录跑 git，而聚合项目的根是「容器目录」不是 git 仓库，失败被静默吞掉后还进了缓存。新增 `AggregateActivity`/`AggregateContributors` 对兄弟仓库做真实聚合（天数/月份按并集去重、提交数求和、贡献者跨仓库累加）；迁移 v13 失效多仓库项目的 repo_meta 坏缓存（真实零提交无法与坏缓存区分，下次打开页面自动重挖）
- **挖掘 activity 的日期窗口从第一天起就是零**（P1）：`git log "2026-07-06..2026-10-06"` 把日期当 ref 解析必然报错，`active_days`/`commit_rate_30d`/`active_months` 三字段自实现以来恒为 0（total 与 last commit 正常所以从未暴露）。日期窗口改走 `--since=`
- **Copy AI Context 内容过薄**（P3）：前端只拼名称/路径/分组/技术栈/最近提交（111 字节，无 README 时更少）。补语言分布（top 8）、活动指标、范围内统计、仓库清单（cap 30），实测 1954 字节且全部为真实数据

## [1.14.3] - 2026-10-06

### 修复

- **知识库冷启动死锁：空库时无法创建第一条笔记**（P1）：Knowledge 页的 Quick Note / Create Note 在零笔记时被 "No projects found" 挡住——`projectNames` 从**已有笔记**派生（`useKnowledgePage`），没笔记就选不了项目，而空状态文案却在引导 "Create your first note"，提示语让用户去扫描仓库（扫了也没用）。改为从 `getProjects()` 扫描项目列表派生，Quick Note 选择器、项目跳转区与空库守卫全部在冷启动下可用；补 4 个 hook 回归测试

## [1.14.2] - 2026-10-06

发布工程批次，无应用代码变更。另：本轮全量测试(Go 32 包 / 前端 89 用例 / GUI 黑盒 15 项)全部通过。

### 变更

- **CI：fill-sha256 占位符校验门收窄到打包清单目录**：Gate 1 用 `grep -rn` 扫整个 `packaging/`，而 `packaging/README.md` 文档里按字面写出的 `__FILL_SHA256_*__` 示例让该门在自身文档上永远失败——v1.13.0 / v1.14.0 / v1.14.1 三个发布都因此卡在 "Fill manifest sha256" 一步。改为只扫 `packaging/homebrew` 与 `packaging/scoop`（与 Gate 2 口径一致），实际清单无占位符时即可通过
- **打包清单：回填 v1.14.1 发布资产 sha256**：CI 步骤失败期间按其同等口径在本地执行 `--fill-sha256 --from-api` 完成回填，Homebrew Cask/Formula 与 Scoop 清单现指向 v1.14.1 实际发布资产的摘要，双门校验（无占位符 + 摘要与 Release API 一致）均通过

## [1.14.1] - 2026-10-05

### 修复

- **abeval A/B 评测两臂同源、质量门恒失败且污染用户配置**（P0）：lexical/hybrid 两个闭包本应经 `semantic_search` 配置区分，但配置在 `Compare` 之前一次性置 1，而 `semanticEnabled()` 每次搜索实时读库——两臂实际都在语义开启下运行，recall delta 恒为 0，gate 永远 FAIL；且无论 pass/fail 都把用户真实库的 `semantic_search` 永久留成 1（与 vector-init「不替用户开启语义检索」的承诺矛盾）。改为每臂调用时自证配置、进入时记录原值、退出（含 SIGINT/SIGTERM）恢复；新增 `db.DeleteConfig` 支持恢复「本来不存在」状态
- **callout 渲染吞掉其后全部正文 / 或重复渲染**（P0）：占位符提取正则 `>\s*\[!TYPE\]\s*[\s\S]*?(?=\n>|$)` 两种破法——callout 后接空行+段落时贪婪匹配吃到文档末尾，之后正文静默消失；后接非 `>` 行时提前截断，剩余行作为普通 blockquote 二次渲染。改为单遍行状态机 `extractCallouts`：解析器消费哪几行就精确删除哪几行，占位符按索引唯一化（同类型多个 callout 各自独立渲染）。补 5 个回归测试
- **扫描中途取消错误被 `filepath.SkipAll` 吞掉，部分结果冒充完整扫描**（P1）：WalkDir 把回调返回的 `SkipAll` 转成 nil，取消后单 root 返回 (部分 repos, nil)，`CleanupStaleDataTx` 用部分路径集清理会把未走到的仓库当「已消失」删掉。回调改传 `ctx.Err()` 原样传播，walk 结束后无条件补一次 ctx 检查；新增 mid-walk 取消回归测试
- **`/api/rpc` 反射桥暴露 Wails 生命周期方法**（P1）：一发 `{"method":"Shutdown"}` 即可关闭整个服务的数据库句柄（进程存活、端口在听、后续请求全失败）。新增生命周期方法黑名单（Startup/Shutdown/Service → 403）、`(T, error)` 签名预校验（违约降级 501 而非 panic）、context 参数缺省时以请求 ctx 替换 nil
- **`/api/rpc` 请求体无上限**（P1）：`Args []json.RawMessage` 会把任意大的 body 缓进内存，本机进程一个 POST 即可 OOM 服务。加 1 MiB `MaxBytesReader` + `ContentLength` 预检，超限 413
- **后台 goroutine panic 杀死整个进程 / 关闭路径与后台写入竞态**（P1）：stats 按需刷新、mining、auto-import 都是裸 `go`，net/http 只 recover handler 自身 goroutine，一处 panic 全进程退出；且 auto-import 不受 Shutdown 管理，DB 可能在其写入中关闭。新增 `Service.bgGo` 统一收口（recover + WaitGroup 跟踪），`Shutdown` 先取消后台任务再等待（3s 兜底）后才允许 `Close()`
- **stats 按需刷新无去重**（P1）：仪表盘轮询「无数据的日期」时每次 load 都会为每个项目再拉起一轮 git 子进程。`refreshProjectStatsForDate` 按 (projectID, date) `LoadOrStore` 去重
- **`diff.Lines` 最坏情况 OOM**（P1）：LCS dp 表按 (m+1)×(n+1) 分配，100 KB 笔记（字节上限内）若由空行/超短行组成可达 ~10 万行，dp 表 ≈ 80 GB 直接 fatal OOM。先 trim 公共前后缀（真实编辑场景大幅缩表），再设 16M cell 预算门（int32），超限降级为「全删全增」；新增病态输入与长笔记单行编辑回归测试
- **TopContributors 恒为空**（P1）：`git shortlog` 无 rev 参数且 Stdin 为 nil 时从 /dev/null 读 log，输出恒空；`-n<limit>` 实为 rev-list `--max-count`（只扫最后 N 个提交）而非「前 N 作者」。加 `HEAD`、Go 侧截断、30s 超时；新增真实 git 仓库回归测试
- **删除笔记不清理向量索引**（P1）：`DeleteNoteEmbedding` 此前只有健康检查 probe 在用，删除的笔记在 `note_embeddings` 留孤儿向量，消耗 KNN k=20 召回预算（结果被内连接悄悄丢弃），只有手动全量 rebuild 才恢复。`DeleteNote` 现在同步删 embedding（best-effort）；扫描 cleanup 级联删项目后按需执行新增的 `PruneNoteEmbeddings`
- **cmd/abeval 等 CLI 入口未调 `platform.SetPrivateUmask`**（P3，顺手）：abeval 已补齐，与 umask 注释承诺一致

### 变更（P2 批次）

- **schema 迁移批内原子**：多步迁移的非 PRAGMA 语句在同一事务内执行——v4 会 DROP 并 RENAME projects 表，进程死在中间会把库留在「projects 表已删、版本号未盖」的状态，下次启动硬失败需手工修复。PRAGMA（foreign_keys 不能在事务内改）在批外执行。新增回滚语义与 PRAGMA 混排两个单测
- **`MergeProjectUp` 兄弟项目匹配去掉 LIKE 预过滤**：原 `root_path LIKE parentDir || '/%'` 从不转义——父目录名含 `%`/`_` 会过匹配（被精确复检兜住），而 Windows 路径里的 `\` 会充当 LIKE 转义符导致**什么都匹配不到、合并在 Windows 上静默无效**。改为全量遍历 + 精确 `filepath.Dir` 比较，新增含元字符路径的回归测试
- **符号链接扫描根可被发现**：WalkDir 对 root 做 Lstat，symlink root 整个 walk 只访问一项、静默 0 结果（而 root-本身是仓库的检查走 `os.Stat` 又能发现，行为自相矛盾）。`scanRoot` 开头只对 root 本身做 `EvalSymlinks`（树内 symlink 维持不下钻防环），新增回归测试
- **桌面 CSP 收紧**：`connect-src` 去掉 `ws: wss:`（`ws:` 按 CSP3 规则同时匹配任意主机的 `wss:`，而发布的前端不使用任何 WebSocket，二者纯属给注入脚本留外传通道）；注释改为如实承认 `img-src https:` 是余下通道，不再声称「无外传通道」
- **`cmd/server` 优雅关闭**：SIGINT/SIGTERM 此前直接终止，defer 清理从不执行。改为 `signal.NotifyContext` + `srv.Shutdown` 排空 → `svc.Shutdown()`（等后台任务）→ 关库；补 `WriteTimeout`/`IdleTimeout`；并补 `EnsureDefaultScanRoots()`（原注释声称「与桌面 App 相同启动序列」但没有 seed，全新 headless 安装 TriggerScan 会失败）
- **HTTP 状态码不再把故障说成不存在**：`GetProjectDetail`/`GetProjectOverview`/`UpdateProjectLevel` 引入 `service.ErrProjectNotFound` 哨兵，db 层拆分/合并保留 `sql.ErrNoRows` 包装；httpapi 对 404 之外的错误返回 500——此前磁盘满/DB 锁死一律表现为 404
- **`GetGitUserName` 改读 `--global`**：原 `git config user.name` 会读进程 CWD 所在仓库的 local config，headless server 由插件拉起时 CWD 任意，导致「我的」统计拆分随启动目录漂移
- **platform 测试与真实用户目录隔离**：`TestMain` 把 HOME/XDG 重定向到临时树——原测试会真实创建 `~/Library/Application Support/reponest` 并可能触发一次真实的 legacy 目录改名
- **`prune-releases.sh` 尊重 `--delete-tags`**：`gh release delete --cleanup-tag` 原来无条件传入，普通 `--yes` 运行会删掉所有被裁剪 release 的 git tag，而输出却声称「tag 将保留」。改为仅在 `--delete-tags` 时传
- **数学渲染排除代码块**：`$...$` 替换此前作用于整段 HTML——代码块/行内代码里的 `$1 + $2$`、`$VAR` 会被改写成 KaTeX 公式垃圾。`<pre>/<code>` 段先抽离占位、数学渲染后还原再消毒，新增 3 个回归测试
- **i18n 补齐缺失 key**：`project.saveFailed`（此前保存失败显示字面量 key）、`project.star/unstar`（星标按钮 tooltip 一直显示 "project.star"）、`project.refreshHistory/refreshingHistory`、`common.date`、`common.importFailed`、`knowledge.pinFailed`、`nav.main`、`nav.language`；BlockEditor 7 处硬编码中文（拖拽排序/上移/下移/下方插入块/删除块/插入块/添加块）改用既有 `blockEditor.*` key（key 早就在、组件没用）；语言切换按钮改用 `nav.language`（此前复用搜索的 aria-label，读屏用户会把语言切换器当成搜索），主导航 aria-label 同步修正
- **API 文档补 `/api/rpc` 章节**（中英）：此前文档只描述 6 个只读端点，实际存在一个能力等同桌面 UI 的全量写面，安全评估会严重低估暴露面。补请求/响应契约、状态码、生命周期黑名单与 1 MiB 上限

### 变更（P3 批次与前端竞态补漏）

- **补上上一轮遗漏的前端竞态修复**（上轮汇总误报为已完成，本轮实际落地）：`useProjectDetail` 加载加序号守卫（/project/:id 复用页面实例，晚到的旧项目响应会覆盖新页面）；`NoteSection`/`TodoSection` 的列表加载同样加序号守卫，且加载失败不再伪装成「暂无」空态而是显示 ErrorBanner（TodoSection 此前根本没有错误展示）；`ProjectSearchDropdown` 搜索响应乱序守卫；`useNoteMutations.run` 返回成功/失败布尔，`handlePin` 改按返回值回滚（原 `lastOpRef` 检查会让任何一次更早的失败导致之后每次成功的 pin 都被错误回滚）；`ProjectPanel` 加 `key={projectId}` 修跨项目草稿泄漏（A 项目开着草稿切到 B 项目，草稿会被写进并保存到 B 名下）
- **CJK 截断不再出现乱码**：`context.go`/`llm.go` 的 4 处按字节截断改用 rune 边界安全的 `truncateBytes`；`makeSnippet` 的起点与 idx<0 分支同样补 UTF-8 边界钳制
- **`CreateTodo` 的 MAX+INSERT 包进事务**：并发创建此前可能拿到同一个 sort_order
- **grouper Rule 3 分支锁死行为**：补重叠 roots（父仓库目录 + 内嵌仓库根）回归测试，明确该分支产出的两个 group 共享 RootPath、DB 端按 root_path upsert 合并为一个项目；合成的父 RepoInfo 带真实 depth 而非硬编码 0
- **插件 Init 失败回滚 handlers**：`Context.On` 在 Init 期间记账，Init 失败或 Import 签名不符时按票据截回——此前加载失败的插件仍持续接收事件
- **MCP `notes_update` 原子化**：合并省略字段后一次 `UpdateNoteFull` 写回（原 UpdateNote + UpdateNoteMeta 两步，元数据失败会留下「新内容 + 旧标题标签」的中间态）
- **`DetectLanguages` 单文件 1 MiB 上限 + `scanner.Err()` 检查**：巨型数据文件不再被逐行读完，读取失败的文件不再贡献部分计数
- **`writeJSON` 先 marshal 后发头**：编码失败不再给客户端空 body 的 200
- **platform 三项**：`DefaultScanRoots` 在 home 不可得时不再播种空字符串根；`fallbackDir` 回退路径加 pid 后缀（消除与自身引用的 CWE-379 相矛盾的固定拼写）；`GetPluginsDir` 也触发 legacy 目录改名（不再依赖 GetDbPath 先被调用）
- **扫描器 `.git` 文件取舍写进注释**：worktree/submodule（`.git` 为文件）不识别是当前的有意取舍，注释说明原因与支持路径
- **前端清理**：mermaid SVG 注入改函数式替换（`$` 模式不再破坏图形、重复内容不再只换第一处）；frontmatter 剥离正则收紧为 `^---\n…\n---`（以 `---` 水平线开头的正文不再被整段吃掉）；删除无调用的 `getPluginStatuses`/`reloadPlugins` 端点与 9 个死 locale key；Heatmap 复用共享 `toDateStr`
- **外围**：VS Code 扩展 MCP 客户端每个请求 60s 超时（卡死的服务器此前会让进度条永久挂起）；dsh 插件只在连接级失败时重试（HTTP 错误状态与 abort 立即抛出）；ci.yml 修正 go 版本注释（1.25 → 1.26）

## [1.14.0] - 2026-10-04

### 修复

- **headless 模式知识源未注册**：`cmd/server` 不执行服务启动流程，而内置知识源注册与启动自动导入都挂在 `service.Startup()` 上，导致 `GetKnowledgeSources` 返回空数组、`TriggerKnowledgeImport` / `ImportClaudeMemory` 报 `unknown knowledge source "claude"`。headless 模式下导入能力实际不可用。改为在构造 `httpapi` handler 前调用 `svc.Startup()`；该方法已有 `sync.Once` 守卫且遵循 `auto_import` 配置项，重复调用安全。实测 5 个源全部可见、`TriggerKnowledgeImport` 正常返回统计

### 文档

- **知识源导入文档与实现对齐**（按文档实测「导入项目 → 生成知识库」全链路后修订）：`plugins/overview.md` 原只记录 `claude` 一个源，实现实际内置 5 个。补齐 5 个源的读取路径、触发方式（`claude` 启动自动，其余 4 个手动）、单文件 100 KB 上限、`MEMORY.md` 跳过规则，以及项目归属的三级匹配规则（项目名精确 → 仓库路径结尾 → 项目名包含）
- **补上两个隐性前置条件**：文档需先扫描入库项目，否则导入按项目名匹配不到会全部计入 `skipped`；`openclaw` 与 `hermes` 的文件本身不含项目线索，目标项目由 `openclaw_project` / `hermes_project` 配置键指定，未配置时**静默全部跳过**（导入不报错但一条笔记都不生成，仅 `skipped` 计数可辨）
- **补充导入统计口径**：`{created, updated, skipped}` 含义，并说明重复导入得到 `created=0 / updated=N` 是幂等生效的证据而非无数据
- **`getting-started.md` 补第 6 步「导入已有的 agent 记忆（可选）」**，原五步路径完全未提导入；并建议在扫描完成后再导入以提高命中率

## [1.13.0] - 2026-10-04

### 修复

- **扫描根归一化与逐条拒绝**（设置→扫描目录 实测修复）：新增 `normalizeScanRoots`，逐条 trim、展开 `~`、拒绝相对路径、`filepath.Clean` 消尾斜杠与 `.`/`..`、`os.Stat` 校验存在性与目录类型，并按平台语义去重（Windows/macOS 折叠大小写，Linux 不折叠）。修复四个实测问题：重复路径致 React 同 key 崩溃、尾斜杠被当作不同目录而重复遍历、不存在的路径被静默接受、缺少「需重新扫描」提示。设计上采用**逐条拒绝并回报**而非整单硬校验——Windows 默认根是所有盘符，盘符不存在是真实场景，整单硬校验会让用户想移除另一个根时被死路径整体卡死。`EnsureDefaultScanRoots` 走同一归一化，避免无效默认根落库后卡死用户后续写入。新增表驱动单测 16 例
- **扫描空根/零结果不再删库**：`CleanupStaleDataTx` 把「扫到 0 个仓库」当作「所有仓库都被删了」，会清空全部 repositories、daily_stats 与孤立项目；而零结果有多种非删除成因（根未配置、根不可读、软链目标消失、外置卷未挂载），且统计由 git 历史算出、不重读每个项目无法重建，属不可恢复丢失。改为两道守卫：扫描根为空直接拒绝；已配置根但零结果且库中已有项目时拒绝并报出根数与在册项目数。同时 `GetScanRoots` 的读取错误不再被当作「未配置根」
- **`TestCleanupStaleDataTx_RefusesEmptyPathsAndKeepsData` 连接池死锁**：事务仍开启时用 `db.QueryRow` 读取，`:memory:` SQLite 连接池上限为 1，连接被 tx 占住导致读操作永久阻塞（整包 600s 超时）。改为先显式 `tx.Rollback()` 再经连接池读取，补上原先被忽略的 `Scan` 错误检查。25s 超时挂死 → 0.07s 通过
- **`.impeccable.md` 字符损坏**：一个中文字被写成 3 个 `U+FFFD` 替换字符，已还原

### 变更

- **设计系统重构与 404 页**：design-system CSS 体系拆分重排（tokens / layout / components / features）；新增 404 页、品牌标记组件与跟随系统的主题 hook；Dashboard / Knowledge / ProjectDetail / Settings 视觉与交互调整，设置页标签新增描述文案
- **文档站品牌标记单一事实源**：`scripts/build-docs.mjs` 从 `build/icon.svg` 内联品牌标记到侧栏，避免文档、桌面应用与产物图标各自漂移
- **API 契约变更**：`UpdateScanRoots` 由返回 `error` 改为返回 `(*ScanRootsResult, error)`，携带实际落库的路径列表与被拒项及原因

### 技术

- **`internal/service/scan.go` 与 `handoff.go` 补 `gofmt`**：均为注释列对齐的纯空白差异，全仓 `gofmt -l` 现为空

## [Unreleased]


### 新增

- **图标单一事实源 + `icon-check` CI 门**：品牌标记只保留 `build/icon.svg` 一份，桌面 appicon、Windows `.ico`、VS Code 扩展图标与文档站 favicon 全部由重写后的 `scripts/generate-icons.mjs` 生成；CI 新增 `icon-check` job，用带容差的像素比对校验各派生图标与母图一致，只改母图或只改派生（半落地重做）会被拦下
- **统一 UI 图标组件 `<Icon>`**：新增 `web/src/components/Icon.tsx` 作为描边几何 UI 图标的唯一出口（17 个图标，并提供 `iconMarkup()` 给 Markdown callout 等非 React HTML 复用）。`App`、`BlockEditor`、`ErrorBoundary`、`NoteSection`、`ProjectCard`、`TodoSection`、`Dashboard`、`Knowledge`、`ProjectDetail`、`KnowledgeCard`、`ProjectSearchDropdown` 中的内联 `<svg>`、`★ ▲▼ ✕ ?` 字形与 emoji（语言旗标、callout、达成提示）统一替换为该组件，并补齐 `aria-label`
- **会话记忆路线图三篇设计提案（ADR-0010/0011/0012）**：把 M1 会话自动捕捉 / M2 多 agent 记忆源导入 / M3 语义检索从待办细化为落地设计——各自盘点现有可复用链路（`handoff` 标签与 `CreateHandoffNote`、`plugin.KnowledgeImporter`+`RegisterSource`+`upsertDoc`、FTS5 trigram），并核实关键外部事实：`modernc.org/sqlite/vec` 使向量检索在零 CGO 下可行（M3 存储侧解禁、本地 embedding 生成是唯一硬门），Cursor 历史存于未公开的 `state.vscdb`（M2 判定为最脆弱、缓行）。三篇均记为 Proposed 并列出实现前必须拍板的决策门
- **Codex 会话导入器（M2 首个新源）**：新增 `internal/importers/codex`，流式解析 `~/.codex/sessions/**/rollout-*.jsonl`，把每个会话（cwd→项目匹配 + 首次指令 + 最近回复）汇成一条 `log` 笔记，复用既有 `KnowledgeImporter`/`upsertDoc` 管线；抽出共享件 `internal/importers/memsrc`（`MatchProject`/`ReadCapped`/`LastPathSegment`），claude importer 改为委托。**隐私门**：`RegisterSourceManual` + `sourceEntry.auto`，启动 `ImportAll` 只跑 auto 源，Codex 需在设置的知识源列表里显式一键触发（ADR-0011 决策 4）
- **M3 语义检索可行性实测 + 混合检索核心**：`internal/vecprobe`（`CGO_ENABLED=0` 下实测 `modernc.org/sqlite/vec` = sqlite-vec v0.1.9 的 vec0 建表 / KNN / `vec_distance_l2` 全跑通，以无生产码的 test-only 包隔离其 `auto_extension` 副作用）作为零 CGO 向量能力的回归锁；`internal/search/hybrid` 落地 vendor 中立的 `Embedder` 接口 + `FuseRRF`（k=60、确定性 tie-break、无外部依赖）+ 单测。端到端接线与本地 embedding 路线仍为待定决策（ADR-0012）
- **M1 会话解析核心（ADR-0010，未接线）**：`internal/importers/claude/session.go` 新增 `ParseSession`/`LatestSessionFile`，流式解析 Claude Code `*.jsonl` 会话（实测对齐 v2.1.278）：抽 sessionId/cwd/gitBranch/timestamp + 最后一条 assistant 文本 + 首次指令 + 去重工具名，忽略 sidechain、宽松跳过未知/超长行。刻意无副作用、不自动读盘——`Session→HandoffInput` 与触发/默认开关属隐私决策，未拍板前不接入
- **OpenCode 会话导入器（M2 源 #3）**：新增 `internal/importers/opencode`（opt-in 手动源），真机核验 `~/.local/share/opencode/storage/session/<hash>/ses_*.json`（自带 title/summary/directory 摘要级字段），每会话→一条 `log` 笔记、`directory` 末段驱动项目匹配；golden-style + 接口测试
- **OpenClaw + Hermes 记忆导入器（M2 源 #4/#5，均 opt-in 手动）**：`internal/importers/openclaw`（`~/.openclaw-autoclaw/workspace/*.md`）与 `internal/importers/hermes`（Nous Research 独立产品；`~/.hermes/memories/{MEMORY,USER}.md`，`$HERMES_HOME` 可覆盖，格式经官网文档核验）。两者**硬 allowlist 到单个记忆目录的 *.md 单层非递归**——两处的父目录都含私钥/vault/`.env`/`mcp-tokens`/`state.db`，绝不读取（各有 allowlist 单测：父级 `.md`、子目录 `.md`、非 md 一律不导入）。二者是 **agent 全局记忆**（无 per-session cwd），经新配置键 `openclaw_project`/`hermes_project`（项目名或 id，未设→skip）定向到目标项目。共享件下沉：`memsrc.StripFrontmatter`、`memsrc.TargetProject`；`config.go` 允许这两个字符串型配置键。Hermes `sessions/`+`state.db` schema 无文档、本机不可核验 → **暂缓**
- **M1 按需会话捕获（ADR-0010，默认关）**：`claude.LatestSessionForRootPath` 按 `path→slug` 约定取项目最新 Claude Code 会话，`Service.CaptureClaudeHandoff(projectID)` 受 `claude_session_capture` 配置门控（仅 `="1"` 才读盘），把 `Session`→`HandoffInput`（最后 assistant 文本→Summary、工具名+git 分支→Changes、打 `auto-captured` 标签）经共享 `CreateHandoffNote` 落为 handoff 笔记；新增 desktop binding `App.CaptureClaudeHandoff` + 单测
- **M1 捕捉前端入口 + opt-in 源人类可配（Settings→PluginsTab）**：`endpoints.ts.captureClaudeHandoff`（transport 按方法名动态派 Wails 绑定，无需改 wailsjs）；设置页新增 Claude 会话捕捉开关（`claude_session_capture`）、「按项目 ID 捕捉最近会话」按钮，以及 `openclaw_project`/`hermes_project` 目标项目输入（让全局记忆导入器人类可定向，不再只能靠 agent 改配置）；端点契约测试 + i18n（en/zh）
- **M1 B端 SessionEnd-hook 自动化**：新增 `cmd/reponest-capture` CLI 作为 Claude Code SessionEnd hook 目标（从 `-cwd`/stdin `.cwd`/进程 cwd 取会话目录）→ `Service.CaptureClaudeSessionByCwd`（cwd 末段匹配项目、走同一 `claude_session_capture` 默认关门控 + `CaptureClaudeHandoff`）；未开启或无匹配项目时打印错误但退出码 0，绝不打断会话收尾。`reponest-init` 的 hook 示例指向此命令
- **M3-C：FTS5 查询松弛召回（默认生效，零词典/零 CGO）**：`internal/db/search.go` 笔记检索在严格 FTS5 AND **命中为零**时，丢英文停用词并把剩余词 **OR 合并**重查（纯 FTS 内、不新增 LIKE 兜底，故「坏/空索引→空结果」的既有语义与修复测试不破；AND 有结果时不误触发）。纯词法召回增量，补一部分「换词/多词」搜索的空结果。单测 `escapeFTSOR` + 松弛/不误松弛两例
- **M3-A：embedding 选型定为 OpenAI 兼容协议 + `RemoteEmbedder` 基座（未接热路径）**：不锁厂商，标准化到 `/v1/embeddings`（云＝OpenAI `text-embedding-3-small`/Voyage/Jina；**本地＝自托管 Ollama → 零 CGO 且数据不出机**，解掉「本地优先 vs 远程 embedding」两难）。`internal/search/hybrid.RemoteEmbedder` 实现 `Embedder`（按 index 乱序回填、HTTP/维度错误宽松、可选 bearer），httptest 单测。A 仍默认关、须过 A/B 门与显式开启后才接进 `db/search.go`/建 `vec0` 表/API-key 安全存储
- **M3-A 向量存储层 + 配置密钥安全（默认关，未接热路径）**：`internal/db/vecindex.go` 提供 vec0 派生索引 `note_embeddings` 的 `EnsureVectorIndex(dim)`（按 dim 变更重建，dim 记在自管 `note_embeddings_meta`、不依赖 sqlite-vec 内部 schema）、`PutNoteEmbedding`/`KnnNoteIDs`/`ClearVectorIndex`/`DropVectorIndex`；`_ modernc.org/sqlite/vec` 零 CGO 链入 `db`（`CGO_ENABLED=0` 全仓构建验证）。配置新增 `semantic_search`/`embedding_base_url`/`embedding_model`/`embedding_dim`/`embedding_api_key`，**`embedding_api_key` 在 `GetConfig` 中掩码不回传前端**（后端经 `db.GetConfig` 读真值）。全库重算 + FTS×vec 的 RRF 融合接进 `db/search.go` + UI + A/B 门为下一步
- **M3-A 后端接线：全库重算 + FTS×vec RRF 融合（默认关，端到端测试）**：`internal/service/search_semantic.go` 新增 `RebuildEmbeddings()`（分批全库重算、dim 可自探、读真值密钥）与 `fuseSemantic()`（把向量召回的笔记 id 经 `hybrid.FuseRRF` 并入词法结果；`semantic_search=1` 且向量索引就绪且端点配好才生效，任何关闭/未配/失败**一律退回纯词法、绝不减少结果**）；`App.RebuildEmbeddings` desktop binding。httptest 桩 OpenAI 兼容端点端到端验证「词法零命中→向量补出、关掉即回纯词法」。面向普通用户的 embedding UI 与 A/B 评测门仍在 A 正式开放前补齐
- **M3-A：A/B 评测门（`internal/search/abeval` + `cmd/abeval`）**：`abeval` 提供 Recall@k / NDCG@k（binary 相关性）+ `Compare`（hybrid−lexical delta）+ 单测；`cmd/abeval` 对活库每条标注 query 跑 lexical(semantic off)/hybrid(semantic on) 两趟、打印指标与 `GATE PASS/FAIL`（`-min-recall` 阈值；无 embedding 端点时 hybrid 退化为 lexical、门自然不过）。这是 ADR-0012「先测再决定要不要给用户开」的闸门工具
- **向量存储选型与安装引导（[ADR-0013](docs/adr/0013-vector-database-selection.md)）**：确立轴 B「本地 vs 远程向量库」的默认 = **本地 sqlite-vec**（校正选型原稿「sqlite-vec 需 CGO」之误——实为 `modernc.org/sqlite/vec` 纯 Go、`CGO_ENABLED=0` 已验证，无需 build tag）；`db.VectorStoreHealthCheck`（建 vec0→写探针→KNN 往返→清理，含单测）+ `cmd/vector-init` 引导式安装：自检 vec 版本、跑健康检查、选 embedding provider（Ollama 本地默认 / 远程 OpenAI 兼容 / 跳过）并写配置（api key 脱敏），收尾**指向 设置→插件** 复核/开启语义检索/重建索引；`semantic_search` 保持默认关
- **远程向量库接缝：Qdrant 实现 + 自动退回本地（轴 B opt-in）**：`internal/search/vectordb` 定义 `Store` 接口，`Local`（sqlite-vec 默认）+ `Qdrant`（REST：建集合 / 写点 / `points/search`，`api-key` 可选）；`vectordb.Open` 按 `vector_store`/`vector_store_url`/`vector_store_api_key`/`vector_store_collection` 选择，**未配或不可达自动退回本地**；`search_semantic` 的重建与融合改走 `Store`；`cmd/vector-init -store qdrant` 写配置并探测；新增 `vector_store*` 配置键（api key 在 GetConfig 脱敏）。httptest 桩已验证契约，真实 Qdrant 冒烟由用户在启用前完成
- **真实服务冒烟测试（build-tag 门控，CI 默认不跑）**：`ollamalive`（`RemoteEmbedder`↔真 Ollama，dim 768 通过）、`qdrantlive`（Qdrant 客户端↔真容器 建集合/写点/search，最近邻正确）、`aelive`（全链路：Ollama 嵌入→真 Qdrant 存→`SearchNotes` 语义召回，"bonjour salutation" 命中 "greeting note" 排首）。本轮已在本地真 Ollama + 真 Qdrant 容器全部跑通
- **向量存储改为可插拔 registry（灵活配置）**：`vectordb.Register(kind, factory)` + `Kinds()`；`Open` 按 `vector_store` 选、**未知 kind/远程不可达一律退回本地**；`cmd/vector-init` 枚举可选后端、消息按实际所选 store 打印。加后端＝一个 `Store` + 一行 Register，调用方零改。ADR-0013 补候选矩阵（sqlite-vec/chromem-go/Bleve/Qdrant/Weaviate/LanceDB 等，标注 CGO 硬筛——LanceDB/go-libsql 破零 CGO）与开放记忆标准 **OMP** 展望（v0.4 早期，暂不硬依赖、留导入导出接缝）
- **新增 Weaviate 向量存储后端（远程，零 CGO 客户端）**：`vectordb.Weaviate` 实现 `Store`（REST 建类含 `note_id` int 属性、`vectorizer:none`；对象带确定性 UUID 幂等 upsert，dup 则 PUT 回落；`nearVector` 走 GraphQL，按 `note_id` 距离升序返回）；注册进 registry，`vector_store=weaviate` 即用、不可达自动退回本地。契约**先在本机真容器核验再写**（类名首字母大写、`/v1/collections` 不存在用 `/v1/schema` 等），httptest 覆盖 + `weavialive` build-tag 真服务冒烟跑通
- **OMP 可移植记忆导出接缝（provisional）**：`service.ExportMemoryJSON` 把知识库导出为 [Open Memory Protocol](https://github.com/SMJAI/open-memory-protocol) 风格 Memory Object 数组（type 由 kind/handoff 投影 episodic/procedural/semantic），desktop binding `App.ExportMemoryJSON` + 前端 `exportMemoryJSON`；为将来取代逐工具逆向 importer 预留对接。仅导出向，OMP v0.4 预-1.0 暂不硬依赖、不做导入向。chromem-go/Bleve（需 `go get`，当前离线）与 Pinecone/Milvus（需凭据/集群）记为 registry-ready 待接入
- **M1/M2/M3 决策落进设计文档**：ADR-0010「默认关 + C端按需/B端可选 hook + 风险项文档化由 B端自担」；ADR-0012「放弃 B（纯 Go 本地模型）、默认 C（FTS5 增强）、A 远程 embedding 作 B端可选默认关 + 强风险披露」；ADR-0011「M2 全源真机核验：OpenCode 可行；Cursor=`state.vscdb` 高风险待核验；OpenClaw 记忆须 allowlist 到 workspace/*.md（同目录含私钥，绝不整树遍历）且为 agent 全局记忆、项目归属待决；Hermes 是 Nous Research 独立产品（先前误判为 OpenClaw 运行时已更正），本机无可检视数据目录、需真实样例再实现」

### 变更

- **文档同步（补英文化与用户文档缺口）**：新增 `docs/en/adr/0010–0013`（此前 ADR 只有中文、英文站缺失），英文 ADR 索引加行并修正递增位；README 补多源导入 / 会话捕捉 / 语义检索+可插拔向量库 / OMP 导出 / CLI 工具；`settings` 功能文档中英双语补齐插件页新控件（自动导入/捕捉开关/目标项目/知识源/隐私取向）
- **品牌重做**：`build/icon.svg`、`docs/favicon.*`、`web/public/favicon.*`、`ide/vscode/media/*` 依据新品牌标记重生成；配套调整图标按钮 / 空状态 / callout 的 CSS 与移除 UI 文案中的 emoji
- **评估类待办收口（Sprint 14）**：P31/P33/P34/P36 逐条验证并落结论（均为「保持现状」——`Domain/types.go` 已收拢、`TrendChart` 实为 chart.js 封装而非纯 SVG、`project_overview.go` 内聚合理且后台 `mineAndCache` 的 `recover()` 已生效、`knowledge.go` 各探测函数仅以 `repoPath` 入参无共享状态）；P32 完成 Wails 绑定层审计（46 个方法无死绑定/无重复，`bindings.go` 顶部落「绑定 ↔ MCP 工具」审计块）
- **KnowledgeCard 迁入 CSS Modules（P35 第二步）**：卡片样式从全局 `knowledge.css` 迁到新建的 `KnowledgeCard.module.css`（`kind-knowledge/idea/log` 动态类改为 `badgeByKind` 查表映射），全局仅保留与 NoteSection 复用的 `.pin-btn` 及 `.markdown-body`/`.btn`，继续把全局 CSS 收敛到 reset/tokens/跨组件基础样式
- **插件运行时文件合并（C11 方案 A）**：`runtime/loader.go`（121 行）合入 `runtime.go` 的「yaegi script loader」段并删除，减少文件碎片；Claude importer 走 Go 原生 `RegisterSource`、不经 yaegi，路径不受影响。方案 B（移除 yaegi 依赖）仍留作 2.0

### 修复

- **`parseTimestamp` 时间戳解析鲁棒性（P29）**：`internal/stats` 的 `parseTimestamp` 原只认 `2006-01-02 15:04:05` 且静默忽略错误，现支持裸 unix 秒（git `%at`）、RFC 3339 / ISO 8601（git `%aI`/`%cI`）、git `%ai`（带 `-0700`）、date-only（`%ad --date=short`）与 git 默认作者日期（含空格补零的日）；不可解析仍返回 0 以保持 latest-commit 比较契约。新增覆盖 10 种格式 + 4 种非法输入的单测
- **记忆导入器内容裁剪（代码审核发现）**：`openclaw`/`hermes` 先按 `MaxNoteContentLen` 截正文再前置来源头 → 合成结果可能超限被 upsertDoc 拒；`codex`/`opencode` 用 `content[:Max]` 会切断 CJK rune。统一 `memsrc.ClipToBytes`（按 rune 边界、不切碎）裁**整条合成内容**，四 importer 一致；补 ClipToBytes 单测 + openclaw 超大 CJK 正文回归

[1.14.0]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.13.0...v1.14.0
[1.13.0]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.12.0...v1.13.0

## [1.12.0] - 2026-10-02

### 新增

- **文档图文搭配体系化 + 图数 8 → 16（zh/en 逐页对齐）**：确立「每张图必配1-3 句读图文字，且只画正文线性文字表达不了的东西」的规则，
  全站补齐读图段（中文「读图：」/ 英文「How to read it:」）：架构页 1 → 4 张（新增扫描管线图、知识库「写一次读两路」图、
  构建双产物线图，分层图补 `ENTRY` 三端入口节点）；`storage-optimization`、`dashboard`、`knowledge`、`project-detail`、
  `command-palette`、`data-management`、`plugins/overview` 七页从零图各补一张结构图；`getting-started` 上手流程图重画
  （原图只是把编号步骤复述一遍，改为体现「实线/虚线 = 知识库可用边界」）并新增第 5 步「沉淀第一条知识笔记」；
  `settings` 因六个标签页为平铺枚举无结构可表达，按不硬凑原则不配图
- **图语言分工，消除 ASCII/mermaid 混用**：结构 / 时序 / 对比 / 界面布局一律 mermaid；目录树、配置片段、JSON、shell
  保留代码块（它们不是图）。架构页扫描管线由 ASCII 箭头升级为 mermaid flowchart

### 修复

- **文档站 mermaid flowchart 被压成一条线（既有缺陷，非本次引入）**：`scripts/build-docs.mjs` 的 `getBBox` polyfill
  对根 `<svg>` 也按 `textContent` 长度估算尺寸，而 mermaid 恰恰读根 svg 的 bbox 定viewBox，导致所有 flowchart 的
  viewBox 变成「宽 4 万 px、高 36px」，站点上显示为发丝线（sequenceDiagram 因布局路径不同幸免）。改为容器元素递归取
  子元素并集并累加 `transform="translate(...)"` 与 `x`/`y` 偏移，仅文本叶子做 8px/char 估算。修复后图形比例恢复正常

## [1.11.0] - 2026-10-02

### 新增

- **文档 Mermaid 图全站化 + 双语全景图**：构建管线启用 mermaid 构建时渲染（```` ```mermaid ````
  代码块 → 内联 SVG，jsdom polyfill，渲染失败自动降级为可读源码文本，构建不失败；GitHub 上同名源码
  原生渲染保持同源可读）：落地页新增核心闭环图 + 会话记忆环时序图 + **「两类用户，一套产物」全景图**
  （个人用户货架：桌面 App / VS Code 扩展 / 博客；Agent 货架：reponest-mcp / dsh 插件 / llms.txt；
  单一事实源 service 层 + reponest-init 接线员，中英双语），架构页加分层图，AI 集成页加协议时序图，
  快速开始加上手流程图，SKILL.md 会话环改图示；README 增全景缩略图
- **语言切换器对齐修复**：侧栏顶部的 `English · 中文` 内联文本在窄视口下悬挂分隔符、中英按钮错位，
  改为固定成对的 pill 按钮（当前语言实心 / 另一语言描边），窄视口不再不对称换行

### 修复

- **Pages CI 补齐 docs 构建依赖**：`.github/workflows/pages.yml` 原只安装 `marked`，
  mermaid 渲染器在 CI 上不可用会静默降级为源码文本发布；补装 `mermaid` + `jsdom`
- `build-docs.mjs` 清理未消费的 `hasMermaid` 死参数（渲染器失败已由 slot 内 fallback 表达）

## [1.10.0] - 2026-10-02

### 新增

- **发布独立博客**：ADR-0009 博客化《AI 不会替我们分发：让插件跟人见面》（杂志风单文件 HTML，`blog/0009-ide-presence.html`，
  不依赖 docs 构建管线，双击即开，内链指向 GitHub 决策原文）
- **一键注册 CLI（ADR-0009 第一步 / TODO M4）**：新增 `scripts/reponest-init/`（零依赖 Node ≥18，可直接发布为 npm 包），
  一条命令探测 `reponest-mcp` 二进制并注册到 Claude Code（`.mcp.json`）/ Cursor（`.cursor/mcp.json`）/
  VS Code（`.vscode/mcp.json`）/ Windsurf，配置只落在对应客户端目录已存在的地方；`--with-hook` 同时安装
  SessionEnd 交接 hook（脚本 + settings.json 合并，改前备份），幂等可重复执行，`--dry-run` 预览，
  二进制缺失时打印安装引导（`--yes` 以裸命令名写入）。JetBrains 打印手动指引。
  同轮新增 [ADR-0009「IDE 存在感——薄客户端分发策略」](docs/adr/0009-ide-presence.md)（Accepted：薄客户端纪律、
  VS Code 扩展为唯一 IDE 扩展、JetBrains 缓议、分发评估门）；README / SKILL.md / ai-integration 文档（中英）
  同步一键注册路径。
- **VS Code 扩展骨架（ADR-0009 第二步 / TODO M5）**：新增 `ide/vscode/`（TypeScript 薄客户端，tsc 零错误）：
  命令面板三条命令（context / handoff / search）经 `reponest-mcp` stdio 调用（复用 MCP 唯一执行接口，零逻辑复制），
  侧边栏笔记检索（FTS5 结果树视图 + 只读打开），状态栏入口 + headless 服务健康探测，
  一键注册命令（终端内运行 `reponest-init`）。一份 VSIX 覆盖 VS Code / Cursor / Windsurf 全 fork 家族；
  VSIX 打包与 marketplace 发布待后续。
- **文档双语化，英文为默认语言**：手册 15 个核心页面全部提供英文版（`docs/en/`，中文源保留在 `docs/`），
  README 拆分为英文主文件 + `README.zh-CN.md`（互链切换）。文档站改为双 locale 构建：
  英文输出到站点根路径，中文在 `/zh/` 子路径，每页侧栏带语言切换器，英文站根页对中文浏览器
  一次性重定向到中文版。8 篇 ADR 详情页补齐英文版（含 ADR 状态规范化为
  Accepted / Superseded 标准用语）；产品评审与定位简报暂保持中文。

### 修复

- **sha256 回填全自动化 + 加固（release.yml fill-sha256 job）**：回填改从 GitHub Release API
  读取每个资产的官方 digest（一次调用替代全量下载后本地算 hash），并修正硬编码的错误仓库名
  `sky-jiangcheng/RepoNest` → 规范 `repo-nest`；新增发后双重校验——占位符清零 + packaging/ 内
  每个 64 位 digest 必须属于本版 SHA256SUMS（防旧版 digest 静默残留导致全线安装失败），
  校验不过则 job 显式失败；API 调用带 GITHUB_TOKEN 鉴权避免共享出口 IP 撞匿名限额。
  `update-manifests.sh` 新增 `--fill-sha256 --from-api` 本地路径（匿名 API，已对 v1.10.0 实测幂等），
  packaging/README 同步为「自动为主、手工兕底」，bump 脚本的过时手工提示改为指向自动流程
- 文档站落地页 `<title>` 不再自我拼接（「RepoNest 文档 · RepoNest 文档」）；
  英文落地页副标题去除重复句
- 统一存储优化文档的 schema 迁移数量口径：8 → 12（以 `internal/db/migrate.go`
  实际版本为准，与架构文档一致）

## [1.9.5] - 2026-09-30

### 新增

- **会话记忆协议（ADR-0007）**：MCP 工具从 10 个扩展到 13 个，补齐 agent 会话边界的记忆两端与发现入口：
  - `reponest_context`（会话开始）：一次调用返回项目完整上下文 Markdown——技术栈 / README 摘要 /
    语言占比 / 依赖 / 贡献者 / 活跃度（`repo_meta` 缓存，未缓存时后台异步挖掘）、最近提交、开放待办、
    高相关知识笔记（`handoff` 标签笔记排序置顶）。项目解析支持 `project_id` 精确 → `project_name`
    模糊 → 单项目无参自动解析；多匹配返回项目目录供 agent 二次选择，绝不猜测注入。
    实现于 `internal/service/context.go`（`ResolveProject` / `BuildProjectContext`）。
  - `reponest_handoff`（会话结束）：结构化会话交接协议，`summary` 必填 + `changes` / `decisions` /
    `gotchas` / `next_steps` 至少一项非空，渲染为固定 Markdown 模板落库（`handoff` 标签自动附加），
    下一个会话经 `reponest_context` 自动读到。实现于 `internal/service/handoff.go`。
  - 工具注册拆分至 `cmd/mcp/tools_context.go`；测试覆盖 service 层（项目解析 / 上下文渲染 /
    交接排序 / 校验）与 MCP 工具层（经真实 server 调用），`SKILL.md` 工作流同步升级为
    「session start → context / session end → handoff」协议。
  - `reponest_scan`（本地仓库发现，第 13 个工具）：一次调用扫描配置根目录、发现 Git 仓库并入库分组，
    纯 MCP 安装（无桌面 App）也能完成发现闭环。实现于 `cmd/mcp/tools_scan.go`。

## [1.8.1] - 2026-09-28

### 修复

- **MCP `reponest_projects_list` 空库返回 `null` 而非 `[]`**：`ListProjects()` 在无项目时返回
  nil 切片，`makeJSONResult` 直接序列化成 `null`，与 httpapi 层已有的空集合回归测试
  （`server_test.go`）口径不一致。新注册的项目列表 handler 先归一化为空切片再编码。
  同时捕获了同病灶的检索路径（搜索空结果本就已归一化，此处仅补充断言）。

### 新增

- **MCP 工具测试**（`cmd/mcp/main_test.go`，13 个用例）：此前 566 行、作为 AI 唯一执行入口的
  `cmd/mcp` 零覆盖。测试经 `registerTools()` 构造真实 server、通过 `GetTool()` 调用 handler，
  与客户端同一路径。覆盖：工具注册与 schema、参数校验（空 query 返回提示而非协议错误）、
  笔记创建/读取/更新/搜索全回环（含 FTS5 索引写入后可检索——正是 1.8.0 修复的静默漏搜路径）、
  not-found 文案、`agent_score` 不再重复计分（DB 连通与“无笔记”是两个独立信号）、
  `integrity` 新库无误报、以及**注入 FTS 索引漂移后 integrity 必须报出 FTS 项**。
- **安装脚本冒烟测试**（`.github/workflows/install-smoke.yml`，Linux/macOS/Windows 三平台）：
  `install.sh` / `install.ps1` 此前从 `releases/latest/download` 下载，错误的资产名只会在
  用户侧失败且无从发现（1.8.0 之前三个平台的一键安装均从未成功）。现在脚本的下载根可被
  `RELEASES` / `REPO_NEST_RELEASE_BASE`、安装目录可被 `INSTALL_DIR` / `APP_INSTALL_DIR` 覆盖，
  CI 用本地 fixture 服务器跑真实下载→解压→安装路径，MCP 二进制是 `./cmd/mcp` 的真实构建
  并实际执行 initialize 握手验证。macOS 侧用 `hdiutil` 构造真实 dmg，覆盖挂载/卸载分支。
- **Dependabot 分组合并**（`.github/dependabot.yml`）：三个生态均由 weekly/10 PR 改为
  monthly + `groups` 全量合并，避免依赖升级淹没有真实议题（此前 14 个 open issue 里 13 个是 bump）。

### 变更

- 删除根目录残留的 `package-lock.json`（87 字节，`web/package-lock.json` 才是真锁文件，
  ci.yml 注释中已标注其为 stray）。

## [1.8.0] - 2026-09-28

### 修复

- **FTS5 索引回填是永久空操作（`migrate.go` v7）**：v7 用
  `WHERE id NOT IN (SELECT rowid FROM project_notes_fts)` 回填索引，但
  `project_notes_fts` 是 external-content 表，该子查询由**内容表**回答，条件恒为假。
  凡是 v7 执行时已有笔记的数据库，索引都是空的，且**没有任何机制能修复**。
  症状是静默的：`SearchNotes` 只在 FTS 查询**报错**时降级 LIKE，查询匹配不到时不报错也不返回结果，
  于是搜索一直只返回笔记的一个子集。新增迁移 **v12** 用 FTS5 官方 `'rebuild'` 命令重建两个索引，
  存量库下次启动自动修复。v7 保持原样（迁移不可变），已在代码中标注该缺陷并说明不可照抄。
  回归测试 `internal/db/migrate_fts_repair_test.go` 直接构造"索引空、内容表有数据"的损坏态；
  已验证移除 v12 后该测试必然失败。
- **`scripts/install.sh` 用 HTTP 下载函数拷贝本地文件**：提取出的 MCP 二进制是经
  `curl -fsSL <本地路径>` 拷过去的，真实 curl 会以 `Protocol not supported` 失败，
  结果装到用户机器上的是一个压缩包而不是可执行文件。拆出 `install_binary()` 走 `install -m 0755`。
- **Windows 一键安装从未成功过**：`install.ps1` 下载 `reponest-windows-amd64.exe`，
  而发布流程只产出 `.zip`。改为下载 zip → `Expand-Archive` → 校验 exe 存在。
- **macOS / Linux 一键安装从未成功过**：`install.sh` 下载无扩展名的 `reponest-$TARGET`，
  实际资产是 `.tar.gz`（Linux）和 `.dmg`（macOS）。Linux 改为解压 tar.gz，
  macOS 改为 `hdiutil attach` 后拷贝 `RepoNest.app` 到 `/Applications`。
- **`reponest_agent_score` 两项检查重复计分**：第 1、2 项判据都是 `noteCount > 0`，
  同一信号被数了两遍，抬高分数。第 1 项改为真正检查数据库连通性（`Health()`）。

### 新增

- **CI 测试门禁**（`.github/workflows/ci.yml`）：push 到 master 与所有 PR 触发，
  Go 与 Web 两个独立 job（`go build ./...` + `go test -race`；`npm ci` + `npm test` + `npm run build`）。
  此前 `release.yml` 只在打 tag 时跑、`pages.yml` 只在文档变更时跑，**仓库里 18 个 Go 测试文件
  和 10 个前端测试文件一个都没有进过 CI**。
  门禁同时解决了 `//go:embed all:web/dist` 在干净检出上无法解析的问题（`web/dist` 被 gitignore）。
- **数据可信度审计**（`internal/integrity` + MCP 工具 `reponest_integrity`）：6 项**只读**检查——
  FTS 索引漂移、孤儿行、schema 形状 vs 版本戳、扫描覆盖率、知识缓存新鲜度、版本快照孤儿，
  输出带人类可读证据与 0-100 可信度分数。动机：项目承诺"把知识建模成可被 AI 依赖的结构化数据"，
  此前没有任何机制能回答"这个库还能信吗"。
  与 `reponest_agent_score` 分工明确——后者答"配置好了吗"，前者答"数据还对吗"。
  该工具上线即复现了 v7 的索引漂移 bug。
- **`reponest-mcp` 独立分发**：此前 release.yml **从未构建过 MCP 二进制**，
  README 只能让用户自己 `go build`。现在四个平台的 MCP 产物随 release 一起发布。
- **包管理器支持**（`packaging/`）：Homebrew Cask（macOS 桌面 / macOS MCP）、
  Homebrew Formula（Linux MCP）、Scoop（Windows 桌面 / Windows MCP）。
- **`scripts/build-release-assets.sh`**：本地构建发布产物到 `assets/releases/v<version>/`。
  MCP 是纯 Go 零 CGO，任意宿主都能交叉编译四平台；桌面壳需要各自平台的原生工具链，
  脚本检测到无法构建时会明确说明跳过了什么，而不是半途失败留下一个看起来完整的目录。
- **`scripts/update-manifests.sh`**：把 `wails.json` 的版本号同步到全部包管理器清单
  （已接入 `bump-version.sh`）；`--fill-sha256` 从 `SHA256SUMS` 自动回填校验值。

### 变更

- 笔记导出的 MCP 工具数 9 → 10。
- 安装文档（README / getting-started / SKILL.md）三处统一，并说明各平台实际安装位置。
- `docs/features/ai-integration.md` 补充"就绪度 vs 数据可信度"的分工说明。

### 已知问题

- **历史 release 的资产名仍是旧品牌**：v1.7.9 及之后发的是 `gitbuddy-*`，
  v1.7.6 及之前是 `gitboard-*`。1.7.7 的更名只覆盖了仓库内容，没有传导到发布产物文件名。
  当前 `release.yml` 已统一为 `reponest-*`，从 1.8.0 起一致；历史 release 需要维护者决定是否清理。
- 本版本的 Homebrew / Scoop 清单摘要已用 release 实际产物回填（8/8 匹配）。
  后续版本需在产物发布后跑 `./scripts/update-manifests.sh --fill-sha256`；
  摘要为空的清单 brew / scoop 会拒绝安装——刻意设计，宁可安装失败也不装未校验的二进制。
### 新增

- 手册新增[数据与备份](docs/data-management.md)页：备份、换机迁移、重置与卸载指引
- 新增 `CONTRIBUTING.md` 与 `CODE_OF_CONDUCT.md`
- README 新增目录与「命名分层」规约说明

### 变更

- 项目标识更名为 `repo-nest`：Go module 名、npm 包名、GitHub 仓库名（旧 URL 由 GitHub 自动重定向）；命令名、数据目录与 MCP 工具前缀 `reponest` 保持不变（冻结标识）
- 品牌分层落地（对齐「品牌负责被记住，品类词负责被搜索」原则）：窗口标题与 HTML `<title>` 采用完整展示名 `RepoNest: Local Git Knowledge Base`；`wails.json` comments 同步
- 架构文档新增「命名分层」一节，固化冻结标识（命令名/数据目录/MCP 前缀 `reponest`）不随品牌变化

### 修复

- 架构文档「构建与产物」表移除重复的 `reponest-mcp` 行


## [1.7.9] - 2026-09-03

### 修复

- **项目详情页崩溃**：`GetProjectOverview` 的空切片被 `json.Marshal` 序列化为 `null`，前端读 `recent_commits.length` 抛 TypeError 导致整页白屏；后端在响应出口把 `RecentCommits` / `Dependencies` / `TopContributors` / `TechStack` / `Languages` 归一化为 `[]T{}`，前端补 optional chaining，并加回归测试
- **扫描根目录为空时添加根目录崩溃**：同源问题（`GetConfig` 返回 `scan_roots: null`，且 `db.GetScanRoots` 的错误被 `_` 吞掉）；后端归一化 + 不再吞错，前端 add/remove handler 加 `?? []` 兜底，并加回归测试
- **复制降级**：`wails://` 非安全上下文下 `navigator.clipboard` 不可用，新增 `utils/clipboard.ts`（优先 async Clipboard API，回退 `execCommand('copy')`），详情页与知识库页复用

### 变更

- **详情页设计成熟度三波改进**：右上操作区重排（Copy AI Context 升为主操作、Quick Note 移入侧栏、项目级别 ± 下沉到 meta 区）；头部新增摘要行（主语言 / 最近提交 / 子仓库数）；仓库列表统计标签由「日期: ±N」改为「作者: ±N」，日期移入 hover；趋势图与热力图共用 `ScopeToggle`（周 / 月 / 全部），无活动自动塌缩为空态；侧栏取消 sticky
- 热力图统计计算优化：`today` 提出循环，日期改用 `YYYY-MM-DD` 字符串区间比较，避免逐日 `new Date()` 解析
- 清理 code review 遗留项：删除死代码（`dateParam === 'newNote'` 分支、`.heatmap-title` 死规则、`scope-toggle` 空 class）、消除 `.map(t => ...)` 对 i18n `t` 的遮蔽
- 新增 14 条中英 i18n 键（scope / mainLanguage / lastCommit / groupLevel / noDataInRange / rangeWeek|Month|All 等），双语对齐

### 文档

- 用真实应用截图替换自动生成的仪表盘 / 知识库配图，新增设置页截图
- 新增「存储结构优化与 AI 价值」并补全 AI 集成定位

## [1.7.8] - 2026-09-01

### 修复

- 搜索片段截取越界 panic；schema 版本解析静默吞错
- 收窄笔记版本快照触发条件（不再为无意义变更建快照）；修复首次扫描的新项目没有历史数据

### 变更

- `knowledge` chunk 从 1.83MB 降至 471kB，构建告警清零：`highlight.js` 改用 `lib/common`（37 语言而非全量 190），`mermaid` 改动态 `import()` 按需加载
- 清理 ESLint 10 + react-hooks v7 报出的 7 处问题
- 删除未注册的 MCP 工具文件并统一 gofmt 格式
- 同步 `package-lock.json`，移除 `vite-plugin-pwa` 及其传递依赖
- 升级 brace-expansion 5.0.7 → 5.0.9，修复 high 级 DoS 漏洞

## [1.7.7] - 2026-09-01

### 变更

- 产品正式更名为 RepoNest（旧名 GitBoard）：模块与包路径、文档、可执行文件与文档站徽章一并同步

## [1.7.6] - 2026-09-01

### 修复

- 修复 Wails 绑定命名空间取错导致的桌面端 UI 失效（命名空间由 Go 包名决定：`package app` → `go.app.App`，而非 `go.main.App`）
- code review 的 P0 / P1 / P2 问题全部修复（sprint 6-8 收尾）
- 恢复被误删的 DMG 资源与 MCP `main.go`

## [1.7.5] - 2026-08-31

### 变更

- UI 改版：蓝色主色调，对比度与视觉层级优化

## [1.7.4] - 2026-08-27

### 新增

- 同步远程待办事项
- MCP 笔记工具；`docs/features/ai-integration.md` 的 MCP 工具表补齐为 9 个（含 2 个写操作）

### 变更

- **定位治理（PR1）**：统一对外定位为「本地优先的代码项目上下文库」，核心闭环为「发现本地项目 → 理解项目 → 沉淀知识 → 检索知识 → 交给 AI 使用」；仪表盘与统计降级为支持能力（前端默认页已是知识库，导航顺序 知识库 → 仪表盘 → 设置）。README 截图与功能特性表按核心能力优先重排（知识库 / 项目详情 / AI 接口在前，仪表盘在后）；`docs/positioning-brief.md` 新增「统一对外口径（权威短文案）」节，写入中英文三句定位与 release note。详见 [ADR-0006](docs/adr/0006-scope-freeze.md)。
- **错误一致性与可用性（PR2）**：新增 `web/src/components/ErrorBanner.tsx` 作为页面级错误的统一渲染路径（消息 + 重试按钮 + i18n），替换 Dashboard / ProjectDetail / Knowledge / Settings 四页各自内联的 `error-banner` JSX 与重复 catch 样板。ProjectDetail 与 Settings 的重试改为复用与初次加载相同的加载函数，修复旧实现只 setProject / 漏 reset loading 的半状态问题。`NoteSection` 的 create/save/move/delete/pin/restore/diff 等原本 `/* ignore */` 的静默 catch 改走统一的 `run(op, errMsg)` 包装：失败时 setError 并记录最近失败操作供 ErrorBanner 的重试按钮重放；乐观 pin 失败回滚原状态。
- **TODO 收尾（中优先级）**：补齐 `useConfirmClick` 测试；将 `useApiData`（TTL 缓存 + 请求去重）接入 NoteSection 与 CommandPalette 的「全部项目」下拉列表，共享缓存键 `projects:all`，跨组件只发一次请求；Dashboard 的 projects 拉取现已迁移到 `useApiData`（独立键 `dashProjects`，按 date/starredOnly 作用域，因卡片依赖按日统计），star 切换 `invalidateCache('projects:all')` 使三处组件 starred 状态一致，保留乐观 star 覆盖层避免骨架闪烁；移除 `wails.json` 中从未使用的 `wailsjsdir`；校验 `examples/plugins` 两个示例插件（宿主 SPI 未变，`go build ./...` 通过，预期兼容）；`openapi.json` 契约说明与 `build-docs.mjs` 对 `marked` 的依赖经核实已满足，无额外改动。
- **桌面端路由（HashRouter）**：Wails WebView 在自定义源下 BrowserRouter 的 history/location 变更会抛 DOMException，故桌面壳改用 `HashRouter`、PWA/浏览器仍用 `BrowserRouter`；`spaFallback` 注释同步说明该约定。
- sprint 6-8 代码重构与清理：NoteSection hooks 化、大文件拆分、CSS 死代码清理、懒加载、recover 防护

### 维护

- 依赖升级：wails 2.13.0 → 2.14.0、mcp-go 0.57.0 → 0.58.0、highlight.js 11.11.1 → 11.12.0、katex 0.18.3 → 0.18.4、actions/setup-node 5 → 7
- 从仓库移除 `.omo` 运行态产物

## [1.7.3] - 2026-08-20

### 新增

- 项目上下文主页：快速笔记入口、Copy AI Context、概览空状态引导

### 变更

- 范围冻结 ADR-0006、产品闭环叙事与 CLI/MCP 收敛（#73 #74 #76）

### 修复

- agent-score 的幽灵 CLI 检查替换为 MCP 二进制 + llms.txt 导出检查（#76）；修复 `cliPath` 未定义导致的编译错误

### 测试

- 补齐搜索闭环覆盖：`useApiData` / `useDebouncedCallback` / transport / endpoints（#75）

### 维护

- 加固运行时检查与质量门禁

## [1.7.2] - 2026-08-18

深度代码审查（`docs/code-review/2026-08-18-deep-review.md`）缺陷修复：

### 修复

- **🔴 并发数据库锁**：`InitDB` 限制连接池为单连接（`SetMaxOpenConns(1)` + `SetConnMaxLifetime(0)`）并设置 `PRAGMA busy_timeout=5000`，消除并发 Wails/扫描/插件访问导致的 `database is locked`
- **🔴 知识缓存静默失效（存量库）**：新增 v10 幂等迁移，为早期版本创建的 `repo_meta` 补齐 `dependencies` / `top_contributors` / `activity` 三列（`createTables` 新建表已含，存量库需此修复才能命中缓存）
- **🟠 知识源状态误报**：插件导入 `TriggerImport` 的 `lastErr` 改为记录逐文档 upsert 真实错误，知识源 `Enabled` 不再恒为 true
- **🟠 大仓库挂起**：`DetectContributors` 用 30s 上下文超时包裹 `git shortlog`
- **🟠 首屏阻塞**：`GetProjects` / `GetProjectStats` 的按需 git 统计刷新移至后台 goroutine，仪表盘/概览首开不再卡顿
- **🟠 `git_author` 配置生效**：运行时可设置个人作者，覆盖自动检测的 `git user.name`（"我的"统计/热力图/最近提交随之更新）
- **🟡 健壮性**：`mineAndCache` 记录 `UpsertRepoMeta` 错误而非吞掉；`Mine` 返回非 nil 切片避免 JSON `null`；按语言行数统计 scanner 缓冲放大到 16MB（兼容 minified 文件）；`daily_stats` 新增真实提交数 `commits` 列（此前热力图误用 `COUNT(DISTINCT author)`）

### 维护

- **🟡 `InferRepoMeta` 无超时**：派生仓库展示名时读取 `git config user.name` 改用 30s 上下文超时包裹，避免挂掉的 working tree 阻塞扫描/发现路径
- **🟡 `refreshProjectStatsForDate` 缺失非零守卫**：与 `refreshRepoStatsRange` 对齐，git 出错返回的全 0 `Result` 不再写入每日统计行（原会令仪表盘显示「0」而非「无数据」，掩盖错误）
- **版本号对齐**：`internal/version/version.go` 经 `scripts/bump-version.sh` 同步至 `1.7.2`（`wails.json` / `web/package.json` 一并更新），消除应用内报告版本与 tag 长期漂移

## [1.7.1] - 2026-08-18

### 修复

- 修复 ESLint 被 TypeScript 7.0 兼容性阻塞问题：降级 TypeScript 至 6.0.3，修复 react-hooks/refs 违规（4 个 hook），修复 set-state-in-effect 违规（7 个文件），修复 markdown.ts 不必要转义和 seo.ts 缺失依赖

## [1.7.0] - 2026-08-17

深度重构版本：后端服务化、前端组件化，行为保持不变（除下述明示的修复与契约变更）。决策记录见 [ADR-0005](docs/adr/0005-service-layer.md)。

### 新增

- **internal/service 业务层**：Wails 桌面、CLI、MCP 三端共享同一实现，消除三处重复的查询/格式化逻辑
- **internal/app 薄绑定层**：根目录 14 个 handler 文件（约 1900 行）收敛为每方法 1-3 行委托；`package main` 只剩 `main.go`
- **internal/domain / internal/diff / internal/version**：跨层行类型独立、笔记行级 diff 独立成包、四处硬编码版本号统一为单一常量
- **db 层按域拆分**：1195 行 `queries.go` 拆为 projects / notes / note_versions / todos / repositories / daily_stats / repo_meta / config / scan_roots / search / cleanup；项目升降级 SQL 事务化为受测的 `SplitProjectDown` / `MergeProjectUp`
- **测试补齐**：knowledge 解析（含 go.mod 块状 require）、scanner、diff、项目拆分/合并事务、热力图项目过滤、service 层（fake git provider）；db 测试改用真实 `InitDB` schema（消除手抄 DDL 漂移）
- **前端 API 层拆分**：627 行 client.ts → types / transport / endpoints，统一 `call()` 路由
- **前端 hooks 层**：`useApiData`（TTL 缓存 + 请求去重，待接入页面）、`useDebouncedCallback`、`useScanPolling`、`useConfirmClick`
- **组件拆分**：Dashboard 搜索下拉、Settings 六个 tab、NoteSection 统一 NoteEditor + 版本历史面板、ProjectDetail 概览面板、Knowledge 卡片
- 日志路径按平台（Linux `$XDG_STATE_HOME`、Windows `%APPDATA%\reponest\logs`），修复非 macOS 平台写入 `~/Library/Logs`
- MCP server 进程内单次开库（此前每次工具调用都执行全套迁移）
- vitest + ESLint 工具链（ESLint 受 TS7 兼容性阻塞，见 [TODO](TODO.md)）；tsconfig 恢复 `noUnusedLocals/Parameters`
- 仓库卫生：移除误提交的 20MB 二进制与 AI 工具产物目录，遗留脚本归档至 `scripts/legacy/`

### 修复

- **存量 bug：repository 查询引用不存在的列**（`display_name` / `git_user` / `organization` 从未建列），导致项目仓库列表、扫描后统计刷新、状态栏最近提交、llms.txt 仓库目录在生产环境**全部静默失败**；已核对真实用户数据库确认并修复
- **存量 bug：go.mod 块状 `require (...)` 解析越界 panic**，可致项目概览后台挖掘崩溃；解析器重写并支持单行 + 块状两种形式
- 前端 Rules-of-Hooks 违规（普通函数内调用 `useTranslation`）
- toast 双重定时器互相重置；`EventsOn` 监听器随语言切换累积泄漏（现真实退订）
- 9 处裸 `<a href>` 全页刷新破坏 SPA；ProjectCard 中 `<button>` 嵌套 `<a>` 的非法 HTML/a11y 问题
- **项目详情页热力图显示全局数据**：`GetHeatmapData` 新增 `projectId` 参数（契约变更，前端已适配）
- 笔记两击确认删除、防抖搜索、扫描轮询等三处重复实现合并

### 变更

- 移除死代码约 1100 行：旧扫描管线（`ScanForRepositories` 等）、未使用的 `ExportProjectStats` / `ExportHeatmapCSV` / `GetNoteVersion` / 分支查询 / storage 垫片等（绑定面变更已同步至 API 参考）
- 约 300 行硬编码中文提取至 zh-CN / en locale 文件
- `GetStatusBar` 改为双检锁（不持锁执行 git 命令）；`ToggleProjectStar` 原子化（TOCTOU）；`ReorderTodos` 单事务（自 1.6.x 移植）

## [1.6.3] - 2026-08-11

### 修复

- 代码质量专项：CSV 注入防护（csvSafe）、TOCTOU 与事务化修复（ToggleProjectStar / ReorderTodos）、状态栏锁优化、`wail()` 空守卫、`ensurePath` 去重、跨平台日志目录（`getLogDir`）

## [1.6.2] - 2026-08-11

### 变更

- 第二轮代码质量清理：错误包裹（`%w`）、helpers 归并、`refreshStatsForRepo` 抽取

## [1.6.1] - 2026-08-10

### 新增

- 产品正式更名为 RepoNest（旧名 GitBoard，当时仅 module/包路径级引用待跟进）
- 记录产品定位决策 ADR 0002，并标记 RFC 0001（插件平台）为 Superseded
- 社区健康文件（issue #25）：CHANGELOG / CONTRIBUTING / SECURITY / CODE_OF_CONDUCT / SUPPORT / Issue+PR 模板 / Dependabot
- 进程内插件系统：yaegi 脚本运行时（目录扫描 / 加载 / 事件总线 / panic 隔离），插件接口见 `internal/core/plugin`
- Claude 记忆导入重构为内置 KnowledgeImporter 插件，与脚本插件共享运行时导入/去重/统计路径
- 知识源导入触发：启动自动导入（可开关）+ 设置页手动触发 + 前端 toast 结果通知
- 知识库升级为首页，支持快速创建笔记（issue #31）
- 知识库体验增强（issue #37）：首屏搜索框自动聚焦、顶部「最近编辑」快速访问区、空状态引导创建或导入 AI 记忆、编辑器「关联项目」下拉快速迁移笔记（新增 `MoveNote` API）
- 设计系统重构（issue #9）：拆分单体 global.css 为设计系统 token + 组件样式，LCH 自适应色板
- Markdown 渲染增强（issue #10）：highlight.js 语法高亮、Mermaid 图、GFM callout、KaTeX 数学公式、任务列表样式
- 全局无障碍基线（issue #12）：skip-link + focus-visible + reduced-motion
- 命令面板无障碍（issue #11）：focus trap + ARIA dialog/listbox
- BrowserRouter 路由升级（issue #13）：可分享 URL，Pages `_redirects` fallback
- PWA 规范修复（issue #14）：theme-color 一致 + 离线 fallback + 安全加固
- SEO 规范（issue #21）：OG / Twitter Card / canonical / sitemap
- AI-ready 内容分发层（issue #15）：llms.txt + .md 路由 + 问答入口
- FTS5 全文搜索升级（issue #18）：FTS5 + 中文分词 + 相关性排序 + snippet 高亮
- 笔记版本历史 + Diff view（issue #16）：复用本地 Git，超越 GitBook CRUD
- 仓库知识挖掘加深（issue #20）：LOC / 依赖图 / 活跃度 / 贡献者
- 安全规范（issue #24）：CSP / HSTS / 安全策略声明 / 依赖扫描
- i18n 框架（issue #23）：字符串外提 + hreflang + 语言切换（zh-CN / en）
- 用户文档站（issue #26）：使用手册 / 教程 / FAQ（docs/ 落地页）
- API 参考文档（issue #27）：OpenAPI 渲染 + 端点说明
- OpenAPI spec + REST 版本化（issue #22）+ OpenAPI 自动渲染与 Try-it（issue #17）
- CLI + MCP + agent-score（issue #28）：`reponest-mcp`、`tools/agent-score` 自检工具

## [1.5.7] - 2026-08-10

### 新增

- 块编辑器（issue #19）：Markdown 双轨编辑，输入 `/` 呼起 block 面板插入 callout/tabs/details/代码/Mermaid/公式等结构化块，块级拖拽排序与增删，编辑产物仍为可读 Markdown
- 桌面端深链路由兜底：Wails AssetServer 对未命中的 GET 请求回退 `index.html`

## [1.5.6] - 2026-08-10

### 修复

- 扫描稳定性与收藏同步修复

## [1.5.5] - 2026-08-10

### 新增

- 启动时历史回填、收藏同步及默认扫描根目录播种

## [1.5.3] - 2026-07-25

### 修复

- 全量扫描策略优化为合并同步，增强跨平台支持

## [1.5.2] - 2026-07-25

### 新增

- 全面更新应用图标系统

## [1.5.1] - 2026-08-01

### 新增

- 优化扫描与性能，支持自定义扫描路径

## [1.5.0] - 2026-08-01

### 新增

- 统一 UI 配色为灰阶并优化仪表盘布局
- 增强数据序列化与前端健壮性

## [1.4.0] - 2026-07-01

### 新增

- 知识库：跨项目笔记中心（Markdown、标签、置顶、全文搜索）
- 仓库知识挖掘：README / 技术栈 / 语言占比
- Claude 记忆导入
- 命令面板（⌘/Ctrl+K）
- PWA 可安装

## [1.3.0] - 2026-06-01

### 新增

- 智能项目分组（Monorepo 识别、级别调整）
- 仓库收藏与按需刷新历史
- 项目详情页：趋势折线图、提交热力图

## [1.2.0] - 2026-05-01

### 新增

- 仪表盘：每日目标进度环、项目卡片、工作日检查

## [1.1.0] - 2026-04-01

### 新增

- 自动发现本地 Git 仓库与基础提交统计
- 模糊搜索

## [1.0.0] - 2026-03-01

### 新增

- 首个正式版本：Wails 桌面应用骨架、GitHub Actions 多平台构建发布

[Unreleased]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.15.1...HEAD
[1.15.1]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.15.0...v1.15.1
[1.15.0]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.14.4...v1.15.0
[1.14.4]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.14.3...v1.14.4
[1.14.3]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.14.2...v1.14.3
[1.14.2]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.14.1...v1.14.2
[1.14.1]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.14.0...v1.14.1
[1.9.5]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.9.4...v1.9.5
[1.7.9]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.8...v1.7.9
[1.7.8]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.7...v1.7.8
[1.7.7]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.6...v1.7.7
[1.7.6]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.5...v1.7.6
[1.7.5]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.4...v1.7.5
[1.7.4]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.3...v1.7.4
[1.7.3]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.2...v1.7.3
[1.7.2]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.1...v1.7.2
[1.7.1]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.7.0...v1.7.1
[1.7.0]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.6.3...v1.7.0
[1.6.3]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.6.2...v1.6.3
[1.6.2]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.6.1...v1.6.2
[1.6.1]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.6.1
[1.5.7]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.5.5...v1.5.7
[1.5.6]: https://github.com/sky-jiangcheng/repo-nest/compare/v1.5.5...v1.5.6
[1.5.5]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.5.5
[1.5.3]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.5.3
[1.5.2]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.5.2
[1.5.1]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.5.1
[1.5.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.5.0
[1.4.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.4.0
[1.3.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.3.0
[1.2.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.2.0
[1.1.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.1.0
[1.0.0]: https://github.com/sky-jiangcheng/repo-nest/releases/tag/v1.0.0
