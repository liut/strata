package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/liut/strata/pkg/identity"
	"github.com/liut/strata/pkg/sandbox"
	"github.com/liut/strata/pkg/webapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Router 注册 MCP 路由
type Router interface {
	Route(mux webapi.Handler)
}

// Handler 处理 MCP 工具调用
type Handler struct {
	manager  *sandbox.Manager
	sessions map[string]*SessionInfo
	mcps     *server.MCPServer
}

// SessionInfo MCP 缓存的 session 信息
type SessionInfo struct {
	OwnerID   string
	SessionID string
	CreatedAt string
}

// NewHandler creates a new MCP handler.
func NewHandler(manager *sandbox.Manager, name, version string) *Handler {
	mcpsvr := server.NewMCPServer(name, version)
	h := &Handler{
		manager:  manager,
		sessions: make(map[string]*SessionInfo),
		mcps:     mcpsvr,
	}
	SetupTools(mcpsvr, h)
	return h
}

var _ Router = (*Handler)(nil)

// Route 注册 MCP 路由
func (h *Handler) Route(mux webapi.Handler) {
	mux.Handle("/mcp/", server.NewStreamableHTTPServer(h.mcps,
		server.WithHTTPContextFunc(builtContextFromRequest),
	))
}

func builtContextFromRequest(ctx context.Context, r *http.Request) context.Context {
	return identity.ContextWithScarf(ctx, identity.Scarf{
		OwnerID:   r.Header.Get(identity.HeaderOwnerID),
		SessionID: r.Header.Get(identity.HeaderSessionID),
	})
}

func (h *Handler) handleCreateSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := ParseScarfFromArgs(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	key := sc.GetKey()
	if _, exists := h.sessions[key]; !exists {
		sess, err := h.manager.GetOrCreate(sc.OwnerID, sc.SessionID)
		if err != nil {
			return mcp.NewToolResultError("create session failed: " + err.Error()), nil
		}
		h.sessions[key] = &SessionInfo{
			OwnerID:   sess.UID(),
			SessionID: sess.ID(),
			CreatedAt: sess.Created().Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	sm := h.sessions[key]
	return mcp.NewToolResultText(fmt.Sprintf("Session: %s/%s", sm.OwnerID, sm.SessionID)), nil
}

func (h *Handler) handleExec(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := ParseScarfFromArgs(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command, _ := args["command"].(string)
	timeoutMs, _ := args["timeout_ms"].(float64)

	if command == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	// 确保 session 存在
	key := sc.GetKey()
	if _, exists := h.sessions[key]; !exists {
		_, err := h.manager.GetOrCreate(sc.OwnerID, sc.SessionID)
		if err != nil {
			return mcp.NewToolResultError("create session failed: " + err.Error()), nil
		}
	}

	timeout := 30000
	if timeoutMs > 0 {
		timeout = int(timeoutMs)
	}

	sess, ok := h.manager.Get(sc.OwnerID, sc.SessionID)
	if !ok {
		return mcp.NewToolResultError("session not found"), nil
	}

	output, _, err := webapi.ExecInSession(sess, command, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		slog.Info("exec fail", "cmd", command, "err", err)
		return mcp.NewToolResultError("exec failed: " + err.Error()), nil
	}

	if output == "" {
		output = "(empty output)"
	}
	return mcp.NewToolResultText(output), nil
}

func (h *Handler) handleWriteFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := ParseScarfFromArgs(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	cmd := fmt.Sprintf("cat > '%s' << 'STRATA_EOF'\n%s\nSTRATA_EOF", path, content)
	return h.handleExec(ctx, map[string]any{
		"owner_id":   sc.OwnerID,
		"session_id": sc.SessionID,
		"command":    cmd,
	})
}

func (h *Handler) handleReadFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := ParseScarfFromArgs(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path, _ := args["path"].(string)

	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	return h.handleExec(ctx, map[string]any{
		"owner_id":   sc.OwnerID,
		"session_id": sc.SessionID,
		"command":    fmt.Sprintf("cat %s", path),
	})
}

func (h *Handler) handleCloseSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := ParseScarfFromArgs(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	key := sc.GetKey()
	if h.manager.Close(sc.OwnerID, sc.SessionID) {
		delete(h.sessions, key)
		return mcp.NewToolResultText(fmt.Sprintf("Session %s closed", key)), nil
	}
	return mcp.NewToolResultError("session not found"), nil
}

func (h *Handler) handleStats(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	stats := h.manager.Stats()
	return mcp.NewToolResultText(fmt.Sprintf("Active sessions: %d", stats["active_sessions"])), nil
}
