# Changelog

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