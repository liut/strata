# Docker Deployment

## Why Alpine?

- No `/etc/alternatives` (musl libc)
- Small footprint (~5MB)
- Latest bubblewrap via Alpine `edge`

## Build

```bash
# Build with docker-compose
docker-compose build

# Or use pre-built binary
make dist/linux_amd64/strata
docker build -t strata:runtime -f docker/runtime.Dockerfile .
```

## Run

```bash
docker-compose up -d

# Or manually
docker run -d --name strata --privileged \
  --cap-add=SYS_ADMIN --device=/dev/fuse \
  -m 1g \
  -p 2280:2280 -v strata-sessions:/tmp/strata/sessions \
  strata:runtime
```

## Required Capabilities

| Capability | Purpose |
|------------|---------|
| SYS_ADMIN | overlayfs mount |
| NET_ADMIN | network isolation (optional) |
| /dev/fuse | fuse-overlayfs |

## Environment

| Variable | Default |
|----------|---------|
| STRATA_SERVER_ADDR | :2280 |
| STRATA_SANDBOX_SESSION_ROOT | /tmp/strata/sessions |
| STRATA_SANDBOX_SESSION_TTL | 30m |
