# Scripts

| Script | Description |
|--------|-------------|
| `strata-mcp.ts` | MCP client for AI agents (STDIO) |
| `test-mcp.ts` | MCP HTTP endpoint test |
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

# MCP client
STRATA_API=http://localhost:2280 npx tsx scripts/strata-mcp.ts
STRATA_UID=alice npx tsx scripts/strata-mcp.ts
```
