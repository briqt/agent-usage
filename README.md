# agent-usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()

Lightweight, cross-platform AI coding agent usage & cost tracker.  
Single binary + SQLite — replaces a full Grafana LGTM observability stack.

**[中文文档](README_CN.md)**

## Why

AI coding tools (Claude Code, Codex, etc.) generate usage data across scattered local files and telemetry streams. Monitoring costs and token usage typically requires a complex observability stack (Grafana + Loki + Tempo + Prometheus + Alloy + MinIO + Redpanda = 7 containers).

**agent-usage** replaces all of that with a single binary and one SQLite file.

## Features

- 📁 **Local file parsing** — reads Claude Code and Codex CLI session files directly
- 💰 **Automatic cost calculation** — fetches model pricing from [litellm](https://github.com/BerriAI/litellm), supports backfill when prices update
- 🗄️ **SQLite storage** — single file, zero ops, data is correctable (unlike append-only log stores)
- 📊 **Web dashboard** — dark-themed UI with ECharts: cost breakdown, token trends, session list
- 🔄 **Incremental scanning** — watches for new sessions, deduplicates automatically
- 📦 **Single binary** — `go:embed` packs the web UI into the executable
- 🖥️ **Cross-platform** — Linux, macOS, Windows

## Quick Start

```bash
# Clone
git clone https://github.com/briqt/agent-usage.git
cd agent-usage

# Build
go build -o agent-usage .

# Edit config
cp config.yaml config.local.yaml
# Adjust paths if needed

# Run
./agent-usage

# Open dashboard
open http://localhost:9800
```

## Configuration

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"  # use "0.0.0.0" for remote access

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

## Supported Data Sources

| Source | Session Location | Format |
|--------|-----------------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |

### Adding New Sources

Each source needs a collector that:
1. Scans session directories for JSONL files
2. Parses entries and extracts token usage per API call
3. Writes records to SQLite via the storage layer

See `internal/collector/claude.go` as a reference implementation.

## Dashboard

The web dashboard provides:

- **Summary cards** — total cost, tokens, sessions, prompts
- **Cost by model** — pie chart breakdown
- **Cost over time** — daily cost trend line
- **Token usage over time** — input/output token trends
- **Session list** — sortable table with source, project, branch, cost details
- **Date range filter** — focus on any time period

## Architecture

```
agent-usage
├── main.go                     # Entry point, orchestrates components
├── config.yaml                 # Configuration
├── internal/
│   ├── config/                 # YAML config loader
│   ├── collector/
│   │   ├── claude.go           # Claude Code session scanner
│   │   ├── claude_process.go   # Claude Code JSONL parser
│   │   └── codex.go            # Codex CLI JSONL parser
│   ├── pricing/                # litellm price fetcher + cost formula
│   ├── storage/
│   │   ├── sqlite.go           # DB init + migrations
│   │   ├── api.go              # Query types + read operations
│   │   ├── queries.go          # Write operations
│   │   └── costs.go            # Cost recalculation + backfill
│   └── server/
│       ├── server.go           # HTTP server + REST API
│       └── static/             # Embedded web UI (HTML + JS + ECharts)
└── agent-usage.db              # SQLite database (generated at runtime)
```

## Cost Calculation

Pricing is fetched from [litellm's model price database](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) and stored locally.

```
cost = (input - cache_read - cache_creation) × input_price
     + cache_creation × cache_creation_price
     + cache_read × cache_read_price
     + output × output_price
```

When prices update, historical records are automatically backfilled.

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/stats?from=&to=` | Summary statistics |
| `GET /api/cost-by-model?from=&to=` | Cost grouped by model |
| `GET /api/cost-over-time?from=&to=` | Daily cost series |
| `GET /api/tokens-over-time?from=&to=` | Daily token series |
| `GET /api/sessions?from=&to=` | Session list |

## Tech Stack

- **Go** — pure Go, no CGO required
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — pure Go SQLite driver
- **ECharts** — charting library
- **`go:embed`** — single binary deployment

## Roadmap

- [ ] More agent sources (Cursor, Copilot, OpenCode, etc.)
- [ ] OTLP HTTP receiver for real-time telemetry
- [ ] OS service management (systemd / launchd / Windows Service)
- [ ] Export to CSV/JSON
- [ ] Alerting (cost thresholds)
- [ ] Multi-user support

## License

[Apache 2.0](LICENSE)
