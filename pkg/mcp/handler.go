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
	manager *sandbox.Manager
	mcps   *server.MCPServer
}

// NewHandler creates a new MCP handler.
func NewHandler(manager *sandbox.Manager, name, version string) *Handler {
	mcpsvr := server.NewMCPServer(name, version)
	h := &Handler{
		manager: manager,
		mcps:    mcpsvr,
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

func (h *Handler) handleExec(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	sc, err := identity.ParseScarf(ctx, identity.FromArgs(args))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command, _ := args["command"].(string)
	timeoutMs, _ := args["timeout_ms"].(float64)

	if command == "" {
		return mcp.NewToolResultError("command is required"), nil
	}

	timeout := 30000
	if timeoutMs > 0 {
		timeout = int(timeoutMs)
	}

	sess, err := h.manager.GetOrCreate(sc.OwnerID, sc.SessionID)
	if err != nil {
		return mcp.NewToolResultError("session failed: " + err.Error()), nil
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

func (h *Handler) handleStats(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	stats := h.manager.Stats()
	return mcp.NewToolResultText(fmt.Sprintf("Active sessions: %d", stats["active_sessions"])), nil
}
