# Strata

> 轻量级会话沙箱 — 基于 Namespace + Overlayfs 的隔离 Shell

[English](./README.md)

## 特性

- **轻量**: Linux Namespace + bubblewrap + fuse-overlayfs
- **隔离**: 按用户+会话隔离，独立可写层
- **多协议**: HTTP REST / WebSocket / gRPC / MCP
- **持久化**: overlayfs 层叠，修改不影响基础镜像
- **自动清理**: TTL 超时自动清理

## 架构

```
┌─────────────────────────────────────────────┐
│  API Layer (HTTP/WS + gRPC + MCP)           │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│  Session Manager                            │
│  - GetOrCreate(user, session)               │
│  - TTL 回收                                  │
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

## 快速开始

```bash
# 检查依赖
./scripts/check-env.sh

# 编译
make build

# 运行
./dist/strata
```

## 使用

```bash
# 创建会话
curl -X POST http://localhost:2280/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001"}'

# 执行命令
curl -X POST http://localhost:2280/api/sessions/alice/task-001/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "ls -la"}'

# 交互式 Shell (WebSocket)
wscat -c 'ws://localhost:2280/api/ws/alice/task-001/shell'
```

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions` | 创建会话 |
| DELETE | `/api/sessions/{uid}/{sid}` | 关闭会话 |
| POST | `/api/sessions/{uid}/{sid}/exec` | 执行命令 |
| GET | `/api/stats` | 状态 |
| GET | `/api/ws/{uid}/{sid}/shell` | WebSocket |

## MCP

MCP 端点: `http://localhost:2280/mcp/`

AI Agent 使用:
```bash
npx tsx scripts/strata-mcp.ts
```

## 配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `STRATA_SERVER_ADDR` | `:2280` | 监听地址 |
| `STRATA_SANDBOX_SESSION_ROOT` | `/tmp/strata/sessions` | 会话目录 |
| `STRATA_SANDBOX_SESSION_TTL` | `30m` | 会话超时 |
| `STRATA_SANDBOX_MAX_SESSIONS` | `100` | 最大会话数 |
| `STRATA_SANDBOX_OVERLAY_DRIVER` | `fuse` | fuse/kernel/none |

查看全部: `./dist/strata run --help`

## 依赖

- Linux kernel ≥ 5.11
- bwrap, fuse-overlayfs
- Go ≥ 1.25 (仅编译)

MIT
