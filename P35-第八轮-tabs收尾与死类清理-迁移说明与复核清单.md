# P35 第八轮：tabs.css 收尾 + 死类清理 — 迁移说明与复核清单

范围：`components/tabs.css` 剩下的三个单组件类、`features/markdown.css` 的 `app-icon` 判定，以及全站零引用死类重核。
本轮不提交、不推送，改动全部停在工作区。

## 一、迁移清单

| 全局类 | 原位置 | 去向 | 备注 |
| --- | --- | --- | --- |
| `.settings-tabs` | tabs.css | **新建 Settings.module.css** | 3 条：基础规则 + `@media (max-width: 640px)` 内的收窄规则 + `::-webkit-scrollbar`；sticky 定位那段长注释一并迁入 |
| `.note-filters` | tabs.css | **新建 NoteFilterBar.module.css** | 3 条：基础规则 + `.note-filters .filter-btn` + `.note-filters .filter-btn.active` |
| `.range-toggle` | tabs.css | **新建 ScopeToggle.module.css** | 2 条：基础规则 + `.range-toggle .btn` |
| `.app-icon` | markdown.css | **本轮不动** | 它唯一的规则是 `.callout-header .app-icon`，父级 `.callout-header` 由 `utils/markdown.ts` 生成 HTML 字符串，迁进 module 得反过来 `:global(.callout-header)` 依赖工具模块的类名，收益不抵耦合，留全局 |

跨组件共享类仍留全局，迁入的 module 里用 `:global()` 引用：`.noteFilters :global(.filter-btn)`、`.rangeToggle :global(.btn)`。

**死类清理**（先按模板串逐条复核，确认无动态生成后才删）：
- 删除：`btn-active`（buttons.css）、`badge-info` / `meta-pill` / `stat-tag` / `team`（cards.css）、`tabs.css` 里 `.range-toggle .btn-active` 那条 legacy 旧变体（它指向的类已零引用，随类一起清）。
- **保留**（复核后判定为活类或库生成，删了会真回归）：`hit-type-note` / `hit-type-todo`（由 `hit-type-${h.type}` 拼接）、`callout-caution` / `callout-important` / `callout-question` / `callout-tip` / `callout-warning`（由 `callout-${c.type.toLowerCase()}` 拼接）、`katex-display`（KaTeX 输出）、`mermaid-render`、`task-list`（markdown-it 任务列表插件输出）、`settings-head` / `page-sub` / `section-header-row`（模板串前缀有命中风险，本轮不动）、`form-select`（嵌在 inputs.css 共享族选择器组里）。

规模（口径与上一轮一致）：全局 CSS **1,222 行 / 17 文件**（上一轮 1,310 / 17）；module CSS **4,435 行 / 35 文件**（上一轮 4,318 / 32，+3 为新模块）。

## 二、校验结果

- `npm --prefix web run build` ✓、`npm --prefix web run lint` 零输出、`npm --prefix web test` **15 文件 / 108 用例全过**。
- 类名对账：3 个新 module 的定义与 `s.` 引用一致，无未定义、无未引用。
- 产物残留：当前入口 CSS + Settings 分片里，`note-filters` / `range-toggle` / `settings-tabs` / `btn-active` / `badge-info` / `meta-pill` / `stat-tag` 全部零残留。
- 过程中修掉：① 生成类名时把复合词压平了（`notefilters`），已改回 `noteFilters` / `rangeToggle` / `settingsTabs`；② `tabs.css` 里那条 legacy `.range-toggle .btn-active` 旧变体在第一次清理时因为选择器同时含两个类而被漏掉，已单独删除。

## 三、需人工实机复核项

1. 设置页顶部页签条：滚动时是否仍然吸顶（`position: sticky` + `top: -32px` + 负 margin 那套），以及 ≤640px 下页签条横向滚动、两侧不留白。
2. 知识库/笔记列表的筛选条（NoteFilterBar）：默认态与选中态（选中态来自全局 `.filter-btn.active`，现在由 module 内的 `:global()` 规则命中）。
3. 概览页时间范围切换（ScopeToggle）：两个按钮的间距与内边距。
4. 顺带确认 Dashboard 项目卡上被清掉的 `badge-info` / `meta-pill` / `stat-tag` 没有残留渲染（这几个类已零引用，理论上无影响）。

## 四、后续候选

1. `tabs.css` 现在只剩跨组件共享的 `.btn` / `.tab-btn` / `.tab-active` / `.filter-btn` / `.filter-toggle` 一族，属「共享族」，需等全局 CSS 收口决策。
2. `markdown.css` 整体同理（工具模块生成 markup 特例）。
3. 剩余待定类：`settings-head` / `page-sub` / `section-header-row` / `form-select` / `org` / `w3`（inputs.css 里两个用途不明的零引用类，需先查清来源再删）。
4. `main.tsx` 样式导入顺序决策项仍未动。
