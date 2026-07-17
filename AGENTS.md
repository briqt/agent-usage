# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Build & Run

```bash
go build -o agent-usage .                # build binary
./agent-usage                             # run (reads config.yaml by default)
./agent-usage --config path/to/config.yaml
./agent-usage version                     # print version info
```

## Testing

```bash
go test ./...                             # all tests
go test ./internal/collector/...          # single package
go test ./internal/storage/... -run TestDedup  # single test
```

No CGO required — the SQLite driver (`modernc.org/sqlite`) is pure Go.

## Docker

```bash
docker compose up -d                      # start with default compose
docker build -t agent-usage:local .       # local image build
docker build --build-arg GOPROXY=https://goproxy.cn,direct -t agent-usage:local .  # China proxy
```

Container runs as UID 1000 by default; adjust `user:` in docker-compose.yml if your host UID differs (needed because `~/.claude/projects` is mode 700).

**Important:** Only mount directories for agents you have actually installed. Docker automatically creates missing host directories as root, which causes two problems: (1) agent tools that detect installation by checking directory existence (e.g. Kiro CLI, `npx skills add`) will think the agent is installed when it isn't; (2) the root-owned directory prevents the real agent from writing to it later, breaking session recording.

## Architecture

Single-binary Go application that collects AI coding agent token usage from local JSONL session files, stores it in SQLite, and serves a web dashboard.

**Data flow:** Collectors scan session dirs → normalize/deduplicate usage → write to SQLite → sync LiteLLM plus official overrides → calculate one effective cost per record → serve REST API + embedded web UI.

### Key packages

- `internal/collector` — Source-specific parsers. Each collector implements `Scan()` which walks session directories and calls `processFile()` for incremental parsing. File offsets tracked in `file_state` table to avoid re-reading. `claude.go`/`claude_process.go` is the reference implementation for adding new sources. Collectors must normalize token fields to match the non-overlapping semantics defined below. Collectors also extract individual user prompt events (with timestamps) into the `prompt_events` table for time-accurate prompt counting. The Kiro collector (`kiro.go`/`kiro_process.go`) supports **two data sources** simultaneously: (1) a SQLite database (`~/.local/share/kiro-cli/data.sqlite3`) containing `conversations_v2` with per-request metadata, and (2) JSON session files (`~/.kiro/sessions/cli/*.json` + companion `*.jsonl`) with aggregated turn metadata. `Scan()` auto-detects each path type and routes accordingly. Kiro does not expose actual token counts, so tokens are **estimated**: input from `context_usage_percentage × context_window_tokens`, output from `response_size / 4` (SQLite) or CJK-aware character heuristics on JSONL content (JSON source). Known limitations: (1) subagent sessions (null `session_state` in JSON) cannot be tracked; (2) token counts are estimates, not exact values; (3) the two sources have disjoint session IDs — no overlap or dedup concern. The Pi collector (`pi.go`/`pi_process.go`) shares the same JSONL format as OpenClaw (same underlying framework). It tracks `model_change` entries for mid-session model switching and derives project names from the session CWD (with workspace slug as fallback). Directory structure: `~/.pi/agent/sessions/<workspace-slug>/<file>.jsonl`. The Hermes collector (`hermes.go`/`hermes_process.go`) reads Hermes Agent's SQLite `state.db` directly. Unlike file-based collectors, it does a full DELETE+re-INSERT on each scan because Hermes accumulates tokens at the session level (no per-API-call rows). Auto-discovers `state.db` at the base path and `profiles/*/state.db` for multi-profile setups. Token semantics match directly (input_tokens is already non-cached). Prompt events are extracted from the `messages` table (`role='user'`).
- `internal/storage` — SQLite layer. `sqlite.go` has schema + versioned migrations (tracked via `meta` table with `migration_{id}` keys, each runs once), `queries.go` handles writes, `api.go` handles reads, and `billing.go` performs deterministic recalculation. Key tables: `usage_records` (per-API-call token/cost/provenance data), `sessions`, `prompt_events`, `pricing`, and `file_state`.
- `internal/pricing` — Syncs LiteLLM for broad coverage, then reapplies the small `officialOverrides` table for product-specific fields. Official overrides are seeded even when the LiteLLM fetch fails.
- `internal/server` — HTTP server with REST API endpoints (`/api/stats`, `/api/cost-by-model`, etc.) and `go:embed` static files. `/api/stats` exposes one `total_cost` plus output tokens, cache hit rate, estimated-token records, and unpriced records. All endpoints accept `from`, `to`, `source`, and `model`; time-series endpoints accept `granularity`.
- `internal/config` — YAML config loader. Search order: `--config` flag → `/etc/agent-usage/config.yaml` → `./config.yaml`. Supports `~` expansion in paths.

### Token semantics

All token fields are **non-overlapping components** that sum to the total:

```
input_tokens              — non-cached input (mutually exclusive with cache fields)
cache_read_input_tokens   — input tokens served from cache
cache_creation_input_tokens — input tokens written to cache
output_tokens             — total output tokens
reasoning_output_tokens   — reasoning tokens (subset of output, informational only)

total_input  = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
total_output = output_tokens
total_tokens = total_input + total_output
```

If a data source reports `input_tokens` as the total (including cache), the collector must subtract cache tokens before storing. If a source reports non-cached input natively, store as-is.

### Billing semantics

- `cost_usd` / `total_cost` is the only exposed cost.
- A positive source-returned `native_cost_usd` takes precedence; otherwise calculate from Token counts and model unit prices. `native_cost_kind` remains internal provenance only.
- `codex_credits` is a deprecated compatibility column and must not be calculated or exposed.
- `token_quality` is `exact` or `estimated`; Kiro records must be `estimated`.
- `price_source` records `anthropic_official`, `litellm`, another explicit source, or `unknown`.
- Model matching must be deterministic: exact model and provider-qualified aliases only. Never add substring/fuzzy price matching.
- Cache writes use the 5-minute or 1-hour rate when the source exposes TTL. Legacy unsplit cache writes use the base cache-write rate.
- Recalculation refreshes every effective cost, always preserving source-returned cost precedence.

### Deduplication

The legacy unique index on `(session_id, model, timestamp, input_tokens, output_tokens)` remains for compatibility. Sources with stable request identity must also populate `dedup_key`; the partial unique index on `(source, dedup_key)` deduplicates across differing event timestamps. Claude uses session ID + request ID + message ID, matching ccusage behavior. Incremental scanning uses `file_state`; Codex and OpenClaw persist parser state in `file_state.scan_context`.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `refactor:`, etc.) — GoReleaser generates changelog from these.
- Releases built with GoReleaser; version/commit/date injected via ldflags.
- Web UI is embedded via `go:embed` in `internal/server/static/` — changes to frontend files require rebuilding the binary.
