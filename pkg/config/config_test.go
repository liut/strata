package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDefaultConfig 测试默认配置 - 环境无关
func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.Server.Addr != ":8080" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":8080")
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
	if cfg.GRPC.Addr != ":9090" {
		t.Errorf("GRPC.Addr = %q, want %q", cfg.GRPC.Addr, ":9090")
	}
}

// TestLoadNonExistentFile 测试加载不存在的配置文件 - 环境无关
func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")

	if err != nil {
		t.Errorf("expected no error for non-existent file, got %v", err)
	}
	if cfg == nil {
		t.Error("expected non-nil config")
	}

	// 应该返回默认配置
	if cfg.Server.Addr != ":8080" {
		t.Errorf("expected default Server.Addr, got %q", cfg.Server.Addr)
	}
}

// TestLoadValidFile 测试加载有效配置文件 - 环境无关
func TestLoadValidFile(t *testing.T) {
	// 创建临时配置文件
	content := `
server:
  addr: ":9090"
sandbox:
  session_root: "/custom/sessions"
  session_ttl: "60m"
  max_sessions: 50
  overlay_driver: "none"
grpc:
  addr: ":9999"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
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
	if cfg.GRPC.Addr != ":9999" {
		t.Errorf("GRPC.Addr = %q, want %q", cfg.GRPC.Addr, ":9999")
	}
}

// TestLoadInvalidYAML 测试加载无效 YAML - 环境无关
func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// TestLoadPartialFile 测试加载部分配置 - 环境无关
func TestLoadPartialFile(t *testing.T) {
	// 只提供部分配置，其他用默认值
	content := `
server:
  addr: ":8081"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// 自定义值
	if cfg.Server.Addr != ":8081" {
		t.Errorf("Server.Addr = %q, want %q", cfg.Server.Addr, ":8081")
	}
	// 默认值
	if cfg.Sandbox.SessionRoot != "/tmp/strata/sessions" {
		t.Errorf("SessionRoot = %q, want %q", cfg.Sandbox.SessionRoot, "/tmp/strata/sessions")
	}
}

// TestConfigStructs 测试配置结构体字段 - 环境无关
func TestConfigStructs(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Addr: ":8080",
		},
		Sandbox: SandboxConfig{
			BaseRootfs:     "/base",
			SessionRoot:    "/sessions",
			SessionTTL:     15 * time.Minute,
			MaxSessions:    200,
			IsolateNetwork: true,
			OverlayDriver:  "kernel",
		},
		GRPC: GRPCConfig{
			Addr: ":9090",
		},
	}

	if cfg.Server.Addr != ":8080" {
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
	if cfg.GRPC.Addr != ":9090" {
		t.Errorf("GRPC.Addr = %q", cfg.GRPC.Addr)
	}
}
