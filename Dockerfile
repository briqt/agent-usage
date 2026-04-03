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
COPY config.docker.yaml /etc/agent-usage/config.yaml

EXPOSE 9800

VOLUME ["/data"]

USER nonroot:nonroot

ENTRYPOINT ["/agent-usage"]
