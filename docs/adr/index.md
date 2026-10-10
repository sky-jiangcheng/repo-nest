---
title: 架构决策（ADR）
order: 21
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
| [0017](0017-one-kernel-two-entry-points.md) | 一个内核、两个入口——产品分层而不分叉仓库 | Proposed |
| [0018](0018-knowledge-graph-foundation.md) | 知识图谱底座——受控关系词表让图可查，图感知取证让图被读 | Proposed（lane 1 = 关系词表 + Wikilink + 结构 lint，schema v21；lane 2 = 页面向量 + 图感知取证，schema v22；两半各自晋升） |
| [0019](0019-entity-resolution-merge.md) | 实体消歧与概念合并——治「概念分裂」，合并永远由人触发 | Proposed（仅设计，未落码；排在 ADR-0018 lane 1 Accepted 之后；结构 blocking 可独立推进，向量相似那一路待 lane 2） |

## 约定

- 每个重大不可逆决策一篇：背景 → 决策 → 后果
- 被 superseded 的 ADR 保留原文与横幅，不删除
- 新 ADR 从 `0019` 递增编号，文件名 `NNNN-kebab-title.md`
