package webapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsMessage 是 WS 消息的统一结构
type wsMessage struct {
	Type string `json:"type"` // "input" | "resize" | "output" | "error"
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// HandleShellWS 处理交互式 Shell WebSocket 连接
//
//	GET /api/ws/{uid}/{sid}/shell
//
//	客户端 → 服务端消息：
//	  {"type":"input",  "data":"ls -la\n"}
//	  {"type":"resize", "rows":40, "cols":120}
//
//	服务端 → 客户端消息：
//	  {"type":"output", "data":"<shell output>"}
//	  {"type":"error",  "data":"session closed"}
func (h *handlerImpl) handleShellWS(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("uid")
	sessID := r.PathValue("sid")
	if userID == "" || sessID == "" {
		http.Error(w, "uid and sid are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade error", "error", err)
		return
	}
	defer conn.Close()

	sess, err := h.manager.GetOrCreate(userID, sessID)
	if err != nil {
		_ = sendWSMsg(conn, wsMessage{Type: "error", Data: err.Error()})
		return
	}

	// Shell 输出 → WebSocket（独立 goroutine）
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.Read(buf)
			if err != nil {
				_ = sendWSMsg(conn, wsMessage{Type: "error", Data: "session closed"})
				conn.Close()
				return
			}
			if err := sendWSMsg(conn, wsMessage{Type: "output", Data: string(buf[:n])}); err != nil {
				return
			}
		}
	}()

	// WebSocket 输入 → Shell
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break // 客户端断开
		}

		var msg wsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "input":
			if _, err := sess.Write([]byte(msg.Data)); err != nil {
				return
			}
		case "resize":
			_ = sess.Resize(msg.Rows, msg.Cols)
		}
	}
}

func sendWSMsg(conn *websocket.Conn, msg wsMessage) error {
	data, _ := json.Marshal(msg)
	return conn.WriteMessage(websocket.TextMessage, data)
}
