# TODO — 待办清单

> 本文件只保留**未完成**事项与现状速查。查阅顺序：完成明细见 [CHANGELOG.md](CHANGELOG.md)；设计决策见 [docs/adr/index.md](docs/adr/index.md)；过程报告见 [outputs/](outputs/)；本文件的历史全文（含每项实施细节与教训）见压缩前快照：`git show 167c062:TODO.md`。
> 惯例：事项完成后从本清单移除并写入 CHANGELOG；设计类工作先落 ADR。

## 现状速查（新会话从这里开始）

- 品牌 **Repotory**（原 RepoNest），版本 **v2.1.0**（2026-10-14），SSOT 在 `wails.json`。
- schema **v23**（`page_embeddings` 页面向量空间 + `page_embed_dirty` 增量队列，ADR-0018 lane 2 已落码；`evidencePages` 图感知取证 `wiki_graph_search` 默认关、零回退）；ADR-0018 lane 1（关系词表八型）已 **Accepted**（真库演练 PASS，产物 `v21-rehearse/`）。
- 近期大项均已落地：插件脚本运行时（yaegi）已移除（C11-B）、全局 CSS Modules 迁移收口（全局 1,227 行）、D25 首屏「记忆环 + 项目状态」并排（`web/src/pages/Home.tsx`）。
- **真库事故（2026-10-14，已恢复，损失见「用户动作」）**：`project_notes` 主表页级损坏（101 处 invalid page number，原因未定论；当日数据目录曾出现 15 个「删除时仍被打开」的 sidecar 残留，APFS 本地快照为空）。经 `.recover` 重建 + FTS 三表 rebuild 后换入，quick_check ok、integrity 83/100 零 fail；批页成果（7 页全 approved、7 份版本快照、2 条边）保住。**笔记 75 → 22 条**（AI1 丢 41 条，其余 12 条散布 6 个项目）。损坏原件与首次备份在 `~/Library/Application Support/repotory-corruption-backup-20261014/`。

---

## 当前待办

### M7 Lane 2：页面向量空间 + 图感知取证（schema v23）→ [ADR-0018](docs/adr/0018-knowledge-graph-foundation.md) 第二半 —— **代码已全部落码（2026-10-14）**，仅剩 Phase 0 硬门

- [x] `page_embeddings`（vec0，仅 approved 页）：物理独立于 `note_embeddings` + `page_embed_dirty` 触发器队列 + `memsrc` 无关的专用 `db.Page*` 助手；门控挂既有 `semantic_search` 之后；`vectorStore()` memo 失效路径对页面配置同样生效。回归：`pagevec_test.go` / `page_embed_dirty_test.go` / `page_embed_drain_test.go`
- [x] `evidencePages` 读图：`WITH RECURSIVE` 沿白名单（`ref`/`part-of`/`documents`，不沿 `contradicts`）1 跳扩召回 + 纯 Go PPR 替 bm25 定序；notes 与 pages 按类型分列交织；新门 `wiki_graph_search` 默认关，关时零行为回退（`TestEvidencePages_GraphOffIsFTSPure` 钉住逐条一致）
- [ ] **Phase 0（唯一硬门，用户动作）**：真实标注 query 集 + `abeval` delta。脚手架就绪：`cmd/queryset`（`-seed` 导出带行号工作表 / `-emit` 解析回 `queries.jsonl`）。lane 2 的核心主张「图感知取证 > 纯 FTS 取证」必须量出 evidence+graph 相对 evidence 的正向 delta 才算兑现。**同份标注兼作 M3 语义检索默认上线的门，一次标注两处复用**。进度（2026-10-15）：语料已恢复至 58 条并重导 ✅；标注集完成 ✅ —— `.annotation-prep/notes.labeled.tsv`（51 条人工 query，覆盖 51 个 note id）→ `queries.jsonl`，生成器 `build_labeled.py` 可复现；M3 门 **GATE PASS**（k=3/5/10/20/50 全区间），k=10 时 lexical 0.608 / hybrid 0.735 / delta **+0.127**。embedding 走 LM Studio `localhost:1234`（`text-embedding-nomic-embed-text-v1.5`, dim=768），跑门脚本 `.annotation-prep/run-gate.sh`（复制真库注入配置，不动 live、不改用户 `semantic_search`），逐案诊断 `cmd/abevaldiag`。**过程中修了 `internal/db/search.go` 的 CJK 词法不可达**（`escapeFTSOR` 单段中文整句零召回 + LIKE 整句单子串匹配），修前 lexical 0.392 / delta +0.338 —— 其中约六成 delta 是「CJK 词法坏了」的假信号；另修 `updated_at=''` 触发 `julianday` NULL 导致整条查询归零。

  **M7 lane 2 evidence 门（2026-10-15 已落码并实跑）**：`abeval` 增 `-mode evidence`，两臂走真实 `GatherEvidence`（含预算与交织），唯一差异是 `wiki_graph_search` 0/1。标注集 `.annotation-prep/evidence-cases.jsonl`（16 条人工 query，标注 `page` 类型 id）→ **GATE PASS**，k=10 时 FTS 0.719 / graph 0.812 / delta **+0.094**，k=2..20 全区间为正（k=1 为 0，符合「图是扩召回不改首位」）。逐案诊断（`cmd/abevaldiag -mode evidence`）确认 4 例增益全部形如「命中 page6 → 沿边带出 page7」，**无一例回退**，增益可完全归因于图扩展。

  两处必要的类型改造：`internal/search/abeval` 泛型化为 `Case[T comparable]` 并新增 `DocID{Type,ID}` —— `wiki_pages.id` 与 `project_notes.id` 是独立序列，用裸 int64 打分会让 page #7 满足标注的 note #7（正是 wiki_evidence.go 记录的那个 RRF 串号陷阱）。`wiki_evidence_test.go` 原有的 `pageIDEpoch = 1_000_000` 手工编码已删除，改用 `DocID` 从类型上根除。

  **门的强度须打折看待**：真库 7 页仅 2 条边（`ref`），4 页孤立、无一条 `part-of`/`depends`/`documents`。delta 是真的，但它是**在这个稀疏图上的下限**，不能外推为「图感知取证普遍优于 FTS」。要真正兑现 lane 2，需先让编译产出更多页与边再复跑本门。

  **诊断工具自身踩过的坑（已修）**：`abevaldiag` 的 evidence 两臂初版把 `SetConfig` 写在闭包**外**，两个闭包共享最终值 `"1"`，两臂实际都跑图感知 → delta 恒为 0，且看起来像「图没增益」。`notes` 模式早在 `cmd/abeval/main.go` 有时序注释警告过这一点，我在 evidence 分支重犯。已改为闭包内切换并回退验证可复现。**任何双臂 harness 的开关必须在调用时翻，不是 setup 时翻**——一个分不清两臂的诊断比没有诊断更糟，它把「我抹掉了差异」报成「没有差异」。


### 后续路线（草案已成文，按序触发，各自待批准）

- **ADR-0019 概念合并 / ADR-0020 社区摘要 / ADR-0021 页面版本快照**：已于 2026-10-13 **全部落码**（明细见 CHANGELOG），三份仍标 Proposed——各自差一次「真库副本端到端演练 + 人审」，按惯例由用户亲自跑。批页已由 Phase 0 完成（2026-10-14：7 页全 approved，但仍仅 2 条边——图密度不足时社区检测跑了也是零结果，要等编译产出更多页与边）。实现期得到的与设计不同的事实见各 ADR 末尾「实现修正」节。
- 实施计划总览：[outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md](outputs/process-logs/Repotory-知识图谱实施计划-2026-10-09.md)（Phase 1-4）

### 用户动作 / 外部依赖

- [ ] **真库事故后续（2026-10-14）**：① 源重导找回可再生的丢失笔记——四个源目录都在盘上（`~/.claude/projects`、`~/.codebuddy`、`~/.codex/sessions`、`~/.local/share/opencode`），设置→插件→立即导入，幂等键重导是更新；handoff 类笔记不可再生，只能从原会话人工补录。② 开启 Time Machine（本地快照当前为空）。③ 排查是否有同步类客户端在动 `~/Library/Application Support/repotory/`（当日 15 个「删除时仍被打开」的 sidecar 残留是最大嫌疑）。
- [ ] **桌面 GUI 真机冒烟**：扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答，并覆盖本轮新增的合并建议区 / 社区摘要 / 页面版本历史三块界面。（D25 的 ≤1080px 断点已实测修复——记忆环塌陷、搜索框与 handoff 列表被仪表盘遮盖；ADR-0017 已据此晋升 Accepted）
- [ ] **Hermes sessions 真机核验**：sessions 导入已落码（schema 由 TencentDB-Agent-Memory 的 shipped importer 确证，见 ADR-0011 实现修正节），差一次真机 `~/.hermes/state.db` 跑通 + 设置页点源验证
- [ ] **OMP 导入向**（等 OMP v1 稳定）；Bleve（需先做「是否替代 FTS5」架构决策）、Pinecone/Milvus（需凭据）按 `vectordb` registry 既有流程待接
- [ ] **M3 语义检索默认上线**：等 Phase 0 标注集（见上）
- [ ] **JetBrains 插件**：缓议，待真实需求信号（issue/star）+ 补 ADR 后立项

---

## 已完成

Sprint 1-14 与 M1-M7 Lane 1 全部落地（D1-D24、C1-C15、P3-P38、M6 W0-W5、品牌更名 v2.0.0、D25 首屏）。2026-10-13 另落地：M7 后续三份 ADR（0019/0020/0021）、D25 三项残留收口、工作区路径四阶解析与消费面契约统一、扩展版本跟随应用版本、分析类工具改走一次性副本。2026-10-14：Phase 0 前半（批页 7/7、integrity 复验通过、标注工作表导出）与真库损坏恢复（`.recover` 重建换入，见现状速查事故条目）。
明细与结论按版本见 [CHANGELOG.md](CHANGELOG.md)；单项实施细节、踩坑与教训见压缩前快照 `git show 167c062:TODO.md`；架构决策见 [docs/adr/index.md](docs/adr/index.md)。
