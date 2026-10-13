# TODO — 待办清单

> 本文件只保留**未完成**事项与现状速查。查阅顺序：完成明细见 [CHANGELOG.md](CHANGELOG.md)；设计决策见 [docs/adr/index.md](docs/adr/index.md)；过程报告见 [outputs/](outputs/)；本文件的历史全文（含每项实施细节与教训）见压缩前快照：`git show 167c062:TODO.md`。
> 惯例：事项完成后从本清单移除并写入 CHANGELOG；设计类工作先落 ADR。

## 现状速查（新会话从这里开始）

- 品牌 **Repotory**（原 RepoNest），版本 **v2.0.0**，SSOT 在 `wails.json`。
- schema **v22**（`page_versions` 页面快照）；ADR-0018 lane 1（关系词表八型）已 **Accepted**（真库演练 PASS，产物 `v21-rehearse/`）；**lane 2 的 schema 号相应改为 v23**。
- 近期大项均已落地：插件脚本运行时（yaegi）已移除（C11-B）、全局 CSS Modules 迁移收口（全局 1,227 行）、D25 首屏「记忆环 + 项目状态」并排（`web/src/pages/Home.tsx`）。

---

## 当前待办

### M7 Lane 2：页面向量空间 + 图感知取证（schema v23）→ [ADR-0018](docs/adr/0018-knowledge-graph-foundation.md) 第二半

- [ ] **Phase 0（前置，用户动作）**：真实标注 query 集 + `abeval` delta。脚手架已就绪：`cmd/queryset`（`-seed` 导出带行号工作表 / `-emit` 解析回 `queries.jsonl`）；标注须人判断（「哪条笔记能回答这个问题」是内容判断，工具只做 id 解析）。**同份标注兼作 M3 语义检索默认上线的门，一次标注两处复用**
- [ ] `page_embeddings`（vec0，仅 approved 页）：**物理独立于 `note_embeddings`**（`FuseRRF` 裸 int64 对混合 id 串号）+ `page_embed_dirty` 触发器队列（正文真变/迁入 approved 置脏、迁出删向量）；门控挂既有 `semantic_search` 之后不新设开关；`vectorStore()` memo 的失效路径对页面配置同样生效
- [ ] `evidencePages` 读图（`internal/service/wiki_evidence.go`，注释已预留注入点）：`WITH RECURSIVE` 沿 relation 白名单（`ref`/`part-of`/`documents`，不沿 `contradicts`）1-2 跳扩召回 + 纯 Go PPR 替 bm25 定序；notes 与 pages 按类型分列交织；新门 `wiki_graph_search` 默认关，关时零行为回退
- [ ] 晋升条件：delta 为正过 `abeval` 门；`page#N` 与 `note#N` 并存不串号专项回归；PPR 纯函数单测 + 递归 CTE 上界；三重预算兜得住邻居扩召回。设计全文见 ADR-0018

### 后续路线（草案已成文，按序触发，各自待批准）

- **ADR-0019 概念合并 / ADR-0020 社区摘要 / ADR-0021 页面版本快照**：已于 2026-10-13 **全部落码**（明细见 CHANGELOG），三份仍标 Proposed——各自差一次「真库副本端到端演练 + 人审」，按惯例由用户亲自跑。共同前置是**先批准若干 pending 页**（真库当前 7 页全 pending、仅 2 条边，社区检测必然零结果）。实现期得到的与设计不同的事实见各 ADR 末尾「实现修正」节。
- 实施计划总览：[outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md](outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md)（Phase 1-4）

### 用户动作 / 外部依赖

- [ ] **桌面 GUI 真机冒烟**：扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答，并覆盖本轮新增的合并建议区 / 社区摘要 / 页面版本历史三块界面。（D25 的 ≤1080px 断点已实测修复——记忆环塌陷、搜索框与 handoff 列表被仪表盘遮盖；ADR-0017 已据此晋升 Accepted）
- [ ] **Hermes sessions 真机核验**：sessions 导入已落码（schema 由 TencentDB-Agent-Memory 的 shipped importer 确证，见 ADR-0011 实现修正节），差一次真机 `~/.hermes/state.db` 跑通 + 设置页点源验证
- [ ] **OMP 导入向**（等 OMP v1 稳定）；Bleve（需先做「是否替代 FTS5」架构决策）、Pinecone/Milvus（需凭据）按 `vectordb` registry 既有流程待接
- [ ] **M3 语义检索默认上线**：等 Phase 0 标注集（见上）
- [ ] **JetBrains 插件**：缓议，待真实需求信号（issue/star）+ 补 ADR 后立项

---

## 已完成

Sprint 1-14 与 M1-M7 Lane 1 全部落地（D1-D24、C1-C15、P3-P38、M6 W0-W5、品牌更名 v2.0.0、D25 首屏）。2026-10-13 另落地：M7 后续三份 ADR（0019/0020/0021）、D25 三项残留收口、工作区路径四阶解析与消费面契约统一、扩展版本跟随应用版本、分析类工具改走一次性副本。
明细与结论按版本见 [CHANGELOG.md](CHANGELOG.md)；单项实施细节、踩坑与教训见压缩前快照 `git show 167c062:TODO.md`；架构决策见 [docs/adr/index.md](docs/adr/index.md)。
