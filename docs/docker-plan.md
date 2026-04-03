# Docker Support Plan for agent-usage

## Overview

This document outlines the plan for adding Docker support to **agent-usage** — a single-binary Go application that tracks AI coding agent usage via SQLite and serves a web dashboard on port 9800.

Key properties that simplify containerization:
- Pure Go binary (`CGO_ENABLED=0`), no C dependencies
- Embedded web UI via `go:embed` — fully self-contained
- SQLite with WAL mode (single file, no external DB server)
- Read-only access to host session directories (`~/.claude/projects`, `~/.codex/sessions`)
- Single HTTP port (9800)

---

## 1. Multi-Stage Dockerfile

Two stages: build (Go toolchain) → runtime (distroless/static or scratch).

```dockerfile
# ---- Build Stage ----
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /agent-usage .

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /agent-usage /agent-usage
COPY config.yaml /etc/agent-usage/config.yaml

EXPOSE 9800

VOLUME ["/data"]

USER nonroot:nonroot

ENTRYPOINT ["/agent-usage"]
```

### Design decisions

- **`golang:1.25-alpine`** for the build stage — small base, fast downloads.
- **`distroless/static`** for runtime — no shell, no package manager, minimal attack surface. The `nonroot` tag runs as UID 65534 by default.
- **`scratch`** is an alternative if even distroless is too large, but distroless provides CA certificates (needed for HTTPS pricing API fetch) and timezone data.
- **`CGO_ENABLED=0`** is already used by GoReleaser — `modernc.org/sqlite` is pure Go, so this works.
- Build args `VERSION`, `COMMIT`, `DATE` mirror the GoReleaser ldflags.

### Default config for Docker

The bundled `config.yaml` should be Docker-aware:

```yaml
server:
  port: 9800
  bind_address: "0.0.0.0"        # Must be 0.0.0.0 inside container

collectors:
  claude:
    enabled: true
    paths: ["/sessions/claude"]   # Mount point, not ~
    scan_interval: 60s
  codex:
    enabled: true
    paths: ["/sessions/codex"]    # Mount point, not ~
    scan_interval: 60s

storage:
  path: "/data/agent-usage.db"    # Persistent volume

pricing:
  sync_interval: 1h
```

This config would be bundled as `config.docker.yaml` in the repo and copied into the image. Users can override it via volume mount.

---

## 2. docker-compose.yml

Goal: dead simple — `docker compose up` and it works.

```yaml
services:
  agent-usage:
    image: ghcr.io/briqt/agent-usage:latest
    container_name: agent-usage
    restart: unless-stopped
    ports:
      - "9800:9800"
    volumes:
      # Persistent database
      - agent-usage-data:/data
      # Host session directories (read-only)
      - ~/.claude/projects:/sessions/claude:ro
      - ~/.codex/sessions:/sessions/codex:ro
      # Optional: custom config override
      # - ./config.yaml:/etc/agent-usage/config.yaml:ro

volumes:
  agent-usage-data:
```

### Usage

```bash
# Pull and run — that's it
docker compose up -d

# Dashboard available at http://localhost:9800
```

### Notes

- Named volume `agent-usage-data` persists the SQLite database across container restarts.
- Session directories are mounted read-only (`:ro`) — the app never writes to them.
- Config override is commented out by default; the bundled Docker config works out of the box.
- `restart: unless-stopped` keeps it running after host reboots.

---

## 3. GitHub Actions Workflow — Docker Build & Push

New workflow: `.github/workflows/docker.yml`, triggered alongside the existing GoReleaser release flow.

```yaml
name: Docker

on:
  push:
    tags:
      - "v*"
  workflow_dispatch:

permissions:
  contents: read
  packages: write

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Docker meta
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/briqt/agent-usage
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=raw,value=latest,enable={{is_default_branch}}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ github.ref_name }}
            COMMIT=${{ github.sha }}
            DATE=${{ github.event.head_commit.timestamp }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### Tag strategy

For a tag `v1.2.3`, the workflow produces these image tags:
- `ghcr.io/briqt/agent-usage:1.2.3`
- `ghcr.io/briqt/agent-usage:1.2`
- `ghcr.io/briqt/agent-usage:1`
- `ghcr.io/briqt/agent-usage:latest`

This matches the existing GoReleaser trigger (`v*` tags) so both binary releases and Docker images are published from the same event.

### Build cache

GitHub Actions cache (`type=gha`) is used for Docker layer caching. This dramatically speeds up rebuilds since Go module downloads and compilation are cached across runs.

---

## 4. Multi-Architecture Support

### Approach

Use Docker Buildx with QEMU emulation to build `linux/amd64` and `linux/arm64` in a single manifest.

This is handled in the workflow above via:
```yaml
platforms: linux/amd64,linux/arm64
```

### Why this works cleanly

- `CGO_ENABLED=0` means no cross-compilation toolchain needed — Go's built-in cross-compilation handles it.
- `modernc.org/sqlite` is pure Go — no `.so` or `.a` files to worry about per architecture.
- `distroless/static` provides multi-arch base images.

### Performance consideration

QEMU-emulated arm64 builds on amd64 runners are slow (~3-5x). If build times become a problem, options:
1. Use `arm64` GitHub-hosted runners (available on larger plans)
2. Split into parallel jobs per architecture and merge manifests with `docker buildx imagetools create`

For now, QEMU is the simplest approach and fine for a project of this size.

---

## 5. Config and Data Persistence

### Config (`config.yaml`)

| Strategy | How | When to use |
|----------|-----|-------------|
| Bundled default | Baked into image at `/etc/agent-usage/config.yaml` | Works out of the box for most users |
| Volume mount | `-v ./config.yaml:/etc/agent-usage/config.yaml:ro` | Custom configuration |
| Environment variables | Future enhancement — override config fields via env | Kubernetes / cloud deployments |

The application should look for config in this order:
1. Path specified by `--config` flag (if added)
2. `/etc/agent-usage/config.yaml` (Docker default)
3. `./config.yaml` (current behavior)

**Implementation note**: This requires a small code change to support the `/etc/agent-usage/` path. The simplest approach is to check multiple paths in order, or add a `--config` CLI flag.

### Data persistence (`agent-usage.db`)

The SQLite database must survive container restarts. Two options:

1. **Named volume** (recommended for simplicity):
   ```yaml
   volumes:
     - agent-usage-data:/data
   ```

2. **Bind mount** (recommended for backups/portability):
   ```yaml
   volumes:
     - ./data:/data
   ```

The Docker config sets `storage.path: "/data/agent-usage.db"` so the database always lives in the `/data` volume.

### Backup

Since it's a single SQLite file, backup is trivial:
```bash
# Copy the database out of the container
docker cp agent-usage:/data/agent-usage.db ./backup.db

# Or if using a bind mount, just copy the file
cp ./data/agent-usage.db ./backup.db
```

---

## 6. Mounting Host Session Directories

The app reads JSONL session files from the host. These must be mounted into the container.

### Mount points

| Host path | Container path | Mode |
|-----------|---------------|------|
| `~/.claude/projects` | `/sessions/claude` | `ro` (read-only) |
| `~/.codex/sessions` | `/sessions/codex` | `ro` (read-only) |

### Docker run example

```bash
docker run -d \
  --name agent-usage \
  -p 9800:9800 \
  -v agent-usage-data:/data \
  -v ~/.claude/projects:/sessions/claude:ro \
  -v ~/.codex/sessions:/sessions/codex:ro \
  ghcr.io/briqt/agent-usage:latest
```

### Permission considerations

- The container runs as `nonroot` (UID 65534).
- Host session files are typically owned by the user (UID 1000).
- Read-only access works because the files are world-readable by default.
- If permission issues arise, users can:
  - Add `--user $(id -u):$(id -g)` to match host UID/GID
  - Or ensure session files have `o+r` permissions

### Path mapping in config

The Docker-specific config maps collector paths to the container mount points:
```yaml
collectors:
  claude:
    paths: ["/sessions/claude"]    # Not ~/.claude/projects
  codex:
    paths: ["/sessions/codex"]     # Not ~/.codex/sessions
```

This is why a separate `config.docker.yaml` is bundled — the default `config.yaml` uses `~` paths which don't resolve correctly inside the container.

### Multi-user / remote scenarios

For users who want to track sessions from multiple machines:
- Sync session directories to a shared location (rsync, NFS, etc.)
- Mount the shared location into the container
- Add multiple paths in the config:
  ```yaml
  collectors:
    claude:
      paths:
        - "/sessions/machine1/claude"
        - "/sessions/machine2/claude"
  ```

---

## 7. Image Size Optimization

### Baseline estimate

| Component | Size |
|-----------|------|
| Go binary (stripped, `CGO_ENABLED=0`) | ~15-20 MB |
| `distroless/static` base | ~2 MB |
| Config file | <1 KB |
| **Total** | **~17-22 MB** |

### Optimization strategies (already applied)

1. **Multi-stage build** — Go toolchain (~800 MB) stays in the build stage only.
2. **`-ldflags="-s -w"`** — Strips symbol table and DWARF debug info. Saves ~30% binary size.
3. **`CGO_ENABLED=0`** — Pure static binary, no glibc dependency, enables minimal base images.
4. **`distroless/static`** — ~2 MB base vs ~80 MB for `alpine` or ~140 MB for `debian-slim`.
5. **`go:embed`** — Web UI is already embedded in the binary, no extra files to copy.

### Additional optimizations (if needed)

6. **UPX compression** — Can reduce binary size by ~60%, but adds startup decompression time (~100ms). Not recommended unless image size is critical.
   ```dockerfile
   # In build stage:
   RUN apk add --no-cache upx && upx --best /agent-usage
   ```

7. **`scratch` base** — Saves ~2 MB over distroless, but loses CA certificates and timezone data. Since the app fetches pricing over HTTPS, CA certs are needed. Could work if certs are copied manually:
   ```dockerfile
   FROM scratch
   COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
   COPY --from=builder /agent-usage /agent-usage
   ```

8. **`.dockerignore`** — Prevents unnecessary files from entering the build context:
   ```
   .git
   .github
   dist/
   docs/
   *.db
   agent-usage
   ```

### Recommendation

The default setup (distroless/static + stripped binary) should produce a ~20 MB image. This is already excellent — smaller than most Alpine-based images. UPX and scratch are available if someone needs to squeeze further, but the complexity isn't worth it at this size.

---

## Implementation Checklist

### Files to create

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build |
| `config.docker.yaml` | Docker-specific config (0.0.0.0 bind, /sessions paths, /data storage) |
| `.dockerignore` | Exclude unnecessary files from build context |
| `docker-compose.yml` | One-command deployment for end users |
| `.github/workflows/docker.yml` | CI/CD for building and pushing images |

### Code changes required

1. **Config file search path** — Support loading config from `/etc/agent-usage/config.yaml` as a fallback, or add a `--config` CLI flag. This is the only code change needed.

### Documentation updates

- Add Docker section to README with quick-start instructions
- Reference this plan document for detailed architecture decisions

---

## Quick-Start (What Users Will See)

```bash
# One command to start tracking
docker compose up -d

# Open dashboard
open http://localhost:9800
```

That's it. Pull, run, done.
