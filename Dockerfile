# ---- Build Stage ----
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG GOPROXY=https://proxy.golang.org,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY=${GOPROXY} go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /agent-usage .

# Create an empty directory for the runtime stage (distroless has no mkdir)
RUN mkdir /empty-dir

# ---- Runtime Stage ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /agent-usage /agent-usage
COPY config.docker.yaml /etc/agent-usage/config.yaml

EXPOSE 9800

# Create data directory owned by nonroot (UID 65534) so named volumes work
# without requiring user: override in compose.
COPY --from=builder --chown=nonroot:nonroot /empty-dir /data

USER nonroot:nonroot

ENTRYPOINT ["/agent-usage"]
