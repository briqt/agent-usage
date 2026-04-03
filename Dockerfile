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

RUN CGO_ENABLED=0 GOOS=linux GOPROXY=${GOPROXY} go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /agent-usage .

# ---- Runtime Stage ----
FROM alpine:3.21

# Copy CA certs from builder (needed for HTTPS pricing sync)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

RUN mkdir -p /data /sessions/claude /sessions/codex

COPY --from=builder /agent-usage /agent-usage
COPY config.docker.yaml /etc/agent-usage/config.yaml

EXPOSE 9800

VOLUME ["/data"]

ENTRYPOINT ["/agent-usage"]
