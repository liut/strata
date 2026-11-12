package mcp

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type textDiff struct {
	Diff             string
	Patch            string
	FirstChangedLine int
}

func buildTextDiff(name, before, after string) (textDiff, error) {
	before = strings.ReplaceAll(before, "\r\n", "\n")
	after = strings.ReplaceAll(after, "\r\n", "\n")
	beforeLines := difflib.SplitLines(before)
	afterLines := difflib.SplitLines(after)

	patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        beforeLines,
		B:        afterLines,
		FromFile: name,
		ToFile:   name,
		Context:  4,
	})
	if err != nil {
		return textDiff{}, err
	}
	contextDiff, err := difflib.GetContextDiffString(difflib.ContextDiff{
		A:        beforeLines,
		B:        afterLines,
		FromFile: name,
		ToFile:   name,
		Context:  4,
	})
	if err != nil {
		return textDiff{}, err
	}

	return textDiff{
		Diff:             contextDiff,
		Patch:            patch,
		FirstChangedLine: firstChangedLine(beforeLines, afterLines),
	}, nil
}

func firstChangedLine(before, after []string) int {
	limit := min(len(before), len(after))
	for i := 0; i < limit; i++ {
		if before[i] != after[i] {
			return i + 1
		}
	}
	if len(before) != len(after) {
		return limit + 1
	}
	return 0
}
