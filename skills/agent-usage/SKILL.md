---
name: agent-usage
description: "Query AI coding agent usage, costs, and token consumption. Supports Claude Code, Codex CLI, OpenClaw, OpenCode, kiro, Pi, and Hermes Agent. Ask about spending, token usage, model costs, session history, API call counts. Actions: check usage, show cost, compare models, list sessions, analyze spending, token breakdown. Time ranges: today, this week, this month, this year, last N days, custom dates."
---

# agent-usage — AI Coding Agent Usage Query

Query your AI coding agent usage data directly in conversation. Supports Claude Code, Codex CLI, OpenClaw, OpenCode, kiro, Pi, and Hermes Agent.

## When to Use

Activate when the user asks about:
- Cost / spending / billing / how much did I spend
- Token usage / consumption / input / output tokens
- Model comparison / which model costs most
- Session history / recent sessions / session details
- API call counts
- Usage trends over time
- Any question involving "usage", "cost", "tokens", "spend", "sessions" related to AI coding tools

## How It Works

This skill has two backends. Always detect which one to use first.

### Step 1: Detect Backend

Run the detection script to check if the agent-usage server is running:

```bash
bash SKILL_DIR/scripts/detect.sh
```

- Output `API` → use **API mode** (Step 2a)
- Output `LOCAL` → use **Local mode** (Step 2b)

Where `SKILL_DIR` is the directory containing this SKILL.md file.

### Step 2a: API Mode (preferred)

Use `query-api.sh` to call the agent-usage REST API. This is faster and exposes the unified cost, token quality, and deterministic server-side billing.

```bash
bash SKILL_DIR/scripts/query-api.sh <command> [options]
```

Commands:
| Command | Description | Key Options |
|---------|-------------|-------------|
| `stats` | Unified total cost, token totals/quality, sessions, prompts, calls | `--from`, `--to`, `--source`, `--model` |
| `cost-by-model` | Cost breakdown per model | `--from`, `--to`, `--source` |
| `cost-over-time` | Cost trend over time | `--from`, `--to`, `--granularity`, `--source`, `--model` |
| `tokens-over-time` | Token usage trend | `--from`, `--to`, `--granularity`, `--source`, `--model` |
| `sessions` | List all sessions with cost/tokens | `--from`, `--to`, `--source`, `--model` |
| `session-detail` | Per-model breakdown for one session | `--session-id` |

Options:
- `--from YYYY-MM-DD` — Start date (default: today)
- `--to YYYY-MM-DD` — End date (default: today)
- `--source claude|codex|openclaw|opencode|kiro|pi|hermes` — Filter by source (default: all)
- `--model MODEL_NAME` — Filter by model name, e.g. `claude-sonnet-4.6` (default: all)
- `--granularity 1m|30m|1h|6h|12h|1d|1w|1M` — Time bucket (default: 1d)
- `--session-id ID` — Session ID for detail query

### Step 2b: Local Mode (fallback)

Use `usage.py` to parse local sources directly. No server is needed. It uses a source-returned cost when available, then falls back to Token pricing.

```bash
python3 SKILL_DIR/scripts/usage.py <command> [options]
```

Commands:
| Command | Description |
|---------|-------------|
| `stats` | Summary totals |
| `cost-by-model` | Cost per model |
| `sessions` | Session list |
| `top-models` | Top N models by cost |

Same `--from`, `--to`, `--source` options as API mode. Additional: `-n N` for top-models count.

### Step 3: Interpret and Respond

After getting JSON output from either backend:

1. Parse the JSON response
2. Present `total_cost` as the single cost: source-returned cost first, Token-priced fallback second
3. Mention estimated-token or unpriced records when either count is non-zero
4. Format USD as `$X.XX` and tokens as `X.XK` or `X.XM`
5. Answer the user's specific question; use tables only for multi-row data and add a brief insight when useful

### Time Range Mapping

Map natural language to date parameters:
| User says | --from | --to |
|-----------|--------|------|
| today | today's date | today's date |
| yesterday | yesterday | yesterday |
| this week | Monday of this week | today |
| this month | 1st of this month | today |
| this year | Jan 1 of this year | today |
| last 7 days | 7 days ago | today |
| last 30 days | 30 days ago | today |
| last N days | N days ago | today |

Calculate actual YYYY-MM-DD dates before passing to scripts.

### Source Mapping

| User says | --source value |
|-----------|---------------|
| claude / claude code | claude |
| codex / openai codex | codex |
| openclaw | openclaw |
| opencode | opencode |
| kiro | kiro |
| pi | pi |
| hermes / hermes agent | hermes |
| all / everything / total | (omit --source) |

## Examples

User: "How much did I spend this month?"
→ `bash scripts/query-api.sh stats --from 2026-04-01 --to 2026-04-07`

User: "Which model costs the most?"
→ `bash scripts/query-api.sh cost-by-model --from 2026-01-01 --to 2026-04-07`

User: "Show me today's Claude Code sessions"
→ `bash scripts/query-api.sh sessions --from 2026-04-07 --to 2026-04-07 --source claude`

User: "Token usage trend this week by hour"
→ `bash scripts/query-api.sh tokens-over-time --from 2026-04-01 --to 2026-04-07 --granularity 1h`

## Notes

- Token-price fallback uses a small official override table first and LiteLLM for broad coverage. Unmatched models without a source-returned cost remain unpriced; they are not fuzzy-matched.
- Local mode has fewer data sources and billing fields. Prefer the server for migrations, token-quality flags, and complete pricing provenance.
- Claude rows are deduplicated using session + request ID + message ID when those identifiers are available.
- Kiro token counts are estimates. Do not describe them as exact.
- For complete billing semantics, deploy the agent-usage server: https://github.com/briqt/agent-usage
- Local mode scans `~/.claude/projects`, `~/.codex/sessions`, `~/.openclaw/agents`, `~/.local/share/opencode/opencode.db`, `~/.pi/agent/sessions` by default
- Hermes Agent data is read from SQLite databases at `~/.hermes/state.db` and `~/.hermes/profiles/*/state.db`. Supports multiple profiles automatically.

## Docker Deployment Warning

When using Docker, only mount volume paths for agents you have actually installed. Docker will auto-create missing host directories as root, causing:
1. Agent detection tools (e.g. `npx skills add`, Kiro CLI) falsely detect the agent as installed
2. The root-owned directory prevents the real agent from writing session data when later installed

Only uncomment volume mounts in `docker-compose.yml` for agents whose data directories already exist on your host.
