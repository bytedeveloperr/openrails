# syntax=docker/dockerfile:1
# Build stage
FROM golang:1.24.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies with cache mount for Go modules
# This cache persists between builds, dramatically speeding up builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy source code
COPY . .

# Build the application with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o billing ./cmd/billing/

# Production stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create non-root user
RUN addgroup -g 1001 -S billing && \
    adduser -S -D -H -u 1001 -s /sbin/nologin -G billing billing

# Copy binary from builder stage
COPY --from=builder /app/billing .

# Copy migrations directory
COPY --from=builder /app/migrations ./migrations/

# Change ownership to non-root user
RUN chown -R billing:billing /app

# Switch to non-root user
USER billing

# Expose ports
EXPOSE 2052 8060

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:2052/health || exit 1

# Default command
CMD ["./billing", "server"]