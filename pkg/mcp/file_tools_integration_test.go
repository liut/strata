package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileToolsWriteReadEditRead(t *testing.T) {
	dir := t.TempDir()
	h := testFileHandler()

	writeResult := h.writeWorkspaceFile(context.Background(), dir, "notes/todo.md", "")
	if !writeResult.IsError {
		t.Fatal("write unexpectedly created a missing parent directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes")); !os.IsNotExist(err) {
		t.Fatalf("parent directory was created: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "notes"), 0755); err != nil {
		t.Fatal(err)
	}

	writeResult = h.writeWorkspaceFile(context.Background(), dir, "notes/todo.md", "- first\n- second\n")
	if writeResult.IsError {
		t.Fatalf("write failed: %s", resultText(writeResult))
	}
	readResult, err := readWorkspaceFile(dir, "notes/todo.md", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if readResult.IsError {
		t.Fatalf("read failed: %s", resultText(readResult))
	}
	oldText := resultText(readResult)

	editResult, err := h.editWorkspaceFile(context.Background(), dir, "/root/notes/todo.md", []editReplacement{
		{oldText: oldText, newText: "- first\n- second\n- third\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if editResult.IsError {
		t.Fatalf("edit failed: %s", resultText(editResult))
	}
	readResult, err = readWorkspaceFile(dir, "notes/todo.md", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(readResult); got != "- first\n- second\n- third\n" {
		t.Fatalf("final content=%q", got)
	}
}

func TestFileToolsRejectTraversalConsistently(t *testing.T) {
	dir := t.TempDir()
	h := testFileHandler()

	readResult, err := readWorkspaceFile(dir, "../outside.txt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if !readResult.IsError || !strings.Contains(resultText(readResult), "Path escapes session root") {
		t.Fatalf("read result=%q", resultText(readResult))
	}

	writeResult := h.writeWorkspaceFile(context.Background(), dir, "../outside.txt", "changed")
	if !writeResult.IsError || !strings.Contains(resultText(writeResult), "Path escapes session root") {
		t.Fatalf("write result=%q", resultText(writeResult))
	}

	editResult, err := h.editWorkspaceFile(context.Background(), dir, "../outside.txt", []editReplacement{
		{oldText: "old", newText: "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !editResult.IsError || !strings.Contains(resultText(editResult), "Path escapes session root") {
		t.Fatalf("edit result=%q", resultText(editResult))
	}
}

func TestFileToolsDuplicateMatchLeavesFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("same\nsame\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "same", newText: "changed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "Matching locations") {
		t.Fatalf("result=%q", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "same\nsame\n" {
		t.Fatalf("content=%q", data)
	}
}
