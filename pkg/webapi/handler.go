package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/liut/strata/pkg/sandbox"
)

// Handler 接口用于注册路由
type Handler interface {
	http.Handler
	Handle(string, http.Handler)
}

type Router interface {
	Route(Handler)
}

// handlerImpl 持有所有 HTTP 路由依赖
type handlerImpl struct {
	manager *sandbox.Manager
}

func NewHandler(m *sandbox.Manager) Router {
	return &handlerImpl{manager: m}
}

func (h *handlerImpl) Route(mux Handler) {
	mux.Handle("POST /api/sessions", http.HandlerFunc(h.handleCreateSession))
	mux.Handle("DELETE /api/sessions/{uid}/{sid}", http.HandlerFunc(h.handleCloseSession))
	mux.Handle("POST /api/sessions/{uid}/{sid}/exec", http.HandlerFunc(h.handleExec))
	mux.Handle("GET /api/stats", http.HandlerFunc(h.handleStats))
	mux.Handle("GET /api/ws/{uid}/{sid}/shell", http.HandlerFunc(h.handleShellWS))
}

// handleCreateSession 创建或复用 session
func (h *handlerImpl) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OwnerID   string `json:"user_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OwnerID == "" || req.SessionID == "" {
		jsonError(w, "invalid request: user_id and session_id required", http.StatusBadRequest)
		return
	}

	s, err := h.manager.GetOrCreate(req.OwnerID, req.SessionID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{
		"user_id":    s.UID(),
		"session_id": s.ID(),
		"created_at": s.Created().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleCloseSession 关闭 session
func (h *handlerImpl) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	sid := r.PathValue("sid")

	if ok := h.manager.Close(uid, sid); !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]string{"status": "closed"})
}

// handleStats 返回服务状态
func (h *handlerImpl) handleStats(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.manager.Stats())
}

// ───────────────────────────────────────────
// Helper
// ───────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
