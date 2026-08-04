# Changelog

## v0.1.0

Kernel overlay driver overhaul: mounts now persist for the session lifetime with automatic fallback, and shell exec timeout is configurable.

### Features

- **Kernel overlay overhaul**: mounts persist for the whole session — direct mount with `CAP_SYS_ADMIN`, or in the same user+mount namespace as bwrap for unprivileged/rootless; automatic fallback to tmpfs on mount failure
- **Configurable exec timeout**: `STRATA_SERVER_SHELL_TIMEOUT` (default `30s`) for HTTP / MCP / gRPC exec
- **Rootfs auto-detection**: host system dirs are bound only for the minimal rootfs; a complete rootfs gets full isolation

### Notes

- `py3-uv` added to the runtime Docker image; kernel overlay demo documented in `docs/kernel-overlay-demo.md`
- Linux-only test fixes and errcheck / formatting cleanup

## v0.0.1 (initial)

Initial release of Strata — a lightweight session sandbox providing isolated shell environments via Linux namespaces and overlayfs.

### Features

- **Isolation**: Linux namespace + bubblewrap + fuse-overlayfs per session
- **Multi-protocol API**: HTTP REST, WebSocket, gRPC, MCP
- **Overlay persistence**: writable upper layer with shared lower base; changes do not affect host
- **TTL cleanup**: automatic session expiry
- **Flexible identity**: path-based or header-based (`X-Owner-Id`, `X-Session-Id`)
- **Configurable**: envconfig-driven (`STRATA_*`); overlay driver switchable (`fuse` / `kernel` / `none`)

### Notes

- Requires Linux kernel ≥ 5.11
- Runtime deps: `bubblewrap` (bwrap), `fuse-overlayfs`
- Build dep: Go ≥ 1.25, `meson`, `libcap-dev`
- bwrap needs `cap_sys_admin` capability (`sudo setcap cap_sys_admin+ip /usr/bin/bwrap`)
