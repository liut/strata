package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestEditWorkspaceFileExactReplacement(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("alpha\nbeta\ngamma\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "beta", newText: "BETA"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("content=%q", data)
	}
	details := decodeStructured[editFileDetails](t, result)
	if details.FirstChangedLine != 2 || details.Diff == "" || details.Patch == "" {
		t.Fatalf("details=%+v", details)
	}
}

func TestEditWorkspaceFileMultipleDisjointEdits(t *testing.T) {
	source := "one\ntwo\nthree\nfour\n"
	got, err := applyEdits(source, []editReplacement{
		{oldText: "one", newText: "ONE"},
		{oldText: "three", newText: ""},
	}, "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ONE\ntwo\n\nfour\n" {
		t.Fatalf("content=%q", got)
	}
}

func TestEditWorkspaceFilePreservesBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\ntwo\r\n")...)
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "two", newText: "TWO\nTHREE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{0xEF, 0xBB, 0xBF}, []byte("one\r\nTWO\r\nTHREE\r\n")...)
	if string(got) != string(want) {
		t.Fatalf("content=%q, want %q", got, want)
	}
}

func TestEditWorkspaceFileNormalizedFallback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	source := "He said “hi”\u00a0there  \r\ntail\r\n"
	if err := os.WriteFile(target, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "He said \"hi\" there\r\n", newText: "He said “hello”\r\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	want := "He said “hello”\r\ntail\r\n"
	if string(got) != want {
		t.Fatalf("content=%q, want %q", got, want)
	}
}

func TestEditFallbackDoesNotNormalizeIndentation(t *testing.T) {
	_, err := applyEdits("head\n  item\n", []editReplacement{
		{oldText: "\nitem\n", newText: "\nchanged\n"},
	}, "file.txt")
	if err == nil || !strings.Contains(err.Error(), "Could not find the exact text") {
		t.Fatalf("error=%v", err)
	}
}

func TestEditValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		replacements []editReplacement
		want         string
	}{
		{
			name:         "missing",
			source:       "same\nsame\n",
			replacements: []editReplacement{{oldText: "absent", newText: "x"}},
			want:         "Could not find the exact text",
		},
		{
			name:         "duplicate",
			source:       "same\nsame\n",
			replacements: []editReplacement{{oldText: "same", newText: "x"}},
			want:         "Matching locations",
		},
		{
			name:   "overlap",
			source: "abcdef\n",
			replacements: []editReplacement{
				{oldText: "abcd", newText: "x"},
				{oldText: "cdef", newText: "y"},
			},
			want: "overlap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyEdits(tt.source, tt.replacements, "file.txt")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestEditNoChangeLeavesFileIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("same\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "same", newText: "same"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "No changes made") {
		t.Fatalf("result=%+v text=%q", result, resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "same\n" {
		t.Fatalf("content=%q", data)
	}
}

func TestNormalizeEditArgumentsCompatibility(t *testing.T) {
	cases := []map[string]any{
		{"path": "file.txt", "oldText": "a", "newText": "b"},
		{"path": "file.txt", "edits": []any{map[string]any{"oldText": "a", "newText": "b"}}},
		{"path": "file.txt", "edits": map[string]any{"oldText": "a", "newText": "b"}},
		{"path": "file.txt", "edits": `[{"oldText":"a","newText":"b"}]`},
	}
	for i, args := range cases {
		got, err := normalizeEditArguments(args, "file.txt")
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if len(got) != 1 || got[0].oldText != "a" || got[0].newText != "b" {
			t.Fatalf("case %d: %+v", i, got)
		}
	}
}

func TestNormalizeEditArgumentsValidation(t *testing.T) {
	for _, args := range []map[string]any{
		{"path": "file.txt", "edits": []any{}},
		{"path": "file.txt", "edits": []any{map[string]any{"oldText": "", "newText": "x"}}},
		{"path": "file.txt", "edits": []any{map[string]any{"oldText": "x"}}},
		{"path": "file.txt", "edits": "not-json"},
	} {
		if _, err := normalizeEditArguments(args, "file.txt"); err == nil {
			t.Fatalf("args %#v should fail", args)
		}
	}
}

func TestNormalizeEditArgumentsSingleEmptyOldTextMessage(t *testing.T) {
	cases := []map[string]any{
		{"path": "file.txt", "oldText": "", "newText": "x"},
		{"path": "file.txt", "edits": []any{map[string]any{"oldText": "", "newText": "x"}}},
		{"path": "file.txt", "edits": map[string]any{"oldText": "", "newText": "x"}},
		{"path": "file.txt", "edits": `[{"oldText":"","newText":"x"}]`},
	}
	for i, args := range cases {
		_, err := normalizeEditArguments(args, "file.txt")
		if err == nil || err.Error() != "oldText must not be empty in file.txt." {
			t.Errorf("case %d error=%v", i, err)
		}
	}
}

func TestNormalizeEditArgumentsMultipleEmptyOldTextMessage(t *testing.T) {
	_, err := normalizeEditArguments(map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "a", "newText": "b"},
			map[string]any{"oldText": "", "newText": "c"},
		},
	}, "file.txt")
	if err == nil || err.Error() != "edits[1].oldText must not be empty in file.txt." {
		t.Fatalf("error=%v", err)
	}
}

func TestEditMixedLineEndingsNormalizeToFirstStyle(t *testing.T) {
	tests := []struct {
		name   string
		source string
		old    string
		want   string
	}{
		{
			name:   "crlf first",
			source: "one\r\ntwo\nthree\n",
			old:    "two\n",
			want:   "one\r\nTWO\r\nthree\r\n",
		},
		{
			name:   "lf first",
			source: "one\ntwo\r\nthree\r\n",
			old:    "two\r\n",
			want:   "one\nTWO\nthree\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyEdits(tt.source, []editReplacement{
				{oldText: tt.old, newText: "TWO\n"},
			}, "file.txt")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("content=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestEditMultipleReplacementErrorMessages(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		replacements []editReplacement
		want         string
	}{
		{
			name:   "missing second edit",
			source: "present\n",
			replacements: []editReplacement{
				{oldText: "present", newText: "changed"},
				{oldText: "absent", newText: "other"},
			},
			want: "Could not find edits[1] in file.txt. The old text must match exactly including all whitespace and newlines.",
		},
		{
			name:   "duplicate first edit",
			source: "same\nsame\n",
			replacements: []editReplacement{
				{oldText: "same", newText: "changed"},
				{oldText: "missing", newText: "other"},
			},
			want: "Found 2 occurrences of edits[0] in file.txt. The text must be unique — include more surrounding context to disambiguate.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := applyEdits(tt.source, tt.replacements, "file.txt")
			if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Fatalf("error=%v, want prefix %q", err, tt.want)
			}
		})
	}
}

func TestEditMultipleNoChangeMessage(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("a\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "a", newText: "a"},
		{oldText: "b", newText: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "No changes made to file.txt. The replacements produced identical content."
	if !result.IsError || resultText(result) != want {
		t.Fatalf("result=%q, want %q", resultText(result), want)
	}
}

func TestEditValidationFailuresLeaveFileIntact(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		replacements []editReplacement
	}{
		{
			name:         "missing",
			source:       "original\n",
			replacements: []editReplacement{{oldText: "absent", newText: "changed"}},
		},
		{
			name:         "duplicate",
			source:       "same\nsame\n",
			replacements: []editReplacement{{oldText: "same", newText: "changed"}},
		},
		{
			name:   "overlap",
			source: "abcdef\n",
			replacements: []editReplacement{
				{oldText: "abcd", newText: "x"},
				{oldText: "cdef", newText: "y"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "file.txt")
			if err := os.WriteFile(target, []byte(tt.source), 0644); err != nil {
				t.Fatal(err)
			}
			result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", tt.replacements)
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("expected tool error")
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.source {
				t.Fatalf("file changed to %q", data)
			}
		})
	}
}

func TestEditBOMFileRejectsResultOverLimit(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	source := strings.Repeat("a", maxTextFileSize-6) + "OLD"
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(source)...)
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "OLD", newText: "NEWX"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "4 MiB") {
		t.Fatalf("result=%q", resultText(result))
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatal("file changed after oversized edit")
	}
}

func TestEditDuplicateDiagnosticsLimitAndTruncation(t *testing.T) {
	longLine := strings.Repeat("x", 201)
	source := strings.Join([]string{"same", longLine, "same", "same", "same", "same"}, "\n")
	_, err := applyEdits(source, []editReplacement{
		{oldText: "same", newText: "changed"},
	}, "file.txt")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	message := err.Error()
	for i := 1; i <= 5; i++ {
		if !strings.Contains(message, "Match "+strconv.Itoa(i)+" at line ") {
			t.Errorf("missing diagnostic %d: %s", i, message)
		}
	}
	if strings.Contains(message, "Match 6 at line ") {
		t.Fatalf("diagnostics exceeded five matches: %s", message)
	}
	if !strings.Contains(message, "[line truncated]") {
		t.Fatalf("long line was not truncated: %s", message)
	}
}

func TestEditWorkspaceFileConcurrentCallsSerialize(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("left\nright\n"), 0644); err != nil {
		t.Fatal(err)
	}
	h := testFileHandler()

	var wg sync.WaitGroup
	errorsCh := make(chan error, 2)
	for _, replacement := range []editReplacement{
		{oldText: "left", newText: "LEFT"},
		{oldText: "right", newText: "RIGHT"},
	} {
		wg.Add(1)
		go func(replacement editReplacement) {
			defer wg.Done()
			result, err := h.editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{replacement})
			if err == nil && result.IsError {
				err = &toolResultError{text: resultText(result)}
			}
			errorsCh <- err
		}(replacement)
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "LEFT\nRIGHT\n" {
		t.Fatalf("content=%q", data)
	}
}

func TestEditWorkspaceFileCancellationLeavesFileIntact(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := testFileHandler().editWorkspaceFile(ctx, dir, "file.txt", []editReplacement{
		{oldText: "original", newText: "changed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected canceled edit to fail")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("content=%q", data)
	}
}

func TestEditWorkspaceFileRejectsOversizedResult(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "file.txt", []editReplacement{
		{oldText: "old", newText: strings.Repeat("x", maxTextFileSize+1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(resultText(result), "4 MiB") {
		t.Fatalf("result=%q", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("content=%q", data)
	}
}

func TestEditWorkspaceFileUpdatesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	result, err := testFileHandler().editWorkspaceFile(context.Background(), dir, "link.txt", []editReplacement{
		{oldText: "old", newText: "new"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("target=%q", data)
	}
	if link, err := os.Readlink(filepath.Join(dir, "link.txt")); err != nil || link != "target.txt" {
		t.Fatalf("link=%q err=%v", link, err)
	}
}

type toolResultError struct {
	text string
}

func (e *toolResultError) Error() string {
	return e.text
}

func TestEditDiffFirstChangedLine(t *testing.T) {
	diff, err := buildTextDiff("file.txt", "one\ntwo\n", "one\nthree\n")
	if err != nil {
		t.Fatal(err)
	}
	if diff.FirstChangedLine != 2 {
		t.Fatalf("first changed line=%d", diff.FirstChangedLine)
	}
}
