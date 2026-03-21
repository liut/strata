# Strata

> Lightweight Session Sandbox — Isolated Shell via Namespace + Overlayfs

[中文版](./README.zh.md)

## Features

- **Lightweight**: Linux Namespace + bubblewrap + fuse-overlayfs
- **Isolated**: Per-user + per-session with writable overlay layer
- **Multi-Protocol**: HTTP REST / WebSocket / gRPC / MCP
- **Persistent**: overlayfs layering, changes don't affect base
- **Auto Cleanup**: TTL-based session cleanup

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
│  ├── overlay: lower + upper + merged        │
│  └── PTY (pseudo-terminal)                  │
└─────────────────────────────────────────────┘
```

## Quick Start

```bash
# Check dependencies
./scripts/check-env.sh

# Build
make build

# Run
./dist/strata
```

## Usage

```bash
# Create session
curl -X POST http://localhost:2280/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001"}'

# Execute command
curl -X POST http://localhost:2280/api/sessions/alice/task-001/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "ls -la"}'

# Interactive shell (WebSocket)
wscat -c 'ws://localhost:2280/api/ws/alice/task-001/shell'
```

## API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/sessions` | Create session |
| DELETE | `/api/sessions/{uid}/{sid}` | Close session |
| POST | `/api/sessions/{uid}/{sid}/exec` | Execute command |
| GET | `/api/stats` | Stats |
| GET | `/api/ws/{uid}/{sid}/shell` | WebSocket |

## MCP

MCP available at `http://localhost:2280/mcp/`

For AI agents:
```bash
npx tsx scripts/strata-mcp.ts
```

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATA_SERVER_ADDR` | `:2280` | Listen address (HOST:PORT combined) |
| `STRATA_SANDBOX_SESSION_ROOT` | `/tmp/strata/sessions` | Session directory |
| `STRATA_SANDBOX_SESSION_TTL` | `30m` | Session timeout |
| `STRATA_SANDBOX_MAX_SESSIONS` | `100` | Max sessions |
| `STRATA_SANDBOX_OVERLAY_DRIVER` | `fuse` | fuse/kernel/none |

View all: `./dist/strata run --help`

## Identity

All API endpoints support two identity modes:

**Path-based** (explicit):
```bash
curl -X POST http://localhost:2280/api/sessions/alice/task-001/exec \
  -d '{"command": "ls"}'
```

**Header-based** (alternative):
```bash
curl -X POST http://localhost:2280/api/sessions/exec \
  -H "X-Owner-Id: alice" \
  -H "X-Session-Id: task-001" \
  -d '{"command": "ls"}'
```

Header priority is higher than path values.

## Dependencies

### Build Dependencies

| Package | Install |
|---------|---------|
| `meson` | `apt install meson` |
| `libcap-dev` | `apt install libcap-dev` |

### Runtime Dependencies

| Package | Install |
|---------|---------|
| `bubblewrap` (bwrap) | Build from [github.com/containers/bubblewrap](https://github.com/containers/bubblewrap) |
| `fuse-overlayfs` | `apt install fuse-overlayfs` |

### Building bubblewrap

```bash
git clone https://github.com/containers/bubblewrap
cd bubblewrap
meson _builddir
meson compile -C _builddir
meson test -C _builddir
sudo meson install -C _builddir
sudo setcap cap_sys_admin+ip /usr/local/bin/bwrap
```

### Runtime Requirements

- Linux kernel ≥ 5.11
- Go ≥ 1.25 (build only)

MIT
