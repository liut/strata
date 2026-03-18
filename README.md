# Strata

> Lightweight Session Sandbox Service — Isolated Shell Environments via Namespace + Overlayfs

```
strata v0.1.0 — lightweight session sandbox service
```

## Features

- **Lightweight Isolation**: No Docker Daemon dependency, uses Linux Namespace + bubblewrap + fuse-overlayfs
- **User/Session Isolation**: Isolated by user_id + session_id, each session has its own writable layer
- **Multi-Protocol Support**: HTTP REST / WebSocket / gRPC / MCP
- **Persistent Writes**: overlayfs layering, changes don't affect base image
- **Auto Cleanup**: TTL-based automatic cleanup of inactive sessions

## Architecture

```
┌─────────────────────────────────────────────┐
│  API Layer (HTTP/WS + gRPC + MCP)           │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│  Session Manager                            │
│  - GetOrCreate(user, session)               │
│  - TTL cleanup                              │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│  Isolation Layer                            │
│  bwrap + overlayfs (fuse-overlayfs)         │
│  ├── PID/IPC/UTS Namespace                  │
│  ├── overlay: lower + upper + merged         │
│  └── PTY (pseudo-terminal)                  │
└─────────────────────────────────────────────┘
```

## Quick Start

### 1. Check Environment

```bash
./scripts/check-env.sh
```

Ensure the following dependencies are available:
- `bubblewrap` (bwrap)
- `fuse-overlayfs`
- `/dev/fuse` device

### 2. Run Service

```bash
# Build
make build

# Start (default config)
./strata

# Or with custom environment variables
STRATA_SERVER_ADDR=:9000 ./strata
```

### 3. Use API

```bash
# Create session
curl -X POST http://localhost:2280/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001"}'

# Execute command
curl -X POST http://localhost:2280/api/sessions/alice/task-001/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "ls -la"}'

# Interactive Shell (WebSocket)
wscat -c 'ws://localhost:2280/api/ws/alice/task-001/shell'
# Input: {"type": "input", "data": "ls -la\n"}
```

### 4. MCP Integration (AI Agent)

When the service is running, MCP is available at the `/mcp/` endpoint on the same port:

```bash
# MCP endpoint
http://localhost:2280/mcp/
```

## API Reference

### HTTP REST

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/sessions` | Create/reuse session |
| DELETE | `/api/sessions/{user}/{session}` | Close session |
| POST | `/api/sessions/{uid}/{sid}/exec` | Execute command |
| GET | `/api/stats` | Service stats |

### WebSocket

| Path | Description |
|------|-------------|
| `/api/ws/{uid}/{sid}/shell` | Interactive Shell |

Message format:
- Client → Server: `{"type":"input", "data": "ls -la\n"}`
- Server → Client: `{"type":"output", "data": "..."}`

### gRPC

See `pkg/proto/sandbox/sandbox.proto`

```bash
# Generate Go code
make gen

# Or manually
protoc --go_out=. --go-grpc_out=. proto/sandbox/*.proto
```

## Configuration

Configuration is provided via environment variables.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATA_SERVER_ADDR` | `:2280` | HTTP/WS listen address |
| `STRATA_SANDBOX_BASE_ROOTFS` | - | Base read-only rootfs (optional) |
| `STRATA_SANDBOX_SESSION_ROOT` | `/tmp/strata/sessions` | Session working directory |
| `STRATA_SANDBOX_SESSION_TTL` | `30m` | Inactive session timeout |
| `STRATA_SANDBOX_MAX_SESSIONS` | `100` | Max concurrent sessions |
| `STRATA_SANDBOX_OVERLAY_DRIVER` | `fuse` | Overlay driver: fuse/kernel/none |
| `STRATA_SANDBOX_ISOLATE_NETWORK` | `false` | Enable network isolation per session |

View all options: `./strata run --help`

## Optional: Build Base Image

```bash
# Export from Docker image
./scripts/build-base.sh /opt/sandbox/base ubuntu 22.04
```

Then set `base_rootfs: /opt/sandbox/base` in config.

## Isolation Details

### Why not Docker?

Docker is "heavy" because of:
- Daemon always running
- Full image layer management
- Complex network model

But **isolation itself** (Namespace + cgroups) is extremely lightweight — a bwrap process starts in ~5ms with <1MB memory usage.

### Why fuse-overlayfs?

- Can be mounted by regular users (no root required)
- Semantics identical to kernel overlayfs
- Default storage driver for rootless Podman

### Session Lifecycle

```
Create → overlay mount → bwrap start → PTY establish
                ↓
User runs command → writes to upper layer (doesn't affect base)
                ↓
Close → PTY close → overlay unmount → delete upper
```

## Dependencies

- Linux kernel ≥ 5.11 (recommended)
- bwrap (bubblewrap)
- fuse-overlayfs
- Go ≥ 1.23

## License

MIT
