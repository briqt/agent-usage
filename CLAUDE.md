# CLAUDE.md - Project Instructions for Claude Code

## Project
agent-usage: lightweight AI coding agent usage & cost tracker (Go + SQLite + embedded web UI)

## Git
- Default branch: `main` (NOT master)
- Always push to main: `git push origin main`
- NEVER push to master or create a master branch
- Commit messages: conventional commits (feat/fix/docs/ci/chore/test/perf/refactor)
- Tag format: `v0.x.y` with `git tag -a v0.x.y -m "Release v0.x.y: description"`

## Build
- Go 1.25+, pure Go (CGO_ENABLED=0), no C dependencies
- Build: `go build -o agent-usage .`
- Test: `go test ./...`
- Go SDK location: `~/go-sdk/go/bin/go` (add to PATH if needed)
- GOPROXY: use `https://goproxy.cn,direct` on this machine (China)

## Docker
- Build: `docker build --build-arg GOPROXY=https://goproxy.cn,direct -t agent-usage:test .`
- Run: `docker compose up -d`
- Container runs as host user (user: "1000:1000" in compose), NOT as nonroot/root
- Session dirs (~/.claude/projects, ~/.codex/sessions) are mounted read-only
- SQLite db in ./data/ bind mount
- Network: use `network_mode: host` or proxy env vars for China network access
- After testing with local image, revert docker-compose.yml image back to `ghcr.io/briqt/agent-usage:latest`

## Architecture
- `main.go` - entry point with --config flag
- `internal/collector/` - JSONL parsers (claude.go, codex.go)
- `internal/storage/` - SQLite schema, queries, cost recalc
- `internal/pricing/` - litellm price sync (30s HTTP timeout)
- `internal/server/` - HTTP server + embedded static files
- `internal/config/` - YAML config loader

## Testing
- Real session data available at ~/.claude/projects/ and ~/.codex/sessions/
- After changes, verify: `curl -s http://localhost:9800/api/stats` returns non-zero data
- Run `go vet ./...` before committing

## Release
- Update CHANGELOG.md: move [Unreleased] to [x.y.z] - YYYY-MM-DD
- Commit: `chore: prepare release vx.y.z`
- Tag and push: `git tag -a vx.y.z -m "..." && git push origin main && git push origin vx.y.z`
- GitHub Actions auto-builds binaries (GoReleaser) and Docker image (ghcr.io)
