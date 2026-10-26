# Strata

> 轻量级会话沙箱 — 基于 Namespace + Overlayfs 的隔离 Shell

[English](./README.md)

## 特性

- **轻量**: Linux Namespace + bubblewrap + overlayfs (fuse-overlayfs 或内核 overlay)
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
│  bwrap + overlayfs (fuse / kernel / none)   │
│  ├── PID/IPC/UTS Namespace                  │
│  ├── overlay: lower + upper + merged        │
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
./dist/strata run
```

以上命令面向 Linux，因为 bubblewrap 依赖 Linux namespace。
在 Apple Silicon + macOS 26 及以上环境中，可以使用 Apple 的 `container`
在不依赖 Docker 的情况下运行同一套 Linux 栈，详见
[Apple Container 部署指南](./docs/apple-container.zh.md)。

## 使用

```bash
# 创建会话
curl -X POST http://localhost:2280/api/sessions \
  -H "Content-Type: application/json" \
  -d '{"owner_id": "alice", "session_id": "task-001"}'

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
| `STRATA_SERVER_ADDR` | `:2280` | 监听地址 (HOST:PORT 合并) |
| `STRATA_SERVER_SHELL_TIMEOUT` | `30s` | 命令执行默认超时 (HTTP/MCP exec) |
| `STRATA_SANDBOX_SESSION_ROOT` | `/tmp/strata/sessions` | 会话目录 |
| `STRATA_SANDBOX_SESSION_TTL` | `30m` | 会话超时 |
| `STRATA_SANDBOX_MAX_SESSIONS` | `100` | 最大会话数 |
| `STRATA_SANDBOX_OVERLAY_DRIVER` | `fuse` | fuse/kernel/none |

所有配置均通过 `STRATA_*` 环境变量加载，上表列出常用项。

## 身份标识

所有 API 端点支持两种身份模式：

**路径模式**（显式）：
```bash
curl -X POST http://localhost:2280/api/sessions/alice/task-001/exec \
  -d '{"command": "ls"}'
```

**Header 模式**（替代方案）：
```bash
curl -X POST http://localhost:2280/api/sessions/exec \
  -H "X-Owner-Id: alice" \
  -H "X-Session-Id: task-001" \
  -d '{"command": "ls"}'
```

Header 优先级高于路径参数。

## Linux 原生依赖

### 编译依赖

| 包 | 安装命令 |
|----|---------|
| `meson` | `apt install meson` |
| `libcap-dev` | `apt install libcap-dev` |

### 运行时依赖

| 包 | 安装命令 |
|----|---------|
| `bubblewrap` (bwrap) | 从 [github.com/containers/bubblewrap](https://github.com/containers/bubblewrap) 编译安装 |
| `fuse-overlayfs` | `apt install fuse-overlayfs` |

### 编译安装 bubblewrap

```bash
git clone https://github.com/containers/bubblewrap
cd bubblewrap
meson _builddir
meson compile -C _builddir
meson test -C _builddir
sudo meson install -C _builddir
```

> **注意**：不要给 bwrap 设置文件 capabilities（`sudo setcap cap_sys_admin+ip ...`）——
> 现代 bubblewrap 会拒绝这种配置（`Unexpected capabilities but not setuid,
> old file caps config?`）。非 root 使用依赖非特权 user namespace（Linux ≥ 5.11）：
> Debian 需开启 `kernel.unprivileged_userns_clone=1`；Ubuntu 24.04+ 需放开
> `kernel.apparmor_restrict_unprivileged_userns`。

### 运行环境要求

- Linux kernel ≥ 5.11
- 非特权 user namespace（非 root 运行 bwrap 及 `kernel` overlay 驱动所需）
- Go ≥ 1.25 (仅编译)

macOS 请使用 [Apple Container 部署指南](./docs/apple-container.zh.md)。
该方式在轻量 Linux VM 中运行现有实现，不是原生 macOS 沙箱后端。

MIT
