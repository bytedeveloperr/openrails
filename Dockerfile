# syntax=docker/dockerfile:1

# Stage 1: build

FROM golang:1.24.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies with cache mount for Go modules (with retry)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    for i in 1 2 3; do \
      go mod download && break || (echo "go mod download failed, retrying" && sleep 5); \
    done

# Copy source code
COPY . .

# Build the application with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p bin && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/billing ./


# Stage 2: production
FROM alpine:3.19

# Install runtime dependencies (include wget for healthcheck)
RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /app

# Create non-root user
RUN addgroup -g 1001 -S billing && \
    adduser -S -D -H -u 1001 -s /sbin/nologin -G billing billing

# Copy binary from builder stage
COPY --from=builder /app/bin/billing ./bin/billing

# Copy migrations directory
COPY --from=builder /app/migrations ./migrations/

# Copy configuration defaults
COPY --from=builder /app/config.yaml ./config.yaml
COPY --from=builder /app/config.docker.yaml ./config.docker.yaml

# Copy configuration defaults
COPY --from=builder /app/config.yaml ./config.yaml
COPY --from=builder /app/config.docker.yaml ./config.docker.yaml

# Change ownership to non-root user
RUN chown -R billing:billing /app

# Switch to non-root user
USER billing

# Expose ports (2053 public; 8060 private/internal)
EXPOSE 2053 8060

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:2053/health || exit 1

# Default command
CMD ["./bin/billing", "server"]
