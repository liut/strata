# 使用 Apple Container 运行 Strata

Apple 的 [`container`](https://github.com/apple/container) 会在 Apple Silicon 上通过
轻量 Linux VM 运行 OCI 容器，不依赖 Docker。

这是现有 Linux 实现的一种部署方式：Strata 仍运行在 Linux VM 内，并为每个会话启动
bubblewrap 和 overlayfs。它不是原生 macOS 沙箱后端。

## 环境要求

- Apple Silicon Mac
- macOS 26 或更高版本
- 已安装 Apple `container`
- Apple container 系统服务正在运行

```bash
container --version
container system status
```

如果服务未运行：

```bash
container system start
```

## 构建 arm64 镜像

Apple Silicon 应原生运行 arm64 容器。先构建 arm64 二进制，再构建镜像：

```bash
make dist/linux_arm64/strata
container build --progress plain \
  --arch arm64 \
  -t strata:apple-arm64 \
  -f docker/apple-runtime.Dockerfile .
```

默认基础镜像是 `alpine:3.22`。可以使用镜像站或本地已有镜像覆盖：

```bash
container build --progress plain \
  --arch arm64 \
  --build-arg BASE_IMAGE=docker.1ms.run/library/alpine:latest \
  -t strata:apple-arm64 \
  -f docker/apple-runtime.Dockerfile .
```

Dockerfile 同时会把 Alpine 软件包源替换为 USTC 镜像。

## 启动

Linux VM 内必须具有 `CAP_SYS_ADMIN`。否则 bubblewrap 无法创建会话 namespace：

```text
bwrap: Creating new namespace failed: Operation not permitted
```

启动 Strata：

```bash
container run -d --rm \
  --name strata \
  --cap-add SYS_ADMIN \
  --volume strata-sessions:/tmp/strata/sessions \
  -p 2280:2280 \
  strata:apple-arm64
```

该 capability 只授予容器所在的 Linux VM，不会直接授予 macOS 宿主机进程。

## 访问 API

先尝试发布到宿主机的 loopback 端口：

```bash
curl http://127.0.0.1:2280/api/stats
```

在 Apple container 1.6.0 的实测中，loopback 发布端口可能重置连接。此时可以直接使用
容器 IP：

```bash
container inspect strata
curl http://<container-ip>:2280/api/stats
```

使用该地址运行 API 测试：

```bash
STRATA_ADDR=<container-ip>:2280 ./scripts/test-api.sh
```

也可以从容器内部访问：

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats
```

## 验证隔离

创建并执行会话：

```bash
container exec strata curl -sS \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"owner_id":"alice","session_id":"task-001"}' \
  http://127.0.0.1:2280/api/sessions

container exec strata curl -sS \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"command":"hostname; ps -o pid,ppid,comm"}' \
  http://127.0.0.1:2280/api/sessions/alice/task-001/exec
```

会话内的进程树应以 `bwrap` 作为 PID 1。

排查问题时查看外层进程：

```bash
container exec strata ps -ef
```

输出中应同时存在 `fuse-overlayfs` 和生成的 `bwrap` 命令。

## 清理

```bash
container stop strata
container image delete strata:apple-arm64
container volume delete strata-sessions
make clean
```

删除 volume 会清除保存的会话数据，这一步是可选的。

## 当前限制

- 该模式运行 Linux 命令，不运行 macOS 命令。
- 容器内必须具有 `CAP_SYS_ADMIN`，bubblewrap 才能工作。
- 常规 Docker 运行时镜像面向 `linux/amd64`；Apple Container 使用
  `docker/apple-runtime.Dockerfile` 和 arm64 二进制。
- Apple container 仍在快速发展，自动化环境应固定并验证所用版本。
