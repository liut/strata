# AGENTS.md — Developer Guide

> For AI agents and developers who need to understand, modify, or extend Strata.

## Project Structure

```
strata/
├── cmd/                    # Entry points
│   ├── run.go              # Server (HTTP + gRPC + MCP)
│   ├── root.go             # CLI root
│   ├── cli.go              # Interactive shell
│   └── middleware.go       # HTTP middleware
├── pkg/
│   ├── config/             # Configuration (envconfig)
│   ├── proto/sandbox/      # gRPC definitions
│   ├── sandbox/            # Isolation engine
│   │   ├── session.go      # Session, bwrap, PTY
│   │   ├── manager.go      # Session pool, TTL
│   │   └── overlay.go      # fuse-overlayfs mount
│   ├── webapi/             # HTTP handlers + WebSocket
│   ├── mcp/                # MCP handlers
│   └── rpc/                # gRPC service
└── scripts/                # Test & utility scripts
```

## Dependencies

### Build Dependencies

| Package | Description | Install |
|---------|-------------|---------|
| `meson` | Ninja build system generator | `apt install meson` |
| `libcap-dev` | Libcap development headers (for bwrap caps) | `apt install libcap-dev` |

### Runtime Dependencies

| Package | Description | Install |
|---------|-------------|---------|
| `bubblewrap` | Sandboxing tool (`bwrap` command) | Build from [github.com/containers/bubblewrap](https://github.com/containers/bubblewrap) |
| `fuse-overlayfs` | Userspace overlay filesystem | `apt install fuse-overlayfs` |

### Building bubblewrap

```bash
git clone https://github.com/containers/bubblewrap
cd bubblewrap
meson _builddir
meson compile -C _builddir
meson test -C _builddir
meson install -C _builddir
```

Note: After installing bwrap, you may need to set capabilities for it to allow sandboxing:

```bash
sudo setcap cap_sys_admin+ip /usr/bin/bwrap
```

### Optional Testing Tools

| Tool | Purpose | Install |
|------|---------|---------|
| `websocat` | WebSocket client for testing | `apt install websocat` or build from source |
| `timeout` | Command timeout wrapper (coreutils) | `apt install coreutils` |

## Key Packages

| Package | Responsibility |
|---------|----------------|
| `pkg/sandbox` | Core isolation: overlay, bwrap, PTY |
| `pkg/webapi` | HTTP handlers, WebSocket |
| `pkg/rpc` | gRPC service implementation |
| `pkg/mcp` | MCP protocol handlers |

## Key Files

| File | Purpose |
|------|---------|
| `pkg/sandbox/session.go` | Session lifecycle, bwrap process |
| `pkg/sandbox/overlay.go` | fuse-overlayfs mount |
| `pkg/sandbox/manager.go` | Session pool, TTL cleanup |
| `pkg/rpc/service.go` | gRPC service implementation |

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        Clients                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐     │
│  │  curl   │  │ws client│  │  gRPC   │  │   MCP   │     │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘     │
└───────┼────────────┼────────────┼────────────┼──────────┘
        │            │            │            │
┌───────▼────────────▼────────────▼────────────▼──────────┐
│                    API Layer                            │
│    HTTP Handler  │  WebSocket  │  gRPC Handler          │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│               Session Manager                           │
│   GetOrCreate(userID, sessionID) → *Session             │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│              Isolation Layer                            │
│  ┌────────────────────────────────────────────────────┐ │
│  │  fuse-overlayfs                                    │ │
│  │    lowerdir ──►  upperdir  ──►  merged (/)         │ │
│  └────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────┐ │
│  │  bwrap --bind merged/ / --unshare-pid/ipc/uts      │ │
│  │   └── /bin/bash (+ PTY)                            │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## Session Lifecycle

1. **Create Request** → `Manager.GetOrCreate(userID, sessionID)`
2. **Overlay Mount** → `fuse-overlayfs -o lowerdir=X,upperdir=Y,workdir=Z merged`
3. **bwrap Start** → `bwrap --bind merged/ / --unshare-pid ... /bin/bash`
4. **PTY Allocation** → `pty.Start(cmd)` for terminal I/O
5. **Return Session** → Client gets read/write access to PTY

## Session Directory Structure

Sessions are stored hierarchically under `SESSION_ROOT`:

```
SESSION_ROOT/
└── userID/
    └── sessionID/
        ├── home/      # session root filesystem
        ├── upper/     # overlay writable layer
        ├── work/      # overlay work directory
        └── merged/    # overlay merged view
```

## Common Development Tasks

### Adding an HTTP Endpoint

1. Add handler in `pkg/webapi/handler.go`
2. Register route in `Register()`

```go
func (h *handlerImpl) handleHealth(w http.ResponseWriter, r *http.Request) {
    jsonOK(w, map[string]string{"status": "ok"})
}

func (h *handlerImpl) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /health", h.handleHealth)
}
```

### Adding a gRPC Method

1. Define in `pkg/proto/sandbox/sandbox.proto`
2. Regenerate: `make gen`
3. Implement in `pkg/rpc/service.go`

### Changing Overlay Driver

```go
// In code
driver := sandbox.OverlayDriver("kernel")

# Or via env
STRATA_SANDBOX_OVERLAY_DRIVER=kernel ./dist/strata
```

## Testing

```bash
go test ./...
make build && ./dist/strata
./scripts/test-api.sh
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| bwrap not found | Build and install bubblewrap (see Dependencies) |
| bwrap: permissions error | Run `sudo setcap cap_sys_admin+ip /usr/bin/bwrap` |
| fuse-overlayfs not found | `apt install fuse-overlayfs` |
| /dev/fuse permission denied | `sudo chmod 666 /dev/fuse` |
| overlay mount failed | Try `STRATA_SANDBOX_OVERLAY_DRIVER=none` |
| session hangs | `pkill -f "bwrap.*sessionID"` |
