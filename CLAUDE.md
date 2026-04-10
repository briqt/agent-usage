# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

## Architecture

Single-binary Go application that collects AI coding agent token usage from local JSONL session files, stores it in SQLite, and serves a web dashboard.

**Data flow:** Collectors scan session dirs → parse JSONL → write to SQLite (with dedup) → pricing synced from litellm → costs calculated → served via REST API + embedded web UI.

### Key packages

- `internal/collector` — Source-specific JSONL parsers. Each collector implements `Scan()` which walks session directories and calls `processFile()` for incremental parsing. File offsets tracked in `file_state` table to avoid re-reading. `claude.go`/`claude_process.go` is the reference implementation for adding new sources. **Token semantics:** `input_tokens` must always be the total input count (including `cache_read_input_tokens` and `cache_creation_input_tokens`). If a source reports non-cached input separately, the collector must add cache tokens back before storing.
- `internal/storage` — SQLite layer. `sqlite.go` has schema + versioned migrations (tracked via `meta` table with `migration_{id}` keys, each runs once), `queries.go` handles writes, `api.go` handles reads, `costs.go` does cost recalculation. All DB access serialized through a mutex (`DB.mu`).
- `internal/pricing` — Fetches model prices from litellm's GitHub JSON. Cost formula: `(input - cache_read - cache_creation) × input_price + cache_creation × cache_creation_price + cache_read × cache_read_price + output × output_price`.
- `internal/server` — HTTP server with REST API endpoints (`/api/stats`, `/api/cost-by-model`, etc.) and `go:embed` static files (HTML + ECharts dashboard). `/api/stats` returns aggregate metrics including `cache_hit_rate` (ratio of cache read tokens to total input tokens). All endpoints accept `from`, `to`, `source` (optional: `claude`/`codex`/`openclaw`), and time-series endpoints accept `granularity`. Invalid dates or reversed ranges return `400` with a JSON error message.
- `internal/config` — YAML config loader. Search order: `--config` flag → `/etc/agent-usage/config.yaml` → `./config.yaml`. Supports `~` expansion in paths.

### Deduplication

Usage records are deduped via a unique index on `(session_id, model, timestamp, input_tokens, output_tokens)`. Incremental file scanning uses stored offsets in `file_state` to resume from where it left off.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `refactor:`, etc.) — GoReleaser generates changelog from these.
- Releases built with GoReleaser; version/commit/date injected via ldflags.
- Web UI is embedded via `go:embed` in `internal/server/static/` — changes to frontend files require rebuilding the binary.
