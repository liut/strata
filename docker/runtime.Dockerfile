# =============================================================================
# Runtime-only Dockerfile
# =============================================================================
# Use this when you have a pre-built binary (e.g., from `make dist/linux_amd64/strata`)
# Build: docker build -t strata:runtime -f Dockerfile.runtime .
# =============================================================================
FROM alpine:3.22 AS runtime

# Install runtime dependencies
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories && \
  apk add --no-cache \
    bubblewrap \
    fuse-overlayfs \
    bash \
    shadow \
    coreutils \
    util-linux \
    ca-certificates \
    curl \
    git \
    # Common language runtimes
    python3 \
    py3-pip \
    nodejs \
    npm \
    strace \
    go \
    uv \
  && rm -rf /var/cache/apk/*

# Copy pre-built binary
COPY dist/linux_amd64/strata /opt/bin/

# Create fuse config to allow non-root access and increase mount limit
RUN printf "user_allow_other\nmount_max = 1000\n" > /etc/fuse.conf && \
    chmod 644 /etc/fuse.conf

# Create session directory with proper permissions
RUN mkdir -p /tmp/strata/sessions && \
    chown -R nobody:nobody /tmp/strata

# Note: Running as root because fuse operations require elevated privileges
# USER nobody

# Default environment variables
ENV STRATA_SERVER_ADDR=:8080 \
  STRATA_SANDBOX_SESSION_ROOT=/tmp/strata/sessions \
  STRATA_SANDBOX_SESSION_TTL=30m \
  STRATA_SANDBOX_MAX_SESSIONS=100 \
  STRATA_SANDBOX_OVERLAY_DRIVER=fuse

# Expose port (HTTP + gRPC via cmux)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/stats || exit 1

# Run
ENTRYPOINT ["/opt/bin/strata"]
CMD ["run"]
