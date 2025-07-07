# Multi-stage build for NoiseFS
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev \
    linux-headers

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build all NoiseFS binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/noisefs ./cmd/noisefs
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/noisefs-mount ./cmd/noisefs-mount
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/noisefs-benchmark ./cmd/noisefs-benchmark
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/noisefs-config ./cmd/noisefs-config
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/webui ./cmd/webui

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    fuse \
    fuse-dev \
    bash \
    curl \
    jq

# Create noisefs user
RUN addgroup -g 1000 noisefs && \
    adduser -D -s /bin/bash -u 1000 -G noisefs noisefs

# Create necessary directories
RUN mkdir -p /opt/noisefs/bin \
             /opt/noisefs/config \
             /opt/noisefs/data \
             /opt/noisefs/logs \
             /opt/noisefs/mount \
             /opt/noisefs/cache && \
    chown -R noisefs:noisefs /opt/noisefs

# Copy binaries from builder
COPY --from=builder /app/bin/* /opt/noisefs/bin/

# Copy configuration examples
COPY config.example.json /opt/noisefs/config/

# Create symlinks for easier access
RUN ln -sf /opt/noisefs/bin/noisefs /usr/local/bin/noisefs && \
    ln -sf /opt/noisefs/bin/noisefs-mount /usr/local/bin/noisefs-mount && \
    ln -sf /opt/noisefs/bin/noisefs-benchmark /usr/local/bin/noisefs-benchmark && \
    ln -sf /opt/noisefs/bin/noisefs-config /usr/local/bin/noisefs-config && \
    ln -sf /opt/noisefs/bin/webui /usr/local/bin/noisefs-webui

# Set up FUSE permissions
RUN echo "user_allow_other" >> /etc/fuse.conf

# Create entrypoint script
COPY docker/entrypoint.sh /opt/noisefs/entrypoint.sh
RUN chmod +x /opt/noisefs/entrypoint.sh

# Switch to noisefs user
USER noisefs

# Set working directory
WORKDIR /opt/noisefs

# Environment variables
ENV NOISEFS_CONFIG_FILE=/opt/noisefs/config/config.json
ENV NOISEFS_DATA_DIR=/opt/noisefs/data
ENV NOISEFS_LOG_DIR=/opt/noisefs/logs
ENV NOISEFS_CACHE_DIR=/opt/noisefs/cache
ENV NOISEFS_MOUNT_POINT=/opt/noisefs/mount

# Expose ports
EXPOSE 8080 4001 5001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Default command
ENTRYPOINT ["/opt/noisefs/entrypoint.sh"]
CMD ["daemon"]