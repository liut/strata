package webapi

import (
	"encoding/json"
	"testing"
)

// TestWsMessageJSONMarshal 测试 wsMessage JSON 序列化 - 环境无关
func TestWsMessageJSONMarshal(t *testing.T) {
	msg := wsMessage{
		Type: "output",
		Data: "hello world",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// 检查序列化结果
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["type"] != "output" {
		t.Errorf("type = %q, want %q", decoded["type"], "output")
	}
	if decoded["data"] != "hello world" {
		t.Errorf("data = %q, want %q", decoded["data"], "hello world")
	}
}

// TestWsMessageJSONUnmarshal 测试 wsMessage JSON 反序列化 - 环境无关
func TestWsMessageJSONUnmarshal(t *testing.T) {
	jsonStr := `{"type":"input","data":"ls -la\n"}`

	var msg wsMessage
	err := json.Unmarshal([]byte(jsonStr), &msg)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if msg.Type != "input" {
		t.Errorf("Type = %q, want %q", msg.Type, "input")
	}
	if msg.Data != "ls -la\n" {
		t.Errorf("Data = %q, want %q", msg.Data, "ls -la\n")
	}
}

// TestWsMessageResize 测试 resize 消息 - 环境无关
func TestWsMessageResize(t *testing.T) {
	msg := wsMessage{
		Type: "resize",
		Rows: 40,
		Cols:  120,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["type"] != "resize" {
		t.Errorf("type = %v, want resize", decoded["type"])
	}
	if decoded["rows"].(float64) != 40 {
		t.Errorf("rows = %v, want 40", decoded["rows"])
	}
	if decoded["cols"].(float64) != 120 {
		t.Errorf("cols = %v, want 120", decoded["cols"])
	}
}

// TestWsMessageError 测试 error 消息 - 环境无关
func TestWsMessageError(t *testing.T) {
	msg := wsMessage{
		Type: "error",
		Data: "session closed",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded["type"] != "error" {
		t.Errorf("type = %q, want error", decoded["type"])
	}
	if decoded["data"] != "session closed" {
		t.Errorf("data = %q, want session closed", decoded["data"])
	}
}

// TestWsMessageEmptyData 测试空数据消息 - 环境无关
func TestWsMessageEmptyData(t *testing.T) {
	msg := wsMessage{
		Type: "output",
		Data: "",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Data 为空时不应该出现在 JSON 中（omitempty）
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := decoded["data"]; ok {
		t.Error("empty data should be omitted from JSON")
	}
}

// TestUpgraderConfig 测试 upgrader 配置 - 环境无关
func TestUpgraderConfig(t *testing.T) {
	// 测试 upgrader 字段存在
	if upgrader.ReadBufferSize != 4096 {
		t.Errorf("ReadBufferSize = %d, want 4096", upgrader.ReadBufferSize)
	}
	if upgrader.WriteBufferSize != 4096 {
		t.Errorf("WriteBufferSize = %d, want 4096", upgrader.WriteBufferSize)
	}

	// 测试 CheckOrigin 始终返回 true
	// 这个测试确保配置正确（允许所有来源）
	result := upgrader.CheckOrigin(nil)
	if !result {
		t.Error("CheckOrigin should return true for all origins")
	}
}

// TestExecRequestFields 测试 ExecRequest 字段 - 环境无关
func TestExecRequestFields(t *testing.T) {
	req := ExecRequest{
		Command:   "ls -la",
		TimeoutMs: 5000,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ExecRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Command != "ls -la" {
		t.Errorf("Command = %q, want %q", decoded.Command, "ls -la")
	}
	if decoded.TimeoutMs != 5000 {
		t.Errorf("TimeoutMs = %d, want %d", decoded.TimeoutMs, 5000)
	}
}

// TestExecResponseFields 测试 ExecResponse 字段 - 环境无关
func TestExecResponseFields(t *testing.T) {
	resp := ExecResponse{
		Output:    "total 0\ndrwxr-xr-x   2 root root 4096 Mar 17 00:00 .\n",
		Elapsed:   "10ms",
		Truncated: false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ExecResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Output != resp.Output {
		t.Errorf("Output = %q, want %q", decoded.Output, resp.Output)
	}
	if decoded.Elapsed != "10ms" {
		t.Errorf("Elapsed = %q, want %q", decoded.Elapsed, "10ms")
	}
	if decoded.Truncated != false {
		t.Errorf("Truncated = %v, want false", decoded.Truncated)
	}
}

// TestExecResponseTruncatedOmitzero 测试 Truncated 为 false 时使用 omitzero - 环境无关
func TestExecResponseTruncatedOmitzero(t *testing.T) {
	resp := ExecResponse{
		Output:   "test",
		Elapsed:  "5ms",
		// Truncated: false - 默认为 false
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// false 应该被省略（omitzero）
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := decoded["truncated"]; ok {
		t.Error("Truncated=false should be omitted from JSON with omitzero")
	}
}
