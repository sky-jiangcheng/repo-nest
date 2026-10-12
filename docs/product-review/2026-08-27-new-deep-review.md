# Repotory 产品深度评估 — 2026-08-27（第四轮）

> 评估视角：AI 产品架构 · 从用户价值到代码熵的全链路审计
> 代码量：Go 9,479 行（含测试 2,733）+ TS/TSX 6,706 行 + CSS 4,055 行 | 396 文件 | 23 篇文档

## 与前三轮对比

| 指标 | 08-25 首轮 | 08-26 终态 | **08-27 本轮** |
|------|-----------|-----------|---------------|
| 文件数 | ~240 | 212 | 396（含 web/dist 清理后重建） |
| Go 行数 | ~6,400 | 9,644 | 9,479 |
| TS/TSX 行数 | ~6,400 | 6,581 | 6,706 |
| CSS 行数 | — | 4,083 | 4,055 |
| 删除项已完成 | 0/6 | 11/11 | 11/11 + 本轮新增 |
| 收敛项已完成 | 0/3 | 5/8 | 6/8 |
| 细化项已完成 | 0/9 | 14/24 | 16/24 |

## 总评

一句话：**核心闭环已稳固，但项目正从「功能膨胀」悄然滑向「资产膨胀」**。前八轮 sprint 成功治理了代码质量，但遗留资产清理存在「选择性执行」——D12-D19 中仅 D12(PWA 插件)、D13(sync-knowledge.sh)、D14(icon-mobius.jpg) 被清理，而空目录(skills/)、PWA 图标残留(web/public/)、AI 工具配置泄露(.claude/settings.local.json) 仍在。前端 hooks 拆分成效显著（10 个 hooks、1,062 行），但 NoteSection 仍是 305 行的「逻辑聚合器」。Go 后端插件运行时（669 行）在冻结策略下是最大的沉没成本。

| 维度 | 评分 | 关键判断 |
|------|------|---------|
| 产品定位 | ⭐⭐⭐⭐⭐ | 四轮一致，叙事零漂移 |
| 核心闭环 | ⭐⭐⭐⭐½ | MCP 9 工具 + FTS5 + 版本历史 = 完整闭环 |
| 代码架构 | ⭐⭐⭐⭐ | 文件拆分到位，service 层 13 文件均 ≤221 行 |
| 前端健康度 | ⭐⭐⭐⭐ | hooks 层成熟，懒加载已落地 |
| 功能克制 | ⭐⭐⭐⭐ | 冻结项真正执行（BlockEditor 头部注释明确） |
| 资产卫生 | ⭐⭐⭐ | **本轮重点问题**：空目录、配置泄露、PWA 残留 |
| 测试覆盖 | ⭐⭐⭐½ | Go 8 包通过，前端 hooks 有测试 |
| 文档体系 | ⭐⭐⭐⭐½ | ADR 体系完整，定位简报质量高 |

---

## 🔴 删除项（本轮新增 + 前轮遗留）

> 零功能回退，纯减法。

### D20: 删除空目录 `skills/`

- **现状**：空目录，零内容
- **操作**：`rm -rf skills/`
- **风险**：零
- **估时**：1min

### D21: 删除空目录 `.agents/skills/`

- **现状**：空目录，`.agents/` 整体是 AI 工具产物（已在 .gitignore）
- **操作**：`rm -rf .agents/`
- **风险**：零（gitignore 已覆盖）
- **估时**：1min

### D22: `.claude/settings.local.json` 清理 + gitignore 补全

- **现状**：2,651 字节的本地 AI 工具配置已入库，`.gitignore` 有 `.claude/` 目录但此文件已跟踪
- **操作**：`git rm --cached .claude/settings.local.json`（文件保留在本地，从 git 跟踪中移除）
- **风险**：零
- **估时**：1min

### D23: PWA 图标清理评估（`web/public/`）

- **现状**：5 个文件（favicon.ico 18KB、favicon.svg、icon-192.png 6KB、icon-512.png 25KB、icon-maskable-512.png 19KB），桌面应用无需 PWA manifest 引用的独立图标文件
- **需要决策**：Wails 桌面应用的窗口图标走 `build/icon.svg` → `wails.json`，`web/public/` 下的图标仅为 PWA/浏览器场景
- **建议**：保留 favicon.ico + favicon.svg（开发态浏览器预览需要），删除 icon-192.png / icon-512.png / icon-maskable-512.png（纯 PWA 产物）
- **操作**：删除 3 个 PWA 图标文件
- **风险**：低（确认无 manifest.json 引用后）
- **估时**：5min（含引用检查）

### D19（遗留）: `docs/troubleshooting.md` 评估

- **现状**：87 行，内容覆盖日志路径、FAQ、Windows Defender 误报
- **评估**：与 README 有轻微重叠（日志路径），但 FAQ 格式有独立价值
- **建议**：**保留**，但将日志路径表从 README 移到 troubleshooting 统一引用
- **估时**：10min

### D15（遗留）: `skills/` 空目录 → 已合并到 D20

---

## 🟡 收敛项

### C11（遗留）: 插件运行时精简评估 — 669 行

- **现状**：`runtime/runtime.go` 413 行 + `runtime/loader.go` 52 行 + `plugin.go` 76 行 + `service/plugin.go` 128 行
- **功能**：yaegi 脚本加载、事件总线、Claude importer、知识源导入
- **ADR-0006 冻结状态**：插件系统标记为「实验性，暂停平台基础设施扩展」
- **问题**：413 行的 runtime.go 是当前 Go 后端第二大文件（仅次于 knowledge.go 515 行），但支撑的功能仅 Claude importer 一个
- **建议方案**：
  - **方案 A（推荐）**：将 `loader.go`（52 行）合并入 `runtime.go`，减少文件碎片；`plugin.go` + `service/plugin.go` 保持不动（SPI 接口隔离合理）
  - **方案 B（激进）**：评估 yaegi 运行时是否可替换为简单的 `go generate` 预编译插件，消除运行时解释器依赖
- **风险**：中（需确认 Claude importer 路径不受影响）
- **估时**：1.5h（方案 A）/ 4h（方案 B）

### C15（遗留）: install 脚本评估

- **现状**：`install.sh` 1,089 字节 + `install.ps1` 1,065 字节
- **评估**：脚本仅做 curl/wget 下载 + 解压到 `/usr/local/bin`，GitHub Release 页面已有二进制下载
- **建议**：**保留**，但 README 安装说明应同时提供「直接下载」和「脚本安装」两种方式，脚本作为便利路径而非必需
- **估时**：5min（文案调整）

---

## 🟢 细化项

### P34: `service/project_overview.go` 拆分（221 行 → 评估合并合理）

- **现状**：221 行，包含 `MineAndCache`、`MineAndCacheAsync`、`mineAndCache`、`Mine` 四个函数
- **评估**：函数间高度内聚（Mine 是入口，mineAndCache 是同步实现，MineAndCacheAsync 是 goroutine 包装），拆分反而破坏可读性
- **建议**：**不拆**，但 `MineAndCacheAsync` 的 `recover()` 已添加（1.7.2），确认 goroutine 泄漏风险已消除
- **优先级**： — （确认即可）

### P35: 前端 CSS 架构评估（4,055 行全局 CSS）

- **现状**：CSS 体量仍达 TS/TSX 的 60%，全局类名随组件线性增长
- **问题**：全局 CSS 的维护成本在组件数超过 20 个后呈指数增长
- **建议**：从 **NoteSection** 或 **KnowledgeCard** 开始试点 CSS Modules，步骤：
  1. 新增 `.module.css` 文件，将组件专用样式迁移
  2. 保留全局 CSS 仅用于 reset、design tokens、跨组件基础样式
  3. 逐组件迁移，每轮 sprint 处理 1-2 个组件
- **优先级**：🔸中
- **估时**：首轮试点 2h，全量迁移 8-10h

### P36: `knowledge.go` 进一步拆分评估（515 行）

- **现状**：515 行已在上轮从 572 行拆出 `types.go`（48 行）
- **内部结构**：`Mine()` 入口（~50 行）+ README 提取（~100 行）+ 技术栈检测（~80 行）+ 依赖提取（~80 行）+ 贡献者检测（~60 行）+ 活跃度统计（~60 行）+ 辅助函数（~85 行）
- **问题**：函数间共享 `RepoMeta` 结构体和 git 命令调用，拆文件需传入大量参数
- **建议**：**暂不拆**，515 行在 Go 中属可接受范围，功能内聚度高；若后续新增知识挖掘维度再评估
- **优先级**：🔻低

### P37: MCP 工作流指引前置（SKILL.md 增强）

- **现状**：SKILL.md 76 行，覆盖工具列表和基本说明
- **问题**：AI Agent 首次接入时缺乏「推荐工作流」，不知道先调哪个工具
- **建议**：在 SKILL.md 开头增加「推荐工作流」段落：
  ```
  1. repotory_search_projects(query) → 发现相关项目
  2. repotory_ask(query) → 搜索知识库
  3. repotory_read_notes(note_id) → 读取详情
  4. repotory_create_note(...) → 沉淀新知识
  ```
- **优先级**：🔸中
- **估时**：30min

### P38: 前端路由结构审计

- **现状**：4 个懒加载页面（Dashboard、Knowledge、ProjectDetail、Settings）+ HashRouter
- **问题**：
  1. ProjectDetail 仍 316 行，11 个 hooks 混在一个组件
  2. Settings 页 118 行但 6 个 tab 组件（已拆分），结构健康
  3. Knowledge 239 行 + useKnowledgePage 133 行 = 372 行逻辑量，合理
- **建议**：ProjectDetail 拆出 `useProjectDetail` hook（上轮 C12 已标记完成，但组件仍 316 行），需确认 hook 是否真正下沉了数据层
- **优先级**：🔸中
- **估时**：1.5h

---

## 遗留项审计

| 项目 | 状态 | 说明 |
|------|------|------|
| 桌面 GUI 回归测试 | ⚠️ 未执行 | 建议在真机跑一轮冒烟 |
| `.zcode/` 历史清理 | ✅ 已移出跟踪 | 不需要 history rewrite |
| `mineAndCacheAsync` recover | ✅ 已完成（1.7.2） | goroutine 安全 |
| `parseTimestamp` 鲁棒性（P29） | ⚠️ 未处理 | 低优先级，git log 输出格式稳定 |
| `Domain/types.go` 评估（P31） | ⚠️ 未处理 | 仅 15 行，暂不需要收拢 |
| Wails 绑定层审计（P32） | ⚠️ 未处理 | 218 行绑定层需确认 MCP 对应关系 |
| `TrendChart` 评估（P33） | ⚠️ 未处理 | 100 行纯 SVG，仪表盘「支持」级功能 |

---

## 关键洞察

### 洞察 1：资产清理的「最后一公里」问题

前八轮 sprint 清理了大量代码，但留下了「零散遗留物」：

| 遗留物 | 大小 | 影响 |
|--------|------|------|
| `skills/` 空目录 | 0 | 目录噪音 |
| `.agents/skills/` 空目录 | 0 | 目录噪音 |
| `.claude/settings.local.json` | 2.6KB | **AI 本地配置泄露** |
| `web/public/icon-*.png` | 51KB | PWA 残留 |

这些单项影响微小，但累积起来形成「项目不够干净」的体感。建议执行一个 15 分钟的「资产大扫除」sprint。

### 洞察 2：插件系统是最大的沉没成本

669 行插件代码支撑的功能仅为「Claude 记忆导入」（一个 import.go 约 100 行可完成）。yaegi 运行时提供的「进程内 Go 脚本」能力在 ADR-0006 冻结后失去了扩展意义。

**决策点**：是否在 2.0 版本中将 Claude importer 改为内置函数，移除 yaegi 依赖？
- 收益：减少 669 行代码 + 消除 yaegi 外部依赖 + 简化构建
- 成本：丧失未来「可能的」插件扩展能力（但 ADR-0006 已冻结此方向）
- 建议：**2.0 版本考虑**，当前版本保持不动

### 洞察 3：前端已进入「CSS Modules 迁移窗口」

hooks 拆分后，组件逻辑已与样式解耦。4,055 行全局 CSS 是前端最大的技术债，但迁移成本可控：
- 试点组件：NoteSection（305 行，样式集中）、KnowledgeCard（卡片样式独立）
- 迁移策略：新增 `.module.css` → 逐类迁移 → 全局仅保留 tokens + reset
- 预期收益：消除 435 个未引用类、样式作用域隔离、支持主题变量

### 洞察 4：文档体系是项目的隐藏优势

23 篇文档 + 6 篇 ADR + 定位简报 + 产品评估记录 = 文档密度远超同类项目。这是 AI 产品的重要资产：
- AI Agent 可通过 `llms.txt` + SKILL.md + docs/ 快速理解项目
- ADR 记录了完整的决策链路，新贡献者可快速上手
- 产品评估文档（4 轮）形成了连续的改进记录

**建议**：将产品评估文档纳入 CI 自动化，每次 sprint 结束后自动生成 TODO 更新。

---

## 建议执行节奏

| 阶段 | 内容 | 预估 |
|------|------|------|
| **Sprint 9（资产大扫除）** | D20 + D21 + D22 + D23 + C15 文案调整 | 30min |
| **Sprint 10（前端 CSS 试点）** | P35 NoteSection CSS Modules 试点 | 2h |
| **Sprint 11（ProjectDetail 治理）** | P38 确认 hook 下沉 + 可能的二次拆分 | 1.5h |
| **Sprint 12（SKILL.md 增强）** | P37 工作流指引 + P28 MCP 描述增强（如未完成） | 1h |
| **2.0 规划** | C11 插件系统评估（是否移除 yaegi）+ P35 全量 CSS Modules | 按版本规划 |
