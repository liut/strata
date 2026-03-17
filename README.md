# Strata

> Lightweight Session Sandbox Service — 基于 namespace + overlayfs 的轻量隔离 Shell 环境服务

```
strata v0.1.0 — lightweight session sandbox service
```

## 核心特性

- **轻量隔离**：不依赖 Docker Daemon，使用 Linux Namespace + bubblewrap + fuse-overlayfs
- **用户/会话隔离**：按 user_id + session_id 隔离，每个会话拥有独立的可写层
- **多协议支持**：HTTP REST / WebSocket / gRPC / MCP
- **持久化写入**：overlayfs 层叠机制，修改不影响基础镜像
- **自动回收**：TTL 超时自动清理不活跃会话

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
│  └── PTY (pseudo-terminal)                 │
└─────────────────────────────────────────────┘
```

## 快速开始

### 1. 环境检查

```bash
./scripts/check-env.sh
```

确保以下依赖可用：
- `bubblewrap` (bwrap)
- `fuse-overlayfs`
- `/dev/fuse` 设备

### 2. 运行服务

```bash
# 编译
go build -o bin/strata ./cmd/strata

# 启动（默认配置）
./bin/strata

# 或指定配置文件
./bin/strata -config configs/config.yaml
```

### 3. 使用 API

```bash
# 创建会话
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001"}'

# 执行命令
curl -X POST http://localhost:8080/api/exec \
  -H "Content-Type: application/json" \
  -d '{"user_id": "alice", "session_id": "task-001", "command": "ls -la"}'

# 交互式 Shell（WebSocket）
wscat -c 'ws://localhost:8080/ws/shell?user=alice&session=task-001'
# 输入: {"type": "input", "data": "ls -la\n"}
```

### 4. MCP 集成（AI Agent）

```bash
# 方式一：直接运行（需要 Node.js）
npx tsx mcp/src/server.ts

# 方式二：Docker
docker run -p 8080:8080 strata:latest
```

## API 参考

### HTTP REST

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/sessions` | 创建/复用会话 |
| DELETE | `/api/sessions/{user}/{session}` | 关闭会话 |
| POST | `/api/sessions/{uid}/{sid}/exec` | 执行命令 |
| GET | `/api/stats` | 服务状态 |

### WebSocket

| 路径 | 说明 |
|------|------|
| `/api/ws/{uid}/{sid}/shell` | 交互式 Shell |

消息格式：
- 客户端 → 服务端：`{"type":"input", "data": "ls -la\n"}`
- 服务端 → 客户端：`{"type":"output", "data": "..."}`

### gRPC

参见 `pkg/proto/sandbox/sandbox.proto`

```bash
# 生成 Go 代码
go generate ./...

# 或手动
protoc --go_out=. --go-grpc_out=. proto/sandbox/*.proto
```

## 配置

`configs/config.yaml`:

```yaml
server:
  addr: ":8080"

sandbox:
  base_rootfs: ""                    # 基础只读根（可选）
  session_root: "/tmp/strata/sessions"
  session_ttl: "30m"
  max_sessions: 100
  isolate_network: false
  overlay_driver: "fuse"             # fuse | kernel | none

grpc:
  addr: ":9090"
```

## 可选：制作基础镜像

```bash
# 从 Docker 镜像导出
./scripts/build-base.sh /opt/sandbox/base ubuntu 22.04
```

然后在配置中设置 `base_rootfs: /opt/sandbox/base`

## 隔离机制详解

### 为什么不用 Docker？

Docker 的"重"在于：
- Daemon 常驻
- 完整镜像层管理
- 复杂网络模型

而**隔离本身**（Namespace + cgroups）极轻——一个 bwrap 进程启动只需 ~5ms，内存占用 < 1MB。

### 为什么用 fuse-overlayfs？

- 普通用户可直接挂载（无需 root）
- 语义与内核 overlayfs 完全一致
- rootless Podman 的默认 storage driver

### Session 生命周期

```
创建 → overlay mount → bwrap 启动 → PTY 建立
                ↓
用户执行命令 → 写入 upper 层（不影响 base）
                ↓
关闭 → PTY 关闭 → overlay unmount → 删除 upper
```

## 依赖

- Linux kernel ≥ 5.11（推荐）
- bwrap (bubblewrap)
- fuse-overlayfs
- Go ≥ 1.23

## License

MIT
