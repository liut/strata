package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestReadWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := readWorkspaceFile(dir, "notes.txt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(result))
	}
	if got := resultText(result); got != "one\ntwo\nthree\n" {
		t.Fatalf("content=%q", got)
	}
	details := decodeStructured[readFileDetails](t, result)
	if details.StartLine != 1 || details.EndLine != 3 || details.TotalLines != 3 || details.LineEnding != "lf" {
		t.Fatalf("details=%+v", details)
	}
}

func TestReadWorkspaceFileRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("one\ntwo\nthree"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := readWorkspaceFile(dir, "notes.txt", 2, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(result); got != "two\n" {
		t.Fatalf("content=%q", got)
	}
	details := decodeStructured[readFileDetails](t, result)
	if details.StartLine != 2 || details.EndLine != 2 || details.TotalLines != 3 {
		t.Fatalf("details=%+v", details)
	}

	result, err = readWorkspaceFile(dir, "notes.txt", 4, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(result); got != "" {
		t.Fatalf("EOF content=%q", got)
	}
}

func TestReadWorkspaceFileBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := readWorkspaceFile(dir, "/root/notes.txt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(result); got != "one\r\ntwo\r\n" {
		t.Fatalf("content=%q", got)
	}
	details := decodeStructured[readFileDetails](t, result)
	if !details.HasBOM || details.LineEnding != "crlf" || details.TotalLines != 2 {
		t.Fatalf("details=%+v", details)
	}
}

func TestReadWorkspaceFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.txt"), []byte{0, 1}, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "missing", path: "missing.txt"},
		{name: "directory", path: "folder"},
		{name: "binary", path: "binary.txt"},
		{name: "escape", path: "../outside.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := readWorkspaceFile(dir, tt.path, 0, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("expected tool error")
			}
		})
	}
}

func TestPositiveIntArgument(t *testing.T) {
	value, set, err := positiveIntArgument(map[string]any{"start_line": float64(2)}, "start_line")
	if err != nil || !set || value != 2 {
		t.Fatalf("value=%d set=%v err=%v", value, set, err)
	}
	for _, raw := range []any{float64(0), float64(-1), 1.5, "2"} {
		if _, _, err := positiveIntArgument(map[string]any{"start_line": raw}, "start_line"); err == nil {
			t.Errorf("%v should be rejected", raw)
		}
	}
}

func resultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	text, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		return ""
	}
	return text.Text
}

func decodeStructured[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	var target T
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &target); err != nil {
		t.Fatal(err)
	}
	return target
}
