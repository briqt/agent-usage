# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.15.0] - 2026-07-29

### Performance

- **Hermes collector no longer re-reads its source databases on every scan.** Each `state.db` is gated on a change fingerprint (size + mtime of the database *and* its `-wal`, since a WAL write leaves the main file untouched), so an idle instance costs two `stat` calls per interval. Previously every 60s interval opened each database and ran an unbounded `SELECT ... FROM messages ORDER BY timestamp`, which planned as a full table scan plus an external sort — against a table carrying message content and reasoning traces (250MB in one real profile) — to retrieve a few hundred prompts.
- **Prompt events are now appended from a rowid watermark** (`messages.id > ?`) instead of being deleted and re-inserted wholesale. The query plan changes from `SCAN messages` + `USE TEMP B-TREE FOR ORDER BY` to `SEARCH messages USING INTEGER PRIMARY KEY`. The watermark comes from `MAX(id)`, so messages filtered out by `role`/`timestamp` are stepped over rather than re-examined on every scan forever.
- **Added the partial index `idx_usage_unpriced`** on `usage_records(id) WHERE priced_at IS NULL`. `RecalcPendingCosts` runs after every collector scan and previously full-scanned the whole table (116k rows in one real deployment) to find the unpriced tail.
- **Database opens with `synchronous=NORMAL`**, removing an fsync per commit. This is safe under WAL: a crash can lose the tail of the last transaction but cannot corrupt the database.
- **Added a periodic `wal_checkpoint(TRUNCATE)`.** Under the collectors' continuous write load SQLite's automatic checkpointing can be starved indefinitely; a 50MB WAL against an 82MB database was observed, and an oversized WAL makes every read walk it before reaching the main file.

### Fixed

- Hermes usage records and sessions are now replaced within a single database's project scope (`DeleteBySourceProject`) rather than source-wide. A source-wide delete combined with skipping unchanged databases would have discarded the records of every database that got skipped.
- `idx_usage_unpriced` is created after the billing `ALTER TABLE` statements. On a database predating the billing columns, `CREATE TABLE IF NOT EXISTS` is a no-op and `priced_at` only exists once the `ALTER` has run, so creating the index alongside the main schema failed with "no such column" and took the whole migration — and startup — with it.

### Changed

- Billing semantics and provenance corrected, and cost calculation unified through `internal/storage/billing.go`: costs reported by a source take precedence over token-priced estimates, provenance is recorded per record (`price_source`, `native_cost_kind`, `token_quality`), and `priced_at` marks rows as priced so routine passes only visit new ones. Adds columns to `usage_records` and `pricing` (applied automatically) and provenance fields to API responses.

## [1.13.0] - 2026-07-14

### Added
- Dashboard now shows an **Output Tokens** summary card (between Total Tokens and Total Cost); `/api/stats` returns a new `total_output_tokens` field.

## [1.12.0] - 2026-06-07

### Added
- Kiro collector now supports dual data sources: SQLite database (`~/.local/share/kiro-cli/data.sqlite3`) and JSON/JSONL session files (`~/.kiro/sessions/cli/`). Both are scanned simultaneously with auto-detection based on path type.

### Changed
- Default Kiro config paths now include both data sources.
- Docker Compose examples include volume mount for `~/.kiro/sessions/cli`.

## [1.10.1] - 2026-06-05

### Changed
- kiro collection now uses `~/.local/share/kiro-cli/data.sqlite3` / `/sessions/kiro-cli/data.sqlite3` as the default and documented data source.
- Docker Compose examples now only mount existing agent data directories explicitly, with kiro using the SQLite data directory.

### Fixed
- Count kiro API calls from `conversations_v2.history[].request_metadata` so non-interactive kiro usage is included.
- Preserve concurrent same-millisecond kiro requests by deriving record timestamps from `request_id`.
- Reset old kiro scan state and usage records once so legacy JSON/JSONL counts do not mix with the new SQLite-only counts.
- Standardize user-facing source labels to `kiro`.

## [1.0.1] - 2026-04-07

### Changed
- Color palette: Financial Dashboard scheme (deep navy dark theme, cool white light theme)
- Chart colors: ECharts default palette with consistent model-to-color mapping
- Stat card font: Fira Code monospace for data terminal feel
- All i18n labels refined for clarity (zh: 总耗费→总费用, 提示数→Prompt数, etc.)
- Session Log title separated from Sessions stat card i18n key

### Fixed
- Filter input and select elements now properly styled in dark mode
- Project filter placeholder now follows i18n language setting

## [1.0.0] - 2026-04-07

### Added
- Global source filter (Claude/Codex/OpenClaw) applied to all API endpoints and charts
- API Calls stat card with backend COUNT(*) query
- Sticky top bar merging header and controls into one component
- Empty state graphics for charts when no data
- IBM Plex Mono / Fira Code for stat card numbers
- Project column text truncation with ellipsis
- Responsive breakpoints: 4-col → 2-col → 1-col stats grid
- Inter font loaded from Google Fonts
- Stat card hover lift animation
- Refresh button continuous spin animation
- OpenClaw badge styling

### Changed
- Panel order: Tokens → Cost → Sessions → Prompts (stat cards), Token Usage → Cost Trend → Cost by Model (charts)
- Charts layout: Token Usage full-width, Cost Trend 3/5, Cost by Model 2/5
- Cost trend chart: stacked bar by model (was line chart)
- Pie chart legend: top horizontal with scroll (was right vertical)
- Model color consistency: same model gets same color across pie and bar charts
- Header backdrop-filter fixed with proper RGB CSS variables

### Fixed
- Filter `<synthetic>` model records from Claude Code collector
- Filter `delivery-mirror` internal records from OpenClaw collector
- Clean up synthetic/delivery-mirror records from database on startup
- GetSessions double source filter bug (source param appended twice)
- API date validation: returns 400 JSON error for invalid dates or reversed ranges

## [0.1.0] - 2026-04-03

### Added
- Claude Code session JSONL parser
- Codex CLI session JSONL parser
- SQLite storage with automatic schema migration
- litellm pricing sync with cost backfill
- Web dashboard with ECharts (dark theme)
  - Summary cards: total cost, tokens, sessions, prompts
  - Cost by model (pie chart)
  - Cost over time (line chart)
  - Token usage over time (line chart)
  - Daily sessions (bar chart)
  - Session list table
  - Date range filter
- REST API endpoints for all dashboard data
- Incremental file scanning with deduplication
- GoReleaser CI/CD for cross-platform releases
- Bilingual documentation (English + Chinese)
- Unit tests for collectors, pricing calculation, and storage layer
- Godoc comments on all exported types and functions
- GitHub issue templates (bug report, feature request) and PR template
- Unique index on usage_records for crash-recovery deduplication
- Docker support: multi-stage Dockerfile with distroless runtime
- Docker Compose for one-command deployment
- Docker CI/CD workflow for multi-arch images (amd64 + arm64) on ghcr.io
- `--config` CLI flag with search order: flag > `/etc/agent-usage/config.yaml` > `./config.yaml`
- Docker-specific config (`config.docker.yaml`) with 0.0.0.0 bind and container paths

### Changed
- Server binds to `127.0.0.1` by default instead of `0.0.0.0`
- Added `bind_address` config option for server
- Default database filename changed from `devobs.db` to `agent-usage.db`
- INSERT statements use `INSERT OR IGNORE` for idempotent crash recovery
