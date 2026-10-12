# 贡献指南

Repotory（仓库与包标识 `repotory`）是本地优先的 Git 知识库桌面应用。欢迎 Issue 与 PR——动手前请花两分钟读完本文，尤其是「范围冻结」与「命名分层」两节，它们是本仓库最容易被好心 PR 违反的约定。

## 环境要求

| 依赖 | 版本 | 用途 |
|------|------|------|
| Go | 1.25+ | 后端（零 CGO，内嵌 SQLite） |
| Node.js | 20+ | 前端构建与测试 |
| Wails CLI | v2.13+（可选） | `wails dev` 开发模式 |

## 本地开发

```bash
# 前端依赖与构建（web/dist 被 go:embed 进二进制，改前端后需重新构建）
cd web && npm install && npm run build && cd ..

# 开发模式（前端热更新 + Wails 绑定注入）
wails dev

# 纯后端构建（桌面应用 + MCP server）
go build -ldflags="-s -w" -o repotory .
go build -o repotory-mcp ./cmd/mcp/
```

提交前请跑齐三件套：

```bash
go test ./...            # Go 全量测试（service/db/knowledge/scanner/diff…）
cd web && npm test       # vitest
cd web && npm run build  # tsc 严格检查（ESLint 现状见 TODO.md）
```

## 架构约定

- **业务逻辑只写在 `internal/service`**：Wails 桌面、headless HTTP（`cmd/server`）、MCP（`cmd/mcp`）三端共享同一实现；`internal/app` 是薄绑定层，每个方法 1-3 行委托，不放逻辑
- **改绑定面必须同步文档**：`internal/app` 的方法签名是对外契约，变更需同步 [docs/api/reference.md](docs/api/reference.md)
- **重大决策走 ADR**：在 [docs/adr/](docs/adr/index.md) 新增编号文件，说明背景、决策与代价；历史决策（分层、FTS5、块编辑器、范围冻结）见 ADR 索引

## 范围冻结（重要）

[ADR-0006](docs/adr/0006-scope-freeze.md) 冻结了产品范围：**核心是「发现 → 理解 → 记录 → 检索 → 交给 AI」闭环**，仪表盘与统计是支持能力；插件平台基础设施与 PWA 不再默认扩展。新增大功能前请先开 Issue 讨论，避免与范围声明冲突的投入。

## 命名分层（重要）

品牌展示层与机器标识**有意不一致**，完整规约见 [README 命名分层](README.md#命名分层)。速记：

- 品牌文案写 `Repotory`，完整展示名 `Repotory: Local Git Knowledge Base`，仓库/包标识写 `repotory`
- **不要「顺手统一」冻结标识**：二进制/命令名 `repotory`、数据目录 `repotory`（`internal/platform` 的 `dirName`）、MCP server 名 `repotory-mcp` 与工具前缀 `repotory_*`。它们绑定用户数据迁移史与 AI 客户端对外契约，改名是破坏性变更

## 提交与版本

- 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)（`feat:` / `fix:` / `docs:` / `refactor:` …），scope 用受影响的域（如 `feat(server):`、`fix(kb):`）
- 版本号 SSOT 是 `wails.json` 的 `info.productVersion`，用 `scripts/bump-version.sh` 同步到 `web/package.json`、`internal/version` 与文档站徽章，不要手改散点
- 面向用户的变更请同步更新 [CHANGELOG.md](CHANGELOG.md) 的 `[Unreleased]` 条目与对应手册页（`docs/**/*.md` 是唯一内容源，`docs/**/*.html` 是构建产物，**不要手改**）

## PR 检查清单

- [ ] `go test ./...`、`npm test`、`npm run build` 全绿
- [ ] 不触碰范围冻结与冻结标识（或已在 Issue/ADR 中说明理由）
- [ ] 绑定面变更已同步 API 参考；用户可见变更已更新手册与 CHANGELOG
- [ ] 新增依赖有明确理由（本项目依赖极简：wails、mcp-go、yaegi、modernc sqlite）

## 报告问题

Bug 与功能建议走 [GitHub Issues](https://github.com/sky-jiangcheng/repo-nest/issues)，请附版本号、平台与日志片段（日志位置见[故障排查](docs/troubleshooting.md)）。**安全漏洞不要开公开 Issue**，走[私密安全报告](SECURITY.md)。
