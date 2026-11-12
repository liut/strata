package mcp

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSetupToolsInventory(t *testing.T) {
	h := NewHandler(nil, "strata", "test", 30*time.Second, 45*time.Second)
	tools := h.mcps.ListTools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "edit,exec,read,write"; got != want {
		t.Fatalf("tools=%q, want %q", got, want)
	}

	read := tools["read"].Tool
	if read.Annotations.ReadOnlyHint == nil || !*read.Annotations.ReadOnlyHint {
		t.Fatal("read should be marked read-only")
	}
	for _, name := range []string{"write", "edit"} {
		tool := tools[name].Tool
		if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
			t.Fatalf("%s should be marked destructive", name)
		}
	}

	exec := tools["exec"].Tool
	if len(exec.InputSchema.Required) != 1 || exec.InputSchema.Required[0] != "command" {
		t.Fatalf("exec schema changed: %+v", exec.InputSchema)
	}
}

func TestFileToolsRejectMissingIdentity(t *testing.T) {
	h := NewHandler(nil, "strata", "test", 30*time.Second, 30*time.Second)
	tests := []struct {
		name string
		call func(context.Context, map[string]any) (*mcp.CallToolResult, error)
		args map[string]any
	}{
		{
			name: "read",
			call: h.handleRead,
			args: map[string]any{"path": "file.txt"},
		},
		{
			name: "write",
			call: h.handleWrite,
			args: map[string]any{"path": "file.txt", "content": "text"},
		},
		{
			name: "edit",
			call: h.handleEdit,
			args: map[string]any{
				"path":  "file.txt",
				"edits": []any{map[string]any{"oldText": "old", "newText": "new"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.call(context.Background(), tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError || !strings.Contains(resultText(result), "owner_id and session_id are required") {
				t.Fatalf("result=%+v text=%q", result, resultText(result))
			}
		})
	}
}

func TestEditValidationReturnsToolErrorBeforeSessionLookup(t *testing.T) {
	h := NewHandler(nil, "strata", "test", 30*time.Second, 30*time.Second)
	result, err := h.handleEdit(context.Background(), map[string]any{
		"path":  "file.txt",
		"edits": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "at least one replacement") {
		t.Fatalf("result=%+v text=%q", result, resultText(result))
	}
}
