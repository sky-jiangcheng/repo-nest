# Repotory 产品深度评估 — 2026-08-26（第二轮）

> 评估视角：AI 产品架构 | 基于 Sprint 1 & 2 清理后状态 | 代码量：Go ~6,200 行 + TS/TSX ~5,400 行 | 210 文件 / 1.9 MB

## 与上轮对比

| 维度 | 上轮 (08-25) | 本轮 (08-26) | 变化 |
|------|-------------|-------------|------|
| 文件数 | ~240 | 210 | -30 |
| 叙事一致性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ADR-0006 完全落地，README 已同步 |
| 插件系统 | 仍有 UI 和示例 | 仅保留知识源通道 | ✅ 收敛到位 |
| 块编辑器 | 无冻结标记 | EDITOR-FROZEN + ADR 引用 | ✅ 边界清晰 |
| MCP 工具 | 7 个只读 | 9 个（含写入） | ✅ AI 可写闭环 |

## 评估结论

**一句话**：叙事和架构都已校准，但**前端状态管理混乱**和**功能密度偏高**是当前最大的两个产品风险。上轮的删除项已全部执行，本轮聚焦「深度优化」。

| 维度 | 评分 | 关键判断 |
|------|------|---------|
| 产品定位 | ⭐⭐⭐⭐⭐ | ADR-0006 + positioning-brief + README 三者一致，无偏差 |
| 核心闭环 | ⭐⭐⭐⭐ | 发现→理解→笔记→搜索→MCP 写入已通 |
| 代码架构 | ⭐⭐⭐⭐ | 服务层清晰，Go 侧分层合理 |
| 前端健康度 | ⭐⭐⭐ | 页面组件 hooks 密度高（15+ useState/页），CSS 全局无 scoped |
| 功能克制 | ⭐⭐⭐ | 插件/块编辑器已冻结，但 Todo/待办仍是支持功能却占据大量 UI |
| 测试覆盖 | ⭐⭐⭐ | Go 侧 11 包全通过，前端 hooks 3 个测试文件但页面 0 测试 |
| 文档体系 | ⭐⭐⭐⭐⭐ | ADR 体系成熟，本轮文档已瘦身 |

---

## TODO 总表

### 🔴 删除项（零功能回退的减法）

| ID | 内容 | 理由 | 风险 | 估时 |
|----|------|------|------|------|
| D7 | 删除 `web/dist/` 目录（~200 个构建产物） | 构建产物不应提交到仓库，应在 CI/CD 中构建，或加入 `.gitignore` | 零 | 5min |
| D8 | 删除 `build/dmg-background.svg` + `build/dmg-readme.txt` | macOS DMG 打包素材，Wails 构建自动生成，无需手动维护 | 零 | 5min |
| D9 | 删除 `web/src/styles/tokens.md` | 设计令牌文档，token 使用应直接看 `tokens.css`，多余文档维护负担 | 零 | 2min |
| D10 | 删除 `docs/api/openapi.json` | 347 行手动维护的 OpenAPI spec，与代码不同步的概率极高；不如删除或改为 CI 自动生成 | 低 | 5min |
| D11 | 删除 `.github/ISSUE_TEMPLATE/docs.yml` | 文档 issue 模板，个人项目用不上，GitHub Issues 本身即可 | 零 | 2min |

### 🟡 收敛项（减少维护表面）

| ID | 内容 | 理由 | 风险 | 估时 |
|----|------|------|------|------|
| C4 | **CSS Modules 试点**：从 `Knowledge.tsx` 开始，将 className 从全局改为 CSS Modules | 全局 CSS 类名（`btn`、`stat-label`、`section-header` 等）跨页面共享但无隔离，改一个页面样式可能影响另一个 | 中 | 2h |
| C5 | **Knowledge 页面数据层拆分**：将搜索/过滤/pin/import 逻辑提取到 `useKnowledgePage` 自定义 hook | 15 个 useState + 多个 useCallback，单文件 348 行职责过重 | 低 | 1.5h |
| C6 | **Dashboard 页面数据层拆分**：将项目列表/摘要/日期切换提取到 `useDashboardData` hook | 13 个 useState + 26 个 hook 调用，348 行 | 低 | 1.5h |
| C7 | **TodoSection → 待办组件精简评估**：评估 Todo 功能在核心闭环中的位置 | 待办功能占 146 行组件 + API + 服务层 + DB 层，但不在「发现→理解→沉淀→检索→AI」闭环中 | 中 | 1h |
| C8 | `docs/code-review/2026-08-18-deep-review.md` 删除或压缩 | 上上轮评估报告，历史价值低，保留增加认知负担 | 零 | 5min |
| C9 | `docs/product-review/2026-08-25-deep-review.md` 压缩为摘要 | 上轮完整报告已在本文件覆盖，保留结论即可 | 零 | 5min |

### 🟢 细化项（提升质量与 AI 产品力）

| ID | 内容 | 理由 | 优先级 | 估时 |
|----|------|------|--------|------|
| P10 | **MCP 笔记搜索增强**：`repotory_notes_search` 返回结果当前是文本拼接，应返回结构化 JSON（含 note_id, project, score, snippet） | AI Agent 无法精确定位搜索结果去调用 `notes_read`，当前是盲猜 | **高** | 1h |
| P11 | **MCP 工具链闭环**：`notes_create` 返回 note_id，让 AI 可以紧接着调用 `notes_update` 或 `notes_read` 验证 | 当前 create 返回文本提示，AI 无法确认写入是否成功 | **高** | 30min |
| P12 | **`repotory_ask` 上下文增强**：当前 `ask` 工具只做全文搜索，应补充项目上下文（如传入 project_id 参数限定范围） | AI 在多项目环境下搜索容易命中无关结果 | **中** | 1h |
| P13 | **笔记 Markdown → 知识图谱**：从笔记中自动提取实体（项目名、技术栈、人物）建立关联 | 当前知识库是扁平列表，缺少知识间的关联，AI 缺少推理路径 | **中** | 3h |
| P14 | **前端组件 Storybook 隔离**：为核心组件（GoalRing, Heatmap, KnowledgeCard, ProjectCard）建立 Storybook stories | 当前组件只能在完整页面中看到，交互测试成本高 | 低 | 2h |
| P15 | **`db/db.go` → `db/migrate.go` 拆分**：432 行中 250+ 行是迁移逻辑 | 迁移逻辑独立性强，拆分后可单独测试 | 中 | 1h |
| P16 | **`service/project.go` 拆分**：340 行混杂查询、统计、分组、刷新历史 | 按职责拆为 `project_query.go` / `project_stats.go` | 中 | 1h |
| P17 | **搜索结果排序优化**：FTS5 的 bm25 排序对中文笔记效果有限，应引入笔记更新时间、pin 状态、最近访问等因素 | 搜索是核心闭环的出口，排序质量直接影响 AI 和用户体验 | **高** | 2h |
| P18 | **`cmd/mcp/main.go` 拆分**：415 行单文件包含 9 个工具定义 | 按域拆为 `tools_notes.go` / `tools_projects.go` / `tools_search.go` / `tools_score.go` | 低 | 1h |
| P19 | **笔记版本 diff 可视化**：`diff.go` 已实现行级 diff，但前端 `VersionHistoryPanel` 55 行过于简略 | 版本历史是知识管理的差异化能力，当前 UI 仅显示版本列表 + 纯文本 diff | 中 | 2h |
| P20 | **MCP 健康检查工具**：新增 `repotory_status` 工具返回数据库状态、笔记/项目数量、最后扫描时间 | AI Agent 无法判断 Repotory 实例是否正常运行 | 中 | 30min |
| P21 | **`llms.txt` 优化**：当前生成的 llms.txt 是纯文本，应支持 JSON-LD 结构化输出 | 不同 AI 工具对 llms.txt 格式有不同偏好 | 低 | 1h |
| P22 | **前端错误边界**：页面组件无 ErrorBoundary，API 失败时整个页面白屏 | 当前依赖 ErrorBanner 组件逐个处理，但未捕获渲染异常 | **高** | 1h |
| P23 | **API 响应缓存策略**：`useApiData` hook 有内存缓存但无 TTL 配置项，不同数据新鲜度需求不同 | 项目列表 vs 笔记内容 vs 摘要的缓存策略应该不同 | 中 | 1h |
| P24 | **`SKILL.md` 可发现性增强**：当前 SKILL.md 是静态文档，应支持 MCP 工具 `repotory_help` 动态返回当前版本能力 | AI Agent 无法感知 SKILL.md 的存在，只有通过 MCP 协议才能发现工具 | 低 | 30min |

---

## 建议执行顺序

### Sprint 3（本轮立即执行）
1. **D7-D11** — 纯减法，零功能回退
2. **C8-C9** — 历史评估文档压缩
3. **P10-P11** — MCP 搜索/写入结果结构化（AI 闭环关键）

### Sprint 4（下一轮）
4. **C5-C6** — 页面数据层拆分（降低维护成本）
5. **P22** — 前端错误边界（稳定性）
6. **P17** — 搜索排序优化（核心闭环质量）

### Sprint 5（后续）
7. **C4** — CSS Modules 试点
8. **P15-P16** — Go 后端拆分
9. **P19** — 版本历史 UI 增强

### 冻结 / 长期
- **P13**（知识图谱）— 复杂度高，暂缓
- **P14**（Storybook）— 有余力再做
- **C7**（Todo 评估）— 需要与团队讨论 Todo 在核心闭环中的定位

---

## 关键洞察

### 洞察 1：MCP 工具的「AI 可用性」是最大差距

当前 MCP 工具数量足够（9 个），但**信息结构对 AI 不友好**：
- `notes_search` 返回纯文本，AI 无法程序化解析结果
- `notes_create` 不返回 note_id，AI 无法验证写入
- 无 `status` 工具，AI 无法判断实例健康

这是 AI 产品的核心差异化点。**MCP 工具的设计应该以 AI Agent 的调用链为视角，而不是以人类 API 消费者为视角**。

### 洞察 2：前端进入了「功能丰富但维护困难」阶段

15 个 useState + 26 个 hook 调用在一个文件中，是典型的 React 状态管理退化信号。当前还能驾驭，但新增任何功能都会指数级增加复杂度。

**建议**：在功能冻结期（ADR-0006）内完成数据层拆分，而不是等到下次功能扩展时再做。

### 洞察 3：Todo 功能的战略定位需要明确

Todo（待办）不在「发现→理解→沉淀→检索→AI」核心闭环中，但占据：
- 146 行 TodoSection 组件
- db/todos.go + service/todo.go + API 层
- Settings 中的 StandardsTab（工作日标准检查）
- Dashboard 中的 todoCounts + noteCounts 联动

**三种选择**：
1. **保留但降级**：Todo 从导航入口移到笔记详情页的子面板
2. **提升为核心**：将 Todo 与笔记合并为「知识条目」，支持 type=todo
3. **删除**：用户用笔记的 checkbox 语法（`- [ ]`）替代

需要明确选择，当前是最差状态——**功能存在但战略模糊**。

### 洞察 4：`web/dist/` 提交到仓库是反模式

200+ 个构建产物文件（KaTeX 字体、Mermaid 图表库 chunk、Vite 预构建缓存）不应该在源码仓库中。这会：
- 膨胀 clone 大小
- 造成合并冲突
- 让 PR diff 失去意义

**建议**：删除 `web/dist/`，加入 `.gitignore`，构建产物只在 CI/CD 和 Wails build 中生成。
