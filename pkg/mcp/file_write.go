package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/mark3labs/mcp-go/mcp"
)

type writeFileDetails struct {
	Path             string `json:"path"`
	Created          bool   `json:"created"`
	Changed          bool   `json:"changed"`
	BytesWritten     int    `json:"bytes_written"`
	Diff             string `json:"diff,omitempty"`
	Patch            string `json:"patch,omitempty"`
	FirstChangedLine int    `json:"first_changed_line,omitempty"`
}

func (h *Handler) handleWrite(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	name, _ := args["path"].(string)
	if name == "" {
		return mcp.NewToolResultError("path must not be empty"), nil
	}
	content, ok := args["content"].(string)
	if !ok {
		return mcp.NewToolResultError("content must be a string"), nil
	}
	if err := validateTextContent(content); err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "write", name)), nil
	}

	sess, err := h.sessionForCall(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return h.writeWorkspaceFile(ctx, sess.WorkspacePath(), name, content), nil
}

func (h *Handler) writeWorkspaceFile(ctx context.Context, workspacePath, name, content string) *mcp.CallToolResult {
	ws, err := openWorkspace(workspacePath)
	if err != nil {
		return mcp.NewToolResultError("workspace failed: " + err.Error())
	}
	defer func() { _ = ws.close() }()

	rel, _, err := ws.resolvePath(name, true)
	if err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "write", name))
	}
	release, err := h.fileLocks.acquire(ctx, sessionPathLockKey(workspacePath, rel))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not write file: %s. %v", name, err))
	}
	defer release()

	info, statErr := ws.root.Stat(rel)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return mcp.NewToolResultError(fmt.Sprintf("Could not write file: %s. %v", name, statErr))
	}
	parentInfo, err := ws.root.Stat(path.Dir(rel))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not write file: %s. Parent directory does not exist.", name))
	}
	if !parentInfo.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("Could not write file: %s. Parent path is not a directory.", name))
	}

	var prior *textFile
	mode := os.FileMode(0644)
	preserveMode := false
	created := errors.Is(statErr, fs.ErrNotExist)
	if !created {
		if !info.Mode().IsRegular() {
			return mcp.NewToolResultError(fileToolErrorMessage(errNotFile, "write", name))
		}
		prior, err = ws.readTextFile(rel)
		if err != nil {
			return mcp.NewToolResultError(fileToolErrorMessage(err, "write", name))
		}
		mode = info.Mode().Perm()
		preserveMode = true
	}

	data := []byte(content)
	details := writeFileDetails{
		Path:         name,
		Created:      created,
		BytesWritten: len(data),
	}
	if prior != nil && bytes.Equal(prior.originalRaw, data) {
		details.Changed = false
		return mcp.NewToolResultStructured(details, fmt.Sprintf("No changes made to %s.", name))
	}
	details.Changed = true

	if err := ws.atomicReplace(ctx, rel, data, mode, preserveMode); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not write file: %s. %v", name, err))
	}
	if prior != nil {
		diff, err := buildTextDiff(name, prior.content, content)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Wrote file: %s, but could not build diff. %v", name, err))
		}
		details.Diff = diff.Diff
		details.Patch = diff.Patch
		details.FirstChangedLine = diff.FirstChangedLine
	}

	message := fmt.Sprintf("Successfully wrote %d bytes to %s.", len(data), name)
	return mcp.NewToolResultStructured(details, message)
}

func sessionPathLockKey(workspacePath, rel string) string {
	return workspacePath + "\x00" + rel
}
