# Repotory 执行计划 —— Hermes 导入 & Phase 0 标注

> 日期：2026-10-13。依据：[competitive-positioning.md](../../docs/competitive-positioning.md) 第五节、[TODO.md](../../TODO.md)、[ADR-0011](../../docs/adr/0011-multi-agent-memory-importers.md)、[ADR-0014 决策 3](../../docs/adr/0014-llm-wiki-knowledge-compiler.md)。
> 两项都以**用户动作为前置**（真实样例 / 人工标注），本文档明确划分「用户做什么」与「agent 做什么」，避免互相等待。

## 总览与依赖关系

```
Phase 0（标注）                         Hermes 导入
  │                                      │
  ├─ 0.1 语料先充实（含 Hermes 导入）◄────┤（curated memory 导入已落地，可先跑）
  ├─ 0.2 seed 工作表                      │
  ├─ 0.3 用户标注（人工，不可代做）        │
  ├─ 0.4 emit + abeval 基线               │
  └─ 0.5 双门复用（M3 默认开 + Lane 2）   │
                                         ├─ H.1 真实样例采集（用户动作）
                                         ├─ H.2 格式考古 + 脱敏 fixture
                                         ├─ H.3 ADR-0011 修订（schema 确认）
                                         ├─ H.4 sessions 导入实现
                                         └─ H.5 真机核验 + 收尾
```

关键顺序：**H.1/H.2 越早开始越好**——Hermes 会话数据是一次性机会（会话会滚动），而 Phase 0 的语料充实可以吃下 Hermes curated memory 导入的产出。

---

## Phase 0：真实标注 query 集 + abeval delta

### 0.1 语料充实（agent 可做，先于标注）

标注质量取决于语料是否有东西可标。2026-10-14 前的真库：7 页全 pending、笔记 4 条。

- [ ] 用户在设置里**批准若干 pending 页**（社区摘要/图谱的前提，Lane 2 也需要）——**仍待用户在 GUI 完成**
- [x] 跑知识源导入（agent 于 2026-10-14 直接逐源触发，结果）：claude 更新 1 条；codex 新建 1 条（55 条跳过：2 个 >32MB 超大会话 + 其余 cwd 无项目匹配，按"查不到项目就跳过不猜"设计）；opencode 0（目录无项目匹配）；openclaw 新建 6 条（先写 `openclaw_project=2` 指向同名项目，语义是路径精确对应）；hermes 0（本机无 memories、无 state.db）；cursor 0（1 会话因 workspace 无匹配被跳过）。**真库笔记 4 → 11 条**。
- [x] `repotory_integrity`（agent 以只读副本跑，无漂移）：FTS **pass**（5 笔记 / 6 待办全在索引，触发器齐全）；orphan **pass**；schema **pass**（真库已迁至 v23）；scan 覆盖 / 知识缓存各 **warn**（非标注阻塞项）。

验收：笔记数量足够支撑 ≥10 条真实问题（`queryset` 提示 <10 条时只是方向参考，不是证据）。

### 0.2 Seed 工作表（agent 执行）

```bash
go run ./cmd/queryset -seed > /tmp/notes.tsv
```

- 工具自动走一次性副本（`CopyForTooling`），**不碰真库**
- 输出含全部笔记行号清单 + 待填模板

### 0.3 人工标注（用户动作，工具无法代做）

编辑 `/tmp/notes.tsv`：

**2026-10-14 进展**：agent 已在 `outputs/process-logs/phase0-worksheet-draft.tsv` 预填 12 条候选问题（含猜测 `relevant`，行号与该文件内笔记清单一一绑定）。**这只是草稿**——用户必须逐条对照笔记内容确认/改正后（去掉行首 `#` 生效），才能 `-emit`；未核对直接使用 = 门禁失效（见上原则）。目标 ≥15 条，确认之余请补自己的真实问题。

- 每条问题必须是**你真会问的问题**（已有问题 > 编造问题）
- `relevant=` 填 1-3 个行号，**不凑数**（凑数把门禁结论变成噪音）
- 目标 **≥15 条**（10 条是可用下限，15 条以上单条波动才不主导结论）

标注原则：「哪条笔记能回答这个问题」是内容判断，工具只做 id 解析——这一步没有捷径，fabricated 标注集测的是猜测器不是检索。

### 0.4 Emit + 基线（agent 执行）

```bash
go run ./cmd/queryset -emit /tmp/notes.tsv -out queries.jsonl
go run ./cmd/abeval -cases queries.jsonl -k 10 -min-recall 0.05
```

- `queries.jsonl` 是长期资产，**随库保留**（建议入库到真库同目录或后续决定纳入版本管理）
- 记录本次基线数字（Recall@10 / NDCG@10）写入 CHANGELOG 或本文件附录

### 0.5 双门复用（按各自晋升条件触发）

| 门 | 条件 | 触发 |
|---|---|---|
| M3 语义检索默认上线 | hybrid-lexical recall@10 增益 ≥ 0.05 | 同一份 `queries.jsonl` |
| M7 Lane 2 图感知取证 | `wiki_graph_search` delta 为正（页面向量 + PPR 定序） | 同一份 `queries.jsonl`；额外做 `page#N` vs `note#N` 串号专项回归 |

一次标注、两处复用，这是把它排在所有 Lane 2 工作之前的理由。

---

## H：Hermes sessions 导入

### 背景

- 现状：[hermes.go](file:///workspace/internal/importers/hermes/hermes.go) 只导入 `~/.hermes/memories/*.md`；**sessions 故意不导入**，因为 schema 无文档且本机无样例（ADR-0011 决策 3/5：never guess a live format）
- 转机：TencentDB Agent Memory 原生支持 Hermes（provider 插件 + Docker 镜像），部署后会产生**格式明确的会话/记忆数据**——比猜格式靠谱的真实样例来源

### H.1 真实样例采集（用户动作，有时效性）

在装有 Hermes（或其 TencentDB 插件）的机器上：

1. 正常使用 Hermes 产生 2-3 个会话（含一次工具调用密集的任务，便于看到会话记录全貌）
2. 定位数据根：`~/.hermes/`（或 `$HERMES_HOME`），重点：`sessions/`、`state.db`、`memories/`
3. **安全红线（不可跳过）**：`.env`（密钥）、`mcp-tokens/` **绝不拷贝**；`state.db` 若需带走只取只读副本
4. 样例打包交给我（agent）

### H.2 格式考古（agent 执行）— ✅ 完成 2026-10-13，H.1 的"等真实样例"被更快的路径取代

- [x] 从 TencentDB-Agent-Memory 源码确证格式（`agents/asset-import.ts#scanSessions`）：会话主库 = `$HERMES_HOME/state.db` 的 `messages(session_id, role, content, tool_call_id, tool_calls, tool_name, timestamp)`，按 `(session_id, id)` 排序；`~/.hermes/sessions/` 只有原始 request dump，**不是**会话库
- [x] 确认 schema **不含 cwd/workspace 线索**（与 Codex 的 `session_meta.cwd` 不同）→ 归属只能走 `hermes_project` 配置
- [x] fixture 落在 `sessions_test.go`（程序生成，非脱敏文件）：镜像上述 schema，含两场会话 + 一场纯工具会话 + 空文本行

### H.3 ADR-0011 修订 — ✅ 完成 2026-10-13

- [x] 中文 ADR 追加「实现修正：Hermes sessions 已落地」节，明确 schema 由 shipped consumer 确证、解除决策 3/5 的 defer；英文 ADR 状态行与决策 3 同批更新
- [x] 导入映射：每会话 → 一条 log 笔记（首条提问 + 末条回复各截 3000 字符，同 codex 口径）
- [x] 边界保持：opt-in 手动源、只读 `state.db`、幂等 upsert、不碰 `.env`/`mcp-tokens`

### H.4 实现 — ✅ 完成 2026-10-13

- [x] `internal/importers/hermes/sessions.go`：`importSessions` + `probeSessionsLayout`（表/列漂移时**整源报错**，不安静导入 0 条）
- [x] `hermes.go` 的 `Import` 拆为 memories + sessions 两半合并；包注释更新
- [x] 附带下沉：cursor 的 `openReadOnly` → `memsrc.OpenReadOnly`（第 3 个 sqlite-backed importer，符合决策 2 的抽取时机）
- [x] 测试 5 例全绿（含漂移报错、配置归属、allowlist）；`go build ./internal/... ./cmd/...` + `go vet` 通过；全量 `go test ./...` 除根包缺 `web/dist` 外全绿
- [x] CHANGELOG 与双语手册（`docs/plugins/overview.md` ↔ `docs/en/plugins/overview.md`）同批更新

### H.5 真机核验 + 收尾（用户动作，未做）

- [ ] 用户在真机跑一次导入（设置 → 插件 → Hermes，手动触发），核验笔记条数与会话数对上、标题可读、项目挂载正确

---

## 里程碑与触发关系

1. **M-a**：Phase 0.3 标注完成 → 0.4 出基线 → M3 默认上线与 Lane 2 各自过门（一次标注两门复用）
2. **M-b**：H.2 格式说明定稿 → H.3 ADR 批准 → H.4 落码 → H.5 真机核验 → Hermes sessions 进 CHANGELOG
3. **交叉收益**：Hermes curated memory 导入（已落地）尽早跑，可充实 Phase 0 语料（0.1）；sessions 导入落地后，未来标注集可再吸收会话沉淀的笔记

## 风险

| 风险 | 缓解 |
|---|---|
| 标注 <10 条导致门禁结论是噪音 | queryset 已有警告；目标 15 条，达不到就继续标注再跑门 |
| 会话样例滚动丢失 | H.1 优先级最高，先采集后做其他 |
| 真库 pending 页导致图谱/社区零结果 | 0.1 明确列为用户前置动作 |
| 索引漂移污染基线 | 0.1 用 `repotory_integrity` 前置排除 |
