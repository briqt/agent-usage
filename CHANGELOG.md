# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
