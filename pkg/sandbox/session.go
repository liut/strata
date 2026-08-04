package sandbox

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// 将一些常规字符串转为安全的文件系统路径片段
var keyReplacer = strings.NewReplacer(
	"/", "_", ":", "_", " ", "_",
	"..", "__", "~", "_", "@", "_",
)

// Session 表示一个用户隔离的 Shell 会话
type Session struct {
	id      string
	owner   string
	created time.Time
	lastHit time.Time

	overlay *OverlayMount
	ptmx    *os.File   // PTY 主端（服务侧）
	cmd     *exec.Cmd  // bwrap 进程
	waitCh  chan error // 进程 Wait 结果通道（由 startShell 的探测 goroutine 填充）

	// 保存重启 bwrap 所需的信息
	sessionRoot string
	driver      OverlayDriver
	isolateNet  bool

	mu     sync.Mutex
	closed bool
	done   chan struct{} // 关闭时关闭此 channel
}

// ID 返回 session ID
func (s *Session) ID() string {
	return s.id
}

// UID 返回 owner ID
func (s *Session) UID() string {
	return s.owner
}

// Created 返回会话创建时间
func (s *Session) Created() time.Time {
	return s.created
}

// LastHit 返回最后活动时间
func (s *Session) LastHit() time.Time {
	return s.lastHit
}

// Done 返回一个 channel，会话关闭时关闭
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Write 向 Shell 写入输入数据（用户键盘/指令）
func (s *Session) Write(data []byte) (int, error) {
	slog.Debug("writing", "data", len(data))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	s.lastHit = time.Now()
	return s.ptmx.Write(data)
}

// Read 从 Shell 读取输出数据
func (s *Session) Read(buf []byte) (int, error) {
	n, err := s.ptmx.Read(buf)
	if n > 0 {
		s.mu.Lock()
		s.lastHit = time.Now()
		s.mu.Unlock()
	}
	return n, err
}

// Resize 调整 PTY 终端尺寸
func (s *Session) Resize(rows, cols uint16) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// Close 关闭 session，清理 overlay 和 PTY
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	if s.overlay != nil {
		// 等进程真正退出后再卸载，避免 busy mount
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = s.overlay.Cleanup()
		}()
	}
	close(s.done)
	slog.Info("session close", "sid", s.id)
}

// IsClosed 返回 session 是否已关闭
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// IsBwrapAlive 检测 bwrap 进程是否还在运行
func (s *Session) IsBwrapAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	// 检查进程是否已退出
	if s.cmd.ProcessState != nil && s.cmd.ProcessState.Exited() {
		return false
	}
	return true
}

// RestartBwrap 重新启动 bwrap 进程（当 bwrap 退出但 overlay 还在时）
func (s *Session) RestartBwrap() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session already closed")
	}

	// 检查 overlay 是否还有效
	if s.overlay != nil && s.overlay.active {
		slog.Debug("RestartBwrap: overlay still active, reusing it")
	} else if s.overlay != nil {
		// overlay 不再有在，尝试重新挂载
		slog.Debug("RestartBwrap: overlay not active, recreating")
		if err := s.overlay.Mount(); err != nil {
			return fmt.Errorf("failed to remount overlay: %w", err)
		}
	} else {
		return fmt.Errorf("no overlay to restart")
	}

	// 重新创建 bwrap 命令
	var cmd *exec.Cmd
	homeDir := filepath.Join(s.sessionRoot, s.owner, s.id, "home")

	if s.overlay.active && s.overlay.Merged != "" {
		cmd = buildBwrapWithOverlay(s.overlay, homeDir, s.isolateNet)
	} else {
		cmd = buildBwrapFallback(homeDir, s.isolateNet)
	}

	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf

	slog.Debug("RestartBwrap: starting bwrap", "args", cmd.Args)

	// 启动新的 bwrap 进程并做存活探测
	ptmx, alive, waitCh, err := startShell(cmd, stderrBuf)
	if err != nil {
		return fmt.Errorf("failed to start bwrap: %w", err)
	}
	if !alive {
		_ = ptmx.Close()
		return fmt.Errorf("bwrap exited immediately: %s", stderrBuf.String())
	}

	// 关闭旧的 ptmx
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}

	// 更新 session
	s.ptmx = ptmx
	s.cmd = cmd
	s.waitCh = waitCh
	s.lastHit = time.Now()

	// 启动新的 waitExit goroutine
	go s.waitExit()

	slog.Info("bwrap restarted", "session", s.id, "pid", cmd.Process.Pid)
	return nil
}

// waitExit 监听 bwrap 进程退出，但不清理 overlay（overlay 独立于 bwrap 进程生命周期）
// 这样即使 bwrap 退出，只要 overlay 还在，session 就可以复用
func (s *Session) waitExit() {
	var err error
	if s.waitCh != nil {
		// 由 startShell 的探测 goroutine 执行 Wait，这里只接收结果
		err = <-s.waitCh
	} else {
		err = s.cmd.Wait()
	}

	// 读取 stderr
	stderr := ""
	if s.cmd.Stderr != nil {
		if buf, ok := s.cmd.Stderr.(*bytes.Buffer); ok {
			stderr = buf.String()
		}
	}

	// 尝试获取更多退出信息
	exitCode := 0
	if s.cmd.ProcessState != nil {
		exitCode = s.cmd.ProcessState.ExitCode()
	}

	// kernel 延迟挂载模式下，overlay 挂在 bwrap 的 namespace 内，进程退出即随 namespace 消失
	overlayStatus := "still mounted"
	if s.overlay != nil && s.overlay.kernelDeferred {
		overlayStatus = "unmounted with namespace"
	}
	slog.Info("bwrap exited", "session", s.id, "error", err,
		"exitCode", exitCode, "stderr", stderr, "overlay", overlayStatus)

	// 不调用 Close，让 overlay 继续存在
	// session 会在 Manager 中保持，直到明确被删除或者 overlay 失败
}

// ────────────────────────────────────────────────────────────
// Session 构建
// ────────────────────────────────────────────────────────────

type sessionOptions struct {
	ownerID     string
	sessionID   string
	sessionRoot string
	baseRootfs  string // Manager 初始化好的 base rootfs
	driver      OverlayDriver
	isolateNet  bool
}

// startShell 启动 shell 命令并探测 100ms 内是否立即退出（如 kernel 模式 overlay 挂载失败）。
// 返回 PTY、进程是否存活，以及 Wait 结果通道（由 waitExit 消费；存活进程退出后探测
// goroutine 会把 Wait 结果写入该通道）。
func startShell(cmd *exec.Cmd, stderr *bytes.Buffer) (ptmx *os.File, alive bool, waitCh chan error, err error) {
	cmd.Stderr = stderr
	ptmx, err = pty.Start(cmd)
	if err != nil {
		return nil, false, nil, err
	}

	waitCh = make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-waitCh:
		// 100ms 内退出：进程已由探测 goroutine 回收，ProcessState 可用
		return ptmx, false, waitCh, nil
	case <-time.After(100 * time.Millisecond):
		return ptmx, true, waitCh, nil
	}
}

func newSession(opts sessionOptions) (*Session, error) {
	opts.ownerID = keyReplacer.Replace(opts.ownerID)
	opts.sessionID = keyReplacer.Replace(opts.sessionID)
	sessionDir := filepath.Join(opts.sessionRoot, opts.ownerID, opts.sessionID)
	homeDir := filepath.Join(sessionDir, "home")

	if err := os.MkdirAll(homeDir, 0700); err != nil {
		return nil, fmt.Errorf("strata/session: mkdir home: %w", err)
	}

	// 使用 Manager 初始化好的 base rootfs 作为 lower
	lower := opts.baseRootfs
	if lower == "" {
		return nil, fmt.Errorf("baseRootfs is not initialized")
	}

	overlay := &OverlayMount{
		Lower:  lower,
		Upper:  filepath.Join(sessionDir, "upper"),
		Work:   filepath.Join(sessionDir, "work"),
		Merged: filepath.Join(sessionDir, "merged"),
		driver: opts.driver,
	}

	var cmd *exec.Cmd

	if opts.driver == DriverNone {
		// 降级：不挂 overlay，用 bwrap 直接 bind 宿主只读目录 + tmpfs home
		cmd = buildBwrapFallback(homeDir, opts.isolateNet)
	} else {
		if err := overlay.Mount(); err != nil {
			// overlay 挂载失败 → 自动降级
			slog.Warn("overlay mount failed, falling back to tmpfs mode", "error", err)
			cmd = buildBwrapFallback(homeDir, opts.isolateNet)
		} else {
			cmd = buildBwrapWithOverlay(overlay, homeDir, opts.isolateNet)
		}
	}

	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf

	slog.Debug("starting bwrap", "driver", opts.driver, "args", cmd.Args, "dir", cmd.Dir)

	ptmx, alive, waitCh, err := startShell(cmd, stderrBuf)
	if err != nil {
		slog.Error("pty.Start failed", "error", err, "cmd", cmd.Args)
		_ = overlay.Cleanup()
		return nil, fmt.Errorf("strata/session: pty start: %w", err)
	}
	slog.Debug("pty.Start succeeded", "pid", cmd.Process.Pid)

	// 探测 100ms 内是否立即退出（如 kernel 模式 overlay 挂载失败），
	// 立即退出则依次降级：overlay → bwrap fallback(tmpfs) → unshare fallback
	if !alive {
		slog.Warn("shell exited immediately, falling back to tmpfs mode",
			"exitCode", cmd.ProcessState.ExitCode(),
			"stderr", stderrBuf.String())
		_ = ptmx.Close()
		_ = overlay.Umount() // 只卸载不清理目录，fallback 还要复用 home

		// 尝试用不带 overlay 的 bwrap fallback
		cmd = buildBwrapFallback(homeDir, opts.isolateNet)
		stderrBuf = &bytes.Buffer{}
		cmd.Stderr = stderrBuf
		slog.Debug("starting bwrap fallback", "args", cmd.Args)
		ptmx, alive, waitCh, err = startShell(cmd, stderrBuf)
		if err != nil {
			slog.Error("bwrap fallback pty.Start failed", "error", err)
			_ = overlay.Cleanup()
			return nil, fmt.Errorf("strata/session: bwrap fallback pty start: %w", err)
		}
		if !alive {
			slog.Warn("bwrap fallback also exited immediately, trying unshare fallback",
				"exitCode", cmd.ProcessState.ExitCode(),
				"stderr", stderrBuf.String())
			_ = ptmx.Close()

			// 尝试用 unshare 代替 bwrap
			cmd = buildUnshareFallback(homeDir, opts.isolateNet)
			stderrBuf = &bytes.Buffer{}
			cmd.Stderr = stderrBuf
			ptmx, alive, waitCh, err = startShell(cmd, stderrBuf)
			if err != nil {
				slog.Error("unshare fallback pty.Start failed", "error", err)
				_ = overlay.Cleanup()
				return nil, fmt.Errorf("strata/session: unshare fallback pty start: %w", err)
			}
			if !alive {
				slog.Error("unshare fallback also failed",
					"exitCode", cmd.ProcessState.ExitCode(),
					"stderr", stderrBuf.String())
				_ = ptmx.Close()
				_ = overlay.Cleanup()
				return nil, fmt.Errorf("strata/session: unshare fallback exited with code %d", cmd.ProcessState.ExitCode())
			}
		}
	}

	slog.Info("shell process started", "pid", cmd.Process.Pid, "driver", opts.driver,
		"owner", opts.ownerID, "sess", opts.sessionID)

	s := &Session{
		id:          opts.sessionID,
		owner:       opts.ownerID,
		created:     time.Now(),
		lastHit:     time.Now(),
		overlay:     overlay,
		ptmx:        ptmx,
		cmd:         cmd,
		sessionRoot: opts.sessionRoot,
		driver:      opts.driver,
		isolateNet:  opts.isolateNet,
		waitCh:      waitCh,
		done:        make(chan struct{}),
	}

	go s.waitExit()
	return s, nil
}

// buildBwrapWithOverlay 以完整 overlay merged 目录作为新根
//
// 自动探测 base rootfs：若其为最小 rootfs（仅自动生成的 bash + 依赖库），把宿主/容器的
// /bin /lib /sbin /usr 只读绑定到 merged 之上以提供用户空间；若为完整文件系统则不绑定，
// 获得完整隔离。
func buildBwrapWithOverlay(ov *OverlayMount, homeDir string, isolateNet bool) *exec.Cmd {
	bash := findBash()

	// 按用户测试成功的顺序：先绑定系统目录，再绑定 overlay
	// 注意：--ro-bind /bin /bin 等需要在 --bind merged / 之前
	// bwrap --ro-bind /bin /bin --ro-bind /lib /lib --ro-bind /lib64 /lib64
	//       --bind merged / --proc /proc --dev /dev --tmpfs /tmp --tmpfs /run
	//       --bind homeDir /root --ro-bind /etc/resolv.conf /etc/resolv.conf
	//       --unshare-pid --unshare-ipc --unshare-uts [--unshare-net]
	//       --hostname strata --die-with-parent
	//       --setenv HOME /root --setenv USER root --setenv TERM xterm-256color --setenv PATH ...
	//       -- bash
	// overlay 根目录（放在最前面，后续的 ro-bind 会补充系统目录）
	args := []string{
		"--bind", ov.Merged, "/",
	}

	// 系统目录（在 merged 绑定之后，可以补充/覆盖 merged 中的目录）
	if !rootfsHasUserspace(ov.Lower) {
		args = append(args,
			"--ro-bind", "/bin", "/bin",
			"--ro-bind", "/lib", "/lib",
			"--ro-bind", "/sbin", "/sbin",
			"--ro-bind", "/usr", "/usr",
		)

		// /lib64 可能在某些发行版不存在（如 Alpine）
		if pathExists("/lib64") {
			args = append(args, "--ro-bind", "/lib64", "/lib64")
		}
	}

	// proc/dev/tmpfs/home、网络配置
	args = append(args,
		"--proc", "/proc", "--dev", "/dev",
		"--tmpfs", "/tmp", "--tmpfs", "/run",
		"--bind", homeDir, "/root", "--chdir", "/root",
		"--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf",
	)

	// namespace、网络隔离、环境变量、命令
	if isolateNet {
		args = append(args, "--unshare-net")
	}
	args = append(args,
		"--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--hostname", "strata",
		"--die-with-parent",
		"--setenv", "HOME", "/root",
		"--setenv", "USER", "root",
		"--setenv", "TERM", "xterm-256color",
		"--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin:/sbin",
		"--", bash,
	)

	if ov.driver == DriverKernel && ov.kernelDeferred {
		// kernel 模式：overlay 挂载必须与 bwrap 处于同一个 user+mount namespace 才会存活。
		// 在 unshare 出来的 namespace 里先执行挂载，再 exec bwrap；进程 exec 后
		// namespace 由 bwrap 继承，挂载在整个会话期间持续有效。
		// 只有非 root/无 CAP_SYS_ADMIN（mountKernel 未直接挂载成功）时才需要此包装。
		script := ov.kernelMountScript() + ` && exec "$@"`
		args = append([]string{
			"--user", "--mount", "--map-root-user",
			"sh", "-c", script, "sh", "bwrap",
		}, args...)
		return exec.Command("unshare", args...)
	}

	return exec.Command("bwrap", args...)
}

// buildBwrapFallback 不使用 overlay，只读 bind 宿主目录 + tmpfs home
func buildBwrapFallback(homeDir string, isolateNet bool) *exec.Cmd {
	bash := findBash()

	// fallback 模式：不用 overlay，直接用 bwrap 绑定目录
	// 注意：--ro-bind /bin /lib 等需要在 --bind 之前，这样 pivot_root 后还能访问
	args := []string{
		// 系统目录（这些绑定需要在 pivot_root 之前）
		// 只绑定实际存在的目录（如 Alpine 没有 /lib64）
		"--ro-bind", "/bin", "/bin",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/sbin", "/sbin",
		"--ro-bind", "/usr", "/usr",
	}

	// /lib64 可能在某些发行版不存在（如 Alpine）
	if pathExists("/lib64") {
		args = append(args, "--ro-bind", "/lib64", "/lib64")
	}

	// home 目录、proc dev tmpfs、网络配置
	args = append(args,
		"--bind", homeDir, "/root", "--chdir", "/root",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--tmpfs", "/run",
		"--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf",
	)

	// namespace、网络隔离、环境变量、命令
	if isolateNet {
		args = append(args, "--unshare-net")
	}
	args = append(args,
		"--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--hostname", "strata",
		"--die-with-parent",
		"--setenv", "HOME", "/root",
		"--setenv", "USER", "root",
		"--setenv", "TERM", "xterm-256color",
		"--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin:/sbin",
		"--", bash,
	)

	return exec.Command("bwrap", args...)
}

// buildUnshareFallback 使用 unshare 代替 bwrap（用于 bwrap 不工作的环境）
func buildUnshareFallback(homeDir string, isolateNet bool) *exec.Cmd {
	// 构建一个脚本，在 unshare 环境中设置挂载并启动 shell
	// 使用 mount --bind 来手动挂载目录
	shell := findBash()
	script := fmt.Sprintf(`
		# 挂载 /usr
		mount --bind /usr /usr 2>/dev/null || true
		mount -o remount,ro /usr 2>/dev/null || true

		# 挂载 /etc (只读)
		mount --bind /etc /etc 2>/dev/null || true
		mount -o remount,ro /etc 2>/dev/null || true

		# 挂载 resolv.conf, passwd, group
		mount --bind /etc/resolv.conf /etc/resolv.conf 2>/dev/null || true
		mount --bind /etc/passwd /etc/passwd 2>/dev/null || true
		mount --bind /etc/group /etc/group 2>/dev/null || true

		# 挂载 home 目录
		mount --bind %s /root 2>/dev/null || true

		# 设置 hostname
		hostname strata 2>/dev/null || true

		# 启动 shell
		exec %s --login
	`, homeDir, shell)

	// 使用 unshare 创建 user, pid, ipc, uts namespace
	// 然后在子进程中运行上面的脚本
	args := []string{
		"--user",
		"--map-root-user",
		"--mount",
		"--pid",
		"--ipc",
		"--uts",
		"--fork",
		"--propagation", "private",
		"sh", "-c", script,
	}

	if isolateNet {
		args = append([]string{"--net"}, args...)
	}

	return exec.Command("unshare", args...)
}

// findBash 查找 bash 的实际路径
// 优先在系统常见路径中查找，如果都不存在则 fallback 到 sh
func findBash() string {
	// 常见 bash 路径，按优先级尝试
	// 注意：在某些环境下 /usr/bin/bash 存在但 bwrap 中不可用，优先使用 /bin
	paths := []string{"/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 最后 fallback 到 sh（几乎所有系统都有）
	return "/bin/sh"
}

// pathExists 检查路径是否存在
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// rootfsHasUserspace 判断 base rootfs 是否为完整用户空间（而非仅自动生成的 bash + 依赖库）。
// 完整 rootfs 至少应包含 /bin/sh、/usr/bin/env、/bin/ls 之一；最小 rootfs 只有 /bin/bash。
func rootfsHasUserspace(rootfs string) bool {
	for _, rel := range []string{"bin/sh", "usr/bin/env", "bin/ls"} {
		if pathExists(filepath.Join(rootfs, rel)) {
			return true
		}
	}
	return false
}

// ensureBashInRootfs 确保 baseRootfs 中包含 /bin/bash
func ensureBashInRootfs(baseRootfs string) error {
	// 找到宿主机的 bash
	bashPath := findBash()
	if bashPath == "" {
		return fmt.Errorf("no bash found on host")
	}

	// 检查是否已存在（避免重复复制）
	destBash := filepath.Join(baseRootfs, "bin", "bash")
	if _, err := os.Stat(destBash); err == nil {
		slog.Debug("bash already exists in baseRootfs", "path", destBash)
		return nil
	}

	// 创建目标目录
	binDir := filepath.Join(baseRootfs, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("mkdir bin dir: %w", err)
	}

	// 复制 bash
	if err := copyFile(bashPath, destBash); err != nil {
		return fmt.Errorf("copy bash: %w", err)
	}
	if err := os.Chmod(destBash, 0755); err != nil {
		return fmt.Errorf("chmod bash: %w", err)
	}

	// 复制 bash 依赖的库（通过 ldd 获取）
	if err := copyBashDeps(bashPath, baseRootfs); err != nil {
		slog.Warn("failed to copy bash deps, bash may still work", "error", err)
		return err
	}

	slog.Info("ensured bash in baseRootfs", "bash", destBash)
	return nil
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()

	to, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = to.Close() }()

	_, err = io.Copy(to, from)
	return err
}

// copyBashDeps 使用 ldd 找到 bash 依赖的库并复制到目标 rootfs
func copyBashDeps(bashPath, baseRootfs string) error {
	// 使用 ldd 获取依赖库列表
	out, err := exec.Command("ldd", bashPath).CombinedOutput()
	if err != nil {
		// ldd 可能失败（比如静态链接），直接返回
		return nil
	}

	// 解析 ldd 输出，格式如：
	// linux-vdso.so.1 (0x00007fff...)
	// libtinfo.so.6 => /lib/x86_64-linux-gnu/libtinfo.so.6 (0x...)
	// libdl.so.2 => /lib64/ld-linux-x86-64.so.2 (0x...)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 跳过 "linux-vdso.so.1" 这种虚拟库
		if strings.Contains(line, "=>") {
			// 提取库路径
			parts := strings.Split(line, "=>")
			if len(parts) < 2 {
				continue
			}
			libPath := strings.TrimSpace(parts[1])
			// 去除 "(0x...)" 部分
			if idx := strings.Index(libPath, "("); idx > 0 {
				libPath = strings.TrimSpace(libPath[:idx])
			}

			// 只处理绝对路径
			if !strings.HasPrefix(libPath, "/") {
				continue
			}

			// 复制库文件
			if err := copyLibToRootfs(libPath, baseRootfs); err != nil {
				slog.Debug("failed to copy lib", "lib", libPath, "error", err)
			}
		}
	}

	return nil
}

// copyLibToRootfs 复制库文件到 rootfs 的对应目录
func copyLibToRootfs(libPath, baseRootfs string) error {
	// 获取库文件名，如 /lib/x86_64-linux-gnu/libtinfo.so.6 -> /lib/x86_64-linux-gnu/
	dir := filepath.Dir(libPath)
	relDir, err := filepath.Rel("/", dir)
	if err != nil {
		return err
	}

	// 目标目录
	targetDir := filepath.Join(baseRootfs, relDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", targetDir, err)
	}

	// 复制文件
	targetPath := filepath.Join(targetDir, filepath.Base(libPath))
	return copyFile(libPath, targetPath)
}
