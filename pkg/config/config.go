package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Sandbox SandboxConfig `yaml:"sandbox"`
	GRPC    GRPCConfig    `yaml:"grpc"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"` // HTTP/WS listen address

	// 日志配置
	AccessLog string `yaml:"access_log"` // 访问日志文件路径，空则输出到 stdout
}

type SandboxConfig struct {
	// 基础只读根文件系统路径；为空则自动 fallback 到宿主目录 bind
	BaseRootfs  string        `yaml:"base_rootfs"`
	SessionRoot string        `yaml:"session_root"` // session 工作目录根
	SessionTTL  time.Duration `yaml:"session_ttl"`  // 不活跃超时
	MaxSessions int           `yaml:"max_sessions"` // 全局最大 session 数

	// 网络隔离开关：true 则每个 session 拥有独立 network namespace
	IsolateNetwork bool `yaml:"isolate_network"`

	// Overlay 驱动: "fuse" | "kernel" | "none"
	// "fuse"   → fuse-overlayfs（无 root，推荐）
	// "kernel" → unshare+mount（需 Linux ≥ 5.11 + 正确配置）
	// "none"   → 纯 bwrap tmpfs（无持久写入层，降级模式）
	OverlayDriver string `yaml:"overlay_driver"`
}

type GRPCConfig struct {
	Addr string `yaml:"addr"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // 配置文件不存在则使用默认值
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Addr: ":8080",
		},
		Sandbox: SandboxConfig{
			BaseRootfs:     "",
			SessionRoot:    "/tmp/strata/sessions",
			SessionTTL:     30 * time.Minute,
			MaxSessions:    100,
			IsolateNetwork: false,
			OverlayDriver:  "fuse",
		},
		GRPC: GRPCConfig{
			Addr: ":9090",
		},
	}
}
