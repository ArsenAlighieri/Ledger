# Stage 1: Build binary
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Install certificates for HTTPS requests to TCMB / TEFAS / Yahoo
RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build statically linked pure-Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/ledger ./cmd/ledger

# Stage 2: Minimal runtime
FROM alpine:3.20

WORKDIR /app

# Install ca-certificates, curl for health check, and su-exec for safe permission drop
RUN apk add --no-cache ca-certificates tzdata curl su-exec && \
    addgroup -g 10001 -S ledger && \
    adduser -u 10001 -S ledger -G ledger && \
    mkdir -p /app/data /app/data/backups && \
    chown -R ledger:ledger /app

COPY --from=builder /bin/ledger /app/ledger
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

VOLUME ["/app/data"]

EXPOSE 8085

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:8085/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
