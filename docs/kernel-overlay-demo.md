# Kernel 模式演示（内核 overlayfs）

strata 的 overlay 驱动 `kernel` 使用内核 overlayfs（而非 fuse-overlayfs），需要 Linux ≥ 5.11
与 `unshare` 命令（依赖检查见 `scripts/check-env.sh`）。

实现上区分两种运行环境（见 `pkg/sandbox/overlay.go`）：

- **root / 具备 CAP_SYS_ADMIN**（例如 privileged 容器）：直接 `mount -t overlay`，
  挂载留在当前 mount namespace，会话 overlay 在 `mount` 表里可见，bwrap 以普通命令启动。
- **非 root / 无 CAP_SYS_ADMIN**：在 `unshare --user --mount --map-root-user` 创建的
  user+mount namespace 内挂载，并与 bwrap 同 namespace 运行；挂载随 bwrap 进程存活，
  进程退出时随 namespace 自动卸载。

## 启用

```bash
STRATA_SANDBOX_OVERLAY_DRIVER=kernel ./dist/strata
```

## 手动演示挂载（非 root 路径）

```bash
# 准备目录
mkdir -p /tmp/kernel_overlay/lower /tmp/kernel_overlay/upper /tmp/kernel_overlay/work /tmp/kernel_overlay/merged
echo hello > /tmp/kernel_overlay/lower/hello.txt

# 在 user+mount namespace 内挂载（--map-root-user 把当前用户映射为 namespace 内 root）
unshare --user --mount --map-root-user sh -c '
  mount -t overlay overlay \
    -o lowerdir=/tmp/kernel_overlay/lower,upperdir=/tmp/kernel_overlay/upper,workdir=/tmp/kernel_overlay/work \
    /tmp/kernel_overlay/merged
  echo "--- merged ---"
  ls -la /tmp/kernel_overlay/merged
  echo hello > /tmp/kernel_overlay/merged/hello.txt   # 写入进入 upper
  echo "--- upper ---"
  ls -la /tmp/kernel_overlay/upper
  umount /tmp/kernel_overlay/merged
'
```

注意：unshare 的 mount namespace 随进程退出销毁，挂载不会保留到外层。
端到端测试需把挂载和 bwrap 放在同一个 unshare 会话里：

```bash
unshare --user --mount --map-root-user bash -c '
  mount -t overlay overlay \
    -o lowerdir=/tmp/kernel_overlay/lower,upperdir=/tmp/kernel_overlay/upper,workdir=/tmp/kernel_overlay/work \
    /tmp/kernel_overlay/merged
  bwrap --bind /tmp/kernel_overlay/merged / \
    --ro-bind /bin /bin --ro-bind /lib /lib --ro-bind /sbin /sbin --ro-bind /usr /usr --ro-bind /lib64 /lib64 \
    --proc /proc --dev /dev --tmpfs /tmp --tmpfs /run \
    --bind /tmp/kernel_overlay/upper /root --chdir /root \
    --unshare-pid --unshare-ipc --unshare-uts \
    --hostname strata --die-with-parent \
    --setenv HOME /root --setenv USER root --setenv TERM xterm-256color \
    --setenv PATH /usr/local/bin:/usr/bin:/bin:/sbin \
    -- bash
  umount /tmp/kernel_overlay/merged
'

# 清理
rm -rf /tmp/kernel_overlay
```
