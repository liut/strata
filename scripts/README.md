# Scripts

This directory contains utility scripts for Strata development and testing.

## Overview

| Script | Description |
|--------|-------------|
| `strata-mcp.ts` | MCP client for AI agents to interact with Strata via stdio |
| `check-env.sh` | Verify system dependencies and kernel features |
| `test-api.sh` | HTTP REST API test suite |
| `test-grpc.sh` | gRPC API test script |

## Dependencies

### Strata Server Requirements
- Linux (kernel ≥ 5.11 recommended)
- `bubblewrap` (bwrap)
- `fuse-overlayfs`
- `/dev/fuse` device

### Client Tools Requirements
- `bash` ≥ 4.0
- `curl`

### Individual Scripts

#### check-env.sh
- **Purpose**: Verify runtime environment dependencies (for Strata server)
- **Requirements** (Linux only):
  - `bubblewrap` (bwrap)
  - `fuse-overlayfs`
  - `/dev/fuse` device
  - `fusermount` (from fuse package)
  - `iproute2` (ip command)
  - `util-linux` (unshare)
- **Usage**: `./scripts/check-env.sh`

#### test-api.sh
- **Purpose**: Test HTTP REST API endpoints
- **Requirements**:
  - Running Strata server
  - `curl`
  - Optional: `websocat` or `wscat` for WebSocket testing
- **Environment Variables**:
  - `STRATA_HOST` - Server host (default: localhost)
  - `STRATA_PORT` - Server port (default: 2280)
- **Usage**:
  ```bash
  # Test against default localhost:2280
  ./scripts/test-api.sh

  # Test against custom host/port
  STRATA_HOST=192.168.1.100 STRATA_PORT=9000 ./scripts/test-api.sh
  ```

#### test-grpc.sh
- **Purpose**: Test gRPC API endpoints
- **Requirements**:
  - `grpcurl` (install via: `go install github.com/fullstorydev/grpcurl/...@latest`)
  - Running Strata server
- **Usage**:
  ```bash
  # Test against default localhost:2280
  ./scripts/test-grpc.sh

  # Test against custom host:port
  ./scripts/test-grpc.sh 192.168.1.100:2280
  ```

#### strata-mcp.ts
- **Purpose**: MCP (Model Context Protocol) client for AI agents
- **Protocol**: STDIO (communicates with AI agent via stdin/stdout)
- **Requirements**:
  - Node.js
  - `@modelcontextprotocol/sdk` package
  - Running Strata server
- **Environment Variables**:
  - `STRATA_API` - Strata server URL (default: http://localhost:2280)
  - `STRATA_UID` - Default user ID (optional, makes user_id param optional)
- **Usage**:
  ```bash
  # Install dependencies (if not already)
  npm install @modelcontextprotocol/sdk

  # Run with default settings
  npx tsx scripts/strata-mcp.ts

  # Run with fixed user ID
  STRATA_UID=alice npx tsx scripts/strata-mcp.ts

  # Run with custom Strata server
  STRATA_API=http://192.168.1.100:2280 npx tsx scripts/strata-mcp.ts
  ```

## Configuration

All scripts assume the Strata server is running. Configure via:

- Environment variables (per-script)
- Command-line arguments (where supported)

## Notes

- **Strata server** must run on Linux with proper kernel features
- **Client tools** (MCP, API tests, gRPC tests) can run on any OS that supports the required tools
- All scripts must be run from the project root or with correct path
- Some scripts require root privileges for certain checks (e.g., checking `/dev/fuse`)
- MCP client uses STDIO protocol - suitable for integration with Claude Desktop, Cursor, etc.
