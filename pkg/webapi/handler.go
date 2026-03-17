package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/liut/strata/pkg/sandbox"
)

// Handler 持有所有 HTTP 路由依赖
type Handler struct {
	manager *sandbox.Manager
}

func NewHandler(m *sandbox.Manager) *Handler {
	return &Handler{manager: m}
}

// Routes 注册所有 HTTP 路由
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", h.HandleCreateSession)
	mux.HandleFunc("DELETE /api/sessions/{user}/{session}", h.HandleCloseSession)
	mux.HandleFunc("POST /api/sessions/{uid}/{sid}/exec", h.HandleExec)
	mux.HandleFunc("GET /api/stats", h.HandleStats)
	mux.HandleFunc("GET /api/ws/{uid}/{sid}/shell", h.HandleShellWS) // WebSocket
	return mux
}

// HandleCreateSession 创建或复用 session
//
//	POST /api/sessions
//	Body: {"user_id": "alice", "session_id": "s1"}
func (h *Handler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" || req.SessionID == "" {
		jsonError(w, "invalid request: user_id and session_id required", http.StatusBadRequest)
		return
	}

	s, err := h.manager.GetOrCreate(req.UserID, req.SessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{
		"user_id":    s.UserID,
		"session_id": s.ID,
		"created_at": s.Created.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// HandleCloseSession 关闭 session
//
//	DELETE /api/sessions/{user}/{session}
func (h *Handler) HandleCloseSession(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	session := r.PathValue("session")

	if ok := h.manager.Close(user, session); !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]string{"status": "closed"})
}

// HandleStats 返回服务状态
//
//	GET /api/stats
func (h *Handler) HandleStats(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.manager.Stats())
}

// ────────────────────────────────────────────
// Helper
// ────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
