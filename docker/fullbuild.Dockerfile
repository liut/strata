# =============================================================================
# Stage 1: Build Strata binary
# =============================================================================
FROM --platform=linux/amd64 golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o strata .

# =============================================================================
# Stage 2: Build bun runtime
# =============================================================================
FROM oven/bun:alpine AS bun

# =============================================================================
# Stage 3: Runtime environment
# =============================================================================
FROM alpine:3.22 AS runtime

# Install runtime dependencies
RUN apk add --no-cache \
    bubblewrap \
    fuse-overlayfs \
    bash \
    coreutils \
    util-linux \
    ca-certificates \
    curl \
    git \
    python3 \
    py3-pip \
    nodejs \
    npm \
   && rm -rf /var/cache/apk/*

# Copy bun from build stage
COPY --from=bun /usr/local/bin/bun /usr/local/bin/
RUN test -x /usr/local/bin/bun && test ! -e "/usr/local/bin/bunx" && \
    ln -s /usr/local/bin/bun /usr/local/bin/bunx ; bun --version

# Create fuse config to allow non-root access and increase mount limit
RUN printf "user_allow_other\nmount_max = 1000\n" > /etc/fuse.conf && \
    chmod 644 /etc/fuse.conf

# Create session directory with proper permissions
RUN mkdir -p /tmp/strata/sessions && \
    chown -R nobody:nobody /tmp/strata

# Copy binary from builder stage
COPY --from=builder /build/strata /opt/bin/strata

# Note: Running as root because fuse operations require elevated privileges
# USER nobody

# Default environment variables
ENV STRATA_SERVER_ADDR=:2280
ENV STRATA_SANDBOX_SESSION_ROOT=/tmp/strata/sessions
ENV STRATA_SANDBOX_SESSION_TTL=30m
ENV STRATA_SANDBOX_MAX_SESSIONS=100
ENV STRATA_SANDBOX_OVERLAY_DRIVER=fuse

# Expose port (HTTP + gRPC via cmux)
EXPOSE 2280

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:2280/api/stats || exit 1

# Run
ENTRYPOINT ["/opt/bin/strata"]
CMD ["run"]
