# AGENTS.md — Developer Guide for Strata

> This file is for AI agents and developers who need to understand, modify, or extend the Strata codebase.

---

## What is Strata?

Strata is a **lightweight session sandbox service** that provides isolated shell environments for users. It uses Linux namespaces + bubblewrap + fuse-overlayfs instead of Docker, making it fast (~5ms startup) and lightweight (<1MB memory per session).

**Core capability**: Each session is an isolated Linux environment where users can run shell commands, with changes persisted in an overlay filesystem that doesn't affect the host.

---

## Project Structure

```
strata/
├── cmd/                    # Entry points
│   ├── grpc.go             # gRPC server
│   ├── web.go              # HTTP/WS server
│   └── root.go             # Root command
├── configs/
│   └── config.yaml
├── pkg/                    # Core packages
│   ├── config/             # Configuration loading
│   ├── proto/sandbox/      # gRPC definitions
│   │   └── sandbox.proto
│   ├── sandbox/            # Isolation engine
│   │   ├── manager.go      # Session lifecycle
│   │   ├── overlay.go      # fuse-overlayfs mount
│   │   └── session.go      # bwrap + PTY
│   └── webapi/             # HTTP handlers + WebSocket
└── scripts/
    ├── check-env.sh        # Dependency checker
    └── test-api.sh        # API 测试脚本
```

**Key packages:**

| Package | Responsibility |
|---------|----------------|
| `pkg/sandbox` | Core isolation logic: overlay mount, bwrap process, PTY |
| `pkg/webapi` | HTTP/gRPC handlers, WebSocket shell |
| `pkg/config` | YAML config loading |
| `pkg/proto/sandbox` | gRPC service definition |

---

## Prerequisites

Before running Strata, ensure these dependencies are available:

```bash
# Core (required)
apt install bubblewrap fuse-overlayfs

# Check
./scripts/check-env.sh
```

**Required on the host:**
- Linux (kernel ≥ 5.11 recommended)
- `/dev/fuse` device
- `bubblewrap` (bwrap)
- `fuse-overlayfs`

---

## How to Build

```bash
# Full build
go build -o bin/strata ./cmd/strata
go build -o bin/strata-grpc ./cmd/grpc

# Or run directly
go run ./cmd/strata
```

---

## Configuration

Edit `configs/config.yaml`:

```yaml
server:
  addr: ":8080"

sandbox:
  # Base root filesystem (optional)
  # Leave empty to use host directories as lower layers
  base_rootfs: ""

  # Session working directory
  session_root: "/tmp/strata/sessions"

  # Auto-cleanup inactive sessions
  session_ttl: "30m"

  # Max concurrent sessions
  max_sessions: 100

  # Network isolation per session
  isolate_network: false

  # Overlay driver: fuse | kernel | none
  # fuse = fuse-overlayfs (recommended, no root)
  # kernel = native overlayfs in user namespace
  # none = pure bwrap with tmpfs (no persistence)
  overlay_driver: "fuse"

grpc:
  addr: ":9090"
```

---

## Running the Service

```bash
# HTTP + WebSocket
./bin/strata

# Or with custom config
./bin/strata -config /path/to/config.yaml
```

Service will listen on:
- **HTTP**: `http://localhost:8080`
- **WebSocket**: `ws://localhost:8080/ws/shell`
- **gRPC**: `localhost:9090`

---

## API Reference

### HTTP REST

#### Create Session
```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001"}'
```

#### Execute Command
```bash
curl -X POST http://localhost:8080/api/sessions/alice/task-001/exec \
  -H "Content-Type: application/json" \
  -d '{
    "command": "ls -la /root",
    "timeout_ms": 10000
  }'
```

#### Close Session
```bash
curl -X DELETE http://localhost:8080/api/sessions/alice/task-001
```

#### Get Stats
```bash
curl http://localhost:8080/api/stats
```

---

### WebSocket (Interactive Shell)

**Endpoint**: `ws://localhost:8080/api/ws/alice/task-001/shell`

**Client → Server**:
```json
{"type": "input", "data": "ls -la\n"}
{"type": "resize", "rows": 40, "cols": 120}
```

**Server → Client**:
```json
{"type": "output", "data": "total 0\ndrwxr-xr-x   1 root root 4096 Mar 17 00:00 .\n"}
{"type": "error", "data": "session closed"}
```

---

### gRPC

See `pkg/proto/sandbox/sandbox.proto` for the full definition.

```bash
# Generate Go code
protoc --go_out=. --go-grpc_out=. pkg/proto/sandbox/sandbox.proto
```

**Service**: `SandboxService`

| Method | Description |
|--------|-------------|
| `CreateSession` | Create or reuse a session |
| `CloseSession` | Close and cleanup a session |
| `Exec` | Run a single command (non-interactive) |
| `Shell` | Bidirectional streaming shell |
| `Stats` | Get service statistics |

---

## MCP Integration (AI Agents)

For AI agents (Claude, GPT, etc.), use the MCP server:

```bash
# Set API endpoint
export STRATA_API=http://localhost:8080

# Run MCP server
npx tsx mcp/src/server.ts
```

**Available Tools:**

| Tool | Description |
|------|-------------|
| `strata_create_session` | Create/reuse a sandbox session |
| `strata_exec` | Execute a shell command |
| `strata_write_file` | Write a file to the sandbox |
| `strata_read_file` | Read a file from the sandbox |
| `strata_close_session` | Close and cleanup a session |
| `strata_stats` | Get service statistics |

---

## How It Works (For Developers)

### Session Lifecycle

1. **Create Request** → `Manager.GetOrCreate(userID, sessionID)`
2. **Overlay Mount** → `fuse-overlayfs -o lowerdir=X,upperdir=Y,workdir=Z merged`
3. **bwrap Start** → `bwrap --bind merged/ / --unshare-pid ... /bin/bash`
4. **PTY Allocation** → `pty.Start(cmd)` for terminal I/O
5. **Return Session** → Client gets read/write access to PTY

### Isolation Layers

```
┌─────────────────────────────────────┐
│  Network: unshare-net (optional)   │
├─────────────────────────────────────┤
│  PID:  unshare-pid                  │
├─────────────────────────────────────┤
│  IPC:  unshare-ipc                  │
├─────────────────────────────────────┤
│  UTS:  unshare-uts (hostname)       │
├─────────────────────────────────────┤
│  Mount: overlayfs + bubblewrap      │
├─────────────────────────────────────┤
│  User: user namespace (implicit)   │
└─────────────────────────────────────┘
```

### Writing Persistence

When a user modifies files in the session:
- Changes go to `upper/` directory (per session)
- Base layer (`lowerdir`) remains unchanged
- Other sessions see their own upper layers only

### Cleanup

- Session closed → PTY killed → `fusermount -u merged` → delete `upper/work/merged`

---

## Common Development Tasks

### Adding a New API Endpoint

1. Add handler in `pkg/webapi/handler.go`
2. Register route in `handler.Routes()`

Example:
```go
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
    jsonOK(w, map[string]string{"status": "ok"})
}

// In Routes():
mux.HandleFunc("GET /health", h.HandleHealth)
```

### Adding a gRPC Method

1. Add to `pkg/proto/sandbox/sandbox.proto`
2. Regenerate: `protoc --go_out=. --go-grpc_out=. pkg/proto/sandbox/sandbox.proto`
3. Implement in `cmd/grpc.go`

### Changing Overlay Driver

In `configs/config.yaml`:
```yaml
sandbox:
  overlay_driver: "kernel"  # Use native overlayfs in user namespace
```

Or in code (`pkg/config/config.go`):
```go
driver := sandbox.OverlayDriver("kernel")
```

---

## Testing

```bash
# Unit tests
go test ./...

# Start server
go run .

# Run API test script (in another terminal)
./scripts/test-api.sh

# Or with custom host/port
STRATA_HOST=192.168.1.100 STRATA_PORT=9000 ./scripts/test-api.sh
```

---

## Troubleshooting

### "fuse-overlayfs not found"
```bash
apt install fuse-overlayfs
```

### "/dev/fuse: permission denied"
```bash
# Check fuse device
ls -l /dev/fuse

# If missing, load module (as root)
modprobe fuse

# Or check权限
sudo chmod 666 /dev/fuse
```

### "overlay mount failed"
- Try `overlay_driver: "none"` in config (bypass overlay, no persistence)
- Check user namespace: `cat /proc/sys/kernel/unprivileged_userns_clone`

### Session hangs
- Check PTY: `ps aux | grep bwrap`
- Kill manually: `pkill -f "bwrap.*sessionID"`

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `pkg/sandbox/session.go` | Creates bwrap process with PTY |
| `pkg/sandbox/overlay.go` | Mounts fuse-overlayfs |
| `pkg/sandbox/manager.go` | Session pool + TTL cleanup |
| `pkg/webapi/exec.go` | Command execution via PTY |
| `pkg/webapi/ws.go` | WebSocket terminal |
| `cmd/strata/main.go` | HTTP server entry point |
| `pkg/proto/sandbox/sandbox.proto` | gRPC service definition |

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────┐
│                        Clients                           │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐     │
│  │  curl   │  │ ws client│  │  gRPC   │  │   MCP   │     │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘     │
└───────┼────────────┼────────────┼────────────┼───────────┘
        │            │            │            │
┌───────▼────────────▼────────────▼────────────▼───────────┐
│                    API Layer                             │
│   HTTP Handler  │  WebSocket  │  gRPC Handler          │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│               Session Manager                            │
│   GetOrCreate(userID, sessionID) → *Session            │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│              Isolation Layer                            │
│  ┌────────────────────────────────────────────────────┐ │
│  │  fuse-overlayfs                                    │ │
│  │   lowerdir ──►  upperdir  ──►  merged (/)          │ │
│  └────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────┐ │
│  │  bwrap --bind merged/ / --unshare-pid/ipc/uts     │ │
│  │   └── /bin/bash (+ PTY)                            │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```
