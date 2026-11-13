# Scripts

| Script | Description |
|--------|-------------|
| `strata-mcp.ts` | MCP client for AI agents (STDIO) |
| `test-mcp.ts` | MCP HTTP endpoint test (modern by default, legacy opt-in) |
| `check-env.sh` | Check system dependencies |
| `test-api.sh` | HTTP REST API test |
| `test-grpc.sh` | gRPC API test |

## Usage

```bash
# Check environment
./scripts/check-env.sh

# Test HTTP API
./scripts/test-api.sh
STRATA_ADDR=192.168.1.100:9000 ./scripts/test-api.sh

# Test gRPC
./scripts/test-grpc.sh 192.168.1.100:2280

# Test MCP over HTTP (2026-07-28 stateless flow)
npx tsx scripts/test-mcp.ts
STRATA_MCP=http://localhost:2280/mcp/ npx tsx scripts/test-mcp.ts

# Test MCP legacy initialize/session compatibility
STRATA_MCP_LEGACY=1 npx tsx scripts/test-mcp.ts

# MCP client
STRATA_API=http://localhost:2280 npx tsx scripts/strata-mcp.ts
STRATA_UID=alice npx tsx scripts/strata-mcp.ts
```

## Apple Container

Published ports reset the connection unless the `container-runtime-linux` helper
holds the macOS **Local Network** permission (System Settings → Privacy &
Security → Local Network). The container IP works either way:

```bash
IP=$(container list | awk '$1=="strata"{print $6}' | cut -d/ -f1)
STRATA_ADDR=$IP:2280 ./scripts/test-api.sh
STRATA_MCP=http://$IP:2280/mcp/ bun run scripts/test-mcp.ts
```

See [docs/apple-container.md](../docs/apple-container.md) for the permission and
upgrade steps.
