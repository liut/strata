# 脚本工具

本目录包含 Strata 开发与测试相关的辅助脚本。

## 脚本概览

| 脚本 | 说明 |
|------|------|
| `strata-mcp.ts` | MCP 客户端，通过 stdio 与 AI Agent 交互 |
| `test-mcp.ts` | MCP HTTP 端点测试脚本 |
| `check-env.sh` | 检查系统依赖和内核特性 |
| `test-api.sh` | HTTP REST API 测试套件 |
| `test-grpc.sh` | gRPC API 测试脚本 |

## 依赖要求

### Strata 服务端要求
- Linux（推荐内核 ≥ 5.11）
- `bubblewrap` (bwrap)
- `fuse-overlayfs`
- `/dev/fuse` 设备

### 客户端工具要求
- `bash` ≥ 4.0
- `curl`

### 各脚本依赖

#### check-env.sh
- **用途**：验证运行时环境依赖（用于 Strata 服务端）
- **依赖**（仅 Linux）：
  - `bubblewrap` (bwrap)
  - `fuse-overlayfs`
  - `/dev/fuse` 设备
  - `fusermount` (来自 fuse 包)
  - `iproute2` (ip 命令)
  - `util-linux` (unshare)
- **运行方式**：`./scripts/check-env.sh`

#### test-api.sh
- **用途**：测试 HTTP REST API 端点
- **依赖**：
  - 运行中的 Strata 服务
  - `curl`
  - 可选：`websocat` 或 `wscat`（用于 WebSocket 测试）
- **环境变量**：
  - `STRATA_HOST` - 服务主机（默认：localhost）
  - `STRATA_PORT` - 服务端口（默认：2280）
- **运行方式**：
  ```bash
  # 测试默认 localhost:2280
  ./scripts/test-api.sh

  # 指定主机和端口
  STRATA_HOST=192.168.1.100 STRATA_PORT=9000 ./scripts/test-api.sh
  ```

#### test-grpc.sh
- **用途**：测试 gRPC API 端点
- **依赖**：
  - `grpcurl`（安装方式：`go install github.com/fullstorydev/grpcurl/...@latest`）
  - 运行中的 Strata 服务
- **运行方式**：
  ```bash
  # 测试默认 localhost:2280
  ./scripts/test-grpc.sh

  # 指定主机:端口
  ./scripts/test-grpc.sh 192.168.1.100:2280
  ```

#### strata-mcp.ts
- **用途**：MCP（Model Context Protocol）客户端，供 AI Agent 使用
- **协议**：STDIO（通过 stdin/stdout 与 AI Agent 通信）
- **依赖**：
  - Node.js
  - `@modelcontextprotocol/sdk` 包
  - 运行中的 Strata 服务
- **环境变量**：
  - `STRATA_API` - Strata 服务地址（默认：http://localhost:2280）
  - `STRATA_UID` - 默认用户 ID（可选，设置后 user_id 参数变为可选）
- **运行方式**：
  ```bash
  # 安装依赖（如果尚未安装）
  npm install @modelcontextprotocol/sdk

  # 默认配置运行
  npx tsx scripts/strata-mcp.ts

  # 指定固定用户
  STRATA_UID=alice npx tsx scripts/strata-mcp.ts

  # 指定自定义 Strata 服务
  STRATA_API=http://192.168.1.100:2280 npx tsx scripts/strata-mcp.ts
  ```

#### test-mcp.ts
- **用途**：测试 MCP HTTP 端点的脚本
- **协议**：HTTP JSON-RPC
- **依赖**：
  - Bun（或 Node.js + tsx）
  - 运行中的 Strata 服务（含 MCP 端点）
- **环境变量**：
  - `STRATA_MCP` - MCP 服务地址（默认：http://localhost:2280/mcp/）
- **运行方式**：
  ```bash
  # 默认配置运行
  bun run scripts/test-mcp.ts

  # 指定自定义 MCP 服务
  STRATA_MCP=http://192.168.1.100:2280/mcp/ bun run scripts/test-mcp.ts
  ```

## 配置

所有脚本都假设 Strata 服务正在运行。可通过以下方式配置：

- 环境变量（各脚本独立配置）
- 命令行参数（支持的脚本）

## 注意事项

- **Strata 服务端** 必须运行在 Linux 上并具备相应的内核特性
- **客户端工具**（MCP、API 测试、gRPC 测试）可在任何支持所需工具的操作系统上运行
- 所有脚本需在项目根目录运行，或使用正确路径
- 部分检查可能需要 root 权限（如检查 `/dev/fuse`）
- MCP 客户端使用 STDIO 协议 - 适用于 Claude Desktop、Cursor 等集成
