# =============================================================================
# Runtime-only Dockerfile
# =============================================================================
# Use this when you have a pre-built binary (e.g., from `make dist/linux_amd64/strata`)
# Build: docker build -t strata:runtime -f docker/runtime.Dockerfile .
# =============================================================================
FROM alpine:3.22 AS runtime

# Install runtime dependencies
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories && \
  apk add --no-cache \
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
    py3-uv \
    nodejs \
    npm \
  && rm -rf /var/cache/apk/*

# Copy the pre-built amd64 binary. See docs/apple-container.md for the
# native arm64 build used by Apple Container on Apple silicon.
COPY dist/linux_amd64/strata /opt/bin/

# Create fuse config to allow non-root access and increase mount limit
RUN printf "user_allow_other\nmount_max = 1000\n" > /etc/fuse.conf && \
    chmod 644 /etc/fuse.conf

# Create session directory with proper permissions
RUN mkdir -p /tmp/strata/sessions && \
    chown -R nobody:nobody /tmp/strata

# Note: Running as root because fuse operations require elevated privileges
# USER nobody

# Default environment variables.
# pip installs to the user site by default (PEP 668 externally-managed env),
# so pip CLI scripts land in /root/.local/bin; add it to PATH.
# npm (Alpine package, prefix=/usr) global bins already live in /usr/bin.
ENV STRATA_SERVER_ADDR=:2280 \
  STRATA_SANDBOX_SESSION_ROOT=/tmp/strata/sessions \
  STRATA_SANDBOX_SESSION_TTL=30m \
  STRATA_SANDBOX_MAX_SESSIONS=100 \
  STRATA_SANDBOX_OVERLAY_DRIVER=fuse \
  PATH=/root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Expose port (HTTP + gRPC via cmux)
EXPOSE 2280

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:2280/api/stats || exit 1

# Run
ENTRYPOINT ["/opt/bin/strata"]
CMD ["run"]
