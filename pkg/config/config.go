package config

import (
	"log"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config 应用配置
type Config struct {
	Name    string `ignored:"true"`
	Version string `ignored:"true"`

	Server  ServerConfig  `envconfig:"SERVER"`
	Sandbox SandboxConfig `envconfig:"SANDBOX"`
}

type ServerConfig struct {
	Addr      string `envconfig:"ADDR" default:":8080" desc:"HTTP/WS listen address"`
	AccessLog string `envconfig:"ACCESS_LOG" desc:"Access log file path, empty for stdout"`
}

type SandboxConfig struct {
	// 基础只读根文件系统路径；为空则自动 fallback 到宿主目录 bind
	BaseRootfs string `envconfig:"BASE_ROOTFS" desc:"Base read-only rootfs path"`

	SessionRoot string        `envconfig:"SESSION_ROOT" default:"/tmp/strata/sessions" desc:"Session working directory root"`
	SessionTTL  time.Duration `envconfig:"SESSION_TTL" default:"30m" desc:"Inactive session timeout"`
	MaxSessions int           `envconfig:"MAX_SESSIONS" default:"100" desc:"Global max session count"`

	// 网络隔离开关：true 则每个 session 拥有独立 network namespace
	IsolateNetwork bool `envconfig:"ISOLATE_NETWORK" desc:"Enable network isolation per session"`

	// Overlay 驱动: "fuse" | "kernel" | "none"
	// "fuse"   → fuse-overlayfs（无 root，推荐）
	// "kernel" → unshare+mount（需 Linux ≥ 5.11 + 正确配置）
	// "none"   → 纯 bwrap tmpfs（无持久写入层，降级模式）
	OverlayDriver string `envconfig:"OVERLAY_DRIVER" default:"fuse" desc:"Overlay driver: fuse|kernel|none"`
}

var (
	// Current 当前配置
	Current = new(Config)
)

// Load 从环境变量加载配置
func Load() (*Config, error) {
	cfg := defaultConfig()

	// 从环境变量加载
	if err := envconfig.Process("strata", cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:      ":8080",
			AccessLog: "",
		},
		Sandbox: SandboxConfig{
			BaseRootfs:     "",
			SessionRoot:    "/tmp/strata/sessions",
			SessionTTL:     30 * time.Minute,
			MaxSessions:    100,
			IsolateNetwork: false,
			OverlayDriver:  "fuse",
		},
	}
}

// Usage 打印配置帮助信息
func Usage() error {
	log.Printf("ver: %s", Current.Version)
	return envconfig.Usage("strata", Current)
}
