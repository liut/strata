package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liut/strata/pkg/sandbox"
)

func requireLinux(t *testing.T) {
	if !sandbox.CurrentEnv().IsLinux {
		t.Skip("requires Linux")
	}
}

func requireBwrap(t *testing.T) {
	if !sandbox.CurrentEnv().HasBwrap {
		t.Skip("requires bwrap")
	}
}

// ==================== Handler 基础测试 ====================

func TestNewHandler(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)
	// NewHandler 返回 Handler 接口
	if h == nil {
		t.Error("NewHandler returned nil")
	}
}

// ==================== Helper 函数测试 ====================

func TestJsonOK(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}

	jsonOK(w, &data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestJsonError(t *testing.T) {
	w := httptest.NewRecorder()
	jsonError(w, "test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["error"] != "test error" {
		t.Errorf("expected error 'test error', got '%s'", resp["error"])
	}
}

// ==================== HTTP Handler 测试 ====================

func TestHandleStats(t *testing.T) {
	requireLinux(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m).(*handlerImpl)

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/api/stats", nil)

	h.handleStats(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["active_sessions"] != 0 {
		t.Errorf("expected 0 active sessions, got %d", resp["active_sessions"])
	}
}

func TestHandleCreateSession(t *testing.T) {
	requireLinux(t)
	requireBwrap(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m).(*handlerImpl)

	body := `{"user_id": "testuser", "session_id": "test123"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	h.handleCreateSession(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSessionInvalidBody(t *testing.T) {
	requireLinux(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m).(*handlerImpl)

	// 缺少 session_id
	body := `{"user_id": "testuser"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	h.handleCreateSession(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCloseSession(t *testing.T) {
	requireLinux(t)
	requireBwrap(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m).(*handlerImpl)

	// 先创建
	body := `{"user_id": "testuser", "session_id": "test-close"}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	h.handleCreateSession(w, r)

	// 再关闭
	w = httptest.NewRecorder()
	r, _ = http.NewRequest("DELETE", "/api/sessions/testuser/test-close", nil)

	h.handleCloseSession(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleExecValidation(t *testing.T) {
	requireLinux(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m).(*handlerImpl)

	// 缺少 command
	body := `{"command": ""}`
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("POST", "/api/sessions/user/sid/exec", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")

	h.handleExec(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
