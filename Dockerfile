# ── Build stage ──────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build all three binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api    cmd/api/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/worker cmd/worker/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/smtp   cmd/smtp/main.go

# ── Runtime stage ────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries
COPY --from=builder /bin/api    /app/api
COPY --from=builder /bin/worker /app/worker
COPY --from=builder /bin/smtp   /app/smtp

# Copy migrations
COPY --from=builder /src/migrations /app/migrations

# Non-root user
RUN adduser -D -u 1001 swiftmail
USER swiftmail

EXPOSE 8080 9091

# Default to API server
ENTRYPOINT ["/app/api"]
