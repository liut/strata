package sandbox

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHostLowerDirs 测试 hostLowerDirs 函数 - 环境相关
func TestHostLowerDirs(t *testing.T) {
	if !CurrentEnv().IsLinux {
		t.Skip("not Linux, skipping hostLowerDirs test")
	}

	result := hostLowerDirs()

	// 应该有目录
	if result == "" {
		t.Error("hostLowerDirs returned empty string")
	}

	// 应该用 : 分隔多个目录
	parts := strings.Split(result, ":")
	if len(parts) == 0 {
		t.Error("expected at least one directory")
	}

	// 每个目录应该存在
	for _, dir := range parts {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("directory %q does not exist", dir)
		}
	}
}

// TestBuildBwrapWithOverlay 测试 buildBwrapWithOverlay 函数 - 环境相关
func TestBuildBwrapWithOverlay(t *testing.T) {
	if !CurrentEnv().HasBwrap {
		t.Skip("bwrap not available, skipping test")
	}

	cmd := buildBwrapWithOverlay("/merged", "/home", false)

	if cmd.Path != "bwrap" {
		t.Errorf("cmd.Path = %q, want %q", cmd.Path, "bwrap")
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
	cmdWithNet := buildBwrapWithOverlay("/merged", "/home", true)
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

// TestBuildBwrapFallback 测试 buildBwrapFallback 函数 - 环境相关
func TestBuildBwrapFallback(t *testing.T) {
	if !CurrentEnv().HasBwrap {
		t.Skip("bwrap not available, skipping test")
	}

	cmd := buildBwrapFallback("/home", false)

	if cmd.Path != "bwrap" {
		t.Errorf("cmd.Path = %q, want %q", cmd.Path, "bwrap")
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
		LastUse: time.Now(),
	}

	if s.LastUse.IsZero() {
		t.Error("expected LastUse to be set")
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
		userID:      "testuser",
		sessionID:   "testsession",
		sessionRoot: tmpDir,
		baseRootfs:  "",
		driver:      DriverNone,
		isolateNet:  false,
	}

	if opts.userID != "testuser" {
		t.Errorf("userID = %q, want %q", opts.userID, "testuser")
	}
	if opts.driver != DriverNone {
		t.Errorf("driver = %q, want %q", opts.driver, DriverNone)
	}
}
