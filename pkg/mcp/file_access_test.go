package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceResolvePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0644); err != nil {
		t.Fatal(err)
	}
	ws, err := openWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.close() }()

	relative, _, err := ws.resolvePath("notes.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	absolute, _, err := ws.resolvePath("/root/notes.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if relative != "notes.txt" || absolute != relative {
		t.Fatalf("relative=%q absolute=%q", relative, absolute)
	}

	missing, info, err := ws.resolvePath("new.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	if missing != "new.txt" || info != nil {
		t.Fatalf("missing=%q info=%v", missing, info)
	}
	if _, _, err := ws.resolvePath("missing/new.txt", true); err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestWorkspaceSymlinkContainment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.txt"), []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	ws, err := openWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.close() }()

	got, _, err := ws.resolvePath("link.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "target.txt" {
		t.Fatalf("resolved symlink to %q, want target.txt", got)
	}

	if err := os.Symlink(filepath.Join(dir, "target.txt"), filepath.Join(dir, "absolute-link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.resolvePath("absolute-link.txt", false); !errors.Is(err, errPathEscapes) {
		t.Fatalf("absolute symlink error=%v, want path escape", err)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "outside-link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.resolvePath("outside-link.txt", false); !errors.Is(err, errPathEscapes) {
		t.Fatalf("outside symlink error=%v, want path escape", err)
	}
}

func TestWorkspaceRejectsEscapes(t *testing.T) {
	ws, err := openWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.close() }()

	for _, name := range []string{"../outside.txt", "nested/../../outside.txt", "/tmp/outside.txt", "/rooted/file.txt"} {
		if _, _, err := ws.resolvePath(name, true); !errors.Is(err, errPathEscapes) {
			t.Errorf("%q error=%v, want path escape", name, err)
		}
	}
}

func TestWorkspaceTextValidation(t *testing.T) {
	dir := t.TempDir()
	ws, err := openWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.close() }()

	if err := os.Mkdir(filepath.Join(dir, "folder"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readTextFile("folder"); !errors.Is(err, errNotFile) {
		t.Fatalf("directory error=%v, want errNotFile", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "binary.txt"), []byte{'a', 0, 'b'}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readTextFile("binary.txt"); !errors.Is(err, errBinaryFile) {
		t.Fatalf("binary error=%v, want errBinaryFile", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "invalid.txt"), []byte{0xff, 0xfe}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readTextFile("invalid.txt"); !errors.Is(err, errInvalidUTF8) {
		t.Fatalf("invalid UTF-8 error=%v, want errInvalidUTF8", err)
	}

	large := strings.Repeat("x", maxTextFileSize+1)
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(large), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.readTextFile("large.txt"); !errors.Is(err, errFileTooLarge) {
		t.Fatalf("large error=%v, want errFileTooLarge", err)
	}
}

func TestWorkspaceReadsBOMAndLineEnding(t *testing.T) {
	dir := t.TempDir()
	ws, err := openWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.close() }()

	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	if err := os.WriteFile(filepath.Join(dir, "bom.txt"), data, 0644); err != nil {
		t.Fatal(err)
	}
	file, err := ws.readTextFile("bom.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !file.hasBOM || file.content != "one\r\ntwo\r\n" || file.lineEnding != "crlf" {
		t.Fatalf("file=%+v", file)
	}
	if string(renderTextFile(file.content, file.hasBOM)) != string(data) {
		t.Fatal("BOM round trip changed content")
	}
}

func TestKeyedLocker(t *testing.T) {
	locker := newKeyedLocker()
	ctx := context.Background()

	releaseA, err := locker.acquire(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}

	acquiredSame := make(chan struct{})
	go func() {
		release, err := locker.acquire(ctx, "a")
		if err == nil {
			close(acquiredSame)
			release()
		}
	}()
	select {
	case <-acquiredSame:
		t.Fatal("same key acquired concurrently")
	case <-time.After(30 * time.Millisecond):
	}

	releaseB, err := locker.acquire(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	releaseB()

	releaseA()
	select {
	case <-acquiredSame:
	case <-time.After(time.Second):
		t.Fatal("waiting lock was not released")
	}
}

func TestKeyedLockerCancellation(t *testing.T) {
	locker := newKeyedLocker()
	release, err := locker.acquire(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := locker.acquire(ctx, "a"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}
