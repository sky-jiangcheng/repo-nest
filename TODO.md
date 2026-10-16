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

- [ ] **真库事故后续（2026-10-14）**：① 源重导找回可再生的丢失笔记——**重导 runbook**：源目录在盘上但内容不均（核查 2026-10-16：`~/.claude/projects` 与 `~/.codex/sessions` 为 0 文件、重导无所得；`~/.codebuddy` 5454 文件、`~/.local/share/opencode` 16263 文件才有实际内容），设置→插件→对应源「立即导入」，幂等键 `(project_id, source, title)` 重导是更新不是堆叠；handoff 类笔记不可再生，只能从原会话人工补录。② 备份：**用户已于 2026-10-16 开启 Time Machine 每日自动备份**，数据目录 `~/Library/Application Support/repotory/`（约 5.1 MB）已核实**不在 TM 排除列表**。剩余一步复查（需先连接「时光机」备份磁盘）：确认快照内确实存在该路径——「未被排除」只保证下次备份会进快照，不保证历史快照已有。③ 干扰源定位（**进展 2026-10-16**）：数据目录 `~/Library/Application Support/repotory/` 内现存 **15 个 `.fuse_hidden*` 隐藏文件（10-14 17:43–19:14，与损坏窗口精确吻合、计数即当日记录的 15 个 sidecar 残留）**——该命名是 **FUSE 挂载类工具**（rclone mount / netdrive / clouddrive / s3fs 一类云盘同步）的典型残留，指向「10-14 当天有 FUSE 挂载客户端在动数据目录」；当前（2026-10-16）无 fuse 挂载、无同步类进程在跑，**嫌疑源为 10-14 当时已停止，需用户回忆当天是否挂过云盘/远程目录到此路径**；`~/.hermes/state.db` 目前不存在，Hermes 真机核验项暂不可跑。
  - **备份验证命令（连接备份磁盘后执行）**：
    1. 复查快照内确有该路径：图形化「系统设置 → General → Time Machine → 浏览历史备份」定位 `…/repotory/repotory.db`；或命令行 `tmutil listbackups` 取快照名 `<N>`，`ls /Volumes/<TM盘>/Backups.backupdb/<主机名>/Data/<N>/Users/jiangcheng/Library/Application\ Support/repotory/`
    2. `tmutil status` 确认会话正常结束（Running=0、无 error）
    3. **脱开陷阱**：TM 只在备份盘连接时备份，「每日」在盘脱开期间不成立；本地快照（APFS、存本机、免备份盘）是离线兜底——查看 `sudo tmutil listlocalsnapshotdates`，手动造一份 `sudo tmutil localsnapshot`（本机无 `localsnapshots enable`，已实测）。**恢复**：本机 `tmutil` 无 `restore` verb（`tmutil help` 已核实）——走 Time Machine 备份浏览器，或最稳的兜底：直接挂载备份盘手工拷回 `/Volumes/<TM盘>/Backups.backupdb/<主机名>/Data/<快照>/` 下的 `repotory.db`（单文件库，拷文件即恢复）
- [ ] **桌面 GUI 真机冒烟**：扫描→收藏→刷新历史→笔记 CRUD→版本恢复→知识库搜索→MCP 问答，并覆盖本轮新增的合并建议区 / 社区摘要 / 页面版本历史三块界面。（D25 的 ≤1080px 断点已实测修复——记忆环塌陷、搜索框与 handoff 列表被仪表盘遮盖；ADR-0017 已据此晋升 Accepted）
- [ ] **Hermes sessions 真机核验**：sessions 导入已落码（schema 由 TencentDB-Agent-Memory 的 shipped importer 确证，见 ADR-0011 实现修正节），差一次真机 `~/.hermes/state.db` 跑通 + 设置页点源验证。（2026-10-16 核查：本机 `~/.hermes/state.db` 尚不存在，需先产生 Hermes 会话数据再跑通）
- [ ] **OMP 导入向**（等 OMP v1 稳定）；Bleve（需先做「是否替代 FTS5」架构决策）、Pinecone/Milvus（需凭据）按 `vectordb` registry 既有流程待接
- [ ] **M3 语义检索默认上线（决策）**：Phase 0 门已 GATE PASS（delta +0.127，k 全区间为正）。剩两步：① 用户决策是否把 `semantic_search` 默认打开（涉及「文本发往 embedding 端点」的隐私授权，默认值只能由用户定）；② 若开，需补一条说明默认行为变化的 CHANGELOG/文档条目与迁移口径（参照 v15 自动导入默认关那次「破坏性变更」的写法）
  - **草案（待拍板后落地）**：若用户选择「默认开」，CHANGELOG `[Unreleased]` 拟写：「**变更**：`semantic_search` 默认值由关改为开。开启后命中语义检索时会把待检索文本发往你在设置页配置的 embedding 端点（默认 `localhost:1234`，即本机 LM Studio）；未配置端点或端点不可达时自动回退纯 FTS5，行为与之前一致。隐私口径：仅当你**主动配置**了端点且端点可达时才会外发，端点为空即零外发。迁移口径：既有用户开库时若 `semantic_search` 仍为默认关（未显式改过），本次升级**不自动翻转**其值——仅对新安装/未配置库把默认置为开，避免对存量用户的静默隐私行为变更」。文档同步 `docs/features/ai-integration.md` 与 `docs/en/features/ai-integration.md` 的语义检索默认值说明。
- [ ] **ADR-0018 lane 2 晋升 Accepted（决策）**：evidence 门已 GATE PASS（+0.094），但 delta 是稀疏图（7 页 2 边）上的**下限**。晋升条件是否写入该限定（如「图密度 ≥ N 条边时复跑本门仍为正」）未定——上一轮提过，待用户拍板
  - **草案（待拍板）**：建议在 ADR-0018 lane 2 的晋升条件写入如下限定条款原文：「**晋升前置限定**：当前 evidence delta（+0.094 @k=10）度量于 7 页 2 边的稀疏图，为该机制的**下限**而非稳态收益。本 ADR 晋升 Accepted 需满足：① 编译产出使图密度达到 **≥ 20 条边且 ≥ 15 个 approved 页**（现 7 页 2 边远未到）；② 在该密度下复跑 `cmd/abeval -mode evidence`，delta 仍 **> +0.05**（沿用 M3 语义门的最低可感知增益阈值）且 k=2..20 全区间为正、无一例回退；③ 逐案诊断（`cmd/abevaldiag -mode evidence`）确认增益仍形如「命中种子页→沿白名单边带出邻接页」。未达 ①②③ 前 lane 2 维持 Proposed、`wiki_graph_search` 维持默认关。」
- [ ] **真库副本演练（一次覆盖 0019/0020/0021/0023 晋升条件 5/4）**：四份 ADR 落码完成、只差用户亲自「真库副本端到端演练 + 人审」。建议在同一份 `repotory.db` 副本上按序一次性跑通（`.recover` 副本，不碰 live）：① 触发 `wiki_lint` 合并建议 → 在审核页人审并手动采纳一条概念合并（验证 ADR-0019 无自动路径不变量）；② 开启 `wiki_community_summary` 跑一次 → 人审产出的 synthesis 页（验证 ADR-0020 预算与门控）；③ 批准/修订一张已批准页 → 检查 `page_versions` 快照是否触发写入（验证 ADR-0021 append-only）；④ 跑一次导入 → 确认无匹配项目落占位/未分类、整理区可归口（验证 ADR-0023 v24 迁移可逆）。全部 PASS 后，四份 ADR 各自晋升 Accepted 并回填「实现修正」节。
- [ ] **图密度欠账（lane 2 兑现前置，承接上方 lane 2 草案 ①）**：现图 7 页 2 边（4 页孤立），社区检测与图感知取证都因密度不足「跑了也是零结果/只是下限」。需在编译管线（`wiki_compile`）产出更多页与边、且人审把 pending 批成 approved 后，才谈得上「兑现 lane 2」。可作一个子目标：**让真库 approved 页 ≥ 15、边 ≥ 20**，作为复跑 evidence 门（见 lane 2 草案 ②）的触发条件；触发时先跑 `cmd/wiki-rehearse`（`v21-rehearse/` 现有脚手架）导出当前图密度报告，确认达标后再 `cmd/abeval -mode evidence` 复跑。
- [ ] **JetBrains 插件**：缓议，待真实需求信号（issue/star）+ 补 ADR 后立项

---

## 已完成

Sprint 1-14 与 M1-M7 Lane 1 全部落地（D1-D24、C1-C15、P3-P38、M6 W0-W5、品牌更名 v2.0.0、D25 首屏）。2026-10-13 另落地：M7 后续三份 ADR（0019/0020/0021）、D25 三项残留收口、工作区路径四阶解析与消费面契约统一、扩展版本跟随应用版本、分析类工具改走一次性副本。2026-10-14：Phase 0 前半（批页 7/7、integrity 复验通过、标注工作表导出）与真库损坏恢复（`.recover` 重建换入，见现状速查事故条目）。
明细与结论按版本见 [CHANGELOG.md](CHANGELOG.md)；单项实施细节、踩坑与教训见压缩前快照 `git show 167c062:TODO.md`；架构决策见 [docs/adr/index.md](docs/adr/index.md)。
