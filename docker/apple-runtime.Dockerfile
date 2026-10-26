# =============================================================================
# Apple Container runtime image
#
# Build:
#   make dist/linux_arm64/strata
#   container build --arch arm64 \
#     -f docker/apple-runtime.Dockerfile \
#     -t strata:apple-arm64 .
#
# BASE_IMAGE can point at a mirror or a locally available Alpine image.
# Alpine package repositories are redirected to the USTC mirror.
# =============================================================================
ARG BASE_IMAGE=alpine:3.22
FROM ${BASE_IMAGE}

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

COPY dist/linux_arm64/strata /opt/bin/

RUN printf "user_allow_other\nmount_max = 1000\n" > /etc/fuse.conf && \
    chmod 644 /etc/fuse.conf && \
    mkdir -p /tmp/strata/sessions && \
    chown -R nobody:nobody /tmp/strata

ENV STRATA_SERVER_ADDR=:2280 \
  STRATA_SANDBOX_SESSION_ROOT=/tmp/strata/sessions \
  STRATA_SANDBOX_SESSION_TTL=30m \
  STRATA_SANDBOX_MAX_SESSIONS=100 \
  STRATA_SANDBOX_OVERLAY_DRIVER=fuse \
  PATH=/root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

EXPOSE 2280

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:2280/api/stats || exit 1

ENTRYPOINT ["/opt/bin/strata"]
CMD ["run"]
