# dsh-plugin-reponest

DeepSeek Harness (`dsh`) 插件：把 RepoNest 本地的 git 分析能力暴露给 Harness 作为
三个模型可见的工具（Tool）。所有分析逻辑都在 RepoNest 共享的 Go service 层
（`internal/service`），本插件只是薄客户端，**桌面 App 与 Harness 共用同一份代码，零逻辑复制**。

## 架构

```
RepoNest 桌面 App  ─┐
                    ├─→ internal/service（单一事实源）
dsh-plugin-reponest ─┴─→ Headless HTTP 服务 (reponest-server) ─→ SQLite/FTS5
```

- 桌面 App：保持独立，走 Wails 绑定，完全不动。
- `reponest-server`：Go 编写的 headless HTTP 服务，复用 `internal/service`。
- 本插件：注册 3 个 Tool，调用 localhost 上的 headless 服务；可自启并托管其生命周期。

## 提供的工具

| Tool | 对应 RepoNest 能力 | 说明 |
|---|---|---|
| `reponest_ai_context` | Copy AI Context / llms.txt | 返回整库 AI 可读上下文（项目目录、技术栈、README 摘要、知识笔记） |
| `reponest_repo_overview` | 项目概览 | 技术栈 / 语言分布 / 依赖 / 主要贡献者 / 活跃度(Heatmap) / 最近提交 |
| `reponest_search` | FTS5 知识检索 | 跨笔记/Todo 的全文搜索 |

## 构建 headless 服务

```bash
# 在 RepoNest 仓库根目录
GOPROXY=https://goproxy.cn,direct GOSUMDB=off CGO_ENABLED=0 \
  go build -o reponest-server ./cmd/server
```

## 配置（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `REPONEST_HTTP_PORT` | `18765` | headless 服务端口（仅绑 127.0.0.1） |
| `REPONEST_SERVER_BIN` | 空 | headless 服务二进制路径；设置后插件自启它 |
| `REPONEST_AUTOSTART` | `1` | 设为 `0` 关闭自启（需你手动启动服务） |

## 安装到 dsh

方式一（推荐，外置插件）：

```bash
# 先装好依赖
cd dsh-plugin-reponest && pnpm install

# 注册到 web profile
pnpm dsh plugin --profile web add /abs/path/to/dsh-plugin-reponest

# 启动并设置环境变量
export REPONEST_SERVER_BIN=/abs/path/to/reponest-server
pnpm dsh web
```

方式二（patch 调试）：用仓库内的 `cordis.yml` 直接 `--patch`：

```bash
export REPONEST_SERVER_BIN=/abs/path/to/reponest-server
pnpm dsh web --patch ./dsh-plugin-reponest/cordis.yml
```

## 手动冒烟测试（无需 dsh）

启动 headless 服务后，用 curl 验证插件调用的同一批端点：

```bash
REPONEST_HTTP_PORT=18765 ./reponest-server &
curl -s http://127.0.0.1:18765/health
curl -s -X POST http://127.0.0.1:18765/api/ai_context
curl -s "http://127.0.0.1:18765/api/search?q=deploy"
curl -s http://127.0.0.1:18765/api/project/1/overview
```
