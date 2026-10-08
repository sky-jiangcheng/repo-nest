# ADR-0016: 长时批任务的异步化——提交即返回，进度可查，结果可取

- 状态：Proposed（本文是方案，未实现）
- 日期：2026-10-08
- 关联：[ADR-0015](0015-w3-minimal-compile-loop.md)（编译）、[ADR-0014](0014-llm-wiki-knowledge-compiler.md)（W3/W5）、TODO M6

## 背景

真库演练暴露的不是编译的正确性，而是它的**传输形状**。一次 `CompileProjectNotes` 在本地 27B 模型上跑了 173 秒（单条笔记），5 条则要十几分钟。同步 RPC 在这个尺度上有两个坏性质：

1. 客户端超时/断线时，**服务端的工作其实已经提交**——`cmd/server` 原先的 60s 写超时正是这样：curl 收到 `http=000`，而库里多了 3 张待审页。一次成功操作被报成失败，是能让用户从此不信任这个工具的那类 bug。已用「写超时对齐批量天花板 + 启动期漂移断言」止血（见 CHANGELOG），但那只是让同步调用不太会断，不是让它合理。
2. 同步意味着**界面必须挂在那里等**：进度看不见、中途不能取消、切走页面就失去结果。批量编译恰恰是最需要"我能走开，回来看结果"的交互。

## 决策

### 1. 任务表 + 提交/轮询语义，不改编译本身

`compile_jobs(id, project_id, requested_notes, status, notes_done, notes_total, pages_created, pages_updated, links_created, attachments, revision_todos, rejected_ops, stopped, llm_note, error, created_at, started_at, finished_at)`。`status IN ('queued','running','succeeded','failed','canceled')`。

编译内核（`wiki_compile.go`）保持现状不动——它已经是纯函数式的"给一条笔记、产出操作并落库"。异步化只是把它放进一个 job 循环里。**不搞第二套编译实现**。

### 2. 走既有的后台协程纪律，不引新调度器

job 由 `bgGo` 驱动的单个 worker 顺序消费（一次一个 job，避免和 SQLite 单写者抢锁），`Shutdown` 已经会等它。崩溃后 `running` 的 job 在下次启动时按 `started_at` 重新排队或标 `failed`——宁可显式失败，不假装续跑（我们不知道它断在哪一步）。

### 3. 取消是"停在下一条"，不是中断进行中的请求

正在等 LLM 的那一条**不抢占**：抢占要 ctx 穿透到 HTTP 客户端层，收益只是省一次已经花掉的请求，代价是整条调用链多一套取消语义。`canceled` 状态让循环在处理完当前笔记后停下。

### 4. 界面只在"有 job 在跑"时轮询

`GetCompileJob(jobID)` / `ListCompileJobs(projectID)`；面板显示进度（`notes_done/notes_total`）与结果摘要，完成后把审核队列的角标更新。不引 WebSocket——本仓事件通道（Wails events）在浏览器模式下要另接，而一个几秒一次的轮询在单机上没有任何成本问题。

## 后果

正面：长任务不再依赖任何超时数字侥幸对上；进度与取消成为一等状态；桌面/浏览器/`/api/rpc` 三种传输看到同一份任务记录。负面：多一张表和一套状态机，以及"点了按钮没立刻出结果"需要界面把等待表达清楚，否则会被读成卡死——这比同步版的体验风险更需要文案配合。批量编译的真实成本（本地模型每条 2-3 分钟）也不会因为异步化而变小，只是变得可见。

## 待决

1. job 表是否同时承载 lint 的模型侧（它同样长）——倾向**是**，共用一张 job 表与一套轮询；
2. 崩溃后 `running` 的处置（重排队 vs 标 failed）；
3. 是否需要并发多 job（默认单 worker，除非用户会同时编译多个项目）。

## 晋升条件（Proposed → Accepted）

1. 编译内核零改动即被复用（若出现第二套实现，说明本 ADR 的边界划错）；
2. 断连演练：任务运行中杀掉客户端，job 记录与产物完整，且状态最终收敛正确；
3. 取消演练：`canceled` 后不再处理新笔记，且不留下半条笔记的孤儿产物；
4. 崩溃恢复路径有测试（不能只靠注释说明"下次启动会怎样"）。
