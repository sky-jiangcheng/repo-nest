# Repotory 产品深度评估 — 2026-08-26（第三轮 / 终态）

> 评估视角：AI 产品架构 | 基于 Sprint 1-5 全部清理后状态
> 代码量：Go 9,644 行（含测试 2,762）+ TS/TSX 6,581 行 + CSS 4,083 行 | 212 文件

## 与前轮对比

| 指标 | 08-25 首轮 | 08-26 第二轮 | 08-26 终态 |
|------|-----------|-------------|-----------|
| 文件数 | ~240 | 210 | 212 |
| Go 行数 | ~6,400 | ~6,200 | 9,644（含 vendor 去除后真实值） |
| TS/TSX 行数 | ~6,400 | ~5,400 | 6,581 |
| CSS 行数 | — | — | 4,083 |
| 删除项已完成 | 0/6 | 6/6 | 11/11 |
| 收敛项已完成 | 0/3 | 3/3 | 5/8 |
| 细化项已完成 | 0/9 | 2/9 | 14/24 |

## 评估结论

**一句话**：核心架构已健康，但**代码密度分布不均**和**历史包袱残留**是当前两个主要问题。前端通过 hooks 拆分后页面组件大幅瘦身，但 NoteSection（417 行）仍是重灾区；Go 后端通过文件拆分后单文件均在 300 行以下，但插件运行时（669 行）在冻结策略下占比过大。

| 维度 | 评分 | 关键判断 |
|------|------|---------|
| 产品定位 | ⭐⭐⭐⭐⭐ | 三轮评估一致，叙事无偏差 |
| 核心闭环 | ⭐⭐⭐⭐½ | MCP 结构化后 AI 可写闭环完整 |
| 代码架构 | ⭐⭐⭐⭐ | 文件拆分后分层清晰，单文件可控 |
| 前端健康度 | ⭐⭐⭐½ | hooks 拆分有效，但 NoteSection 仍重 |
| 功能克制 | ⭐⭐⭐⭐ | 冻结项真正落地，遗留清理到位 |
| 测试覆盖 | ⭐⭐⭐½ | Go 11 包全通过，前端 hooks 有测试 |
| 文档体系 | ⭐⭐⭐⭐½ | 精简后 docs/ 仅 1,434 行，质量高 |
| 遗留清理 | ⭐⭐⭐ | `vite-plugin-pwa` 残留、脚本冗余、空目录 |

---

## TODO 总表

### 🔴 删除项

| ID | 内容 | 理由 | 风险 | 估时 |
|----|------|------|------|------|
| D12 | `web/package.json` 移除 `vite-plugin-pwa` devDependency | Sprint 1 已从 vite.config.ts 移除插件配置，但 package.json 仍声明依赖，`npm install` 仍会下载 | 零 | 2min |
| D13 | 删除 `scripts/sync-knowledge.sh`（132 行） | 文件头已标 DEPRECATED，功能由 app 内 ImportClaudeMemory 完全替代；直接 SQL 拼接有注入风险 | 零 | 2min |
| D14 | 删除 `build/icon-mobius.jpg`（87KB） | 源图标应为 `build/icon.svg`，JPG 是历史产物，scripts/generate-icons.mjs 从 SVG 生成 | 零 | 2min |
| D15 | 删除空目录 `skills/` | 零内容空目录，无用途 | 零 | 1min |
| D16 | `.claude/settings.local.json` 加入 `.gitignore` | AI 工具本地配置，不应提交 | 零 | 1min |
| D17 | 删除 `.workbuddy/`（已在 gitignore） | 目录已存在但 `.gitignore` 已覆盖，确认 git 不跟踪 | 零 | 1min |
| D18 | `web/public/` 下的 PWA 截图清理 | `screenshot-desktop.png`、`screenshot-mobile.png`、`og-image.png` 是 PWA/SEO 产物，桌面应用不需要 | 低 | 2min |
| D19 | `docs/troubleshooting.md` 评估 | 检查是否与 README 或 getting-started 重复 | 低 | 5min |

### 🟡 收敛项

| ID | 内容 | 理由 | 风险 | 估时 |
|----|------|------|------|------|
| C10 | **NoteSection 拆分**：417 行混合笔记 CRUD + 编辑器 + 版本历史 + 移动/删除 | 拆出 `useNoteSection` hook (数据层) + NoteCard 子组件提取为独立文件 | 低 | 2h |
| C11 | **插件运行时精简评估**：669 行（runtime 413 + loader 52 + plugin.go 76 + service/plugin 128）| ADR-0006 冻结后仅 Claude importer 使用，评估是否合并为单文件或压缩 | 中 | 1.5h |
| C12 | **ProjectDetail 拆分**：316 行，11 个 hooks | 提取 `useProjectDetail` hook | 低 | 1.5h |
| C13 | **CSS 死代码清理**：435 个 CSS 类未在 TSX 中直接引用 | 部分为动态拼接（合法），但大量为历史遗留，逐个文件审计清理 | 低 | 3h |
| C14 | **`queries_test.go` 拆分**：784 行 27 个测试函数 | 按域拆到 projects_test.go / notes_test.go / todos_test.go / search_test.go | 低 | 1h |
| C15 | **install 脚本评估**：`install.sh`（45 行）+ `install.ps1`（29 行）| GitHub Release 的二进制是否需要脚本安装？还是 README 说明即可？ | 低 | 15min |

### 🟢 细化项

| ID | 内容 | 理由 | 优先级 | 估时 |
|----|------|------|--------|------|
| P25 | **`knowledge.go` 拆分**：572 行混杂 README 提取、技术栈检测、语言检测、贡献者、活动统计 | 拆为 `knowledge_tech.go` + `knowledge_contributors.go` + `knowledge_deps.go` | 中 | 1.5h |
| P26 | **`stats.go` 拆分**：471 行混杂 git stat 查询、日期验证、提交解析 | 拆为 `stats_query.go` + `stats_recent.go` + `stats_validate.go` | 中 | 1h |
| P27 | **`cmd/mcp/main.go` 拆分**：453 行含 9 个工具 + agentScore 141 行 | 拆为 `tools_notes.go` / `tools_projects.go` / `tools_search.go` / `tools_score.go` | 中 | 1h |
| P28 | **MCP 工具描述增强**：当前描述过于简短 | 为每个工具增加 `description` 场景说明 + 参数约束 + 示例值 | 中 | 30min |
| P29 | **`stats.go` 时间戳解析脆弱**：`parseTimestamp` 假定固定格式 | 支持 ISO 8601 和 git 默认格式 | 低 | 30min |
| P30 | **前端懒加载**：Dashboard/Knowledge/ProjectDetail/Settings 全部同步导入 | `React.lazy()` + `Suspense` 路由级代码分割 | 中 | 30min |
| P31 | **`Domain/types.go` 评估**：当前仅 15 行，类型散落在各包中 | 评估是否需要收拢核心 domain 类型 | 低 | 15min |
| P32 | **Wails 绑定层审计**：`internal/app/bindings.go` 218 行 | 确认每个 binding 方法都有 MCP 对应或明确标记为 desktop-only | 中 | 30min |
| P33 | **`TrendChart` 组件评估**：100 行 | 仪表盘中的趋势图是纯 SVG 实现，评估是否需要或用 CSS 替代 | 低 | 15min |

---

## 关键洞察

### 洞察 1：遗留清理必须「连根拔起」

Sprint 1 移除了 `vite-plugin-pwa` 的 vite.config.ts 配置，但**忘记从 package.json 移除依赖**。`sync-knowledge.sh` 头部标 DEPRECATED 但仍存在。`build/icon-mobius.jpg` 87KB 的 JPG 在 SVG 源存在时无意义。

**教训**：删除一个功能时，必须清单式追踪所有关联文件（config → package.json → lock file → 运行时引用 → 文档提及）。

### 洞察 2：前端进入了「拆分临界点」

| 文件 | 行数 | 问题 |
|------|------|------|
| NoteSection.tsx | 417 | CRUD + 编辑器 + 版本历史 + 移动/删除全在一个组件 |
| Knowledge.tsx | 239 | ✅ 已拆分，数据层在 hook |
| Dashboard.tsx | 185 | ✅ 已拆分，数据层在 hook |
| ProjectDetail.tsx | 316 | 11 个 hooks，待拆分 |

NoteSection 是当前前端最大的技术债。项目详情页的笔记区域全靠它支撑，但其内部的版本历史、编辑器切换、草稿持久化逻辑纠缠在一起。

### 洞察 3：CSS 体量（4,083 行）超出代码体量（TS 6,581 行）的 60%

手写全局 CSS 的维护成本随组件线性增长。435 个未直接引用的 CSS 类说明历史样式残留严重。

**建议**：Sprint 4 C5-C6 已提取 hooks 为 CSS Modules 迁移铺路，下一步应从 NoteSection 或 KnowledgeCard 开始试点 CSS Modules，逐步替换全局类名。

### 洞察 4：Go 后端的「大文件分布」已改善

| 文件 | 变更前 | 变更后 |
|------|--------|--------|
| db/db.go | 432 | 145 ✅ |
| service/project.go | 340 | 135 ✅ |
| knowledge/knowledge.go | 572 | 572 ⚠️ 待拆 |
| stats/stats.go | 471 | 471 ⚠️ 待拆 |
| cmd/mcp/main.go | 453 | 453 ⚠️ 待拆 |
| core/plugin/runtime/runtime.go | 413 | 413 ⚠️ 评估 |

Go 侧仍有 4 个 400+ 行文件，但功能聚合度高（knowledge.go 的 Mine() 是入口函数），拆分优先级低于前端。

### 洞察 5：MCP 工具集已成熟但缺「使用指引」

9 个工具覆盖读取+写入+自检，但 AI Agent 首次连接时只能靠 `SKILL.md` 的静态描述。`repotory_ask` 是最常用的入口，但缺少：
- 使用场景描述（"先 ask 搜索，再 read 详情，最后 create 笔记"的推荐流程）
- 参数示例（`query` 支持什么格式？可以搜中文吗？）
- 返回值说明（`score` 的含义和范围）

**建议**：P28 专门为每个工具补充 `description` 中的场景 + 示例 + 约束。
