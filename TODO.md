# TODO — 待办清单

> 本文件只保留**未完成**事项与现状速查。查阅顺序：完成明细见 [CHANGELOG.md](CHANGELOG.md)；设计决策见 [docs/adr/index.md](docs/adr/index.md)；过程报告见 [outputs/](outputs/)；本文件的历史全文（含每项实施细节与教训）见压缩前快照：`git show 167c062:TODO.md`。
> 惯例：事项完成后从本清单移除并写入 CHANGELOG；设计类工作先落 ADR。

## 现状速查（新会话从这里开始）

- 品牌 **Repotory**（原 RepoNest），版本 **v2.4.0**（2026-10-16），SSOT 在 `wails.json`。
- schema **v24**（ADR-0023 笔记归属 `project_kind` 已随 v2.3.0 发布；`page_embeddings` 页面向量空间 + `page_embed_dirty` 增量队列，ADR-0018 lane 2 已落码；`evidencePages` 图感知取证 `wiki_graph_search` 默认关、零回退）；ADR-0018 lane 1（关系词表八型）已 **Accepted**（真库演练 PASS，产物 `v21-rehearse/`）。
- 近期大项均已落地：插件脚本运行时（yaegi）已移除（C11-B）、全局 CSS Modules 迁移收口（全局 1,227 行）、D25 首屏经 **ADR-0024** 演进为**首页工作台**——单栏融合仪表盘 + 记忆环条（`web/src/pages/Home.tsx`），全局搜索独立为 `/search` 顶级页（空闲态含热门项目 / 热门标签推荐，`web/src/pages/Search.tsx`），`/dashboard` 重定向到 `/`，导航以「搜索」取代「仪表盘」。
- **真库事故（2026-10-14，已恢复，损失见「用户动作」）**：`project_notes` 主表页级损坏（101 处 invalid page number，原因未定论；当日数据目录曾出现 15 个「删除时仍被打开」的 sidecar 残留，APFS 本地快照为空）。经 `.recover` 重建 + FTS 三表 rebuild 后换入，quick_check ok、integrity 83/100 零 fail；批页成果（7 页全 approved、7 份版本快照、2 条边）保住。**笔记 75 → 22 条**（AI1 丢 41 条，其余 12 条散布 6 个项目）。损坏原件与首次备份在 `~/Library/Application Support/repotory-corruption-backup-20261014/`。

---

## 当前待办

### M7 Lane 2：页面向量空间 + 图感知取证（schema v23）→ [ADR-0018](docs/adr/0018-knowledge-graph-foundation.md) 第二半 —— **代码已全部落码（2026-10-14）**，仅剩 Phase 0 硬门

- [x] `page_embeddings`（vec0，仅 approved 页）：物理独立于 `note_embeddings` + `page_embed_dirty` 触发器队列 + `memsrc` 无关的专用 `db.Page*` 助手；门控挂既有 `semantic_search` 之后；`vectorStore()` memo 失效路径对页面配置同样生效。回归：`pagevec_test.go` / `page_embed_dirty_test.go` / `page_embed_drain_test.go`
- [x] `evidencePages` 读图：`WITH RECURSIVE` 沿白名单（`ref`/`part-of`/`documents`，不沿 `contradicts`）1 跳扩召回 + 纯 Go PPR 替 bm25 定序；notes 与 pages 按类型分列交织；新门 `wiki_graph_search` 默认关，关时零行为回退（`TestEvidencePages_GraphOffIsFTSPure` 钉住逐条一致）
- [x] **Phase 0（唯一硬门）—— 两门均已 GATE PASS（2026-10-15，随 v2.2.0 发布）**：真实标注 query 集 + `abeval` delta 全部量出。脚手架：`cmd/queryset`（`-seed` 导出带行号工作表 / `-emit` 解析回 `queries.jsonl`）。**M3 门**：`.annotation-prep/notes.labeled.tsv`（51 条人工 query，覆盖 51 个 note id）→ k=3/5/10/20/50 全区间 PASS，k=10 时 lexical 0.608 / hybrid 0.735 / delta **+0.127**。**M7 lane 2 evidence 门**：`.annotation-prep/evidence-cases.jsonl`（16 条人工 query，标注 `page` 类型 id）→ k=2..20 全区间为正，k=10 时 FTS 0.719 / graph 0.812 / delta **+0.094**，逐案诊断确认 4 例增益全部形如「命中 page6 → 沿边带出 page7」、无一例回退。embedding 走 LM Studio `localhost:1234`（`text-embedding-nomic-embed-text-v1.5`, dim=768），跑门脚本 `.annotation-prep/run-gate.sh` / `run-evidence-gate.sh`（复制真库注入配置，不动 live、不改用户 `semantic_search`），逐案诊断 `cmd/abevaldiag`（两模式）。过程中修了 `internal/db/search.go` 的 CJK 词法不可达（`escapeFTSOR` 单段中文整句零召回 + LIKE 整句单子串匹配）与 `updated_at=''` 触发 `julianday` NULL 导致整条查询归零——修前 delta +0.338 里约六成是「CJK 词法坏了」的假信号，修后 +0.127 才是语义检索的真实增益。另修 `abevaldiag` 两臂开关时序（写在闭包外致两臂同值、delta 恒 0）。**两门的强度限定**：M3 的 delta 真实但幅度小；evidence 门的 delta 是在稀疏图（7 页 2 边）上的下限，不能外推为「图感知普遍优于 FTS」——要真正兑现 lane 2，需先让编译产出更多页与边再复跑。**门已过，剩下的都是决策**（M3 默认上线 / lane 2 晋升条件），见「用户动作」


### 后续路线（草案已成文，按序触发，各自待批准）

- **ADR-0019 概念合并 / ADR-0020 社区摘要 / ADR-0021 页面版本快照**：已于 2026-10-13 **全部落码**（明细见 CHANGELOG），三份仍标 Proposed——各自差一次「真库副本端到端演练 + 人审」，按惯例由用户亲自跑。批页已由 Phase 0 完成（2026-10-14：7 页全 approved，但仍仅 2 条边——图密度不足时社区检测跑了也是零结果，要等编译产出更多页与边）。实现期得到的与设计不同的事实见各 ADR 末尾「实现修正」节。
- 实施计划总览：[outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md](outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md)（Phase 1-4）

### 用户动作 / 外部依赖

- [ ] **真库事故后续（2026-10-14）**：① 源重导找回可再生的丢失笔记——四个源目录都在盘上（`~/.claude/projects`、`~/.codebuddy`、`~/.codex/sessions`、`~/.local/share/opencode`），设置→插件→立即导入，幂等键重导是更新；handoff 类笔记不可再生，只能从原会话人工补录。② 开启 Time Machine（本地快照当前为空）。③ 排查是否有同步类客户端在动 `~/Library/Application Support/repotory/`（当日 15 个「删除时仍被打开」的 sidecar 残留是最大嫌疑）。
- [ ] **桌面 GUI 真机冒烟**：扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答，并覆盖本轮新增的合并建议区 / 社区摘要 / 页面版本历史三块界面。（D25 的 ≤1080px 断点已实测修复——记忆环塌陷、搜索框与 handoff 列表被仪表盘遮盖；ADR-0017 已据此晋升 Accepted）
- [ ] **Hermes sessions 真机核验**：sessions 导入已落码（schema 由 TencentDB-Agent-Memory 的 shipped importer 确证，见 ADR-0011 实现修正节），差一次真机 `~/.hermes/state.db` 跑通 + 设置页点源验证
- [ ] **OMP 导入向**（等 OMP v1 稳定）；Bleve（需先做「是否替代 FTS5」架构决策）、Pinecone/Milvus（需凭据）按 `vectordb` registry 既有流程待接
- [ ] **M3 语义检索默认上线（决策）**：Phase 0 门已 GATE PASS（delta +0.127，k 全区间为正）。剩两步：① 用户决策是否把 `semantic_search` 默认打开（涉及「文本发往 embedding 端点」的隐私授权，默认值只能由用户定）；② 若开，需补一条说明默认行为变化的 CHANGELOG/文档条目与迁移口径（参照 v15 自动导入默认关那次「破坏性变更」的写法）
- [ ] **ADR-0018 lane 2 晋升 Accepted（决策）**：evidence 门已 GATE PASS（+0.094），但 delta 是稀疏图（7 页 2 边）上的**下限**。晋升条件是否写入该限定（如「图密度 ≥ N 条边时复跑本门仍为正」）未定——上一轮提过，待用户拍板
- [ ] **JetBrains 插件**：缓议，待真实需求信号（issue/star）+ 补 ADR 后立项

---

## 已完成

Sprint 1-14 与 M1-M7 Lane 1 全部落地（D1-D24、C1-C15、P3-P38、M6 W0-W5、品牌更名 v2.0.0、D25 首屏）。2026-10-13 另落地：M7 后续三份 ADR（0019/0020/0021）、D25 三项残留收口、工作区路径四阶解析与消费面契约统一、扩展版本跟随应用版本、分析类工具改走一次性副本。2026-10-14：Phase 0 前半（批页 7/7、integrity 复验通过、标注工作表导出）与真库损坏恢复（`.recover` 重建换入，见现状速查事故条目）。
明细与结论按版本见 [CHANGELOG.md](CHANGELOG.md)；单项实施细节、踩坑与教训见压缩前快照 `git show 167c062:TODO.md`；架构决策见 [docs/adr/index.md](docs/adr/index.md)。
