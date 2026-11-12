package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/text/unicode/norm"
)

type editReplacement struct {
	oldText string
	newText string
}

type editFileDetails struct {
	Path             string `json:"path"`
	Diff             string `json:"diff"`
	Patch            string `json:"patch"`
	FirstChangedLine int    `json:"first_changed_line,omitempty"`
}

type editMatch struct {
	index int
	start int
	end   int
}

func (h *Handler) handleEdit(ctx context.Context, args map[string]any) (*mcp.CallToolResult, error) {
	name, _ := args["path"].(string)
	if name == "" {
		return mcp.NewToolResultError("path must not be empty"), nil
	}
	replacements, err := normalizeEditArguments(args, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if h.editTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.editTimeout)
		defer cancel()
	}

	sess, err := h.sessionForCall(ctx, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return h.editWorkspaceFile(ctx, sess.WorkspacePath(), name, replacements)
}

func (h *Handler) editWorkspaceFile(ctx context.Context, workspacePath, name string, replacements []editReplacement) (*mcp.CallToolResult, error) {
	ws, err := openWorkspace(workspacePath)
	if err != nil {
		return mcp.NewToolResultError("workspace failed: " + err.Error()), nil
	}
	defer func() { _ = ws.close() }()

	rel, _, err := ws.resolvePath(name, false)
	if err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "edit", name)), nil
	}
	release, err := h.fileLocks.acquire(ctx, sessionPathLockKey(workspacePath, rel))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not edit file: %s. %v", name, err)), nil
	}
	defer release()

	file, err := ws.readTextFile(rel)
	if err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "edit", name)), nil
	}
	finalText, err := applyEdits(file.content, replacements, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateTextContent(finalText); err != nil {
		return mcp.NewToolResultError(fileToolErrorMessage(err, "edit", name)), nil
	}
	if finalText == file.content {
		if len(replacements) == 1 {
			return mcp.NewToolResultError(fmt.Sprintf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", name)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("No changes made to %s. The replacements produced identical content.", name)), nil
	}
	data := renderTextFile(finalText, file.hasBOM)
	if len(data) > maxTextFileSize {
		return mcp.NewToolResultError(fileToolErrorMessage(errFileTooLarge, "edit", name)), nil
	}

	info, err := ws.root.Stat(rel)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not edit file: %s. %v", name, err)), nil
	}
	if !info.Mode().IsRegular() {
		return mcp.NewToolResultError(fileToolErrorMessage(errNotFile, "edit", name)), nil
	}
	diff, err := buildTextDiff(name, file.content, finalText)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not edit file: %s. %v", name, err)), nil
	}
	if err := ws.atomicReplace(ctx, rel, data, info.Mode().Perm(), true); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not edit file: %s. %v", name, err)), nil
	}
	details := editFileDetails{
		Path:             name,
		Diff:             diff.Diff,
		Patch:            diff.Patch,
		FirstChangedLine: diff.FirstChangedLine,
	}
	message := fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(replacements), name)
	return mcp.NewToolResultStructured(details, message), nil
}

func normalizeEditArguments(args map[string]any, name string) ([]editReplacement, error) {
	var rawItems []any
	if rawEdits, ok := args["edits"]; ok {
		items, err := decodeEditItems(rawEdits)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
		}
		rawItems = items
	} else {
		rawItems = []any{args}
	}

	replacements := make([]editReplacement, 0, len(rawItems))
	for i, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Edit tool input is invalid. edits[%d] must be an object.", i)
		}
		oldText, ok := item["oldText"].(string)
		if !ok {
			oldText = ""
		}
		if oldText == "" {
			if len(rawItems) == 1 {
				return nil, fmt.Errorf("oldText must not be empty in %s.", name)
			}
			return nil, fmt.Errorf("edits[%d].oldText must not be empty in %s.", i, name)
		}
		newText, ok := item["newText"].(string)
		if !ok {
			return nil, fmt.Errorf("edits[%d].newText is required in %s.", i, name)
		}
		replacements = append(replacements, editReplacement{oldText: oldText, newText: newText})
	}
	return replacements, nil
}

func decodeEditItems(value any) ([]any, error) {
	switch v := value.(type) {
	case string:
		var decoded any
		if err := json.Unmarshal([]byte(v), &decoded); err != nil {
			return nil, fmt.Errorf("edits must be valid JSON: %v", err)
		}
		return decodeEditItems(decoded)
	case []any:
		return v, nil
	case map[string]any:
		return []any{v}, nil
	case nil:
		return nil, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
	default:
		return nil, fmt.Errorf("edits must be an array, object, or JSON-encoded array or object")
	}
}

func applyEdits(source string, replacements []editReplacement, name string) (string, error) {
	matches, needsFallback, err := matchExactEdits(source, replacements, name)
	if err != nil {
		return "", err
	}

	base := source
	if needsFallback {
		base = normalizeMatchText(source)
		matches = make([]editMatch, len(replacements))
		for i, replacement := range replacements {
			needle := normalizeMatchText(replacement.oldText)
			if needle == "" {
				return "", notFoundEditError(i, len(replacements), name)
			}
			positions := findMatchPositions(base, needle)
			if len(positions) == 0 {
				return "", notFoundEditError(i, len(replacements), name)
			}
			if len(positions) > 1 {
				return "", duplicateEditError(i, len(positions), positions, base, len(replacements), name)
			}
			matches[i] = editMatch{index: i, start: positions[0], end: positions[0] + len(needle)}
		}
	}

	if err := detectOverlaps(matches, name); err != nil {
		return "", err
	}

	lineEnding := detectLineEnding(source)
	ordered := append([]editMatch(nil), matches...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].start > ordered[j].start
	})
	result := base
	replacementLineEnding := lineEnding
	if needsFallback {
		replacementLineEnding = "lf"
	}
	for _, match := range ordered {
		replacement := normalizeReplacementLineEndings(replacements[match.index].newText, replacementLineEnding)
		result = result[:match.start] + replacement + result[match.end:]
	}
	if lineEnding != "" {
		result = normalizeReplacementLineEndings(result, lineEnding)
	}
	return result, nil
}

func matchExactEdits(source string, replacements []editReplacement, name string) ([]editMatch, bool, error) {
	matches := make([]editMatch, len(replacements))
	needsFallback := false
	for i, replacement := range replacements {
		positions := findMatchPositions(source, replacement.oldText)
		if len(positions) == 0 {
			needsFallback = true
			continue
		}
		if len(positions) > 1 {
			return nil, false, duplicateEditError(i, len(positions), positions, source, len(replacements), name)
		}
		matches[i] = editMatch{index: i, start: positions[0], end: positions[0] + len(replacement.oldText)}
	}
	return matches, needsFallback, nil
}

func findMatchPositions(text, needle string) []int {
	if needle == "" {
		return nil
	}
	var positions []int
	for offset := 0; offset <= len(text)-len(needle); {
		index := strings.Index(text[offset:], needle)
		if index < 0 {
			break
		}
		positions = append(positions, offset+index)
		offset += index + len(needle)
	}
	return positions
}

func detectOverlaps(matches []editMatch, name string) error {
	ordered := append([]editMatch(nil), matches...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].start == ordered[j].start {
			return ordered[i].end < ordered[j].end
		}
		return ordered[i].start < ordered[j].start
	})
	for i := 1; i < len(ordered); i++ {
		if ordered[i].start < ordered[i-1].end {
			return fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.",
				ordered[i-1].index, ordered[i].index, name)
		}
	}
	return nil
}

func notFoundEditError(index, total int, name string) error {
	if total == 1 {
		return fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", name)
	}
	return fmt.Errorf("Could not find edits[%d] in %s. The old text must match exactly including all whitespace and newlines.", index, name)
}

func duplicateEditError(index, count int, positions []int, text string, total int, name string) error {
	var prefix string
	if total == 1 {
		prefix = fmt.Sprintf("Found %d occurrences of the text in %s. The text must be unique — include more surrounding context to disambiguate.", count, name)
	} else {
		prefix = fmt.Sprintf("Found %d occurrences of edits[%d] in %s. The text must be unique — include more surrounding context to disambiguate.", count, index, name)
	}

	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString("\nMatching locations (line labels are diagnostic; do not include them in oldText):\n")
	limit := min(5, len(positions))
	for i := 0; i < limit; i++ {
		line := 1 + strings.Count(text[:positions[i]], "\n")
		fmt.Fprintf(&b, "Match %d at line %d:\n", i+1, line)
		writeDiagnosticContext(&b, text, line)
	}
	b.WriteString("Read the intended region and include enough surrounding text to make oldText unique before retrying.")
	return fmt.Errorf("%s", b.String())
}

func writeDiagnosticContext(b *strings.Builder, text string, line int) {
	lines := strings.Split(text, "\n")
	start := max(0, line-2)
	end := min(len(lines), line+1)
	for i := start; i < end; i++ {
		snippet := lines[i]
		if runes := []rune(snippet); len(runes) > 200 {
			snippet = string(runes[:200]) + " [line truncated]"
		}
		fmt.Fprintf(b, "%d: %s\n", i+1, snippet)
	}
}

func normalizeMatchText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = norm.NFKC.String(text)

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	text = strings.Join(lines, "\n")
	return strings.NewReplacer(
		"‘", "'", "’", "'", "‚", "'", "‛", "'",
		"“", `"`, "”", `"`, "„", `"`, "‟", `"`,
		"‐", "-", "‑", "-", "‒", "-", "–", "-", "—", "-", "―", "-", "−", "-",
		"\u00a0", " ", "\u2002", " ", "\u2003", " ", "\u2004", " ", "\u2005", " ",
		"\u2006", " ", "\u2007", " ", "\u2008", " ", "\u2009", " ", "\u200a", " ",
		"\u202f", " ", "\u205f", " ", "\u3000", " ",
	).Replace(text)
}

func normalizeReplacementLineEndings(text, lineEnding string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if lineEnding == "crlf" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}
