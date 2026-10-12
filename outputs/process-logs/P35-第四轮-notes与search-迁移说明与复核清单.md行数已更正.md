# P35 第四轮：`notes.css` 与 `search.css` 两个簇迁移

> 仓库：`/Users/jiangcheng/Workspace/Script/RepoNest`（Go + Wails v2 + React）
> 日期：2026-10-09　状态：**已改在工作区，未提交**（按约定等你审阅后再落库）
> 对应 TODO：P35「逐组件迁移」条目；CHANGELOG 口径上这是第 9、10 块（前一块是 fab.css 拆分）
> 累计规模：全局 CSS 2,562 → **2,191 行**（22 → 21 文件），module CSS 2,997 → **3,425 行**（22 → 25 文件）　*（原记 3,400 行，第五轮逐文件实测后更正）*

---

## 一、迁移清单

### 簇 A：`features/notes.css`（3,225 B，23 类）→ 两个 module

| 去向 | 类 | 处数 |
|---|---|---|
| 新建 `components/notes/NoteEditor.module.css`（11 类） | `note-editor-block` `note-meta-row` `note-title-input` `note-kind-select` `note-tags-input` `note-textarea` `note-editor-actions` `note-editor-split` `note-preview` `draft-hint` `note-pin-toggle` | TSX 13 处 |
| 新建 `components/notes/VersionHistoryPanel.module.css`（11 类） | `version-history-panel` `version-history-header` `version-list` `version-item` `version-time` `version-title` `version-actions` `diff-panel` `diff-pre` `diff-line-add` `diff-line-del` | TSX 11 处 |

- `notes.css` **整文件删除**；`styles/index.css` 去掉对应 `@import`；`semanticInk.test.ts` 的清单同步删掉 `'features/notes.css'`（该清单必须与 `index.css` 的导入顺序一致，删文件不同步会让测试读到的级联顺序与真实顺序不符）。
- 死类 `.note-editor`（`margin-bottom: 14px`）**删而不迁**：全站（含模板串拼接）零引用，按 Knowledge 轮 `heatmap.css` 的同一判例处理。
- 两个 module 各 11 类，`s.*` 引用 13 / 11 处——定义与引用一一对应，无缺失、无冗余（见第三节对账）。

### 簇 B：`components/search.css`（3,579 B，24 类）→ 两个去向

| 去向 | 类 | 处数 |
|---|---|---|
| 追加进已有的 `pages/Knowledge.module.css`（+7 类，现 35 类） | `hit-list` `hit-item` `hit-head` `hit-type` `hit-project` `hit-title` `hit-snippet` | TSX 9 处 |
| 新建 `pages/dashboard/ProjectSearchDropdown.module.css`（17 类） | `search-box` `search-input` `search-dropdown` `search-loading` `search-empty` `search-result-item` `search-result-header` `search-result-project` `search-result-preview` `search-result-title` `search-group` `search-group-header` `search-result-project-item` `search-project-name` `hit-type-mini` | TSX 18 处 |
| **留全局** `search.css`（22 → 2 类） | `hit-type-note` `hit-type-todo` | 两个组件动态引用 |

- `search.css` 未删除，收为 12 行：只剩两条跨组件共享的类型徽章配色，文件头写明为什么留下。

---

## 二、边界判定依据（本轮新增/沿用的判据）

1. **动态类必须按模板串复核，不能只看审计结果。** 上一轮的归属审计把 `.hit-type-note` / `.hit-type-todo` 报成「零引用死类」，实际两个组件都写成 `` className={`hit-type hit-type-${h.type}`} ``——字符串里没有字面量，正则匹配不到。**它们是活的**，且被 Knowledge 与 ProjectSearchDropdown 共用，因此按「跨组件共享留全局」留在 `search.css`；markup 里只把基类换成 module 类，动态部分继续用全局名拼接（`` `${s.hitType} hit-type-${h.type}` ``）。这与 Heatmap 轮 `level-${n}` 改查表是两类情况：那里类名要进 module 作用域才需要查表，这里类名留在全局就不需要。
2. **复合选择器跨作用域时用 `:global()` 复合。** `search.css` 的 `.hit-type-mini.hit-type-note`（同一元素两个类）下沉到 Dropdown module 后写成 `.hitTypeMini:global(.hit-type-note)`——**不能**写成后代选择器（那会变成「祖先 + 后代」，静默失效）。
3. **共享输入族重名类沿用 `.date-input` 的处理**：`.note-title-input` / `.note-tags-input` / `.note-textarea` / `.search-input` 都列在 `components/inputs.css` 的族选择器组里（基础态 / `:hover` / `:focus` / `::placeholder`），所以 markup **保留全局类名**，module 里只叠加组件自有规则，例如 `` `form-input note-textarea ${s.noteTextarea}` ``。`.note-kind-select` 不在族里，完整下沉（`form-input` 基元仍来自全局）。
4. **`markdown-body` / `card-star` / `starred` / `empty-hint` / `.btn` 保持全局**：预览区 `note-preview markdown-body` 只把前者换成 `s.notePreview`；搜索下拉里的 `card-star ${is_starred ? 'starred' : ''}` 一字未动。

---

## 三、本轮撞上的真问题：产物里级联顺序与 dev 相反

**现象**：改完之后 `npm run build` 通过、测试全绿，但用真实浏览器量计算样式发现 3 条 module 覆盖被吃掉：

| 元素 | 属性 | 迁移后实测（修复前） | 应为 |
|---|---|---|---|
| `.note-textarea` | `font-family` | 系统 sans（`-apple-system…`） | `SF Mono, Menlo, Monaco, Consolas, monospace` |
| `.note-textarea` | `font-size` | `13px` | `12px` |
| `.note-tags-input` | `font-size` | `13px` | `12px` |
| `.search-input` | `padding-left` / `padding-top` | `14px` / `9px` | `32px` / `7px`（左侧要让出放大镜图标） |
| `.search-input` | `transition-property` | `border-color, box-shadow` | `all`（focus 时宽度 200→260px 的过渡） |

**根因**（与本次迁移无关的既有结构）：`main.tsx` 先 `import App`（第 3 行）再 `import './styles/index.css'`（第 4 行），而 `App` 里是 `lazy(() => import('./pages/Knowledge'))` 这类路由。产物 `index.html` 的样式表顺序因此是 **dashboard → knowledge → projectDetail → index**，设计系统 CSS 排在最后：

```
<link rel="stylesheet" href="./assets/dashboard-*.css">
<link rel="stylesheet" href="./assets/knowledge-*.css">
<link rel="stylesheet" href="./assets/projectDetail-*.css">
<link rel="stylesheet" href="./assets/index-*.css">   ← 设计系统在这里
```

于是所有「同特异性、靠后赢」的覆盖都翻了个方向：dev 下 module CSS 注入在 `index.css` 之后（组件赢，正常），产物里 module CSS 在前（基态赢）。**这就是「CSS 改名 CI 完全无感」的另一种形态——改名没错、类也没丢，只是级联顺序翻了。**

**本轮的处理**：给这三条规则用仓库已有的 doubled 选择器写法把特异性提到 (0,2,0)，与顺序无关：

```css
.noteTextarea.noteTextarea { … }
.noteTagsInput.noteTagsInput { … }
.searchInput.searchInput { … }
```

只在**与族基态存在同属性冲突**的三条上加，`.note-title-input`（flex/min-width/font-weight）与 `.note-kind-select`（width/max-width）与基态没有同属性冲突，保持原样——这一点用浏览器实测确认过（`font-weight: 500`、`min-width: 120px`、`width: 150px` 等 module 值都正常生效）。

**修复后实测**（同一探针页、同一批元素）：`font-family` 回到 mono、`font-size` 12px、`padding-left` 32px、`transition-property: all`，其余元素维持原值。

> **建议的根治方案（等你决策，本轮没动）**：把 `main.tsx` 第 4 行的 `import './styles/index.css'` 提到 `import App` 之前（一行）。这样产物里设计系统在前、组件 module 在后，dev 与产物行为一致，本轮这三处 doubled 也可以撤掉。风险面是全局的：它会把目前「基态赢」的地方翻回 module 值——理论上那才是组件作者的本意，但需要按屏过一遍（Dashboard / 知识库 / 项目详情 / 设置 / 审核 / 笔记编辑器）。**这是结构性决策，比继续迁一两个簇重要**，所以单独列出来而不是顺手改。

---

## 四、校验结果

| 项 | 结果 |
|---|---|
| `npm --prefix web run build`（tsc + vite） | ✓ 591ms |
| `npm --prefix web run lint`（eslint src） | ✓ 零错 |
| `npm --prefix web test` | ✓ 15 文件 / 108 用例全过 |
| `s.*` 对账（module 定义 vs 组件引用） | NoteEditor 11/11、VersionHistoryPanel 11/11、ProjectSearchDropdown 17/15、Knowledge 35/31 |
| 未引用项定性 | PSD 的 2 项是 `:global(.hit-type-note/-todo)`；Knowledge 的 4 项是上一轮遗留的 `:global(.btn)` / `.filter-btn` / `.page-head-actions` / `.pinned-active`——都是共享名，属预期 |
| 残留扫描 | 四个组件里本轮类名的字面量残留 **0**；`hit-type-${h.type}` 动态拼接在两个组件都保留 |
| 构建产物 | 旧类名（`note-editor-block` / `version-history-panel` / `search-dropdown` / `hit-list` / `diff-panel` 等）在产物 CSS 里零残留；三条 doubled 选择器确认进了产物 |
| 计算样式实测 | 见第三节（修复前后各一轮，用本次构建的 4 个 CSS 直接量 `getComputedStyle`） |
| 文件同步 | `notes.css` 已删、`index.css` 无残留 import、`semanticInk.test.ts` 清单已同步 |

**新增的校验手段**（建议并入后续每轮）：把本次构建的 CSS 按 `index.html` 的真实顺序拼起来，构造一批带真实类名（含哈希 module 类）的元素，用浏览器读 `getComputedStyle` 对照期望值。它能抓到「改名没错但覆盖失效」这一类 CI 完全看不见的问题——本轮就是靠它抓到的。（临时探针页写在 `web/dist/` 下、已删除，下次 `npm run build` 也会清空该目录。）

---

## 五、需人工实机复核项

CSS 改名对 CI 完全无感，以下四处请实机看一遍（本轮已用计算样式量过关键属性，但观感仍需眼睛）：

1. **笔记编辑器**（项目详情内）：标题/kind 下拉/标签输入的排布与高度、正文区等宽字体与 12px、预览区卡片、`/` 块面板上方动作行、草稿提示的位置。
2. **版本历史与 diff**：面板外壳、版本条目、时间等宽字体、`+/-` 行的底色（绿/红 soft）、`diff-pre` 的横向滚动。
3. **首页搜索命中列表**：命中卡片的边框/悬停、`note`/`todo` 徽章配色（这条走全局 `hit-type-note/-todo`，最需要确认没变色）。
4. **Dashboard 搜索下拉**：输入框左侧图标位（32px 内边距）、focus 时宽度过渡、下拉里的分组标题、`hit-type-mini` 徽章配色。

---

## 六、后续候选

| 优先级 | 内容 | 备注 |
|---|---|---|
| 决策项 | `main.tsx` 导入顺序（第三节） | 一行改动、全局影响，建议先定 |
| 下一轮 | `components/command-palette.css`（14 独占）、`components/toast.css`（6 独占）、`features/todos.css`（6 独占） | 整文件迁入各自组件 |
| 合并一轮 | 零散残留：`.skeleton-card`→Dashboard、`.tech-chip`→ProjectOverviewSection、`.badge-*`→ProjectCard/NoteSection、`.toggle-*`→PluginsTab、`.skeleton-value`→SummaryBar、`.skip-link`→App、`.card-refresh-btn`+`.spin`→ProjectCard、`.empty-section`→ProjectDetail、`.heatmap-empty-state`→Heatmap | 单类归属，收益小但清得干净 |
| 单独一轮 | 32 个零引用死类清理并写进 CHANGELOG | **必须先按模板串复核**（本轮已证明审计会把动态类误报成死类） |
| 待定性 | 空壳类累计 4 个：`knowledge`、`knowledge-section`、`ai-url-row`、`status-warn` | markup 在用、全站无 CSS 定义 |
