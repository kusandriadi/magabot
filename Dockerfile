# Magabot Dockerfile
# Multi-stage build for minimal image size

# Stage 1: Build
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN VERSION=$(cat VERSION 2>/dev/null || echo 'docker') && \
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') && \
    BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") && \
    CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X github.com/kusa/magabot/internal/version.Version=${VERSION} -X github.com/kusa/magabot/internal/version.GitCommit=${GIT_COMMIT} -X github.com/kusa/magabot/internal/version.BuildTime=${BUILD_TIME}" \
    -o magabot ./cmd/magabot

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    tzdata

# Security: non-root user
RUN adduser -D -u 1000 magabot
USER magabot

WORKDIR /app

# Copy binary
COPY --from=builder /build/magabot .

# Create data directory
RUN mkdir -p data/sessions data/backups

# Default config location
VOLUME ["/app/data", "/app/platform"]

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD pgrep magabot || exit 1

EXPOSE 8080 8443

ENTRYPOINT ["./magabot"]
CMD ["-config", "config.yaml"]
