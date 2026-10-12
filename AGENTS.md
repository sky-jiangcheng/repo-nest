# AGENTS — 开发入口（面向 AI agent / 新成员）

Repotory（原 RepoNest）：本地优先的代码项目知识库。Go + Wails 桌面/无头双端，React + TypeScript + Vite 前端，单 SQLite（FTS5 + sqlite-vec，零 CGO）。

## 按需查文档（按此顺序，避免全文探索）

1. **[TODO.md](TODO.md)** — 未完成事项 + 现状速查（品牌/版本/schema/近期大项）
2. **[CONTRIBUTING.md](CONTRIBUTING.md)** — 架构约定、命名分层、范围冻结、提交规范
3. **[docs/adr/index.md](docs/adr/index.md)** — 21 份架构决策（每份含背景/决策/代价）
4. **[CHANGELOG.md](CHANGELOG.md)** — 近期版本明细；v1.15.1 及更早在 [outputs/archive/CHANGELOG-v1.0-v1.15.md](outputs/archive/CHANGELOG-v1.0-v1.15.md)
5. **[outputs/](outputs/)** — 过程报告归档（P35 迁移清单、知识图谱实施计划、评估稿）

## 常用命令

```bash
# 后端测试（发布门）
go test ./...

# 前端测试 / 构建门 / lint（web/ 目录下）
npm test && npm run build && npm run lint

# 本地开发（无头 API 18731 + Vite 5173，双进程一键起）
bash scripts/dev.sh
```

## 高频陷阱

- **双语手册必须同批改**：`docs/**` 与 `docs/en/**` 是镜像，只改一边 = 双语漂移（历史上英文站曾讲出不存在的产品架构）
- **`docs/**/*.html` 是构建产物，不手改**；内容源是 `docs/**/*.md`
- **schema 变更**：`ExpectedSchemaVersion`（`internal/db/migrate.go`）与迁移测试是防漂移守卫，版本号必须同步推进且可逆测试跟上
- **CSS 语义色顺序不变量**由 `web/src/styles/semanticInk.test.ts` 钉住；改全局 CSS 顺序前先读它
- **冻结标识**（`repotory` 二进制名/数据目录/MCP 前缀 `repotory_*`）绑定用户数据迁移史，不「顺手统一」——见 CONTRIBUTING「范围冻结」
