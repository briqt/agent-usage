# Contributing

感谢你对 agent-usage 的关注！欢迎提交 Issue 和 Pull Request。

Thanks for your interest in agent-usage! Issues and Pull Requests are welcome.

## Development Setup

```bash
# Clone
git clone https://github.com/briqt/agent-usage.git
cd agent-usage

# Requirements
# Go 1.25+

# Build
go build -o agent-usage .

# Run
cp config.yaml config.local.yaml  # edit as needed
./agent-usage
```

## Project Structure

```
├── main.go                  # Entry point
├── internal/
│   ├── config/              # YAML config loader
│   ├── collector/           # Data source parsers (Claude Code, Codex)
│   ├── pricing/             # litellm price sync + cost calculation
│   ├── storage/             # SQLite schema, read/write, cost backfill
│   └── server/              # HTTP server, REST API, embedded web UI
└── .github/workflows/       # CI/CD
```

## Adding a New Data Source

1. Create `internal/collector/<source>.go`
2. Implement a scanner that:
   - Walks the session directory for JSONL files
   - Parses entries and extracts per-API-call token usage
   - Calls `storage.DB` methods to write records
3. Register the collector in `main.go`
4. Add config fields in `internal/config/config.go`

See `internal/collector/claude.go` as a reference.

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Purpose | Example |
|--------|---------|---------|
| `feat:` | New feature | `feat: add Cursor session parser` |
| `fix:` | Bug fix | `fix: handle empty JSONL files` |
| `perf:` | Performance | `perf: batch SQLite inserts` |
| `refactor:` | Code restructure | `refactor: extract common JSONL reader` |
| `docs:` | Documentation | `docs: add API examples` |
| `ci:` | CI/CD changes | `ci: add lint workflow` |
| `chore:` | Maintenance | `chore: update dependencies` |
| `test:` | Tests | `test: add collector unit tests` |

GoReleaser uses these prefixes to generate changelogs.

## Release Process

Releases are manual. Two options:

```bash
# Option 1: Push a tag
git tag v0.1.0
git push origin v0.1.0

# Option 2: GitHub Actions UI
# Go to Actions → Release → Run workflow → Enter version
```

This triggers GoReleaser to cross-compile binaries for 6 platforms and publish a GitHub Release.

## Versioning

We follow [Semantic Versioning](https://semver.org/):

- `MAJOR` — breaking changes (config format, API response schema, DB migration)
- `MINOR` — new features (new data source, new dashboard panel, new API endpoint)
- `PATCH` — bug fixes, performance improvements, dependency updates

## Code Style

- `go fmt` and `go vet` must pass
- `go test ./...` must pass
- Keep dependencies minimal — prefer stdlib where reasonable
- Pure Go only (no CGO) to ensure easy cross-compilation
- Embed static assets via `go:embed`

## Pull Request Guidelines

1. One PR per feature/fix
2. Include a clear description of what and why
3. Update README if adding user-facing features
4. Test against real session data if modifying collectors

## Reporting Issues

Please include:
- OS and architecture
- Go version
- Steps to reproduce
- Relevant log output
