---
name: agent-usage
description: "Query AI coding agent usage, costs, and token consumption. Supports Claude Code, Codex CLI, OpenClaw, and OpenCode. Ask about spending, token usage, model costs, session history, API call counts. Actions: check usage, show cost, compare models, list sessions, analyze spending, token breakdown. Time ranges: today, this week, this month, this year, last N days, custom dates."
---

# agent-usage — AI Coding Agent Usage Query

Query your AI coding agent usage data directly in conversation. Supports Claude Code, Codex CLI, OpenClaw, and OpenCode.

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

Use `query-api.sh` to call the agent-usage REST API. This is faster and has accurate pricing data.

```bash
bash SKILL_DIR/scripts/query-api.sh <command> [options]
```

Commands:
| Command | Description | Key Options |
|---------|-------------|-------------|
| `stats` | Summary: total cost, tokens, sessions, prompts, API calls | `--from`, `--to`, `--source` |
| `cost-by-model` | Cost breakdown per model | `--from`, `--to`, `--source` |
| `cost-over-time` | Cost trend over time | `--from`, `--to`, `--granularity`, `--source` |
| `tokens-over-time` | Token usage trend | `--from`, `--to`, `--granularity`, `--source` |
| `sessions` | List all sessions with cost/tokens | `--from`, `--to`, `--source` |
| `session-detail` | Per-model breakdown for one session | `--session-id` |

Options:
- `--from YYYY-MM-DD` — Start date (default: today)
- `--to YYYY-MM-DD` — End date (default: today)
- `--source claude|codex|openclaw|opencode` — Filter by source (default: all)
- `--granularity 1m|30m|1h|6h|12h|1d|1w|1M` — Time bucket (default: 1d)
- `--session-id ID` — Session ID for detail query

### Step 2b: Local Mode (fallback)

Use `usage.py` to parse JSONL session files directly. No server needed, but pricing is approximate (built-in price table for common models).

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
2. Format numbers: costs as `$X.XX`, tokens as `X.XK` or `X.XM`
3. Answer the user's specific question — don't dump raw data
4. Use markdown tables for multi-row data (sessions, model breakdown)
5. Add brief insights when relevant (e.g., "claude-opus-4-6 accounts for 85% of your spending")

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

- Local mode pricing is approximate — only common models have built-in prices
- For accurate pricing, deploy the agent-usage server: https://github.com/briqt/agent-usage
- Local mode scans `~/.claude/projects`, `~/.codex/sessions`, `~/.openclaw/agents`, `~/.local/share/opencode/opencode.db` by default
