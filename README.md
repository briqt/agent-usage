# agent-usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-blue?logo=docker)](https://ghcr.io/briqt/agent-usage)

Lightweight, cross-platform AI coding agent usage & cost tracker.  
Single binary + SQLite — zero infrastructure required.

**[中文文档](README_CN.md)**

## Why

AI coding tools (Claude Code, Codex, etc.) generate usage data across scattered local files and telemetry streams. Keeping track of costs and token usage across tools is tedious without a unified view.

**agent-usage** gives you that view with a single binary and one SQLite file.

## Features

- 📁 **Local file parsing** — reads Claude Code, Codex CLI, and OpenClaw session files directly
- 💰 **Automatic cost calculation** — fetches model pricing from [litellm](https://github.com/BerriAI/litellm), supports backfill when prices update
- 🗄️ **SQLite storage** — single file, zero ops, data is correctable
- 📊 **Web dashboard** — dark-themed UI with ECharts: cost breakdown, token trends, session list
- 🔄 **Incremental scanning** — watches for new sessions, deduplicates automatically
- 📦 **Single binary** — `go:embed` packs the web UI into the executable
- 🖥️ **Cross-platform** — Linux, macOS, Windows

## Quick Start (Docker)

```bash
# One command to start
mkdir -p ./data && docker compose up -d

# Open dashboard
open http://localhost:9800
```

The default `docker-compose.yml` mounts `~/.claude/projects`, `~/.codex/sessions`, and `~/.openclaw/agents` read-only. Data persists in `./data/`.

The container uses `config.docker.yaml` by default (binds to `0.0.0.0`, stores data in `/data/`). To override, mount your own config:

```yaml
# In docker-compose.yml, uncomment:
volumes:
  - ./config.yaml:/etc/agent-usage/config.yaml:ro
```

See [Docker Details](#docker-details) for UID/GID permissions and local builds.

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
  openclaw:
    enabled: true
    paths:
      - "~/.openclaw/agents"
    scan_interval: 60s

storage:
  path: "./agent-usage.db"

pricing:
  sync_interval: 1h  # fetched from GitHub; set HTTPS_PROXY env var if this fails
```

Config search order: `--config` flag > `/etc/agent-usage/config.yaml` > `./config.yaml`.

## Build from Source

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

## Supported Data Sources

| Source | Session Location | Format |
|--------|-----------------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |
| [OpenClaw](https://github.com/openclaw/openclaw) | `~/.openclaw/agents/<agentId>/sessions/<sessionId>.jsonl` | JSONL |

### Adding New Sources

Each source needs a collector that:
1. Scans session directories for JSONL files
2. Parses entries and extracts token usage per API call
3. Writes records to SQLite via the storage layer

See `internal/collector/claude.go` as a reference implementation.

## Dashboard

The web dashboard provides:

- **Sticky top bar** — time presets, granularity, source filter (Claude/Codex/OpenClaw), auto-refresh
- **Summary cards** — total tokens, cost, sessions, prompts, API calls
- **Token usage** — stacked bar chart (input/output/cache read/cache write)
- **Cost trend** — stacked bar chart by model with consistent color mapping
- **Cost by model** — doughnut chart with percentage labels
- **Session list** — sortable, filterable table with expandable per-model detail
- **Dark/Light theme** — system-aware with manual toggle
- **i18n** — English and Chinese

## Architecture

```
agent-usage
├── main.go                     # Entry point, orchestrates components
├── config.yaml                 # Configuration
├── internal/
│   ├── config/                 # YAML config loader
│   ├── collector/
│   │   ├── collector.go        # Collector interface
│   │   ├── claude.go           # Claude Code session scanner
│   │   ├── claude_process.go   # Claude Code JSONL parser
│   │   ├── codex.go            # Codex CLI JSONL parser
│   │   ├── openclaw.go         # OpenClaw session scanner
│   │   └── openclaw_process.go # OpenClaw JSONL parser
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

All endpoints accept `from` and `to` (YYYY-MM-DD) query parameters. Optional: `source` (`claude`, `codex`, `openclaw`) to filter by agent, `granularity` (`1m`, `30m`, `1h`, `6h`, `12h`, `1d`, `1w`, `1M`) for time-series endpoints.

| Endpoint | Description |
|----------|-------------|
| `GET /api/stats` | Summary: total cost, tokens, sessions, prompts, API calls |
| `GET /api/cost-by-model` | Cost grouped by model |
| `GET /api/cost-over-time` | Cost time series (supports `granularity`) |
| `GET /api/tokens-over-time` | Token usage time series (supports `granularity`) |
| `GET /api/sessions` | Session list with cost/token totals |
| `GET /api/session-detail?session_id=ID` | Per-model breakdown for a session |

Invalid date formats or reversed date ranges return a `400` JSON error with a descriptive message.

## Tech Stack

- **Go** — pure Go, no CGO required
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — pure Go SQLite driver
- **ECharts** — charting library
- **`go:embed`** — single binary deployment

## Docker Details

Pre-built multi-arch images (amd64 + arm64) are published to `ghcr.io/briqt/agent-usage`.

The default `docker-compose.yml` runs as UID 1000. If your host user has a different UID, edit the `user:` field:

```bash
# Check your UID/GID
id -u  # e.g. 1000
id -g  # e.g. 1000

# Edit docker-compose.yml: user: "YOUR_UID:YOUR_GID"
```

This is required because `~/.claude/projects` is mode 700 — only the owning UID can read it.

### Building locally

```bash
docker build -t agent-usage:local .

# For China mainland, use GOPROXY:
docker build --build-arg GOPROXY=https://goproxy.cn,direct -t agent-usage:local .
```

## AI Agent Skill

Query your usage data directly in any AI coding agent conversation — no browser needed.

```bash
npx skills add briqt/agent-usage
```

Once installed, ask your agent things like "how much did I spend this month?" or "which model costs the most?". The skill auto-detects whether the agent-usage server is running (API mode) or falls back to parsing local JSONL files directly (local mode).

See [`skills/agent-usage/SKILL.md`](skills/agent-usage/SKILL.md) for details.

## Roadmap

- [ ] More agent sources (Cursor, Copilot, OpenCode, etc.)
- [ ] OTLP HTTP receiver for real-time telemetry
- [ ] OS service management (systemd / launchd / Windows Service)
- [ ] Export to CSV/JSON
- [ ] Alerting (cost thresholds)
- [ ] Multi-user support

## License

[Apache 2.0](LICENSE)
