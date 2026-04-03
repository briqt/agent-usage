# agent-usage

Lightweight, cross-platform tool for tracking AI coding agent usage, costs, and performance.

Replaces a 7-container Grafana LGTM observability stack with a single binary + SQLite.

## Features

- Parses local session files from **Claude Code** and **Codex CLI**
- Calculates costs using [litellm](https://github.com/BerriAI/litellm) pricing data
- Stores everything in **SQLite** (single file, zero ops)
- Serves a **web dashboard** with ECharts visualizations
- Single binary, cross-platform (Linux / macOS / Windows)

## Quick Start

```bash
# Build
go build -o agent-usage ./...

# Run (uses config.yaml in current directory)
./agent-usage

# Open dashboard
open http://localhost:9800
```

## Configuration

```yaml
server:
  port: 9800

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

## Data Sources

| Source | Session Path | Format |
|--------|-------------|--------|
| Claude Code | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| Codex CLI | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |

## Dashboard

- Total cost, tokens, sessions, prompts
- Cost breakdown by model
- Cost and token usage over time
- Session list with drill-down
- Date range filtering

## Tech Stack

- Go (pure, no CGO)
- SQLite via `modernc.org/sqlite`
- ECharts for visualization
- `go:embed` for single-binary deployment

## License

MIT
