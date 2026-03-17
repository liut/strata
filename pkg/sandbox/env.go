package sandbox

import (
	"os"
	"os/exec"
	"runtime"
)

// EnvInfo 描述当前运行环境的状态和可用性
type EnvInfo struct {
	IsLinux          bool // 是否 Linux
	HasBwrap         bool // bubblewrap 可用
	HasFuse          bool // /dev/fuse 存在
	HasFuseOverlayFS bool // fuse-overlayfs 命令可用
	HasUnshare       bool // unshare 命令可用
}

// CurrentEnv 检测当前环境基本信息
func CurrentEnv() EnvInfo {
	env := EnvInfo{
		IsLinux: runtime.GOOS == "linux",
	}

	if _, err := exec.LookPath("bwrap"); err == nil {
		env.HasBwrap = true
	}

	if _, err := os.Stat("/dev/fuse"); err == nil {
		env.HasFuse = true
	}

	if _, err := exec.LookPath("fuse-overlayfs"); err == nil {
		env.HasFuseOverlayFS = true
	}

	if _, err := exec.LookPath("unshare"); err == nil {
		env.HasUnshare = true
	}

	return env
}

// CheckOverlay 检查指定驱动的依赖，返回缺失列表
func (e EnvInfo) CheckOverlay(driver OverlayDriver) []string {
	var missing []string

	switch driver {
	case DriverFuse:
		if !e.HasFuseOverlayFS {
			missing = append(missing, "fuse-overlayfs (apt install fuse-overlayfs)")
		}
		if !e.HasFuse {
			missing = append(missing, "/dev/fuse device (modprobe fuse)")
		}
	case DriverKernel:
		if !e.HasUnshare {
			missing = append(missing, "unshare (apt install util-linux)")
		}
	}

	if !e.HasBwrap {
		missing = append(missing, "bwrap (apt install bubblewrap)")
	}

	return missing
}

// IsFuseSupported 检查 fuse-overlayfs 驱动是否可用
func (e EnvInfo) IsFuseSupported() bool {
	return e.IsLinux && e.HasFuse && e.HasFuseOverlayFS && e.HasBwrap
}

// IsKernelOverlaySupported 检查内核 overlay 驱动是否可用
func (e EnvInfo) IsKernelOverlaySupported() bool {
	return e.IsLinux && e.HasUnshare && e.HasBwrap
}

// IsFullySupported 检查是否支持所有功能（完整功能）
func (e EnvInfo) IsFullySupported() bool {
	return e.IsFuseSupported()
}
