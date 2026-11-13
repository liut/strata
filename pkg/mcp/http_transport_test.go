package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type rpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Result  map[string]any `json:"result"`
	Error   *rpcError      `json:"error"`
}

type rpcError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func newMCPHTTPTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := NewHandler(nil, "strata", "test", 30*time.Second, 30*time.Second)
	mux := http.NewServeMux()
	handler.Route(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func modernRequestMeta() map[string]any {
	return map[string]any{
		mcp.MetaKeyProtocolVersion: mcp.ProtocolVersion20260728,
		mcp.MetaKeyClientInfo: map[string]any{
			"name":    "strata-test",
			"version": "1.0.0",
		},
		mcp.MetaKeyClientCapabilities: map[string]any{},
	}
}

func postModernRPC(
	t *testing.T,
	srv *httptest.Server,
	method mcp.MCPMethod,
	params map[string]any,
	mutate func(*http.Request),
) *http.Response {
	t.Helper()

	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = modernRequestMeta()
	return postRPC(t, srv, method, params, mutate)
}

func postRPC(
	t *testing.T,
	srv *httptest.Server,
	method mcp.MCPMethod,
	params map[string]any,
	mutate func(*http.Request),
) *http.Response {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/mcp/", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(mcp.HeaderProtocolVersion, mcp.ProtocolVersion20260728)
	req.Header.Set(mcp.HeaderMethod, string(method))

	rawParams, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := mcp.ExtractHeaderName(method, rawParams); ok {
		encoded, ok := mcp.EncodeHeaderValue(name)
		if !ok {
			t.Fatalf("encode %s header value %q", mcp.HeaderName, name)
		}
		req.Header.Set(mcp.HeaderName, encoded)
	}
	if mutate != nil {
		mutate(req)
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func postLegacyRPC(
	t *testing.T,
	srv *httptest.Server,
	method mcp.MCPMethod,
	params map[string]any,
	sessionID string,
) *http.Response {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/mcp/", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set(mcp.HeaderSessionID, sessionID)
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeRPCResponse(t *testing.T, resp *http.Response) rpcResponse {
	t.Helper()

	var message rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&message); err != nil {
		t.Fatal(err)
	}
	return message
}

func rpcToolNames(t *testing.T, result map[string]any) string {
	t.Helper()

	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools=%T, want array", result["tools"])
	}
	names := make([]string, 0, len(tools))
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			t.Fatalf("tool=%T, want object", rawTool)
		}
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func TestModernHTTPDiscover(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	resp := postModernRPC(t, srv, mcp.MethodServerDiscover, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get(mcp.HeaderSessionID); got != "" {
		t.Fatalf("%s=%q, want empty", mcp.HeaderSessionID, got)
	}

	message := decodeRPCResponse(t, resp)
	if message.Error != nil {
		t.Fatalf("unexpected error: %+v", message.Error)
	}
	if got := message.Result["resultType"]; got != string(mcp.ResultTypeComplete) {
		t.Fatalf("resultType=%v, want %q", got, mcp.ResultTypeComplete)
	}

	versions, ok := message.Result["supportedVersions"].([]any)
	if !ok || len(versions) == 0 || versions[0] != mcp.ProtocolVersion20260728 {
		t.Fatalf("supportedVersions=%v, want %q first", versions, mcp.ProtocolVersion20260728)
	}

	meta, ok := message.Result["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("result _meta=%T, want object", message.Result["_meta"])
	}
	serverInfo, ok := meta[mcp.MetaKeyServerInfo].(map[string]any)
	if !ok || serverInfo["name"] != "strata" {
		t.Fatalf("serverInfo=%v, want Strata identity", meta[mcp.MetaKeyServerInfo])
	}
}

func TestModernHTTPToolsListWithoutInitialize(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	resp := postModernRPC(t, srv, mcp.MethodToolsList, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	if resp.Request.URL.Path != "/mcp/" {
		t.Fatalf("request path=%q, want registered /mcp/ endpoint", resp.Request.URL.Path)
	}
	if got := resp.Header.Get(mcp.HeaderSessionID); got != "" {
		t.Fatalf("%s=%q, want empty", mcp.HeaderSessionID, got)
	}

	message := decodeRPCResponse(t, resp)
	if message.Error != nil {
		t.Fatalf("unexpected error: %+v", message.Error)
	}
	if got := message.Result["resultType"]; got != string(mcp.ResultTypeComplete) {
		t.Fatalf("resultType=%v, want %q", got, mcp.ResultTypeComplete)
	}
	if _, ok := message.Result["ttlMs"]; !ok {
		t.Fatal("tools/list result is missing ttlMs")
	}
	if got := message.Result["cacheScope"]; got != string(mcp.CacheScopePrivate) {
		t.Fatalf("cacheScope=%v, want %q", got, mcp.CacheScopePrivate)
	}

	if got, want := rpcToolNames(t, message.Result), "edit,exec,read,write"; got != want {
		t.Fatalf("tools=%q, want %q", got, want)
	}
}

func TestModernHTTPToolCallReturnsToolError(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	resp := postModernRPC(t, srv, mcp.MethodToolsCall, map[string]any{
		"name": "edit",
		"arguments": map[string]any{
			"path":  "file.txt",
			"edits": []any{},
		},
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}

	message := decodeRPCResponse(t, resp)
	if message.Error != nil {
		t.Fatalf("unexpected protocol error: %+v", message.Error)
	}
	if got := message.Result["resultType"]; got != string(mcp.ResultTypeComplete) {
		t.Fatalf("resultType=%v, want %q", got, mcp.ResultTypeComplete)
	}
	if isError, _ := message.Result["isError"].(bool); !isError {
		t.Fatalf("isError=%v, want true", message.Result["isError"])
	}
	content, ok := message.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("content=%v, want tool error content", message.Result["content"])
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "at least one replacement") {
		t.Fatalf("tool error=%q, want validation message", text)
	}
}

func TestModernHTTPIgnoresCallerSessionID(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	resp := postModernRPC(t, srv, mcp.MethodToolsList, nil, func(req *http.Request) {
		req.Header.Set(mcp.HeaderSessionID, "caller-chosen-session")
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get(mcp.HeaderSessionID); got != "" {
		t.Fatalf("%s=%q, want no minted or echoed session", mcp.HeaderSessionID, got)
	}
}

func TestModernHTTPRejectsHeaderMismatch(t *testing.T) {
	srv := newMCPHTTPTestServer(t)
	params := map[string]any{
		"name":      "read",
		"arguments": map[string]any{"path": "file.txt"},
	}

	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{
			name: "missing method",
			mutate: func(req *http.Request) {
				req.Header.Del(mcp.HeaderMethod)
			},
		},
		{
			name: "method mismatch",
			mutate: func(req *http.Request) {
				req.Header.Set(mcp.HeaderMethod, string(mcp.MethodToolsList))
			},
		},
		{
			name: "missing name",
			mutate: func(req *http.Request) {
				req.Header.Del(mcp.HeaderName)
			},
		},
		{
			name: "name mismatch",
			mutate: func(req *http.Request) {
				req.Header.Set(mcp.HeaderName, "write")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postModernRPC(t, srv, mcp.MethodToolsCall, params, tt.mutate)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
			message := decodeRPCResponse(t, resp)
			if message.Error == nil || message.Error.Code != mcp.HEADER_MISMATCH {
				t.Fatalf("error=%+v, want header mismatch", message.Error)
			}
		})
	}
}

func TestModernHTTPRejectsUnsupportedProtocolVersion(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	const requested = "2027-01-01"
	params := map[string]any{
		"_meta": map[string]any{
			mcp.MetaKeyProtocolVersion:    requested,
			mcp.MetaKeyClientCapabilities: map[string]any{},
		},
	}
	resp := postRPC(t, srv, mcp.MethodToolsList, params, func(req *http.Request) {
		req.Header.Set(mcp.HeaderProtocolVersion, requested)
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	message := decodeRPCResponse(t, resp)
	if message.Error == nil || message.Error.Code != mcp.UNSUPPORTED_PROTOCOL_VERSION {
		t.Fatalf("error=%+v, want unsupported protocol version", message.Error)
	}
	if got := message.Error.Data["requested"]; got != requested {
		t.Fatalf("requested=%v, want %q", got, requested)
	}
	supported, ok := message.Error.Data["supported"].([]any)
	if !ok || len(supported) == 0 || supported[0] != mcp.ProtocolVersion20260728 {
		t.Fatalf("supported=%v, want modern version first", message.Error.Data["supported"])
	}
}

func TestModernHTTPRejectsGetAndDelete(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+"/mcp/", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(mcp.HeaderProtocolVersion, mcp.ProtocolVersion20260728)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
			}
			if got := resp.Header.Get("Allow"); got != http.MethodPost {
				t.Fatalf("Allow=%q, want %q", got, http.MethodPost)
			}
		})
	}
}

func TestModernHTTPRejectsLastEventID(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	resp := postModernRPC(t, srv, mcp.MethodToolsList, nil, func(req *http.Request) {
		req.Header.Set(mcp.HeaderLastEventID, "42")
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	message := decodeRPCResponse(t, resp)
	if message.Error == nil || message.Error.Code != mcp.HEADER_MISMATCH {
		t.Fatalf("error=%+v, want header mismatch", message.Error)
	}
}

func TestLegacyHTTPInitializeAndToolsList(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	initResp := postLegacyRPC(t, srv, mcp.MethodInitialize, map[string]any{
		"protocolVersion": mcp.ProtocolVersion20251125,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "legacy-test",
			"version": "1.0.0",
		},
	}, "")
	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status=%d, want %d", initResp.StatusCode, http.StatusOK)
	}
	sessionID := initResp.Header.Get(mcp.HeaderSessionID)
	if sessionID == "" {
		t.Fatalf("%s is empty after legacy initialize", mcp.HeaderSessionID)
	}

	initMessage := decodeRPCResponse(t, initResp)
	if initMessage.Error != nil {
		t.Fatalf("initialize error: %+v", initMessage.Error)
	}
	if got := initMessage.Result["protocolVersion"]; got != mcp.ProtocolVersion20251125 {
		t.Fatalf("protocolVersion=%v, want %q", got, mcp.ProtocolVersion20251125)
	}
	if _, ok := initMessage.Result["resultType"]; ok {
		t.Fatal("legacy initialize unexpectedly returned modern resultType")
	}

	listResp := postLegacyRPC(t, srv, mcp.MethodToolsList, nil, sessionID)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list status=%d, want %d", listResp.StatusCode, http.StatusOK)
	}
	listMessage := decodeRPCResponse(t, listResp)
	if listMessage.Error != nil {
		t.Fatalf("tools/list error: %+v", listMessage.Error)
	}
	if got, want := rpcToolNames(t, listMessage.Result), "edit,exec,read,write"; got != want {
		t.Fatalf("legacy tools=%q, want %q", got, want)
	}
}

func TestModernHTTPIsStatelessAfterLegacyInitialize(t *testing.T) {
	srv := newMCPHTTPTestServer(t)

	initResp := postLegacyRPC(t, srv, mcp.MethodInitialize, map[string]any{
		"protocolVersion": mcp.ProtocolVersion20251125,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "legacy-test",
			"version": "1.0.0",
		},
	}, "")
	legacySessionID := initResp.Header.Get(mcp.HeaderSessionID)
	if legacySessionID == "" {
		t.Fatalf("%s is empty after legacy initialize", mcp.HeaderSessionID)
	}
	_ = decodeRPCResponse(t, initResp)

	resp := postModernRPC(t, srv, mcp.MethodToolsList, nil, func(req *http.Request) {
		req.Header.Set(mcp.HeaderSessionID, legacySessionID)
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("modern tools/list status=%d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get(mcp.HeaderSessionID); got != "" {
		t.Fatalf("%s=%q, want modern response without transport session", mcp.HeaderSessionID, got)
	}

	message := decodeRPCResponse(t, resp)
	if message.Error != nil {
		t.Fatalf("modern tools/list error: %+v", message.Error)
	}
	if got := message.Result["resultType"]; got != string(mcp.ResultTypeComplete) {
		t.Fatalf("resultType=%v, want %q", got, mcp.ResultTypeComplete)
	}
	if got, want := rpcToolNames(t, message.Result), "edit,exec,read,write"; got != want {
		t.Fatalf("modern tools=%q, want %q", got, want)
	}
}
