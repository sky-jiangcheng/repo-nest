# ADR-0010: 会话自动捕捉——零人工参与的 Claude Code 会话交接（M1）

- 状态：Accepted-in-principle（解析核心 + 按需捕获（服务/绑定/前端）+ B端 SessionEnd-hook CLI `cmd/repotory-capture` 均已落地；`claude_session_capture` 默认关，走共享 `CreateHandoffNote`，未开启零隐私暴露）
- 日期：2026-10-02
- 关联：[ADR-0007](0007-session-memory-protocol.md)（context/handoff 协议与 `handoff` 标签）、[ADR-0009](0009-ide-presence.md)（SessionEnd hook 一键接入）、[ADR-0006](0006-scope-freeze.md)（范围冻结）、TODO 会话记忆路线 M1

## 背景

今天 Repotory 的「交接」是**由 agent 主动推送**的：Claude Code 等客户端在会话里调用 `repotory_handoff`（MCP），把结构化的 exit record 交给 `internal/service/handoff.go` 的 `CreateHandoffNote` 落成一条带 `handoff` 标签的笔记，`repotory_context` 再把它排在下次会话的最前（`internal/service/context.go`）。这条链路的前提是 **agent 记得、且愿意去调那个工具**。一旦用户/agent 忘了调，会话就无痕流失。

M1 要补的是「零人工参与」的下限：**不依赖 agent 主动调用**，由 Repotory 直接读取 Claude Code 自己落盘的原始会话记录 `~/.claude/projects/<project-slug>/<session-uuid>.jsonl`（逐行 JSON 事件流：user/assistant 消息、tool_use、tool_result），从中提取「最后一段 assistant 结论 + 关键工具调用摘要」自动生成一条 handoff。

注意与既有 `internal/importers/claude` 的**边界**：那个 importer 读的是 `~/.claude/projects/*/memory/*.md`（Claude 手动维护的**记忆文档**），走 `plugin.KnowledgeImporter` 通道；M1 读的是 `*.jsonl` **原始逐字会话**，是另一份、更敏感、格式更不稳定的数据源。两者不可混为一谈。

## 决策（提案，均需拍板后方实现）

1. **默认关，显式开**。自动捕捉读取的是完整对话逐字稿，属敏感数据。能力默认关闭，在 设置 → AI 集成 下单独开关；未开启时任何代码路径都不得触碰 `~/.claude/projects/*.jsonl`。
2. **本地只读 + 路径白名单**。仅读 `~/.claude/projects/<slug>/*.jsonl`，不遍历其它目录、不外传、不进任何遥测。落盘 handoff 笔记复用 `CreateHandoffNote`（本地 SQLite），产物与 agent 推送的 handoff 同构，`repotory_context` 无差别消费。
3. **有界提取，控体积与隐私**：只解析尾部 N 条消息（默认建议 N≈最后一段 assistant + 其前一条 user + 其间 tool_use 名称），不整文件入库、不保留逐字全文；生成的是「摘要级」handoff 而非会话存档。原始 jsonl 不回写、不复制。
4. **触发方式二选一（待决）**：(A) 事件驱动——复用 ADR-0009 的 SessionEnd hook，会话结束时调一个只读端点/CLI 触发解析；(B) 按需扫描——设置页「导入最近会话」按钮 + 可选手动扫描。两者都不引入后台轮询常驻。推荐先做 (A)（真正零人工），(B) 作兜底。
5. **格式漂移防护**：Claude Code 的 jsonl schema 非公开契约、随版本变。解析器必须「宽松失败」——字段缺失/结构不认识时跳过并记日志，绝不因单条记录解析失败中断扫描；以 golden-file 测试锁定当前已知的几种事件形状。

## 理由

- 「零人工参与」是产品级卖点（会话记忆环自动闭合），但其代价是直接读取全量对话逐字稿——这是隐私与合规的实质扩张，必须由用户显式知情并选择开启，不能默认生效。
- 有界提取（尾部 + 摘要）同时压低体积与敏感面：handoff 的价值在「下一次会话能接上」，不在「回放整段历史」。
- 复用既有 `handoff` 标签与 `CreateHandoffNote`/`repotory_context` 链路，意味着自动捕捉只是**多一个 handoff 生产者**，不新增消费端、不碰 ADR-0006 冻结面。

## 后果

- 正面：不依赖 agent 自觉也能沉淀会话交接；记忆环对「忘了调工具」的用户自动兜底。
- 负面/风险：绑定 Claude Code 未公开的 jsonl 格式，需持续跟随其变更；引入一个敏感只读路径，隐私声明与文案必须同步。
- **待决（实现前需回答）**：① 默认触发用 (A) hook、(B) 按需，还是都上；② 尾部 N 的取值与是否需要「去工具结果」红白名单；③ 生成的 handoff 是否标注「自动捕捉」来源以与 agent 推送区分；④ 非 Claude 客户端（见 ADR-0011）是否纳入同一机制。

## 决策落地（2026-10-02 产品拍板）

1. **按用户分层（C端 / B端）**：这是与 M3 embedding 路线共享的一条产品分层。
   - **C端（个人用户）**：会话逐字稿敏感度最高，**默认关**，只走**按需**触发（设置里手动「捕获最近会话」），风险自负面最小。
   - **B端（企业用户）**：有自有风控、愿意接受更强自动化（含远程服务，见 ADR-0012）。可开启 hook 式自动捕捉，但**必须在文档里把「读取本地会话逐字稿」这一风险项与相关配置显式说明清楚**，由 B端自行评估并承担其合规决定；Repotory 不替其默认开启。
2. **默认关 + 显式开**：无论 C/B，功能安装即关；开启是用户的一次明确动作。
3. **触发**：C端＝按需按钮；B端＝可选 SessionEnd hook（复用 ADR-0009 已交付的 hook 接入），二者都不引入后台轮询。
4. **风险文档为交付物的一部分**：设置项与文档需写清「本功能会读取 `~/.claude/projects/*.jsonl` 会话逐字稿（含潜在敏感信息），产物仅存本地 SQLite、不外传」，并把 hook 自动触发标为面向 B端的进阶项。
5. 待补（实现细化）：`Session`→`service.HandoffInput` 映射的字段取舍（最后 assistant 文本进 Summary、工具名进 Changes、首次指令进 title 上下文）、是否给自动 handoff 加「auto-captured」标签与 agent 推送版区分。
6. **接线进度（2026-10-03）**：按需捕获已落地——`claude.LatestSessionForRootPath`（按 `path→slug` 约定找最新会话）+ `Service.CaptureClaudeHandoff(projectID)`：受 `claude_session_capture` 配置门控（默认关，仅 `=="1"` 才读盘），映射 `Session`→`HandoffInput`（最后 assistant 文本→Summary，缺则退回首条指令；工具名 + git 分支→Changes，恒满足「至少一节」；打 `auto-captured` 标签区分），走共享 `CreateHandoffNote`。desktop binding `App.CaptureClaudeHandoff` 已加。**仍待**：B端 SessionEnd-hook 自动化触发（复用 ADR-0009 hook）。
7. **前端入口已接（2026-10-03）**：`web/src/api/endpoints.ts` 加 `captureClaudeHandoff`（transport 按方法名字符串动态派发 Wails 绑定，无需手改 wailsjs）；`Settings → PluginsTab` 新增 Claude 捕捉开关 + 「按项目 ID 捕捉最近会话」按钮 + `openclaw_project`/`hermes_project` 目标项目输入（让 opt-in 全局记忆源人类可配）。tsc + vitest 端点契约测试通过。
8. **B端 SessionEnd-hook 自动化已接（2026-10-03）**：`cmd/repotory-capture` CLI 作为 Claude Code SessionEnd hook 目标（从 `-cwd`/stdin `.cwd`/进程 cwd 解析会话目录）→ `Service.CaptureClaudeSessionByCwd`（cwd 末段匹配项目 + 走同一 `CaptureClaudeHandoff`）。仍受 `claude_session_capture` 默认关约束；未开启/无匹配项目时打印错误但**退出码 0**，绝不打断用户会话收尾。`repotory-init` 的 hook 示例指向此命令。
