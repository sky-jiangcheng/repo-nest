# P35 第五轮：`command-palette.css`、`toast.css`、`todos.css` 三个簇迁移

> 仓库：`/Users/jiangcheng/Workspace/Script/RepoNest`（Go + Wails v2 + React）
> 日期：2026-10-09　状态：**已改在工作区，未提交**（按约定等你审阅后再落库）
> 对应 TODO：P35「逐组件迁移」条目；CHANGELOG 口径上是第 11、12、13 块
> 本轮规模：全局 CSS 2,191 → **1,905 行**（21 → 18 文件），module CSS **3,425 → 3,728 行**（25 → 28 文件）
> 口径说明：module 行数本轮改为逐文件实测；上一轮文档记的「3,400 行」比实测的 3,425 行少 25 行，以本轮实测为准（本轮增量 303 行 = 三个新 module 的 132 + 105 + 66，自洽）。

---

## 一、迁移清单

| 原全局文件 | 去向 | 类数 | TSX 改写 |
|---|---|---|---|
| `components/command-palette.css`（122 行 / 16 类） | 新建 `components/CommandPalette.module.css`（132 行） | 16 | `CommandPalette.tsx` 17 处 |
| `components/toast.css`（100 行 / 10 类） | 新建 `components/Toast.module.css`（105 行） | 10 | `Toast.tsx` 9 处 |
| `features/todos.css`（61 行 / 8 类） | 新建 `components/TodoSection.module.css`（66 行） | 7 | `TodoSection.tsx` 6 处 |

- 三个全局 CSS **全部整文件删除**；`styles/index.css` 移除 3 条 `@import`；`semanticInk.test.ts` 的清单同步移除 3 条（该清单必须与导入顺序一致）。
- 三个组件里其余全局类**一字未动**：`visually-hidden`、`panel-section`、`skeleton`/`skeleton-text`、`empty-hint`、`btn`/`btn-primary`/`btn-sm`/`btn-icon`、`form-input`，以及 `btn-delete`/`btn-delete-confirm`（确认删除按钮）。
- `CommandPalette.tsx` 的 `id="cmdk-results"` / `id={cmdk-opt-${i}}` 是 DOM id（`aria-controls` / `aria-activedescendant` 用），不是类名，原样保留。

## 二、边界判定依据

1. **`.cmdk-input` 是 `inputs.css` 共享族成员，但只在基础组里。** 逐行核对：族的基础组（`inputs.css:5-13`）包含 `.cmdk-input`，而 `:hover`（25-30 行）、`:focus`（35-41 行）、`::placeholder`（47-48 行）三组**都没有它**。所以：
   - markup 保留全局类名（`className={`cmdk-input ${s.cmdkInput}`}`），基础组里 module 没重写的声明（`transition`、`max-width` 等）继续生效；
   - module 只叠加组件自有规则，并用 **doubled 选择器**保证在产物级联顺序下仍然生效（理由见第四轮说明第三节：设计系统 CSS 排在路由 chunk 之后）；
   - 实测确认这个输入框 focus 态没有共享强调环（`border-bottom-color` 仍是 `--border-subtle`、`box-shadow: none`）——**迁移前后一致，不是回归**，只是既有的一处 a11y 小缺口，另行记录。
2. **三处动态类按「改查表/三元」处理**（沿用 Heatmap `level-${n}` 的判例）：
   - `cmdk-type-${h.type}` → `${h.type === 'note' ? s.cmdkTypeNote : s.cmdkTypeTodo}`（与同一行既有的类型文案三元一致）；
   - `toast-${toast.kind}` → 模块内 `kindClass: Record<ToastItem['kind'], string>` 查表（`kind` 是联合类型，表是穷尽的，类型安全）。
   - 静态审计把 `cmdk-type-note` / `cmdk-type-todo` / `toast-success` / `toast-error` / `toast-info` 报成「零引用死类」，**它们都是活类**——这是第三次撞到同一类误报，清理死类时必须先按模板串复核。
3. **裸类 `completed` 随 module 作用域化。** 它只在 `.todo-item.completed .todo-title` 里用（全站 CSS 只有这一处定义），所以 markup 从 `todo.completed ? 'completed' : ''` 改成 `s.completed`，不再往全局留泛名类。
4. **`.todo-add .form-input` 用 `:global()` 包裹**：`form-input` 是全局基元，module 内写成 `.todoAdd :global(.form-input)`，只声明 todo 输入框自己的 `flex` / `max-width`。
5. **`@keyframes` 随 module 作用域化**：`toast-in` 被改名为 `_toast-in_*`，同文件内的 `animation: toast-in …` 引用自动同步——这点已用计算样式确认（见下表），不是「看起来没坏」。
6. **同名标识符不受影响**：`AIAskPanel.tsx` / `Review.tsx` / `ProjectCommitLog.tsx` 里出现的 `toast` 是推送通知的函数/变量名，不是类名——写前预检逐行确认过（含 `className` 检查）。

## 三、校验结果

| 项 | 结果 |
|---|---|
| `npm --prefix web run build`（tsc + vite） | ✓ 535ms |
| `npm --prefix web run lint`（eslint src） | ✓ 零错 |
| `npm --prefix web test` | ✓ 15 文件 / 108 用例全过（含 semanticInk 的导入顺序一致性） |
| `s.*` 对账 | CommandPalette 16/16、Toast 10/10、TodoSection 7/7；**无未引用、无未定义** |
| 产物残留 | 三个文件的旧类名（`cmdk-overlay`、`toast-host`、`todo-item` 等 30 个）在产物 CSS 里**零残留**；唯一保留的是有意留下的 `cmdk-input`（共享族基础组需要） |
| keyframes | 产物里 `@keyframes _toast-in_*` 定义与 `animation-name` 引用一致 |
| 计算样式实测 | 见下表，全部符合预期 |

**计算样式实测**（用本次构建的 4 个 CSS、按 `index.html` 的真实顺序加载，真实浏览器读 `getComputedStyle`）：

| 元素 | 属性 → 实测值 |
|---|---|
| `.cmdk-input` | padding 16px/18px、font-size 15px、border-top-width 0px、border-bottom 1px `--border-subtle`（doubled 生效，未被族基态吃掉） |
| `.toast-host` | position fixed、max-width 400px、z-index 2000 |
| `.toast` + `.toast-success` | padding 14px/36px、border-left 3px、animation-name `_toast-in_*`、duration 0.18s |
| `.toast-close` / `.toast-title` | absolute / 16px；13px / 600 |
| `.cmdk-type` + `-note` | 22×22、背景 `--accent-soft` |
| `.todo-add` / `.todo-add .form-input` | flex / gap 6px / margin-bottom 14px；flex-grow 1、max-width none |
| `.todo-list` / `.todo-item` 首末位 | list-style none；下边框 1px / 0px（`:last-child` 生效） |
| `.todo-item.completed .todo-title` | line-through、`--text-muted`、flex-grow 1、13px |

## 四、需人工实机复核项

CSS 改名对 CI 完全无感，以下三处请实机看一遍：

1. **Cmd/Ctrl+K 命令面板**：浮层遮罩与模糊、面板宽度与圆角、输入框（16px/18px 内边距、15px 字号、只有下边框）、分组标题、高亮行底色、note/todo/project 三种类型徽章配色、底部快捷键提示、Esc/回车交互仍正常。
2. **Toast 通知**：三种 kind 的左侧色条（success 绿 / error 红 / info 蓝）、入场动画、右上角关闭按钮、动作按钮的悬停色；长路径消息的换行（`overflow-wrap: anywhere`）。
3. **项目详情的 Todo 区**：添加行的输入框是否撑满（`flex: 1` + `max-width: none`）、条目下边框（末条无）、勾选后标题的删除线与弱化色、悬停时才出现的上移/下移/删除按钮。

## 五、后续候选

| 优先级 | 内容 | 备注 |
|---|---|---|
| 决策项 | `main.tsx` 导入顺序（第四轮第三节） | 一行改动、全局影响，仍未动 |
| 下一轮 | 零散残留合并一轮：`.skeleton-card`→Dashboard、`.tech-chip`→ProjectOverviewSection、`.badge-*`→ProjectCard/NoteSection、`.toggle-*`→PluginsTab、`.skeleton-value`→SummaryBar、`.skip-link`→App、`.card-refresh-btn`+`.spin`→ProjectCard、`.empty-section`→ProjectDetail、`.heatmap-empty-state`→Heatmap | 单类归属，收益小但清得干净 |
| 单独一轮 | 32 个零引用死类清理并写进 CHANGELOG | **必须先按模板串复核**（本轮第三次证明审计会误报） |
| 待定性 | 空壳类累计 4 个：`knowledge`、`knowledge-section`、`ai-url-row`、`status-warn` | markup 在用、全站无 CSS 定义 |
| 小记 | `.cmdk-input` 没有可见 focus 样式（module 设 `outline: none`，共享族 focus 组不含它） | 迁移前后一致，属既有 a11y 小缺口，可另记一条 |
