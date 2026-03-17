package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools 注册所有 MCP 工具
func SetupTools(srv *server.MCPServer, h *Handler) {
	srv.AddTool(mcp.Tool{
		Name:        "create_session",
		Description: "创建或复用沙盒会话",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
			},
			Required: []string{"user_id", "session_id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleCreateSession(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "exec",
		Description: "执行 Shell 命令",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"command":    map[string]string{"type": "string", "description": "Shell 命令"},
				"timeout_ms": map[string]any{"type": "number", "description": "超时毫秒", "default": float64(30000)},
			},
			Required: []string{"user_id", "session_id", "command"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleExec(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "write_file",
		Description: "写入文件",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"path":       map[string]string{"type": "string", "description": "文件路径"},
				"content":    map[string]string{"type": "string", "description": "文件内容"},
			},
			Required: []string{"user_id", "session_id", "path", "content"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleWriteFile(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "read_file",
		Description: "读取文件",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"path":       map[string]string{"type": "string", "description": "文件路径"},
			},
			Required: []string{"user_id", "session_id", "path"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleReadFile(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "close_session",
		Description: "关闭会话",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
			},
			Required: []string{"user_id", "session_id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleCloseSession(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "stats",
		Description: "服务统计",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleStats(ctx, request.GetArguments())
	})
}
