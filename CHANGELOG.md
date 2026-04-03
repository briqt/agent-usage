# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
