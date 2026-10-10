# P35 第二轮：BlockEditor + StatusBar 迁移

**改动状态**：已就地改在本机仓库工作区，**未提交**（与上一轮 `main.css` 清理一起等你审阅）。

---

## 1. 本轮做法：先做全站审计，再挑簇

不再逐文件猜，而是脚本化核了一遍：把 `web/src/styles/**` 下 24 个 CSS 文件定义的每个类，
拿去和 80 个 `.ts`/`.tsx` 文件做 token 匹配，按引用文件数分类。结果：

- **独占类（全站只有 1 个组件引用）170 个**
- **无引用类（没有任何组件用）32 个** — 死代码，待清理

本轮从审计结果里挑了边界最干净的两个「一文件对一组件」簇：

| 簇 | 定义文件 | 引用方 | 判定 |
|---|---|---|---|
| BlockEditor | `features/block-editor.css`（18 类） | 仅 `components/BlockEditor.tsx` | 整文件迁入 |
| StatusBar | `features/status-bar.css`（7 类） | 仅 `components/StatusBar.tsx` | 整文件迁入 |

两处跨组件类的处理（module 内必须 `:global()` 包裹）：

- `.block-palette-item.active` → module 内写 `.blockPaletteItem:global(.active)`。`.active`
  是裸工具类，全站 17 个文件在用，必须留全局。
- `.status-item.muted` → `.statusItem:global(.muted)`。`.muted` 被 `SummaryBar.tsx` 一并使用，
  留全局。
- `.form-input`（inputs.css 基元）在 TSX 里保留全局类名，只把组件自己的类换成 module 类。

## 2. 实际改动（8 个文件）

- `web/src/components/BlockEditor.module.css` — **新增**，16 类（含 `.active` 复合选择器）。
- `web/src/components/BlockEditor.tsx` — 加 module 导入，16 处类名改写；其中
  `block-editor block-editor-empty`、`block-item` + 动态 `block-item-dragging`、
  `block-palette-item` + 动态 `active`、`block-btn block-btn-danger` 四处改为模板串。
- `web/src/components/StatusBar.module.css` — **新增**，6 类 + `@keyframes pulse`；
  并把原先住在 `styles/index.css` 的 `.status-dot-error` 收进来（它只有 StatusBar 用）。
- `web/src/components/StatusBar.tsx` — 加 module 导入，11 处类名改写。
- `web/src/styles/index.css` — 删掉 `block-editor.css`、`status-bar.css` 两条 `@import`
  和 `.status-dot-error` 规则块。
- `web/src/styles/semanticInk.test.ts` — 文件清单删掉这两条（该清单必须与 index.css 的
  导入顺序一致；上一轮删 `features/heatmap.css` 是同一处理）。
- `web/src/styles/design-system/features/block-editor.css` — **删除**。
- `web/src/styles/design-system/features/status-bar.css` — **删除**。

## 3. 校验结果

- `s.*` 引用与 module 类名逐一比对：BlockEditor 17 个引用、StatusBar 7 个引用，
  **无缺失定义、无定义了却没引用的类**（CSS module 的类型不会因拼错报错，这步必须手工核）。
- `npm run build`（tsc + vite）：✓ built in 571ms
- `npm run lint`（eslint src）：零错
- `npm test`（vitest）：15 文件 / 108 用例全过（含 semanticInk.test.ts）
- 全局 CSS 已无任何 `.block-*` / `.status-*` 定义

## 4. 需人工实机复核

1. 笔记编辑器的块编辑区：块悬停边框、拖拽中半透明、`/` 块面板定位（`left: 84px`、
   `top: calc(100% - 4px)`）、面板项 hover 与选中态、上下移/插入/删除按钮的禁用态。
2. 底部状态栏：左右分组的间距与换行、连接异常时的红点（`.statusDotError` 带 `!important`）、
   「无提交」那条的 muted 灰字、768px 下改纵向排列。

## 5. 本轮顺带发现

- `components/StatusBar.tsx:53` 的 `className="status-warn"` 全站检索**无任何 CSS 定义**。
  与之前记的 `ai-url-row`、`knowledge`、`knowledge-section` 同类（空壳类），一并待定性后清理。
- `features/status-bar.css` 的 `.status-bar` 曾被 `api/endpoints.ts` 命中，是字符串误报，
  非类名引用——审计脚本的 token 匹配需要人工过一遍这类误报。

## 6. 后续轮次候选（按审计结果）

| 簇 | 定义文件 | 拆给谁 |
|---|---|---|
| QuickCaptureFab + AIAskPanel | `features/fab.css`（19 独占） | QuickCaptureFab 12 类；AIAskPanel 7 类（其 module 已存在，追加即可） |
| NoteEditor + VersionHistoryPanel | `features/notes.css`（22 独占） | 各 11 类；注意 `note-textarea`/`note-title-input`/`note-tags-input` 与 inputs.css 共享族重名，按 `.date-input` 的办法只迁组件自有规则 |
| Knowledge 命中列表 + 项目搜索 | `components/search.css`（22 独占） | `hit-*` 7 类归 Knowledge；`search-*` 15 类归 ProjectSearchDropdown |
| CommandPalette | `components/command-palette.css`（14 独占） | 整文件迁入 |
| Toast | `components/toast.css`（6 独占） | 整文件迁入 |
| TodoSection | `features/todos.css`（6 独占） | 整文件迁入 |
| 零散残留 | `cards.css`/`inputs.css`/`main.css`/`reset.css`/`dashboard.css`/`project-detail.css` | `.skeleton-card`→Dashboard、`.tech-chip`→ProjectOverviewSection、`.badge-*`→ProjectCard/NoteSection、`.toggle-*`→PluginsTab、`.skeleton-value`→SummaryBar、`.skip-link`→App、`.card-refresh-btn`+`.spin`→ProjectCard、`.empty-section`→ProjectDetail、`.heatmap-empty-state`→Heatmap、`.status-dot-error`（已处理） |

另有 32 个无引用类（如 `.btn-active`、`.meta-pill`、`.stat-tag`、`.team`、`.tech-framework`、
`.callout-*` 的 6 个变体、`.toast-success/error/info`、`.cmdk-type-note/todo`、`.hit-type-note/todo`、
`.form-select`、`.page-head`、`.page-sub`、`.section-header-row`、`.lang-flag` 等）属死代码，
建议单独一轮统一清理并写进 CHANGELOG。
