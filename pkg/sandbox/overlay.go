package sandbox

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

// OverlayDriver 定义 overlay 实现
type OverlayDriver string

const (
	DriverFuse   OverlayDriver = "fuse"   // fuse-overlayfs（无 root，推荐）
	DriverKernel OverlayDriver = "kernel" // 内核 overlayfs in user namespace（Linux ≥ 5.11）
	DriverNone   OverlayDriver = "none"   // 降级：纯 bwrap + tmpfs，无持久层
)

// OverlayMount 表示一个 overlay 文件系统挂载实例
type OverlayMount struct {
	Lower  string // lowerdir：只读基础层（可多层，用 : 分隔）
	Upper  string // upperdir：session 专属可写层
	Work   string // workdir：overlayfs 内部工作目录
	Merged string // 挂载点：合并后的完整文件系统视图

	driver OverlayDriver
	active bool
}

// Mount 根据配置的 driver 执行挂载
func (o *OverlayMount) Mount() error {
	// slog.Debug("overlay mount start", "driver", o.driver, "lower", o.Lower, "upper", o.Upper, "work", o.Work, "merged", o.Merged)

	for _, d := range []string{o.Upper, o.Work, o.Merged} {
		if err := os.MkdirAll(d, 0700); err != nil {
			slog.Error("overlay mkdir failed", "dir", d, "error", err)
			return fmt.Errorf("strata/overlay: mkdir %s: %w", d, err)
		}
	}

	var err error
	switch o.driver {
	case DriverFuse:
		err = o.mountFuse()
	case DriverKernel:
		err = o.mountKernel()
	case DriverNone:
		return nil // 无 overlay，bwrap 使用 tmpfs 降级
	default:
		err = o.mountFuse()
	}

	if err != nil {
		slog.Error("overlay mount failed", "driver", o.driver, "error", err)
		return err
	}
	o.active = true
	slog.Debug("overlay mounted successfully", "driver", o.driver, "merged", o.Merged)
	return nil
}

// mountFuse 使用 fuse-overlayfs（普通用户可执行）
//
//	fuse-overlayfs \
//	  -o lowerdir=<lower>,upperdir=<upper>,workdir=<work> \
//	  <merged>
func (o *OverlayMount) mountFuse() error {
	bin, err := exec.LookPath("fuse-overlayfs")
	if err != nil {
		return fmt.Errorf("strata/overlay: fuse-overlayfs not found (apt install fuse-overlayfs): %w", err)
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", o.Lower, o.Upper, o.Work)
	out, err := exec.Command(bin, "-o", opts, o.Merged).CombinedOutput()
	if err != nil {
		return fmt.Errorf("strata/overlay: fuse mount failed: %w\n%s", err, out)
	}
	return nil
}

// mountKernel 在 user namespace 内挂载内核 overlayfs（需 Linux ≥ 5.11）
//
//	unshare --user --mount --map-root-user \
//	  mount -t overlay overlay \
//	    -o lowerdir=...,upperdir=...,workdir=... <merged>
func (o *OverlayMount) mountKernel() error {
	script := fmt.Sprintf(
		"mount -t overlay overlay -o lowerdir=%s,upperdir=%s,workdir=%s %s",
		o.Lower, o.Upper, o.Work, o.Merged,
	)
	out, err := exec.Command(
		"unshare", "--user", "--mount", "--map-root-user",
		"sh", "-c", script,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("strata/overlay: kernel overlay failed: %w\n%s", err, out)
	}
	return nil
}

// Umount 卸载 overlay
func (o *OverlayMount) Umount() error {
	if !o.active {
		return nil
	}
	o.active = false

	// 优先使用 fusermount（fuse 驱动），fallback 到 umount -l
	if err := exec.Command("fusermount", "-u", o.Merged).Run(); err != nil {
		if err2 := exec.Command("umount", "-l", o.Merged).Run(); err2 != nil {
			return fmt.Errorf("strata/overlay: umount failed: fusermount: %v, umount: %v", err, err2)
		}
	}
	return nil
}

// Cleanup 卸载并删除 session 全部工作目录
func (o *OverlayMount) Cleanup() error {
	umountErr := o.Umount()
	// 无论卸载是否成功都尝试删除目录，避免残留
	if err := os.RemoveAll(o.sessionDir()); err != nil {
		return fmt.Errorf("strata/overlay: cleanup failed: %v (umount: %v)", err, umountErr)
	}
	return umountErr
}

// sessionDir 返回 upper/work/merged 的上级目录
func (o *OverlayMount) sessionDir() string {
	// Upper 形如 /tmp/strata/sessions/<key>/upper，取父目录
	if len(o.Upper) > 6 {
		return o.Upper[:len(o.Upper)-6] // 去掉 "/upper"
	}
	return o.Upper
}
