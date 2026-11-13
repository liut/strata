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

macOS 26 及更高版本还会用「本地网络」隐私权限限制宿主机侧的发布端口。Apple
`container` 自带的 `container-runtime-linux` helper 需要该权限，否则发布端口会重置
连接。详见[发布端口重置连接](#发布端口重置连接)。

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

如果连接被重置，按[发布端口重置连接](#发布端口重置连接)排查。也可以直接使用容器
IP 作为兜底：

```bash
container inspect strata
curl http://<container-ip>:2280/api/stats
```

使用该地址运行 API 测试：

```bash
STRATA_ADDR=<container-ip>:2280 ./scripts/test-api.sh
```

MCP 测试同样使用该地址：

```bash
STRATA_MCP=http://<container-ip>:2280/mcp/ bun run scripts/test-mcp.ts
```

也可以从容器内部访问：

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats
```

## 发布端口重置连接

发布端口可能接受 TCP 连接后又立刻重置：

```text
curl http://127.0.0.1:2280/api/stats
curl: (56) Recv failure: Connection reset by peer
```

这不是 Strata 的问题。所有容器的所有发布端口都会受影响，而容器本身是健康的：

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats   # 正常
curl http://<container-ip>:2280/api/stats                        # 正常
```

### 授予「本地网络」权限

macOS 26 及更高版本会把宿主机侧的发布端口置于「本地网络」隐私权限管控之下：

1. 打开 系统设置 → 隐私与安全性 → 本地网络
2. 启用 `container-runtime-linux`
3. 重新创建容器，再试访问发布端口

这是上游 issue 中确认过的解法
（[apple/container#919](https://github.com/apple/container/issues/919)、
[#2029](https://github.com/apple/container/issues/2029)、
[#1180](https://github.com/apple/container/issues/1180)）。

已在 macOS 26.7.1 + Apple container 1.6.0 上验证：授予权限并重建容器后，所有发布
端口恢复正常，全程没有升级 `container`。

### 使用容器 IP

```bash
container list                                  # IP 在第 6 列
IP=$(container list | awk '$1=="strata"{print $6}' | cut -d/ -f1)
curl http://$IP:2280/api/stats
```

容器重建后该地址会变化。

### 升级 Apple container

上面的重置不需要靠升级解决——只授予「本地网络」权限即可。需要其他修复时再升级。
Apple `container` 通过签名的
`container-<version>-installer-signed.pkg` 安装，并记录 receipt
`com.apple.container-installer`。同一个包还自带官方升级脚本
`/usr/local/bin/update-container.sh`，旁边是
`/usr/local/bin/uninstall-container.sh`。

只要还有任何 `com.apple.container*` 的 launchd 任务注册着，升级脚本就会拒绝执行，
所以要先停掉正在运行的容器：

```bash
container stop strata
container system stop
launchctl list | grep 'com\.apple\.container'   # 必须为空

sudo /usr/local/bin/update-container.sh           # 升到最新版
sudo /usr/local/bin/update-container.sh -v 1.7.1  # 指定版本

container system start
```

升级脚本从 GitHub releases 下载签名包，用 `sudo installer` 重新安装到 `/usr/local`。
容器不会在升级后保留，需要用原来的 `container run` 命令重新创建 `strata`。

只有在该架构没有配置默认 kernel 时，`container system start` 才会询问是否安装。
脚本里可以用 `--enable-kernel-install` 直接回答「是」，避免交互。

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
- 发布端口是否可用取决于 `container-runtime-linux` helper 的 macOS「本地网络」权限，
  请保留容器 IP 作为兜底。
