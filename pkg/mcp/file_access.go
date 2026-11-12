package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxTextFileSize = 4 << 20

var (
	errFileNotFound = errors.New("file not found")
	errNotFile      = errors.New("path is not a regular file")
	errFileTooLarge = errors.New("file exceeds 4 MiB limit")
	errInvalidUTF8  = errors.New("file is not valid UTF-8")
	errBinaryFile   = errors.New("file contains binary data")
)

type workspace struct {
	root    *os.Root
	rootDir string
}

func openWorkspace(dir string) (*workspace, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open workspace: %w", err)
	}
	absDir, err := filepath.Abs(root.Name())
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	return &workspace{root: root, rootDir: resolvedDir}, nil
}

func (w *workspace) close() error {
	return w.root.Close()
}

func (w *workspace) resolvePath(name string, allowMissing bool) (string, os.FileInfo, error) {
	rel, err := workspaceRelativePath(name)
	if err != nil {
		return "", nil, err
	}
	if err := w.rejectAbsoluteSymlinks(rel); err != nil {
		return "", nil, err
	}

	hostTarget := filepath.Join(w.rootDir, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(hostTarget)
	if err != nil {
		if !allowMissing || !errors.Is(err, fs.ErrNotExist) {
			if errors.Is(err, fs.ErrNotExist) {
				return "", nil, errFileNotFound
			}
			return "", nil, fmt.Errorf("resolve path %s: %w", name, err)
		}

		parent := filepath.Dir(hostTarget)
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr != nil {
			if errors.Is(parentErr, fs.ErrNotExist) {
				return "", nil, fmt.Errorf("parent directory does not exist for %s", name)
			}
			return "", nil, fmt.Errorf("resolve parent of %s: %w", name, parentErr)
		}
		resolved = filepath.Join(resolvedParent, filepath.Base(hostTarget))

		canonical, canonicalErr := w.rootRelative(resolved)
		if canonicalErr != nil {
			return "", nil, canonicalErr
		}
		if _, lstatErr := w.root.Lstat(canonical); lstatErr == nil {
			return "", nil, fmt.Errorf("path is a broken symbolic link: %s", name)
		} else if !errors.Is(lstatErr, fs.ErrNotExist) {
			return "", nil, fmt.Errorf("inspect path %s: %w", name, lstatErr)
		}
		return canonical, nil, nil
	}

	canonical, err := w.rootRelative(resolved)
	if err != nil {
		return "", nil, err
	}
	info, err := w.root.Stat(canonical)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil, errFileNotFound
		}
		return "", nil, fmt.Errorf("stat path %s: %w", name, err)
	}
	return canonical, info, nil
}

func (w *workspace) rootRelative(hostPath string) (string, error) {
	rel, err := filepath.Rel(w.rootDir, hostPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errPathEscapes
	}
	return filepath.ToSlash(rel), nil
}

func (w *workspace) rejectAbsoluteSymlinks(rel string) error {
	if rel == "." {
		return nil
	}

	current := "."
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." {
			continue
		}
		current = path.Join(current, part)
		info, err := w.root.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("inspect path %s: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := w.root.Readlink(current)
		if err != nil {
			return fmt.Errorf("read symbolic link %s: %w", current, err)
		}
		if filepath.IsAbs(target) || path.IsAbs(target) {
			return errPathEscapes
		}
	}
	return nil
}

func workspaceRelativePath(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("path must not be empty")
	}

	if path.IsAbs(name) {
		clean := path.Clean(name)
		if clean == "/root" {
			return ".", nil
		}
		if !strings.HasPrefix(clean, "/root/") {
			return "", errPathEscapes
		}
		return strings.TrimPrefix(clean, "/root/"), nil
	}

	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", errPathEscapes
	}
	return clean, nil
}

var errPathEscapes = errors.New("path escapes session root")

func pathEscapeMessage(name string) string {
	return fmt.Sprintf("Path escapes session root: %s.", name)
}

type textFile struct {
	content     string
	hasBOM      bool
	lineEnding  string
	originalRaw []byte
}

func (w *workspace) readTextFile(rel string) (*textFile, error) {
	f, err := w.root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errFileNotFound
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errNotFile
	}
	if info.Size() > maxTextFileSize {
		return nil, errFileTooLarge
	}

	data, err := io.ReadAll(io.LimitReader(f, maxTextFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTextFileSize {
		return nil, errFileTooLarge
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, errBinaryFile
	}
	if !utf8.Valid(data) {
		return nil, errInvalidUTF8
	}

	hasBOM := bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if hasBOM {
		data = data[3:]
	}
	return &textFile{
		content:     string(data),
		hasBOM:      hasBOM,
		lineEnding:  detectLineEnding(string(data)),
		originalRaw: append([]byte(nil), data...),
	}, nil
}

func detectLineEnding(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		if idx > 0 && text[idx-1] == '\r' {
			return "crlf"
		}
		return "lf"
	}
	if strings.ContainsRune(text, '\r') {
		return "crlf"
	}
	return ""
}

func renderTextFile(content string, hasBOM bool) []byte {
	data := []byte(content)
	if !hasBOM {
		return data
	}
	result := make([]byte, 0, 3+len(data))
	result = append(result, 0xEF, 0xBB, 0xBF)
	return append(result, data...)
}

func validateTextContent(content string) error {
	if len(content) > maxTextFileSize {
		return errFileTooLarge
	}
	if strings.IndexByte(content, 0) >= 0 {
		return errBinaryFile
	}
	if !utf8.ValidString(content) {
		return errInvalidUTF8
	}
	return nil
}

func (w *workspace) atomicReplace(ctx context.Context, rel string, data []byte, mode os.FileMode, preserveMode bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := path.Dir(rel)
	var tempRel string
	var f *os.File
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return fmt.Errorf("create temporary file name: %w", err)
		}
		tempRel = path.Join(dir, ".strata-tmp-"+hex.EncodeToString(random[:]))
		f, err = w.root.OpenFile(tempRel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create temporary file: %w", err)
		}
	}
	if f == nil {
		return errors.New("create temporary file: exhausted unique names")
	}

	tempExists := true
	defer func() {
		_ = f.Close()
		if tempExists {
			_ = w.root.Remove(tempRel)
		}
	}()

	if _, err := io.Copy(f, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if preserveMode {
		if err := w.root.Chmod(tempRel, mode); err != nil {
			return fmt.Errorf("preserve file mode: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.root.Rename(tempRel, rel); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	tempExists = false

	dirFile, err := w.root.OpenFile(dir, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open destination directory: %w", err)
	}
	defer func() { _ = dirFile.Close() }()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync destination directory: %w", err)
	}
	return nil
}

type keyedLocker struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

type lockEntry struct {
	token chan struct{}
	refs  int
}

func newKeyedLocker() *keyedLocker {
	return &keyedLocker{entries: make(map[string]*lockEntry)}
}

func (l *keyedLocker) acquire(ctx context.Context, key string) (func(), error) {
	l.mu.Lock()
	entry := l.entries[key]
	if entry == nil {
		entry = &lockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		l.releaseRef(key, entry)
		return nil, ctx.Err()
	case <-entry.token:
		return func() { l.release(key, entry) }, nil
	}
}

func (l *keyedLocker) release(key string, entry *lockEntry) {
	entry.token <- struct{}{}
	l.releaseRef(key, entry)
}

func (l *keyedLocker) releaseRef(key string, entry *lockEntry) {
	l.mu.Lock()
	entry.refs--
	if entry.refs == 0 {
		delete(l.entries, key)
	}
	l.mu.Unlock()
}

func fileToolErrorMessage(err error, action, name string) string {
	switch {
	case errors.Is(err, errPathEscapes):
		return pathEscapeMessage(name)
	case errors.Is(err, errFileNotFound):
		return fmt.Sprintf("Could not %s file: %s. File does not exist.", action, name)
	case errors.Is(err, errNotFile):
		return fmt.Sprintf("Could not %s file: %s. Path is not a file.", action, name)
	case errors.Is(err, errFileTooLarge):
		return fmt.Sprintf("Could not %s file: %s. File exceeds the 4 MiB limit.", action, name)
	case errors.Is(err, errInvalidUTF8):
		return fmt.Sprintf("Could not %s file: %s. File is not valid UTF-8 text.", action, name)
	case errors.Is(err, errBinaryFile):
		return fmt.Sprintf("Could not %s file: %s. Binary files are not supported.", action, name)
	default:
		return fmt.Sprintf("Could not %s file: %s. %v", action, name, err)
	}
}
