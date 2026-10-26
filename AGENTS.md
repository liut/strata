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
├── rootfs/        # shared base root filesystem
└── userID/
    └── sessionID/
        ├── home/      # writable home directory
        ├── upper/     # overlay writable layer
        ├── work/      # overlay work directory
        └── merged/    # overlay merged view
```

## Common Development Tasks

### Adding an HTTP Endpoint

1. Add handler in `pkg/webapi/handler.go`
2. Register route in `Route()`

```go
func (h *handlerImpl) handleHealth(w http.ResponseWriter, r *http.Request) {
    jsonOK(w, map[string]string{"status": "ok"})
}

func (h *handlerImpl) Route(mux Handler) {
    mux.Handle("GET /health", http.HandlerFunc(h.handleHealth))
}
```

### Adding a gRPC Method

1. Define in `pkg/proto/sandbox/sandbox.proto`
2. Regenerate: `make gen-rpc`
3. Implement in `pkg/rpc/service.go`

### Changing Overlay Driver

```go
// In code
driver := sandbox.OverlayDriver("kernel")

# Or via env
STRATA_SANDBOX_OVERLAY_DRIVER=kernel ./dist/strata run
```

### Identity Parsing

Identity is parsed using `ParseScarf` with getter functions. Later getters override earlier ones.

```go
// Header priority over PathValue
sc, err := identity.ParseScarf(ctx, r.PathValue, identity.FromHeader(r.Header))
```

### Adding an MCP Tool

1. Add tool definition in `pkg/mcp/tools.go`
2. Implement handler in `pkg/mcp/handler.go`

## Testing

```bash
go test ./...
make build && ./dist/strata run
./scripts/test-api.sh
```

For the macOS Apple Container deployment path, see
[docs/apple-container.md](./docs/apple-container.md).

## Troubleshooting

| Issue | Solution |
|-------|----------|
| bwrap: permissions error on Linux | Verify unprivileged user namespaces; do not apply file capabilities to modern bwrap |
| bwrap: namespace error in a container | Grant `CAP_SYS_ADMIN` to the container, for example `--cap-add SYS_ADMIN` |
| overlay mount failed | Try `STRATA_SANDBOX_OVERLAY_DRIVER=none` |
| session hangs | `pkill -f "bwrap.*sessionID"` |

For dependency installation, see [README.md](./README.md#dependencies).
