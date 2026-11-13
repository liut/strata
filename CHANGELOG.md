# Changelog

## v0.2.0

Expanded the MCP file workflow and protocol support, and added an Apple Container deployment path.

### Features

- **MCP file tools**: added `read`, `write`, and `edit` for persistent sandbox files under `/root`, with workspace containment, UTF-8 validation, atomic writes, path locking, exact-match validation, and diff/patch results
- **MCP 2026-07-28**: upgraded `mcp-go` from `v0.45.0` to `v1.5.0` and enabled stateless Streamable HTTP requests without `initialize` or an MCP transport session
- **Legacy compatibility**: retained initialize-based MCP `2025-11-25` support, including transport sessions
- **Apple Container**: added an arm64 runtime image and documentation for running Strata on Apple silicon

### Upgrade Notes

- Building now requires Go 1.25.5 or newer
- Strata sandbox identity remains explicit through `owner_id` / `session_id` or `X-Owner-Id` / `X-Session-Id`; `X-Session-Id` is unrelated to the removed `Mcp-Session-Id` transport header
- Modern MCP requests carry their own protocol metadata; responses include `resultType`, server identity, and cache hints. Legacy responses retain the initialize/session contract

### Notes

- Added protocol-level HTTP coverage for discovery, tool listing, header and version validation, removed transport behaviors, and modern/legacy interoperability
- The MCP smoke test now defaults to the modern flow; set `STRATA_MCP_LEGACY=1` to exercise legacy compatibility
- Added `make test` for the packages under `pkg/` that contain tests

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
- bwrap needs access to Linux namespaces; do not apply file capabilities to modern bwrap.
  Containers must grant `CAP_SYS_ADMIN` when bwrap runs inside them.
