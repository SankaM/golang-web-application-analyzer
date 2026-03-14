# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifests first so Docker can cache the download layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/server ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

# Run as a non-root user for security.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/bin/server .
COPY --from=builder /app/web ./web

RUN chown -R appuser:appgroup /app
USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/ || exit 1

CMD ["./server"]
