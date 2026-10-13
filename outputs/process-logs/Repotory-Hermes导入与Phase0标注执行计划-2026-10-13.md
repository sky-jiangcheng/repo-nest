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

标注质量取决于语料是否有东西可标。当前真库 7 页全 pending、笔记量少。

- [ ] 用户在设置里**批准若干 pending 页**（社区摘要/图谱的前提，Lane 2 也需要）
- [ ] 跑一次已落地的 Hermes / Claude curated memory 导入（opt-in 手动），充实笔记量
- [ ] `repotory_integrity` 过一遍，确认 FTS 索引无漂移（**索引漂移会让检索静默漏结果，标注前必须排除**）

验收：笔记数量足够支撑 ≥10 条真实问题（`queryset` 提示 <10 条时只是方向参考，不是证据）。

### 0.2 Seed 工作表（agent 执行）

```bash
go run ./cmd/queryset -seed > /tmp/notes.tsv
```

- 工具自动走一次性副本（`CopyForTooling`），**不碰真库**
- 输出含全部笔记行号清单 + 待填模板

### 0.3 人工标注（用户动作，工具无法代做）

编辑 `/tmp/notes.tsv`：

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

### H.2 格式考古（agent 执行）

- [ ] 解析 sessions 文件格式（参考 codex importer 的 `rollout-*.jsonl` 流式解析经验）
- [ ] `state.db` 只读打开，dump 表结构，确认是否为会话主存储
- [ ] 产出格式说明（字段、分段方式、时间戳语义），落 fixture 到 `internal/importers/hermes/testdata/`（**脱敏后**）
- [ ] 与 TencentDB 插件的写入行为对照：确认哪些文件是 Hermes 原生、哪些是插件追加

### H.3 ADR-0011 修订（agent 起草，用户批准）

- [ ] 在「实现修正」惯例下补充：sessions schema 已确认（引用 H.2 说明），解除决策 3/5 的 defer
- [ ] 明确导入映射：每会话 → 一条 log 笔记（对齐 codex importer 先例），项目匹配策略（cwd / directory 末段）
- [ ] 保持既定边界：**opt-in 手动源**、allowlist 范围、幂等 upsert、不碰 `.env`/`mcp-tokens`

### H.4 实现（agent 执行）

- [ ] `internal/importers/hermes/`：新增 sessions 解析，复用 `memsrc` 的 ReadCapped / StripFrontmatter / ClipToBytes
- [ ] 测试：fixture 单测 + 幂等重跑 + 超长内容截断；hermes_test.go 现有 memories 用例保持绿
- [ ] `go test ./...` 全绿

### H.5 真机核验 + 收尾（用户 + agent）

- [ ] 用户在真机跑一次导入（设置 → 知识源 → Hermes，手动触发）
- [ ] 核验：笔记条数与会话数对上、标题可读、项目挂载正确
- [ ] CHANGELOG 记录；**双语手册同批改**（`docs/**` 与 `docs/en/**` 的 knowledge sources 页）

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
