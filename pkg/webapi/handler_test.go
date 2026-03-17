package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liut/strata/pkg/sandbox"
)

// 需要复用到 webapi 包，因为 webapi 依赖 sandbox 的环境
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
	// 创建 Handler 不需要真实环境
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)
	if h == nil {
		t.Error("NewHandler returned nil")
	}
	if h.manager == nil {
		t.Error("manager is nil")
	}
}

// ==================== Helper 函数测试 ====================

func TestJsonOK(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}

	jsonOK(w, &data)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", w.Header().Get("Content-Type"), "application/json")
	}
}

func TestJsonError(t *testing.T) {
	w := httptest.NewRecorder()

	jsonError(w, "test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "test error" {
		t.Errorf("error = %q, want %q", resp["error"], "test error")
	}
}

// ==================== ExecRequest/Response 测试 ====================

func TestExecRequestJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantReq  ExecRequest
		wantErr  bool
	}{
		{
			name:    "valid request",
			jsonStr: `{"command":"ls -la","timeout_ms":5000}`,
			wantReq: ExecRequest{Command: "ls -la", TimeoutMs: 5000},
			wantErr: false,
		},
		{
			name:    "default timeout",
			jsonStr: `{"command":"ls"}`,
			wantReq: ExecRequest{Command: "ls", TimeoutMs: 0}, // 0 会触发默认值
			wantErr: false,
		},
		{
			name:    "invalid json",
			jsonStr: `{invalid}`,
			wantReq: ExecRequest{},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req ExecRequest
			err := json.Unmarshal([]byte(tc.jsonStr), &req)

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.Command != tc.wantReq.Command {
				t.Errorf("Command = %q, want %q", req.Command, tc.wantReq.Command)
			}
			// TimeoutMs 为 0 时会在 handler 中设置为默认值 30000
		})
	}
}

func TestExecResponseJSON(t *testing.T) {
	resp := ExecResponse{
		Output:   "hello world",
		Elapsed:  "10ms",
		Truncated: false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ExecResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Output != resp.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, resp.Output)
	}
	if decoded.Elapsed != resp.Elapsed {
		t.Errorf("Elapsed = %q, want %q", decoded.Elapsed, resp.Elapsed)
	}
}

// ==================== stripEcho 测试 ====================

func TestStripEcho(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
		cmd    string
		want   string
	}{
		{
			name:   "normal case with newline",
			output: []byte("ls -la\ntotal 4\ndrwxr-xr-x   2 root root 4096 Mar 17 00:00 .\n"),
			cmd:    "ls -la",
			want:   "total 4\ndrwxr-xr-x   2 root root 4096 Mar 17 00:00 .\n",
		},
		{
			name:   "no newline in output",
			output: []byte("hello"),
			cmd:    "echo hello",
			want:   "hello",
		},
		{
			name:   "only command echo",
			output: []byte("ls\n"),
			cmd:    "ls",
			want:   "",
		},
		{
			name:   "output with crlf",
			output: []byte("ls -la\r\ntotal 4\r\n"),
			cmd:    "ls -la",
			want:   "total 4\r\n",
		},
		{
			name:   "empty output",
			output: []byte{},
			cmd:    "ls",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stripEcho(tc.output, tc.cmd)
			if string(result) != tc.want {
				t.Errorf("stripEcho() = %q, want %q", string(result), tc.want)
			}
		})
	}
}

// ==================== 路由测试 ====================

func TestRoutes(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)
	mux := h.Routes()

	// 测试路由是否正确注册
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "POST /api/sessions",
			method:     "POST",
			path:       "/api/sessions",
			wantStatus: http.StatusBadRequest, // 会到达 handler，但 body 解析失败
		},
		{
			name:       "GET /api/stats",
			method:     "GET",
			path:       "/api/stats",
			wantStatus: http.StatusOK, // 正确路由
		},
		{
			name:       "DELETE /api/sessions/user/session",
			method:     "DELETE",
			path:       "/api/sessions/user/session",
			wantStatus: http.StatusNotFound, // session 不存在
		},
		{
			name:       "GET /api/ws/uid/sid/shell (no body)",
			method:     "GET",
			path:       "/api/ws/uid/sid/shell",
			wantStatus: http.StatusBadRequest, // WebSocket upgrade 失败，但路由存在
		},
		{
			name:       "unknown path",
			method:     "GET",
			path:       "/api/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong method on sessions",
			method:     "GET",
			path:       "/api/sessions",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status code = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// ==================== HandleStats 测试 ====================

func TestHandleStats(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	h.HandleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var stats map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if stats["active_sessions"] != 0 {
		t.Errorf("active_sessions = %d, want 0", stats["active_sessions"])
	}
}

// ==================== HandleCreateSession 测试 ====================

func TestHandleCreateSession(t *testing.T) {
	requireBwrap(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)

	body := `{"user_id":"alice","session_id":"test-001"}`
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["user_id"] != "alice" {
		t.Errorf("user_id = %q, want %q", resp["user_id"], "alice")
	}
	if resp["session_id"] != "test-001" {
		t.Errorf("session_id = %q, want %q", resp["session_id"], "test-001")
	}

	// 清理
	m.Close("alice", "test-001")
}

func TestHandleCreateSessionInvalidBody(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty body", "", http.StatusBadRequest},
		{"invalid json", "{invalid}", http.StatusBadRequest},
		{"missing user_id", `{"session_id":"s1"}`, http.StatusBadRequest},
		{"missing session_id", `{"user_id":"u1"}`, http.StatusBadRequest},
		{"empty user_id", `{"user_id":"","session_id":"s1"}`, http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.HandleCreateSession(w, req)

			if w.Code != tc.want {
				t.Errorf("status code = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// ==================== HandleCloseSession 测试 ====================

func TestHandleCloseSession(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)

	// 关闭不存在的 session
	req := httptest.NewRequest("DELETE", "/api/sessions/nonexistent/session", nil)
	w := httptest.NewRecorder()

	h.HandleCloseSession(w, req)

	// 应该返回 404
	if w.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ==================== HandleExec 参数验证测试 ====================

func TestHandleExecValidation(t *testing.T) {
	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)

	tests := []struct {
		name       string
		uid        string
		sid        string
		body       string
		wantStatus int
	}{
		{
			name:       "empty uid",
			uid:        "",
			sid:        "s1",
			body:       `{"command":"ls"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty sid",
			uid:        "u1",
			sid:        "",
			body:       `{"command":"ls"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty command",
			uid:        "u1",
			sid:        "s1",
			body:       `{"command":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			uid:        "u1",
			sid:        "s1",
			body:       `invalid`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/sessions/"+tc.uid+"/"+tc.sid+"/exec", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.HandleExec(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status code = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// ==================== 集成测试：完整请求流程 ====================

func TestFullFlow(t *testing.T) {
	requireBwrap(t)

	m := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: "/tmp/strata/sessions",
		Driver:      sandbox.DriverNone,
		MaxSessions: 10,
	})
	h := NewHandler(m)
	mux := h.Routes()

	// 1. 创建 session
	body := `{"user_id":"testuser","session_id":"testflow"}`
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create session failed: %d %s", w.Code, w.Body.String())
	}

	// 2. 获取 stats
	req = httptest.NewRequest("GET", "/api/stats", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("stats failed: %d", w.Code)
	}

	var stats map[string]int
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats["active_sessions"] != 1 {
		t.Errorf("active_sessions = %d, want 1", stats["active_sessions"])
	}

	// 3. 关闭 session
	req = httptest.NewRequest("DELETE", "/api/sessions/testuser/testflow", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("close session failed: %d", w.Code)
	}
}
