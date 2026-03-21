package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// ExecRequest 非交互式命令执行请求
type ExecRequest struct {
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms"` // 默认 30000
}

// ExecResponse 命令执行结果
type ExecResponse struct {
	Output    string `json:"output"`
	Elapsed   string `json:"elapsed"`
	Truncated bool   `json:"truncated,omitempty"`
}

// HandleExec 非交互式单次命令执行
//
//	POST /api/sessions/{uid}/{sid}/exec
//	Body: {"command":"ls -la","timeout_ms":5000}
func (h *handlerImpl) handleExec(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	sid := r.PathValue("sid")

	if uid == "" || sid == "" {
		jsonError(w, "uid and sid are required", http.StatusBadRequest)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Command == "" {
		jsonError(w, "command is required", http.StatusBadRequest)
		return
	}
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 30000
	}

	sess, err := h.manager.GetOrCreate(uid, sid)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	start := time.Now()
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond

	output, truncated, err := ExecInSession(sess, req.Command, timeout)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, ExecResponse{
		Output:    output,
		Elapsed:   time.Since(start).Round(time.Millisecond).String(),
		Truncated: truncated,
	})
}

// SessionExecuter 标识可执行命令的 session
type SessionExecuter interface {
	io.ReadWriter
	ID() string
}

// ExecInSession 在指定 session 中执行命令并返回输出
// 返回: output, 是否截断, error
func ExecInSession(sess SessionExecuter, cmd string, timeout time.Duration) (string, bool, error) {
	marker := fmt.Sprintf("__STRATA_EXEC_END_%s__", sess.ID())
	fullCmd := cmd + "; echo '" + marker + "'\n"

	if _, err := sess.Write([]byte(fullCmd)); err != nil {
		return "", false, fmt.Errorf("write to session failed: %w", err)
	}

	var buf bytes.Buffer
	readBuf := make([]byte, 4096)
	markerEnd := marker + "\r\n"
	deadline := time.Now().Add(timeout)
	const maxOutput = 1 << 20 // 1MB

	for {
		if time.Now().After(deadline) {
			slog.Info("timeout", "cmd", cmd)
			return "", false, fmt.Errorf("command timeout")
		}
		n, err := sess.Read(readBuf)
		if err != nil {
			return "", false, fmt.Errorf("read from session failed: %w", err)
		}
		buf.Write(readBuf[:n])

		if idx := bytes.Index(buf.Bytes(), []byte(markerEnd)); idx >= 0 {
			output := buf.Bytes()[:idx]
			output = stripEcho(output, fullCmd)
			truncated := len(output) > maxOutput
			if truncated {
				output = output[:maxOutput]
			}
			return string(output), truncated, nil
		}

		if buf.Len() > maxOutput*2 {
			// 防止内存爆炸：截断 buffer，保留尾部（可能包含 marker）
			tail := buf.Bytes()[buf.Len()-4096:]
			buf.Reset()
			buf.Write(tail)
		}
	}
}

// stripEcho 去掉终端回显的输入行（PTY 会将写入内容回显给读端）
func stripEcho(output []byte, cmd string) []byte {
	// 找第一个换行符，越过回显行
	if idx := bytes.IndexByte(output, '\n'); idx >= 0 {
		output = bytes.TrimLeft(output[idx+1:], "\r\n")
	}
	// 去掉末尾的空白字符
	return bytes.TrimRight(output, " \t\r\n")
}
