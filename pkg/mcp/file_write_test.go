package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testFileHandler() *Handler {
	return &Handler{
		fileLocks:   newKeyedLocker(),
		editTimeout: time.Second,
	}
}

func TestWriteWorkspaceFileCreatesFile(t *testing.T) {
	dir := t.TempDir()
	h := testFileHandler()
	result := h.writeWorkspaceFile(context.Background(), dir, "new.txt", "hello\n")
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("content=%q", data)
	}
	details := decodeStructured[writeFileDetails](t, result)
	if !details.Created || !details.Changed || details.BytesWritten != 6 {
		t.Fatalf("details=%+v", details)
	}
}

func TestWriteWorkspaceFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h := testFileHandler()

	result := h.writeWorkspaceFile(context.Background(), dir, "file.txt", "new\n")
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("content=%q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o, want 600", info.Mode().Perm())
	}
	details := decodeStructured[writeFileDetails](t, result)
	if details.Created || !details.Changed || details.Diff == "" || details.Patch == "" {
		t.Fatalf("details=%+v", details)
	}

	result = h.writeWorkspaceFile(context.Background(), dir, "file.txt", "new\n")
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	details = decodeStructured[writeFileDetails](t, result)
	if details.Changed {
		t.Fatalf("identical write reported change: %+v", details)
	}
}

func TestWriteWorkspaceFileEmptyContentTruncates(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	result := testFileHandler().writeWorkspaceFile(context.Background(), dir, "file.txt", "")
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("content=%q", data)
	}
}

func TestWriteWorkspaceFileErrorsLeaveContentIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	h := testFileHandler()

	tests := []struct {
		name string
		path string
	}{
		{name: "missing parent", path: "missing/file.txt"},
		{name: "directory", path: "."},
		{name: "escape", path: "../outside.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.writeWorkspaceFile(context.Background(), dir, tt.path, "changed")
			if !result.IsError {
				t.Fatal("expected tool error")
			}
		})
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("content changed to %q", data)
	}
}

func TestWriteWorkspaceFileUpdatesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}

	result := testFileHandler().writeWorkspaceFile(context.Background(), dir, "link.txt", "new")
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	target, err := os.ReadFile(filepath.Join(dir, "target.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(target) != "new" {
		t.Fatalf("target=%q", target)
	}
	linkTarget, err := os.Readlink(filepath.Join(dir, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if linkTarget != "target.txt" {
		t.Fatalf("link target=%q", linkTarget)
	}
}

func TestWriteWorkspaceFileCancellationLeavesFileIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := testFileHandler().writeWorkspaceFile(ctx, dir, "file.txt", "changed")
	if !result.IsError {
		t.Fatal("expected canceled write to fail")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("content=%q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".strata-tmp-") {
			t.Fatalf("temporary file leaked: %s", entry.Name())
		}
	}
}

func TestValidateTextContent(t *testing.T) {
	if err := validateTextContent("valid\n"); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"\xff", "a\x00b", strings.Repeat("x", maxTextFileSize+1)} {
		if err := validateTextContent(content); err == nil {
			t.Errorf("content of length %d should be rejected", len(content))
		}
	}
}
