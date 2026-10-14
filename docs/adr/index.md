---
title: 架构决策（ADR）
order: 22
---

# 架构决策记录（ADR）

| 编号 | 标题 | 状态 |
|------|------|------|
| [0001](0001-plugin-platform.md) | 插件平台（M1-M4：HTTP server / RBAC / PG-ES / K8s） | Superseded（被 0002 取代） |
| [0002](0002-c-end-repositioning.md) | C 端重新定位：本地优先「代码项目第二大脑」+ 进程内插件 | Accepted |
| [0003](0003-fts5-search.md) | FTS5 trigram 全文搜索 | Accepted |
| [0004](0004-block-editor.md) | 块编辑器（产物保持纯 Markdown） | Accepted |
| [0005](0005-service-layer.md) | 服务层重构（service / app / domain 分层） | Accepted |
| [0006](0006-scope-freeze.md) | 范围冻结与功能分级（核心闭环优先） | Accepted |
| [0007](0007-session-memory-protocol.md) | 会话记忆协议（context / handoff 双工具） | Accepted |
| [0008](0008-pwa-removal.md) | PWA 移出桌面主构建（落实 ADR-0006 暂缓档） | Accepted |
| [0009](0009-ide-presence.md) | IDE 存在感——薄客户端分发策略（一键注册 → VS Code 扩展 → JetBrains 缓议） | Accepted |
| [0010](0010-session-auto-capture.md) | 会话自动捕捉——零人工参与的 Claude Code 会话交接（M1） | Accepted-in-principle（解析、按需捕获与 B 端 hook 已落地；默认关） |
| [0011](0011-multi-agent-memory-importers.md) | 多 agent 记忆源导入——可复用 importer 框架与各源可行性（M2） | Proposed |
| [0012](0012-semantic-search.md) | 语义检索评估——向量召回补 FTS5 盲区，守住零 CGO（M3） | Accepted-in-principle（真实标注 A/B 门未过，仍默认关） |
| [0013](0013-vector-database-selection.md) | 本地向量存储选型（sqlite-vec，纯 Go）与安装引导（vector-init） | Accepted |
| [0014](0014-llm-wiki-knowledge-compiler.md) | LLM Wiki 知识编译层——摄入时编译、消费时取证（M6） | Proposed（W1/W1b/W4 已落地、W2 核心与前端已落地；其晋升条件仍需真实标注 query 集，W3 见 0015） |
| [0015](0015-w3-minimal-compile-loop.md) | W3 最小编译闭环——LLM 只写 pending 页，人批准才进检索 | Accepted |
| [0016](0016-async-batch-jobs.md) | 长时批任务的异步化——提交即返回、进度可查、结果可取 | Accepted |
| [0017](0017-one-kernel-two-entry-points.md) | 一个内核、两个入口——产品分层而不分叉仓库 | **Accepted（2026-10-13）**；条件 3 措辞于晋升当日由「第一安装路径」修订为「首屏叙事里的一等入口」，安装排序仍归 ADR-0009 管 |
| [0018](0018-knowledge-graph-foundation.md) | 知识图谱底座——受控关系词表让图可查，图感知取证让图被读 | **lane 1 Accepted（2026-10-12 真库演练 PASS）**；lane 2 **已落码（schema v23，2026-10-14）：页面向量空间 + 图感知取证（`wiki_graph_search` 默认关、零回退）+ 纯 Go PPR**，晋升 Proposed 待 Phase 0 delta（唯一硬门） |
| [0019](0019-entity-resolution-merge.md) | 实体消歧与概念合并——治「概念分裂」，合并永远由人触发 | Proposed（**2026-10-13 已落码**：结构 blocking + 合并事务 + 模型二判→`[lint] merge` 待办 + 审核页合并建议区；晋升条件 1-4 满足，条件 5 真库演练待用户跑；向量相似那一路仍待 lane 2） |
| [0020](0020-community-synthesis.md) | 社区摘要——用 Label Propagation 让 synthesis 层自己长出来 | Proposed（**2026-10-13 已落码**：待决 4 定为同步 + 硬预算，不做 job、不重建 `compile_jobs`；条件 1/2/3/5 满足，条件 4 真库演练待用户跑） |
| [0021](0021-temporal-graph-page-versions.md) | 时态图——`page_versions` 快照让「修订已批准结论」可以安全发生 | Proposed（**2026-10-13 地基已落码 = schema v22**：触发器写入 + append-only 守卫 + 逐页回填；条件 1/2 满足；条件 4 于同日改写为「真库落地后的回填对账」——真库当天在无演练下推进到 v22，原措辞失去对象，改写后的对账需先有 approved 页（批准是人独有的动作）；条件 3 随 `revise_page` 等待） |

## 约定

- 每个重大不可逆决策一篇：背景 → 决策 → 后果
- 被 superseded 的 ADR 保留原文与横幅，不删除
- 新 ADR 从 `0021` 递增编号，文件名 `NNNN-kebab-title.md`
