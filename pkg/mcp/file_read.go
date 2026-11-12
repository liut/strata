package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type readFileDetails struct {
	Path       string `json:"path"`
	HasBOM     bool   `json:"has_bom"`
	LineEnding string `json:"line_ending"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
}

func (h *Handler) handleRead(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	name, _ := args["path"].(string)
	if name == "" {
		return mcp.NewToolResultError("path must not be empty"), nil
	}
	startLine, startSet, err := positiveIntArgument(args, "start_line")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	lineCount, countSet, err := positiveIntArgument(args, "line_count")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if countSet && !startSet {
		startLine = 1
	}

	sess, err := h.sessionForCall(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return readWorkspaceFile(sess.WorkspacePath(), name, startLine, lineCount, startSet)
}

func readWorkspaceFile(workspacePath, name string, startLine, lineCount int, startSet bool) (*mcp.CallToolResult, error) {
	ws, err := openWorkspace(workspacePath)
	if err != nil {
		return mcp.NewToolResultError("workspace failed: " + err.Error()), nil
	}
	defer func() { _ = ws.close() }()

	rel, _, err := ws.resolvePath(name, false)
	if err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "read", name)), nil
	}
	file, err := ws.readTextFile(rel)
	if err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "read", name)), nil
	}

	content, startLine, endLine, totalLines, err := sliceTextLines(file.content, startLine, lineCount, startSet)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not read file: %s. %v", name, err)), nil
	}
	details := readFileDetails{
		Path:       name,
		HasBOM:     file.hasBOM,
		LineEnding: file.lineEnding,
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: totalLines,
	}
	return mcp.NewToolResultStructured(details, content), nil
}

func positiveIntArgument(args map[string]any, key string) (int, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return 0, false, nil
	}
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case float32:
		number = float64(v)
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	default:
		return 0, true, fmt.Errorf("%s must be a positive integer", key)
	}
	if number <= 0 || number != math.Trunc(number) || number > math.MaxInt32 {
		return 0, true, fmt.Errorf("%s must be a positive integer", key)
	}
	return int(number), true, nil
}

func sliceTextLines(content string, startLine, lineCount int, startSet bool) (string, int, int, int, error) {
	totalLines := lineCountOf(content)
	if !startSet {
		return content, firstReadLine(totalLines), totalLines, totalLines, nil
	}
	if startLine > totalLines+1 {
		return "", 0, 0, totalLines, fmt.Errorf("start_line %d is beyond the end of the file", startLine)
	}
	if totalLines == 0 {
		return "", startLine, totalLines, totalLines, nil
	}

	offsets := lineOffsets(content)
	startIndex := startLine - 1
	endIndex := totalLines
	if lineCount > 0 && startIndex+lineCount < endIndex {
		endIndex = startIndex + lineCount
	}
	if startIndex >= len(offsets) {
		return "", startLine, totalLines, totalLines, nil
	}
	startOffset := offsets[startIndex]
	endOffset := len(content)
	if endIndex < len(offsets) {
		endOffset = offsets[endIndex]
	}
	return content[startOffset:endOffset], startLine, endIndex, totalLines, nil
}

func firstReadLine(totalLines int) int {
	if totalLines == 0 {
		return 0
	}
	return 1
}

func lineCountOf(content string) int {
	if content == "" {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func lineOffsets(content string) []int {
	offsets := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}
