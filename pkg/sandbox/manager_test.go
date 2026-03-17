package sandbox

import (
	"testing"
	"time"
)

// TestManagerConfig 测试 ManagerConfig 结构体
func TestManagerConfig(t *testing.T) {
	cfg := ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		BaseRootfs:  "",
		Driver:      DriverNone,
		IsolateNet:  false,
		TTL:         30 * time.Minute,
		MaxSessions: 100,
	}

	if cfg.SessionRoot != "/tmp/strata/sessions" {
		t.Errorf("SessionRoot = %q, want %q", cfg.SessionRoot, "/tmp/strata/sessions")
	}
	if cfg.MaxSessions != 100 {
		t.Errorf("MaxSessions = %d, want %d", cfg.MaxSessions, 100)
	}
}

// TestManagerStats 测试 Stats 返回值
func TestManagerStats(t *testing.T) {
	m := NewManager(ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      DriverNone,
		MaxSessions: 10,
		TTL:         time.Minute,
	})

	stats := m.Stats()
	if stats["active_sessions"] != 0 {
		t.Errorf("Stats() = %v, want active_sessions=0", stats)
	}
}

// TestManagerCloseNonExistent 测试关闭不存在的 session
func TestManagerCloseNonExistent(t *testing.T) {
	m := NewManager(ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      DriverNone,
		MaxSessions: 10,
		TTL:         time.Minute,
	})

	// 关闭不存在的 session 应返回 false
	ok := m.Close("nonexistent", "session")
	if ok {
		t.Errorf("Close() = true, want false for non-existent session")
	}
}

// TestManagerGetNonExistent 测试获取不存在的 session
func TestManagerGetNonExistent(t *testing.T) {
	m := NewManager(ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      DriverNone,
		MaxSessions: 10,
		TTL:         time.Minute,
	})

	// 获取不存在的 session
	_, ok := m.Get("nonexistent", "session")
	if ok {
		t.Errorf("Get() = true, want false for non-existent session")
	}
}

// TestManagerMaxSessions 测试达到最大 session 限制
func TestManagerMaxSessions(t *testing.T) {
	env := CurrentEnv()
	if !env.HasBwrap {
		t.Skip("bwrap not available, skipping test")
	}

	m := NewManager(ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      DriverNone,
		MaxSessions: 1,
		TTL:         time.Minute,
	})

	// 创建第一个 session
	_, err := m.GetOrCreate("user1", "session1")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	// 尝试创建第二个 session 应该失败
	_, err = m.GetOrCreate("user2", "session2")
	if err == nil {
		t.Error("expected error when max sessions reached, got nil")
	}

	// 清理
	m.Close("user1", "session1")
}
