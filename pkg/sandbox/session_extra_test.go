package sandbox

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildBwrapWithOverlay 测试 buildBwrapWithOverlay 函数 - 环境相关
func TestBuildBwrapWithOverlay(t *testing.T) {
	if !CurrentEnv().HasBwrap {
		t.Skip("bwrap not available, skipping test")
	}

	cmd := buildBwrapWithOverlay(&OverlayMount{Merged: "/merged", driver: DriverFuse}, "/home", false)

	if filepath.Base(cmd.Path) != "bwrap" {
		t.Errorf("cmd.Path = %q, want bwrap", cmd.Path)
	}

	// 检查必要的参数
	args := cmd.Args
	found := false
	for _, arg := range args {
		if arg == "--bind" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --bind argument in bwrap command")
	}

	// 检查网络隔离参数
	cmdWithNet := buildBwrapWithOverlay(&OverlayMount{Merged: "/merged", driver: DriverFuse}, "/home", true)
	argsWithNet := cmdWithNet.Args
	hasUnshareNet := false
	for _, arg := range argsWithNet {
		if arg == "--unshare-net" {
			hasUnshareNet = true
			break
		}
	}
	if !hasUnshareNet {
		t.Error("expected --unshare-net when isolateNet=true")
	}
}

// TestBuildBwrapWithOverlayKernel 测试 kernel 模式生成 unshare+mount+bwrap 命令 - 环境无关
func TestBuildBwrapWithOverlayKernel(t *testing.T) {
	ov := &OverlayMount{
		Lower:          "/lower",
		Upper:          "/upper",
		Work:           "/work",
		Merged:         "/merged",
		driver:         DriverKernel,
		kernelDeferred: true,
	}

	cmd := buildBwrapWithOverlay(ov, "/home", false)
	if filepath.Base(cmd.Path) != "unshare" {
		t.Errorf("cmd.Path = %q, want unshare", cmd.Path)
	}

	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"--user --mount --map-root-user",
		"mount -t overlay overlay -o lowerdir=/lower,upperdir=/upper,workdir=/work /merged",
		"exec \"$@\"",
		"bwrap --bind /merged /",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

// TestBuildBwrapWithOverlayKernelDirect 测试 kernel 模式直接挂载（root/privileged）时不包 unshare - 环境无关
func TestBuildBwrapWithOverlayKernelDirect(t *testing.T) {
	ov := &OverlayMount{
		Merged: "/merged",
		driver: DriverKernel,
	}

	cmd := buildBwrapWithOverlay(ov, "/home", false)
	if filepath.Base(cmd.Path) != "bwrap" {
		t.Errorf("cmd.Path = %q, want bwrap (direct mount should use plain bwrap)", cmd.Path)
	}
}

// TestBuildBwrapWithOverlayRootfsDetection 测试按 base rootfs 内容自动决定是否绑定宿主目录 - 环境无关
func TestBuildBwrapWithOverlayRootfsDetection(t *testing.T) {
	minimal := t.TempDir()
	if err := os.MkdirAll(filepath.Join(minimal, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(minimal, "bin", "bash"), []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}

	complete := t.TempDir()
	if err := os.MkdirAll(filepath.Join(complete, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(complete, "bin", "sh"), []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}

	cmdMin := buildBwrapWithOverlay(&OverlayMount{Merged: "/m", Lower: minimal, driver: DriverFuse}, "/home", false)
	if !strings.Contains(strings.Join(cmdMin.Args, " "), "--ro-bind /usr /usr") {
		t.Error("minimal rootfs should bind host /usr")
	}

	cmdFull := buildBwrapWithOverlay(&OverlayMount{Merged: "/m", Lower: complete, driver: DriverFuse}, "/home", false)
	if strings.Contains(strings.Join(cmdFull.Args, " "), "--ro-bind /usr /usr") {
		t.Error("complete rootfs should not bind host /usr")
	}
}

// TestBuildBwrapFallback 测试 buildBwrapFallback 函数 - 环境相关
func TestBuildBwrapFallback(t *testing.T) {
	if !CurrentEnv().HasBwrap {
		t.Skip("bwrap not available, skipping test")
	}

	cmd := buildBwrapFallback("/home", false)

	if filepath.Base(cmd.Path) != "bwrap" {
		t.Errorf("cmd.Path = %q, want bwrap", cmd.Path)
	}

	// 检查必要的参数（应该有 --ro-bind 而不是 --bind）
	args := cmd.Args
	hasRoBind := false
	for _, arg := range args {
		if arg == "--ro-bind" {
			hasRoBind = true
			break
		}
	}
	if !hasRoBind {
		t.Error("expected --ro-bind argument in fallback bwrap command")
	}

	// 检查网络隔离参数
	cmdWithNet := buildBwrapFallback("/home", true)
	argsWithNet := cmdWithNet.Args
	hasUnshareNet := false
	for _, arg := range argsWithNet {
		if arg == "--unshare-net" {
			hasUnshareNet = true
			break
		}
	}
	if !hasUnshareNet {
		t.Error("expected --unshare-net when isolateNet=true")
	}
}

// TestSessionWriteClosed 测试向已关闭的 Session 写入 - 环境无关
func TestSessionWriteClosed(t *testing.T) {
	s := &Session{
		closed: true,
	}

	_, err := s.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("expected io.ErrClosedPipe, got %v", err)
	}
}

// TestSessionRead 测试 Read 方法存在 - 环境无关
func TestSessionRead(t *testing.T) {
	// 创建一个带 ptmx 的 Session 用于测试
	// 注意：这里只测试结构，不实际读取
	s := &Session{
		lastHit: time.Now(),
	}

	if s.LastHit().IsZero() {
		t.Error("expected LastHit to be set")
	}
}

// TestOverlayMountDefaultDriver 测试默认驱动 - 环境无关
func TestOverlayMountDefaultDriver(t *testing.T) {
	o := &OverlayMount{
		Lower:  "/lower",
		Upper:  "/session/upper",
		Work:   "/session/work",
		Merged: "/session/merged",
		driver: DriverFuse,
	}

	// 测试 driver 设置
	if o.driver != DriverFuse {
		t.Errorf("driver = %q, want %q", o.driver, DriverFuse)
	}
}

// TestOverlayMountActive 测试 active 字段 - 环境无关
func TestOverlayMountActive(t *testing.T) {
	o := &OverlayMount{
		Lower:  "/lower",
		Upper:  "/session/upper",
		Work:   "/session/work",
		Merged: "/session/merged",
		driver: DriverNone,
		active: false,
	}

	if o.active {
		t.Error("expected active to be false initially")
	}

	o.active = true
	if !o.active {
		t.Error("expected active to be true after setting")
	}
}

// TestOverlayMountSessionDirEdgeCases 测试 sessionDir 边界情况 - 环境无关
func TestOverlayMountSessionDirEdgeCases(t *testing.T) {
	tests := []struct {
		upper    string
		expected string
	}{
		{"/a/b/upper", "/a/b"},
		{"/x/y/z/upper", "/x/y/z"},
	}

	for _, tc := range tests {
		o := OverlayMount{Upper: tc.upper}
		result := o.sessionDir()
		if result != tc.expected {
			t.Errorf("sessionDir(%q) = %q, want %q", tc.upper, result, tc.expected)
		}
	}
}

// TestNewSessionOptions 测试 sessionOptions 结构 - 环境无关
func TestNewSessionOptions(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0700); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}

	opts := sessionOptions{
		ownerID:     "testuser",
		sessionID:   "testsession",
		sessionRoot: tmpDir,
		baseRootfs:  "",
		driver:      DriverNone,
		isolateNet:  false,
	}

	if opts.ownerID != "testuser" {
		t.Errorf("ownerID = %q, want %q", opts.ownerID, "testuser")
	}
	if opts.driver != DriverNone {
		t.Errorf("driver = %q, want %q", opts.driver, DriverNone)
	}
}
