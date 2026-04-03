# devobs - AI Coding Tool Usage Observer

Lightweight, cross-platform Go app replacing a 7-container Grafana LGTM stack.

## What It Does
- Parses local session files from Claude Code and Codex CLI
- Stores data in SQLite
- Serves a web dashboard with cost/token/usage analytics
- Single binary, cross-platform (Win/Linux/Mac)

## Data Sources

### Claude Code Sessions
Path: `~/.claude/projects/<project>/<session-id>.jsonl` (JSONL, one JSON per line)

Key types:
- `type=assistant`: `message.model`, `message.usage` with `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. `costUSD` is always null.
- `type=user`: User prompts (count these)
- `type=system`: Has `subtype`, `sessionId`, `version`, `cwd`, `gitBranch`

Dedup note: Claude streams multiple assistant entries per response. The "final" one has `cache_creation_input_tokens` field in usage. Skip entries where usage only has `input_tokens` and `output_tokens` (no cache fields = streaming chunk).

### Codex Sessions
Path: `~/.codex/sessions/<year>/<month>/<day>/rollout-<ts>-<id>.jsonl`

Key types:
- `type=session_meta`: `payload.id`, `payload.cwd`, `payload.cli_version`, `payload.model_provider`
- `type=event_msg, payload.type=token_count`: `payload.info.last_token_usage` has per-turn `input_tokens`, `cached_input_tokens`, `output_tokens`, `reasoning_output_tokens`
- `type=turn_context`: `payload.model` (e.g. "gpt-5.4")
- `type=event_msg, payload.type=task_complete`: Turn end
- `type=response_item, payload.role=user`: User prompts

### Pricing
URL: https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json
Fields: `input_cost_per_token`, `output_cost_per_token`, `cache_read_input_token_cost`, `cache_creation_input_token_cost`

Cost = (input - cache_read - cache_creation) * input_price + cache_creation * cache_creation_price + cache_read * cache_read_price + output * output_price

## Tech Stack
- `modernc.org/sqlite` (pure Go, no CGO)
- `fsnotify/fsnotify` for file watching
- `kardianos/service` for OS service management
- `gopkg.in/yaml.v3` for config
- `go:embed` for static web files
- ECharts for charts

## Config (config.yaml)
```yaml
server:
  port: 9800
collectors:
  claude:
    enabled: true
    paths: ["~/.claude/projects"]
    scan_interval: 60s
  codex:
    enabled: true
    paths: ["~/.codex/sessions"]
    scan_interval: 60s
storage:
  path: "./devobs.db"
pricing:
  sync_interval: 1h
```

## Web UI
- Dashboard: total cost, tokens, sessions, prompts
- Cost by model (pie chart)
- Cost over time (line chart)
- Token usage over time
- Session list with details
- Date range filter
- Dark mode, clean design

## Test Data Available
- 39 Codex sessions in ~/.codex/sessions/
- 251 Claude Code sessions in ~/.claude/projects/

## Instructions
Build it, test against real data, verify numbers make sense, commit.
