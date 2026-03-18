package config

import (
	"os"
	"testing"
	"time"
)

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.Server.Addr != ":2280" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":2280")
	}
	if cfg.Sandbox.SessionRoot != "/tmp/strata/sessions" {
		t.Errorf("SessionRoot = %q, want %q", cfg.Sandbox.SessionRoot, "/tmp/strata/sessions")
	}
	if cfg.Sandbox.SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL = %v, want %v", cfg.Sandbox.SessionTTL, 30*time.Minute)
	}
	if cfg.Sandbox.MaxSessions != 100 {
		t.Errorf("MaxSessions = %d, want %d", cfg.Sandbox.MaxSessions, 100)
	}
	if cfg.Sandbox.OverlayDriver != "fuse" {
		t.Errorf("OverlayDriver = %q, want %q", cfg.Sandbox.OverlayDriver, "fuse")
	}
}

// TestLoadEnvVars 测试从环境变量加载配置
func TestLoadEnvVars(t *testing.T) {
	// 设置环境变量
	os.Setenv("STRATA_SERVER_ADDR", ":9090")
	os.Setenv("STRATA_SANDBOX_SESSION_ROOT", "/custom/sessions")
	os.Setenv("STRATA_SANDBOX_SESSION_TTL", "60m")
	os.Setenv("STRATA_SANDBOX_MAX_SESSIONS", "50")
	os.Setenv("STRATA_SANDBOX_OVERLAY_DRIVER", "none")
	defer func() {
		os.Unsetenv("STRATA_SERVER_ADDR")
		os.Unsetenv("STRATA_SANDBOX_SESSION_ROOT")
		os.Unsetenv("STRATA_SANDBOX_SESSION_TTL")
		os.Unsetenv("STRATA_SANDBOX_MAX_SESSIONS")
		os.Unsetenv("STRATA_SANDBOX_OVERLAY_DRIVER")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":9090")
	}
	if cfg.Sandbox.SessionRoot != "/custom/sessions" {
		t.Errorf("SessionRoot = %q, want %q", cfg.Sandbox.SessionRoot, "/custom/sessions")
	}
	if cfg.Sandbox.SessionTTL != 60*time.Minute {
		t.Errorf("SessionTTL = %v, want %v", cfg.Sandbox.SessionTTL, 60*time.Minute)
	}
	if cfg.Sandbox.MaxSessions != 50 {
		t.Errorf("MaxSessions = %d, want %d", cfg.Sandbox.MaxSessions, 50)
	}
	if cfg.Sandbox.OverlayDriver != "none" {
		t.Errorf("OverlayDriver = %q, want %q", cfg.Sandbox.OverlayDriver, "none")
	}
}

// TestLoadDefaults 测试使用默认值（无环境变量）
func TestLoadDefaults(t *testing.T) {
	// 确保没有环境变量干扰
	os.Unsetenv("STRATA_SERVER_ADDR")
	os.Unsetenv("STRATA_SANDBOX_SESSION_ROOT")
	os.Unsetenv("STRATA_SANDBOX_SESSION_TTL")
	os.Unsetenv("STRATA_SANDBOX_MAX_SESSIONS")
	os.Unsetenv("STRATA_SANDBOX_OVERLAY_DRIVER")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// 验证默认值
	if cfg.Server.Addr != ":2280" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":2280")
	}
	if cfg.Sandbox.SessionRoot != "/tmp/strata/sessions" {
		t.Errorf("SessionRoot = %q, want %q", cfg.Sandbox.SessionRoot, "/tmp/strata/sessions")
	}
	if cfg.Sandbox.SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL = %v, want %v", cfg.Sandbox.SessionTTL, 30*time.Minute)
	}
	if cfg.Sandbox.MaxSessions != 100 {
		t.Errorf("MaxSessions = %d, want %d", cfg.Sandbox.MaxSessions, 100)
	}
	if cfg.Sandbox.OverlayDriver != "fuse" {
		t.Errorf("OverlayDriver = %q, want %q", cfg.Sandbox.OverlayDriver, "fuse")
	}
}

// TestConfigStructs 测试配置结构体字段
func TestConfigStructs(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Addr: ":2280",
		},
		Sandbox: SandboxConfig{
			BaseRootfs:     "/base",
			SessionRoot:    "/sessions",
			SessionTTL:     15 * time.Minute,
			MaxSessions:    200,
			IsolateNetwork: true,
			OverlayDriver:  "kernel",
		},
	}

	if cfg.Server.Addr != ":2280" {
		t.Errorf("Server.Addr = %q", cfg.Server.Addr)
	}
	if cfg.Sandbox.BaseRootfs != "/base" {
		t.Errorf("BaseRootfs = %q", cfg.Sandbox.BaseRootfs)
	}
	if cfg.Sandbox.SessionRoot != "/sessions" {
		t.Errorf("SessionRoot = %q", cfg.Sandbox.SessionRoot)
	}
	if cfg.Sandbox.SessionTTL != 15*time.Minute {
		t.Errorf("SessionTTL = %v", cfg.Sandbox.SessionTTL)
	}
	if cfg.Sandbox.MaxSessions != 200 {
		t.Errorf("MaxSessions = %d", cfg.Sandbox.MaxSessions)
	}
	if !cfg.Sandbox.IsolateNetwork {
		t.Error("IsolateNetwork should be true")
	}
	if cfg.Sandbox.OverlayDriver != "kernel" {
		t.Errorf("OverlayDriver = %q", cfg.Sandbox.OverlayDriver)
	}
}
