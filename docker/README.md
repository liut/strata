# Docker Deployment

## Why Alpine?

- No `/etc/alternatives` (musl libc)
- Small footprint (~5MB)
- Runtime packages are pinned to the Alpine release used by the Dockerfile

## Build

The runtime image consumes a prebuilt `linux/amd64` binary:

```bash
make dist/linux_amd64/strata
docker build -t strata:runtime -f docker/runtime.Dockerfile .
```

To build the binary and image from source in one stage, use the full build image:

```bash
docker build -t strata:runtime -f docker/fullbuild.Dockerfile .
```

The Compose file uses the runtime-only Dockerfile, so build the binary before
invoking Compose:

```bash
make dist/linux_amd64/strata
docker compose build
```

## Run

```bash
docker compose up -d

# Or manually
docker run -d --name strata --privileged \
  -e STRATA_SANDBOX_OVERLAY_DRIVER=kernel \
  -e STRATA_SERVER_SHELL_TIMEOUT=45s \
  -m 1g \
  -p 2280:2280 -v strata-sessions:/tmp/strata/sessions \
  strata:runtime
```

For macOS without Docker, see the
[Apple Container guide](../docs/apple-container.md).
It uses `docker/apple-runtime.Dockerfile` with the native arm64 binary.

## Required Capabilities

| Capability | Purpose |
|------------|---------|
| SYS_ADMIN | bubblewrap namespaces and overlayfs mount |
| NET_ADMIN | network isolation (optional) |
| /dev/fuse | fuse-overlayfs |

## Environment

| Variable | Default |
|----------|---------|
| STRATA_SERVER_ADDR | :2280 |
| STRATA_SANDBOX_SESSION_ROOT | /tmp/strata/sessions |
| STRATA_SANDBOX_SESSION_TTL | 30m |
| PATH | /root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin |

## Troubleshooting

If `docker compose build` reports a missing `dist/linux_amd64/strata`, run
`make dist/linux_amd64/strata` first. The Compose service uses
`docker/runtime.Dockerfile`, which copies an existing binary rather than compiling it.

## pip / npm Install Locations

The image is Alpine-based, and Alpine's Python is externally managed (PEP 668), so
`pip install` goes to the **user site** instead of the system site. Running as root:

| Installer | Package files | Executables | On PATH? |
|-----------|---------------|-------------|----------|
| pip | `/root/.local/lib/python3.12/site-packages` | `/root/.local/bin` | yes — added by the image |
| npm -g | `/usr/local/lib/node_modules` (or `/usr/lib/node_modules` when apk npm sets `prefix=/usr`) | `/usr/local/bin` (or `/usr/bin`) | yes — already in the default PATH |

For example, `pip install requests` puts the module at
`/root/.local/lib/python3.12/site-packages/requests/__init__.py` and any CLI scripts the
package ships into `/root/.local/bin`. The image prepends `/root/.local/bin` to `PATH`, so
those commands work immediately.

`npm install -g` needs no PATH change: on this image npm's prefix is `/usr/local`
(verified on a running container), so global modules land in `/usr/local/lib/node_modules`
and executable symlinks in `/usr/local/bin`, which is already first in the default `PATH`.
If you want Node scripts to `require()` globally installed modules, set
`NODE_PATH=/usr/local/lib/node_modules` explicitly — it is not set by default.
