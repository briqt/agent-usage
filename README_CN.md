# agent-usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-blue?logo=docker)](https://ghcr.io/briqt/agent-usage)

轻量级、跨平台的 AI 编程工具用量与费用追踪器。  
单二进制 + SQLite —— 替代完整的 Grafana LGTM 可观测性栈。

**[English](README.md)**

## 为什么做这个

AI 编程工具（Claude Code、Codex 等）的使用数据分散在本地文件和遥测流中。要监控费用和 token 用量，通常需要一套复杂的可观测性栈（Grafana + Loki + Tempo + Prometheus + Alloy + MinIO + Redpanda = 7 个容器）。

**agent-usage** 用一个二进制文件和一个 SQLite 文件替代了这一切。

## 特性

- 📁 **本地文件解析** —— 直接读取 Claude Code 和 Codex CLI 的会话文件
- 💰 **自动费用计算** —— 从 [litellm](https://github.com/BerriAI/litellm) 获取模型价格，价格更新后自动回填历史记录
- 🗄️ **SQLite 存储** —— 单文件、零运维、数据可修正（不像 append-only 的日志存储）
- 📊 **Web 仪表板** —— 暗色主题 UI，ECharts 图表：费用分布、token 趋势、会话列表
- 🔄 **增量扫描** —— 监听新会话，自动去重
- 📦 **单二进制** —— `go:embed` 将 Web UI 打包进可执行文件
- 🖥️ **跨平台** —— Linux、macOS、Windows

## 快速开始

### Docker（推荐）

```bash
# 一条命令启动
docker compose up -d

# 打开仪表板
open http://localhost:9800
```

默认 `docker-compose.yml` 以只读方式挂载 `~/.claude/projects` 和 `~/.codex/sessions`，数据持久化在命名卷中。

### 从源码编译

```bash
# 克隆
git clone https://github.com/briqt/agent-usage.git
cd agent-usage

# 编译
go build -o agent-usage .

# 编辑配置
cp config.yaml config.local.yaml
# 按需调整路径

# 运行
./agent-usage

# 打开仪表板
open http://localhost:9800
```

## 配置

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"  # 远程访问请改为 "0.0.0.0"

collectors:
  claude:
    enabled: true
    paths:
      - "~/.claude/projects"
    scan_interval: 60s
  codex:
    enabled: true
    paths:
      - "~/.codex/sessions"
    scan_interval: 60s

storage:
  path: "./agent-usage.db"

pricing:
  sync_interval: 1h
```

## 支持的数据源

| 来源 | 会话路径 | 格式 |
|------|---------|------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<项目>/<会话>.jsonl` | JSONL |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<年>/<月>/<日>/<会话>.jsonl` | JSONL |

### 添加新数据源

每个数据源需要一个采集器：
1. 扫描会话目录中的 JSONL 文件
2. 解析条目，提取每次 API 调用的 token 用量
3. 通过存储层写入 SQLite

参考 `internal/collector/claude.go` 的实现。

## 仪表板

Web 仪表板提供：

- **汇总卡片** —— 总费用、总 token、会话数、提示数
- **模型费用分布** —— 饼图
- **费用趋势** —— 每日费用折线图
- **Token 用量趋势** —— 输入/输出 token 趋势
- **会话列表** —— 可排序表格，包含来源、项目、分支、费用详情
- **日期范围筛选** —— 聚焦任意时间段

## 架构

```
agent-usage
├── main.go                     # 入口，编排各组件
├── config.yaml                 # 配置文件
├── internal/
│   ├── config/                 # YAML 配置加载
│   ├── collector/
│   │   ├── claude.go           # Claude Code 会话扫描
│   │   ├── claude_process.go   # Claude Code JSONL 解析
│   │   └── codex.go            # Codex CLI JSONL 解析
│   ├── pricing/                # litellm 价格获取 + 计费公式
│   ├── storage/
│   │   ├── sqlite.go           # 数据库初始化 + 迁移
│   │   ├── api.go              # 查询类型 + 读取操作
│   │   ├── queries.go          # 写入操作
│   │   └── costs.go            # 费用重算 + 回填
│   └── server/
│       ├── server.go           # HTTP 服务 + REST API
│       └── static/             # 内嵌 Web UI（HTML + JS + ECharts）
└── agent-usage.db              # SQLite 数据库（运行时生成）
```

## 费用计算

价格从 [litellm 模型价格数据库](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) 获取并存储在本地。

```
费用 = (输入 - 缓存读取 - 缓存创建) × 输入价格
     + 缓存创建 × 缓存创建价格
     + 缓存读取 × 缓存读取价格
     + 输出 × 输出价格
```

价格更新后，历史记录会自动回填。

## API 接口

| 接口 | 说明 |
|------|------|
| `GET /api/stats?from=&to=` | 汇总统计 |
| `GET /api/cost-by-model?from=&to=` | 按模型分组的费用 |
| `GET /api/cost-over-time?from=&to=` | 每日费用序列 |
| `GET /api/tokens-over-time?from=&to=` | 每日 token 序列 |
| `GET /api/sessions?from=&to=` | 会话列表 |

## 技术栈

- **Go** —— 纯 Go 实现，无需 CGO
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) —— 纯 Go SQLite 驱动
- **ECharts** —— 图表库
- **`go:embed`** —— 单二进制部署

## Docker

预构建多架构镜像（amd64 + arm64）发布在 `ghcr.io/briqt/agent-usage`。

```bash
docker run -d \
  --name agent-usage \
  -p 9800:9800 \
  -v agent-usage-data:/data \
  -v ~/.claude/projects:/sessions/claude:ro \
  -v ~/.codex/sessions:/sessions/codex:ro \
  ghcr.io/briqt/agent-usage:latest
```

也可以使用 `docker compose up -d` 配合项目自带的 `docker-compose.yml`。

配置文件搜索顺序：`--config` 参数 > `/etc/agent-usage/config.yaml` > `./config.yaml`。

## 路线图

- [ ] 更多 agent 数据源（Cursor、Copilot、OpenCode 等）
- [ ] OTLP HTTP 接收端，支持实时遥测
- [ ] 系统服务管理（systemd / launchd / Windows Service）
- [ ] 导出 CSV/JSON
- [ ] 告警（费用阈值）
- [ ] 多用户支持

## 许可证

[Apache 2.0](LICENSE)
