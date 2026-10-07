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
| [0010](0010-session-auto-capture.md) | 会话自动捕捉——零人工参与的 Claude Code 会话交接（M1） | Proposed |
| [0011](0011-multi-agent-memory-importers.md) | 多 agent 记忆源导入——可复用 importer 框架与各源可行性（M2） | Proposed |
| [0012](0012-semantic-search.md) | 语义检索评估——向量召回补 FTS5 盲区，守住零 CGO（M3） | Proposed |
| [0013](0013-vector-database-selection.md) | 本地向量存储选型（sqlite-vec，纯 Go）与安装引导（vector-init） | Accepted |
| [0014](0014-llm-wiki-knowledge-compiler.md) | LLM Wiki 知识编译层——摄入时编译、消费时取证（M6） | Proposed |

## 约定

- 每个重大不可逆决策一篇：背景 → 决策 → 后果
- 被 superseded 的 ADR 保留原文与横幅，不删除
- 新 ADR 从 `0014` 递增编号，文件名 `NNNN-kebab-title.md`
