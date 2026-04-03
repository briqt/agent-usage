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
