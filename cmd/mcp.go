package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var (
	mcpAddr   string
	mcpAPI    string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run MCP server for AI agents",
	RunE:  runMCP,
}

func init() {
	mcpCmd.Flags().StringVar(&mcpAddr, "addr", ":8081", "MCP server listen address")
	mcpCmd.Flags().StringVar(&mcpAPI, "api", "http://localhost:8080", "Strata API endpoint")
}

// Session 缓存已创建的 session
type Session struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	CreatedAt string `json:"created_at,omitempty"`
}

type MCPServer struct {
	API     string
	sessions map[string]*Session
}

func NewMCPServer(api string) *MCPServer {
	return &MCPServer{
		API:      api,
		sessions: make(map[string]*Session),
	}
}

func (s *MCPServer) apiCall(ctx context.Context, path string, body any) (*http.Response, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.API+path, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

func (s *MCPServer) handleCreateSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)

	if userID == "" || sessionID == "" {
		return mcp.NewToolResultError("user_id and session_id are required"), nil
	}

	key := userID + ":" + sessionID

	if _, exists := s.sessions[key]; !exists {
		resp, err := s.apiCall(ctx, "/api/sessions", map[string]string{
			"user_id":    userID,
			"session_id": sessionID,
		})
		if err != nil {
			return mcp.NewToolResultError("API call failed: " + err.Error()), nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return mcp.NewToolResultError("API returned status: " + resp.Status), nil
		}

		var sess Session
		if err := json.NewDecoder(resp.Body).Decode(&sess); err != nil {
			return mcp.NewToolResultError("Failed to parse response: " + err.Error()), nil
		}
		s.sessions[key] = &sess
	}

	sm := s.sessions[key]
	return mcp.NewToolResultText(fmt.Sprintf("Session created/retrieved: %s/%s", sm.UserID, sm.SessionID)), nil
}

func (s *MCPServer) handleExec(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	command, _ := args["command"].(string)
	timeoutMs, _ := args["timeout_ms"].(float64)

	if userID == "" || sessionID == "" || command == "" {
		return mcp.NewToolResultError("user_id, session_id and command are required"), nil
	}

	// 确保 session 存在
	key := userID + ":" + sessionID
	if _, exists := s.sessions[key]; !exists {
		s.handleCreateSession(ctx, map[string]any{"user_id": userID, "session_id": sessionID})
	}

	timeout := 30000
	if timeoutMs > 0 {
		timeout = int(timeoutMs)
	}

	body := map[string]any{
		"user_id":     userID,
		"session_id":  sessionID,
		"command":     command,
		"timeout_ms":  timeout,
	}

	resp, err := s.apiCall(ctx, "/api/exec", body)
	if err != nil {
		return mcp.NewToolResultError("API call failed: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError("API returned status: " + resp.Status), nil
	}

	var result struct {
		Output string `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return mcp.NewToolResultError("Failed to parse response: " + err.Error()), nil
	}

	output := result.Output
	if output == "" {
		output = "(empty output)"
	}
	return mcp.NewToolResultText(output), nil
}

func (s *MCPServer) handleWriteFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)

	if userID == "" || sessionID == "" || path == "" {
		return mcp.NewToolResultError("user_id, session_id and path are required"), nil
	}

	// 使用 cat heredoc 方式写入
	cmd := fmt.Sprintf("cat > '%s' << 'STRATA_EOF'\n%s\nSTRATA_EOF", path, content)
	return s.handleExec(ctx, map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"command":    cmd,
	})
}

func (s *MCPServer) handleReadFile(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)
	path, _ := args["path"].(string)

	if userID == "" || sessionID == "" || path == "" {
		return mcp.NewToolResultError("user_id, session_id and path are required"), nil
	}

	cmd := fmt.Sprintf("cat %s", path)
	return s.handleExec(ctx, map[string]any{
		"user_id":    userID,
		"session_id": sessionID,
		"command":    cmd,
	})
}

func (s *MCPServer) handleCloseSession(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	userID, _ := args["user_id"].(string)
	sessionID, _ := args["session_id"].(string)

	if userID == "" || sessionID == "" {
		return mcp.NewToolResultError("user_id and session_id are required"), nil
	}

	key := userID + ":" + sessionID

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/sessions/%s/%s", s.API, userID, sessionID), nil)
	if err != nil {
		return mcp.NewToolResultError("Failed to create request: " + err.Error()), nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mcp.NewToolResultError("API call failed: " + err.Error()), nil
	}
	defer resp.Body.Close()

	delete(s.sessions, key)
	return mcp.NewToolResultText(fmt.Sprintf("Session %s closed", key)), nil
}

func (s *MCPServer) handleStats(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.API+"/api/stats", nil)
	if err != nil {
		return mcp.NewToolResultError("Failed to create request: " + err.Error()), nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mcp.NewToolResultError("API call failed: " + err.Error()), nil
	}
	defer resp.Body.Close()

	var stats any
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return mcp.NewToolResultError("Failed to parse response: " + err.Error()), nil
	}

	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	return mcp.NewToolResultText(string(statsJSON)), nil
}

func runMCP(cmd *cobra.Command, args []string) error {
	mcpServer := NewMCPServer(mcpAPI)

	srv := server.NewMCPServer("strata", version)

	// 添加工具
	srv.AddTool(mcp.Tool{
		Name:        "create_session",
		Description: "创建或复用一个新的沙盒会话。每个用户可以有多个会话，但通常一个会话对应一次任务。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识，如 'alice'"},
				"session_id": map[string]string{"type": "string", "description": "会话标识，如 'task-001'"},
			},
			Required: []string{"user_id", "session_id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleCreateSession(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "exec",
		Description: "在指定会话中执行一条 Shell 命令，返回命令输出。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"command":    map[string]string{"type": "string", "description": "要执行的 Shell 命令"},
				"timeout_ms": map[string]any{"type": "number", "description": "超时毫秒数，默认 30000", "default": float64(30000)},
			},
			Required: []string{"user_id", "session_id", "command"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleExec(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "write_file",
		Description: "在沙盒会话的文件系统中创建或覆写文件。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"path":       map[string]string{"type": "string", "description": "目标文件路径，如 '/tmp/test.py'"},
				"content":    map[string]string{"type": "string", "description": "文件内容"},
			},
			Required: []string{"user_id", "session_id", "path", "content"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleWriteFile(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "read_file",
		Description: "读取沙盒会话中的文件内容。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
				"path":       map[string]string{"type": "string", "description": "要读取的文件路径"},
			},
			Required: []string{"user_id", "session_id", "path"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleReadFile(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "close_session",
		Description: "关闭并清理一个沙盒会话，释放资源。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"user_id":    map[string]string{"type": "string", "description": "用户标识"},
				"session_id": map[string]string{"type": "string", "description": "会话标识"},
			},
			Required: []string{"user_id", "session_id"},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleCloseSession(ctx, request.GetArguments())
	})

	srv.AddTool(mcp.Tool{
		Name:        "stats",
		Description: "查询当前服务状态（活跃会话数等）。",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{},
		},
	}, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpServer.handleStats(ctx, request.GetArguments())
	})

	// 使用 stdio 模式启动服务器
	fmt.Fprintln(os.Stderr, "Starting Strata MCP server...")
	fmt.Fprintln(os.Stderr, "Strata API:", mcpAPI)
	return server.ServeStdio(srv)
}
