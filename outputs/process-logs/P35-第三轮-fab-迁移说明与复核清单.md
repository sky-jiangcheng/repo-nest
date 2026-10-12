# P35 第三轮：fab.css 拆分（QuickCaptureFab + AIAskPanel）

**改动状态**：已就地改在本机仓库工作区，**未提交**（与第一轮 `main.css` 清理、第二轮 BlockEditor/StatusBar 一起等你审阅）。

---

## 1. 边界判定

`features/fab.css` 定义 22 个类，按引用方拆成三份：

| 类 | 引用方 | 判定 |
|---|---|---|
| `.fab-ball` / `.fab-backdrop` / `.fab-panel` / `.fab-panel-head` / `.fab-title` / `.fab-close` / `.fab-project` / `.fab-kinds` / `.fab-actions` / `.fab-empty` / `.fab-tabs` / `.fab-tab`（12 类） | 仅 `QuickCaptureFab.tsx` | 迁入新 module |
| `.ai-ask-body` / `.ai-hint` / `.ai-hint-project` / `.ai-hint-config` / `.ai-actions` / `.ai-answer` / `.link-btn`（7 类） | 仅 `AIAskPanel.tsx` | 追加进已有 module |
| `.fab-content` | **AIAskPanel + QuickCaptureFab 都用** | **留全局** |

三处跨组件类的处理：

- `.fab-tab.active` → module 内 `.fabTab:global(.active)`。`.active` 是裸工具类，全站 17 个文件在用。
- `.ai-actions .btn` → `.aiActions :global(.btn)`。`.btn` 是 buttons.css 基元，23 个文件在用。
- `.fab-content` 留在 `fab.css`：它是两个组件共用的面板文本域（都写成 `fab-content form-input`），
  归任何一个组件都不对。`.form-input` 是 inputs.css 基元，一并留在 TSX 里。

## 2. 实际改动（5 个文件）

- `web/src/components/QuickCaptureFab.module.css` — **新增**，12 类。
- `web/src/components/QuickCaptureFab.tsx` — 加 module 导入，13 处类名改写
  （含 2 处 `fab-tab` + 动态 `active` 模板串、1 处 `fab-project form-input` 保留基元）。
- `web/src/components/AIAskPanel.module.css` — 追加 8 类（7 个 ai-*/link-btn + `:global(.btn)` 规则）。
- `web/src/components/AIAskPanel.tsx` — 8 处类名改写（该文件原本已导入 `s`）。
- `web/src/styles/design-system/features/fab.css` — 22 → 1 类，只留共享的 `.fab-content` 并写明边界注释。

`semanticInk.test.ts` 无需改动：fab.css 仍在，`index.css` 的导入行不变。

## 3. 校验结果

- `s.*` 与 module 类名比对：AIAskPanel 18 个引用、QuickCaptureFab 12 个引用，
  **无缺失定义、无定义未引用**。
- 残留 kebab 类名检查：只剩 `fab-content`（共享，预期保留），`filter-btn` 属 tabs.css 全局类未动。
- `npm run build`（tsc + vite）：✓ built in 523ms
- `npm run lint`（eslint src）：零错
- `npm test`（vitest）：15 文件 / 108 用例全过

## 4. 需人工实机复核

1. 悬浮球本体：`right: 22px` / `bottom: 52px` 定位、hover 放大到 1.06、层级在状态栏之上。
2. 面板：宽 380px、`z-index: 96`（遮罩 95、悬浮球 90）、面板内上下间距。
3. 两个 tab 的选中态（`.fabTab:global(.active)` 的底色/加粗/阴影）——这条依赖 `:global()` 写法，是重点。
4. AI 问答 tab：hint 三态（普通 / 项目名加粗 / 未配置时的 warning 色）、按钮行 `:global(.btn)` 的图标对齐与 5px 间距。
5. 捕获 tab：类型选择 chips、项目下拉宽度、文本域可纵向拉伸。

## 5. 后续轮次候选

| 簇 | 定义文件 | 拆给谁 |
|---|---|---|
| NoteEditor + VersionHistoryPanel | `features/notes.css`（22 独占） | 各 11 类；`note-textarea`/`note-title-input`/`note-tags-input` 与 inputs.css 共享族重名，按 `.date-input` 办法只迁组件自有规则 |
| Knowledge 命中列表 + 项目搜索 | `components/search.css`（22 独占） | `hit-*` 7 类归 Knowledge；`search-*` 15 类归 ProjectSearchDropdown |
| CommandPalette | `components/command-palette.css`（14 独占） | 整文件迁入 |
| Toast | `components/toast.css`（6 独占） | 整文件迁入 |
| TodoSection | `features/todos.css`（6 独占） | 整文件迁入 |
| 零散残留 | `cards.css`/`inputs.css`/`main.css`/`reset.css`/`dashboard.css`/`project-detail.css` | `.skeleton-card`→Dashboard、`.tech-chip`→ProjectOverviewSection、`.badge-*`→ProjectCard/NoteSection、`.toggle-*`→PluginsTab、`.skeleton-value`→SummaryBar、`.skip-link`→App、`.card-refresh-btn`+`.spin`→ProjectCard、`.empty-section`→ProjectDetail、`.heatmap-empty-state`→Heatmap |

另有 32 个无引用类（死代码）建议单独一轮统一清理并写进 CHANGELOG。
累计空壳类（markup 在用、全站无 CSS 定义）已有 4 个：`knowledge`、`knowledge-section`、`ai-url-row`、`status-warn`。
