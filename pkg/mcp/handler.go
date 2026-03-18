package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/liut/strata/pkg/sandbox"
	"github.com/liut/strata/pkg/webapi"
	"github.com/mark3labs/mcp-go/mcp"
)

// Handler 处理 MCP 工具调用
type Handler struct {
	manager  *sandbox.Manager
	sessions map[string]*SessionInfo
}

// SessionInfo MCP 缓存的 session 信息
type SessionInfo struct {
	UserID    string
	SessionID string
	CreatedAt string
}

func NewHandler(manager *sandbox.Manager) *Handler {
	return &Handler{
		manager:  manager,
		sessions: make(map[string]*SessionInfo),
	}
}

func (h *Handler) handleCreateSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)

	if userID == "" || sessionID == "" {
		return mcp.NewToolResultError("user_id and session_id are required"), nil
	}

	key := userID + ":" + sessionID
	if _, exists := h.sessions[key]; !exists {
		sess, err := h.manager.GetOrCreate(userID, sessionID)
		if err != nil {
			return mcp.NewToolResultError("create session failed: " + err.Error()), nil
		}
		h.sessions[key] = &SessionInfo{
			UserID:    sess.GetUID(),
			SessionID: sess.GetID(),
			CreatedAt: sess.Created.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	sm := h.sessions[key]
	return mcp.NewToolResultText(fmt.Sprintf("Session: %s/%s", sm.UserID, sm.SessionID)), nil
}

func (h *Handler) handleExec(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	command, _ := args["command"].(string)
	timeoutMs, _ := args["timeout_ms"].(float64)

	if userID == "" || sessionID == "" || command == "" {
		return mcp.NewToolResultError("user_id, session_id and command are required"), nil
	}

	// 确保 session 存在
	key := userID + ":" + sessionID
	if _, exists := h.sessions[key]; !exists {
		_, err := h.manager.GetOrCreate(userID, sessionID)
		if err != nil {
			return mcp.NewToolResultError("create session failed: " + err.Error()), nil
		}
	}

	timeout := 30000
	if timeoutMs > 0 {
		timeout = int(timeoutMs)
	}

	sess, ok := h.manager.Get(userID, sessionID)
	if !ok {
		return mcp.NewToolResultError("session not found"), nil
	}

	output, err := webapi.ExecInSession(sess, command, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		return mcp.NewToolResultError("exec failed: " + err.Error()), nil
	}

	if output == "" {
		output = "(empty output)"
	}
	return mcp.NewToolResultText(output), nil
}

func (h *Handler) handleWriteFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if userID == "" || sessionID == "" || path == "" {
		return mcp.NewToolResultError("user_id, session_id and path are required"), nil
	}

	cmd := fmt.Sprintf("cat > '%s' << 'STRATA_EOF'\n%s\nSTRATA_EOF", path, content)
	return h.handleExec(ctx, map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"command":    cmd,
	})
}

func (h *Handler) handleReadFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	path, _ := args["path"].(string)

	if userID == "" || sessionID == "" || path == "" {
		return mcp.NewToolResultError("user_id, session_id and path are required"), nil
	}

	return h.handleExec(ctx, map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"command":    fmt.Sprintf("cat %s", path),
	})
}

func (h *Handler) handleCloseSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)

	if userID == "" || sessionID == "" {
		return mcp.NewToolResultError("user_id and session_id are required"), nil
	}

	key := userID + ":" + sessionID
	if h.manager.Close(userID, sessionID) {
		delete(h.sessions, key)
		return mcp.NewToolResultText(fmt.Sprintf("Session %s closed", key)), nil
	}
	return mcp.NewToolResultError("session not found"), nil
}

func (h *Handler) handleStats(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	stats := h.manager.Stats()
	return mcp.NewToolResultText(fmt.Sprintf("Active sessions: %d", stats["active_sessions"])), nil
}
