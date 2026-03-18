# Strata Docker Deployment Guide

This guide explains how to run Strata in Docker containers.

## Why Alpine?

Strata uses **Alpine Linux** as the base image because:

1. **No `/etc/alternatives`**: Alpine uses musl libc instead of glibc, so it doesn't have the complex symlink management that causes issues in bubblewrap
2. **Small footprint**: ~5MB base image
3. **Fast startup**: Minimal initialization
4. **Latest bubblewrap**: Using Alpine `edge` for the newest bubblewrap version

## Build Process

The fullbuild.Dockerfile has two stages:

1. **Builder**: Compiles the Go binary
2. **Runtime**: Contains only runtime dependencies (bubblewrap, fuse-overlayfs, etc.)

### Option 1: Build Everything (Development)

```bash
# Build both stages
docker-compose build

# Or manually
docker build -t strata .
```

### Option 2: Use Pre-built Binary

If you already have a pre-built binary, use `runtime.Dockerfile`:

```bash
# Step 1: Build binary
make dist/linux_amd64/strata

# Step 2: Build runtime image (from project root)
docker build -t strata:runtime -f docker/runtime.Dockerfile .
```

### Option 3: Build Everything (Development)

```bash
# Build binary and runtime in one command
docker build -t strata .
```

## Quick Start

### Using Docker Compose

```bash
# Build and start
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Using Docker Directly

```bash
# Build
docker build -t strata .

# Run
docker run -d \
  --name strata --privileged \
  --cap-add=SYS_ADMIN \
  --cap-add=NET_ADMIN \
  --device=/dev/fuse \
  -p 2280:2280 \
  -v strata-sessions:/tmp/strata/sessions \
  strata:runtime
```

## Required Capabilities

| Capability | Purpose |
|------------|---------|
| `SYS_ADMIN` | Required for overlayfs mount operations |
| `NET_ADMIN` | Optional - for network isolation per session |
| `/dev/fuse` | Required for fuse-overlayfs driver |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATA_SERVER_ADDR` | `:2280` | HTTP/WS listen address |
| `STRATA_SANDBOX_SESSION_ROOT` | `/tmp/strata/sessions` | Session working directory |
| `STRATA_SANDBOX_SESSION_TTL` | `30m` | Inactive session timeout |
| `STRATA_SANDBOX_MAX_SESSIONS` | `100` | Max concurrent sessions |
| `STRATA_SANDBOX_OVERLAY_DRIVER` | `fuse` | Overlay driver: fuse/kernel/none |
| `STRATA_SANDBOX_ISOLATE_NETWORK` | `false` | Enable network isolation per session |

## API Endpoints

After starting:

- **HTTP**: http://localhost:2280
- **gRPC**: localhost:2280 (via HTTP/2)
- **WebSocket**: ws://localhost:2280/api/ws/{user_id}/{session_id}/shell

## Testing

```bash
# Create a session
curl -X POST http://localhost:2280/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "test-001"}'

# Execute command
curl -X POST http://localhost:2280/api/sessions/alice/test-001/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "echo hello"}'

# Get stats
curl http://localhost:2280/api/stats
```

## Troubleshooting

### "/dev/fuse: permission denied"

Ensure the fuse device is properly exposed:

```yaml
devices:
  - /dev/fuse:/dev/fuse
```

And in `/etc/fuse.conf` (already configured in image):

```
user_allow_other
```

### "Operation not permitted" with overlayfs

Make sure `SYS_ADMIN` capability is granted:

```yaml
cap_add:
  - SYS_ADMIN
```

### Session commands fail

Check logs:

```bash
docker-compose logs strata
```

## Production Considerations

1. **Use external volume**: Replace the named volume with a host mount for better persistence
2. **Add authentication**: Currently no auth - add reverse proxy with auth if exposing to public
3. **Resource limits**: Add memory/CPU limits in docker-compose
4. **Monitoring**: Add Prometheus metrics exporter

Example with production config:

```yaml
services:
  strata:
    # ... other config
    volumes:
      - /data/strata-sessions:/tmp/strata/sessions
    deploy:
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M
```

## Custom Bubblewrap Version

If you need the latest bubblewrap from source:

```dockerfile
# runtime.Dockerfile
FROM alpine:edge AS runtime

# Install build deps for bubblewrap
RUN apk add --no-cache \
    autoconf \
    automake \
    libtool \
    pkgconfig \
    glib-dev \
    gettext \
    bash \
    shadow \
    coreutils \
    util-linux \
    ca-certificates

# Build latest bubblewrap from source
RUN git clone https://github.com/containers/bubblewrap.git /tmp/bubblewrap && \
    cd /tmp/bubblewrap && \
    ./autogen.sh && \
    ./configure --prefix=/usr && \
    make && make install && \
    rm -rf /tmp/bubblewrap

# ... rest of runtime setup
```
