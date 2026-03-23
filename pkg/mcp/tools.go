package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools 注册所有 MCP 工具
func SetupTools(srv *server.MCPServer, h *Handler) {
	srv.AddTool(mcp.Tool{
		Name:        "exec",
		Description: "Execute shell command in sandbox session (auto-creates session)",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"owner_id":   map[string]string{"type": "string", "description": "User identifier"},
				"session_id": map[string]string{"type": "string", "description": "Session identifier"},
				"command":    map[string]string{"type": "string", "description": "Shell command"},
				"timeout_ms": map[string]any{"type": "number", "description": "Timeout in milliseconds", "default": float64(30000)},
			},
			Required: []string{"command"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleExec(ctx, request.GetArguments())
	})
}
