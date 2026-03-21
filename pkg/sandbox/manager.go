package sandbox

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// sessionKey returns "ownerID:sessionID" format key for session map.
func sessionKey(ownerID, sessionID string) string {
	return ownerID + ":" + sessionID
}

// Manager 管理所有用户的 Session 生命周期
type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session // key: ownerID:sessionID
	sessionRoot string
	baseRootfs  string // 共享的 rootfs（用户配置或自动创建，包含 bash）
	driver      OverlayDriver
	isolateNet  bool
	ttl         time.Duration
	maxSessions int
}

// ManagerConfig holds configuration for Manager.
type ManagerConfig struct {
	SessionRoot string
	BaseRootfs  string
	Driver      OverlayDriver
	IsolateNet  bool
	TTL         time.Duration
	MaxSessions int
}

// NewManager creates a new Manager with the given config.
func NewManager(cfg ManagerConfig) *Manager {
	// 初始化 base rootfs
	var baseRootfs string
	if cfg.BaseRootfs != "" {
		baseRootfs = cfg.BaseRootfs
	} else {
		baseRootfs = filepath.Join(cfg.SessionRoot, "rootfs")
		if err := os.MkdirAll(baseRootfs, 0755); err != nil {
			slog.Error("failed to create base rootfs", "path", baseRootfs, "error", err)
		}
	}

	// 确保 base rootfs 中包含 bash
	if err := ensureBashInRootfs(baseRootfs); err != nil {
		slog.Error("failed to ensure bash in base rootfs", "path", baseRootfs, "error", err)
	} else {
		slog.Info("initialized base rootfs", "path", baseRootfs)
	}

	m := &Manager{
		sessions:    make(map[string]*Session),
		sessionRoot: cfg.SessionRoot,
		baseRootfs: baseRootfs,
		driver:     cfg.Driver,
		isolateNet: cfg.IsolateNet,
		ttl:        cfg.TTL,
		maxSessions: cfg.MaxSessions,
	}
	go m.gcLoop()
	return m
}

// GetOrCreate 获取已有 session，不存在则创建
func (m *Manager) GetOrCreate(ownerID, sessionID string) (*Session, error) {
	key := sessionKey(ownerID, sessionID)
	// slog.Debug("GetOrCreate called", "key", key, "activeSessions", len(m.sessions))

	// 先用读锁快速检查
	m.mu.RLock()
	if s, ok := m.sessions[key]; ok && !s.IsClosed() {
		m.mu.RUnlock()
		slog.Debug("GetOrCreate: found existing session", "key", key)
		// 检查 bwrap 是否还在运行，如果不在就尝试重启
		if !s.IsBwrapAlive() {
			slog.Debug("GetOrCreate: bwrap not alive, trying to restart", "key", key)
			if err := s.RestartBwrap(); err != nil {
				slog.Error("GetOrCreate: failed to restart bwrap", "key", key, "error", err)
				// 重启失败，不影响返回 session
			}
		}
		return s, nil
	}
	m.mu.RUnlock()

	// 写锁创建
	m.mu.Lock()
	defer m.mu.Unlock()

	// double-check
	if s, ok := m.sessions[key]; ok && !s.IsClosed() {
		slog.Debug("GetOrCreate: found existing session (after lock)", "key", key)
		return s, nil
	}

	if len(m.sessions) >= m.maxSessions {
		slog.Warn("GetOrCreate: max sessions reached", "current", len(m.sessions), "max", m.maxSessions)
		return nil, fmt.Errorf("strata: max sessions (%d) reached", m.maxSessions)
	}

	// slog.Debug("GetOrCreate: creating new session", "key", key, "currentSessions", len(m.sessions))

	s, err := newSession(sessionOptions{
		ownerID:     ownerID,
		sessionID:   sessionID,
		sessionRoot: m.sessionRoot,
		baseRootfs: m.baseRootfs,
		driver:     m.driver,
		isolateNet: m.isolateNet,
	})
	if err != nil {
		return nil, err
	}

	m.sessions[key] = s

	// 自动从 map 中移除已关闭的 session
	go func() {
		<-s.Done()
		m.mu.Lock()
		if cur, ok := m.sessions[key]; ok && cur == s {
			delete(m.sessions, key)
			slog.Debug("Manager: session removed from map", "key", key, "remainingSessions", len(m.sessions))
		}
		m.mu.Unlock()
	}()

	return s, nil
}

// Get 获取已有 session，不存在返回 nil, false
func (m *Manager) Get(ownerID, sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionKey(ownerID, sessionID)]
	if ok && s.IsClosed() {
		return nil, false
	}
	return s, ok
}

// Close 主动关闭指定 session
func (m *Manager) Close(ownerID, sessionID string) bool {
	m.mu.Lock()
	s, ok := m.sessions[sessionKey(ownerID, sessionID)]
	if ok {
		delete(m.sessions, sessionKey(ownerID, sessionID))
	}
	m.mu.Unlock()

	if ok {
		s.Close()
	}
	return ok
}

// CloseAll 关闭所有 session
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		s.Close()
	}
	clear(m.sessions)
}

// Stats 返回当前活跃 session 数
func (m *Manager) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]int{"active_sessions": len(m.sessions)}
}

// gcLoop 定期清理超时未活跃的 session
func (m *Manager) gcLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for key, s := range m.sessions {
			// 清理条件：超过 TTL 不活跃，或者 bwrap 已退出且超过 5 分钟
			shouldClose := now.Sub(s.LastHit()) > m.ttl
			if !shouldClose && !s.IsBwrapAlive() {
				// bwrap 已退出，额外等待 5 分钟后清理
				shouldClose = now.Sub(s.LastHit()) > 5*time.Minute
			}
			if shouldClose {
				delete(m.sessions, key)
				slog.Debug("gc: closing session", "key", key, "bwrapAlive", s.IsBwrapAlive())
				go s.Close() // 异步关闭，避免持锁时 block
			}
		}
		m.mu.Unlock()
	}
}
