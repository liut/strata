package sandbox

import (
	"runtime"
	"testing"
)

// TestSanitizeKey 测试 sanitizeKey 函数 - 环境无关
func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"alice_task-001", "alice_task-001"},
		{"alice:task-001", "alice_task-001"},
		{"alice/task-001", "alice_task-001"},
		{"alice task 001", "alice_task_001"},
		{"alice..task", "alice__task"},
		{"alice~task", "alice_task"},
		{"alice@task", "alice_task"},
		{"", ""},
		{"a/b:c d..e~f@g", "a_b_c_d__e_f_g"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeKey(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeKey(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestOverlayDriverConstants 测试 OverlayDriver 常量 - 环境无关
func TestOverlayDriverConstants(t *testing.T) {
	if DriverFuse != "fuse" {
		t.Errorf("DriverFuse = %q, want %q", DriverFuse, "fuse")
	}
	if DriverKernel != "kernel" {
		t.Errorf("DriverKernel = %q, want %q", DriverKernel, "kernel")
	}
	if DriverNone != "none" {
		t.Errorf("DriverNone = %q, want %q", DriverNone, "none")
	}
}

// TestOverlayDriverString 测试 OverlayDriver 字符串转换 - 环境无关
func TestOverlayDriverString(t *testing.T) {
	drivers := []OverlayDriver{DriverFuse, DriverKernel, DriverNone}
	expected := []string{"fuse", "kernel", "none"}

	for i, d := range drivers {
		if string(d) != expected[i] {
			t.Errorf("OverlayDriver(%d) = %q, want %q", i, d, expected[i])
		}
	}
}

// TestCheckOverlay 测试 CheckOverlay 方法 - 环境相关
func TestCheckOverlay(t *testing.T) {
	env := CurrentEnv()

	t.Run("DriverNone", func(t *testing.T) {
		missing := env.CheckOverlay(DriverNone)
		if env.HasBwrap {
			if len(missing) > 0 {
				t.Errorf("expected no missing deps with bwrap, got %v", missing)
			}
		} else {
			if len(missing) == 0 {
				t.Error("expected missing bwrap, got none")
			}
		}
	})

	t.Run("DriverFuse", func(t *testing.T) {
		if !env.IsLinux {
			t.Skip("not Linux, skipping fuse driver test")
		}
		missing := env.CheckOverlay(DriverFuse)
		if env.IsFuseSupported() {
			if len(missing) > 0 {
				t.Errorf("expected no missing deps, got %v", missing)
			}
		}
	})

	t.Run("DriverKernel", func(t *testing.T) {
		if !env.IsLinux {
			t.Skip("not Linux, skipping kernel driver test")
		}
		missing := env.CheckOverlay(DriverKernel)
		if env.IsKernelOverlaySupported() {
			if len(missing) > 0 {
				t.Errorf("expected no missing deps, got %v", missing)
			}
		}
	})
}

// TestEnvInfo 测试环境检测 - 环境相关
func TestEnvInfo(t *testing.T) {
	env := CurrentEnv()

	// 基本信息输出（用于调试）
	t.Logf("Environment: IsLinux=%v, HasBwrap=%v, HasFuse=%v, HasFuseOverlayFS=%v, HasUnshare=%v",
		env.IsLinux, env.HasBwrap, env.HasFuse, env.HasFuseOverlayFS, env.HasUnshare)

	// 在 macOS 上这些应该都是 false
	if runtime.GOOS == "darwin" {
		if env.IsLinux {
			t.Error("IsLinux should be false on darwin")
		}
		if env.HasBwrap {
			t.Error("HasBwrap should be false on darwin")
		}
		if env.HasFuse {
			t.Error("HasFuse should be false on darwin")
		}
	}

	// 在 Linux 上 IsLinux 应该为 true
	if runtime.GOOS == "linux" && !env.IsLinux {
		t.Error("IsLinux should be true on linux")
	}
}

// TestOverlayMountStructure 测试 OverlayMount 结构体 - 环境无关
func TestOverlayMountStructure(t *testing.T) {
	o := OverlayMount{
		Lower:  "/lower",
		Upper:  "/session/upper",
		Work:   "/session/work",
		Merged: "/session/merged",
		driver: DriverFuse,
	}

	if o.Lower != "/lower" {
		t.Errorf("Lower = %q, want %q", o.Lower, "/lower")
	}
	if o.Upper != "/session/upper" {
		t.Errorf("Upper = %q, want %q", o.Upper, "/session/upper")
	}
	if o.driver != DriverFuse {
		t.Errorf("driver = %q, want %q", o.driver, DriverFuse)
	}
}

// TestOverlayMountSessionDir 测试 sessionDir 方法 - 环境无关
func TestOverlayMountSessionDir(t *testing.T) {
	tests := []struct {
		upper    string
		expected string
	}{
		{"/tmp/strata/sessions/key/upper", "/tmp/strata/sessions/key"},
		{"/a/upper", "/a"},
		// 边界情况：路径太短时返回原值
		{"/upper", "/upper"},
		{"/x/u", "/x/u"},
	}

	for _, tc := range tests {
		o := OverlayMount{Upper: tc.upper}
		result := o.sessionDir()
		if result != tc.expected {
			t.Errorf("sessionDir(%q) = %q, want %q", tc.upper, result, tc.expected)
		}
	}
}
