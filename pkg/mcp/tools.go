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
				"timeout_ms": map[string]any{"type": "number", "description": "Timeout in milliseconds", "default": float64(h.shellTimeout.Milliseconds())},
			},
			Required: []string{"command"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleExec(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "read",
		Description: "Read UTF-8 text from the session workspace. Relative paths resolve from /root; absolute paths must be under /root.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"owner_id":   map[string]string{"type": "string", "description": "User identifier"},
				"session_id": map[string]string{"type": "string", "description": "Session identifier"},
				"path":       map[string]string{"type": "string", "description": "File path relative to /root, or an absolute path under /root"},
				"start_line": map[string]any{"type": "integer", "minimum": 1, "description": "Optional 1-based first line"},
				"line_count": map[string]any{"type": "integer", "minimum": 1, "description": "Optional number of lines to return"},
			},
			Required: []string{"path"},
		},
		Annotations: mcp.ToolAnnotation{
			Title:        "Read workspace file",
			ReadOnlyHint: boolPointer(true),
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleRead(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "write",
		Description: "Create or atomically replace a UTF-8 text file in the session workspace. Parent directories must already exist.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"owner_id":   map[string]string{"type": "string", "description": "User identifier"},
				"session_id": map[string]string{"type": "string", "description": "Session identifier"},
				"path":       map[string]string{"type": "string", "description": "File path relative to /root, or an absolute path under /root"},
				"content":    map[string]string{"type": "string", "description": "Complete UTF-8 file content"},
			},
			Required: []string{"path", "content"},
		},
		Annotations: mcp.ToolAnnotation{
			Title:           "Write workspace file",
			DestructiveHint: boolPointer(true),
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleWrite(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "edit",
		Description: "Apply one or more unique, non-overlapping exact text replacements to a workspace file. Copy oldText directly from read; do not include line-number prefixes.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"owner_id":   map[string]string{"type": "string", "description": "User identifier"},
				"session_id": map[string]string{"type": "string", "description": "Session identifier"},
				"path":       map[string]string{"type": "string", "description": "File path relative to /root, or an absolute path under /root"},
				"edits": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"oldText": map[string]string{"type": "string", "description": "Unique exact source text"},
							"newText": map[string]string{"type": "string", "description": "Replacement text; empty deletes the match"},
						},
						"required": []string{"oldText", "newText"},
					},
				},
			},
			Required: []string{"path", "edits"},
		},
		Annotations: mcp.ToolAnnotation{
			Title:           "Edit workspace file",
			DestructiveHint: boolPointer(true),
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return h.handleEdit(ctx, request.GetArguments())
	})
}

func boolPointer(value bool) *bool {
	return &value
}
