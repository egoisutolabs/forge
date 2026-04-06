// Package fileedit implements the FileEditTool — the Go port of Claude Code's
// FileEditTool. It performs string-replacement edits on text files with safety
// guards: the file must have been previously Read (cache gate), the file must
// not have changed since the last read (staleness gate), and old_string must
// be uniquely found (or replace_all must be set).
package fileedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// toolInput is the JSON schema for FileEditTool input.
type toolInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// Tool implements the Edit tool — in-place string-replacement edits on files.
//
// This is the Go port of Claude Code's FileEditTool. Key behaviors:
//   - File must appear in FileStateCache (Read first guard)
//   - Cache entry must not be a partial view (IsPartialView guard)
//   - File mtime must not have advanced past the cached timestamp, unless
//     it was a full-file cache entry and content is unchanged (false-positive bypass)
//   - old_string must exist exactly once (or replace_all=true for multiple)
//   - Fuzzy fallback: strip trailing whitespace per line if exact match fails
//   - Cache updated after write with Offset=nil, Limit=nil (signals full knowledge)
type Tool struct{}

func (t *Tool) Name() string        { return "Edit" }
func (t *Tool) Description() string { return "A tool for editing files." }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The absolute path to the file to modify"
			},
			"old_string": {
				"type": "string",
				"description": "The text to replace"
			},
			"new_string": {
				"type": "string",
				"description": "The text to replace it with (must be different from old_string)"
			},
			"replace_all": {
				"type": "boolean",
				"description": "Replace all occurrences of old_string (default false)"
			}
		},
		"required": ["file_path", "old_string", "new_string"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return fmt.Errorf("file_path is required and cannot be empty")
	}
	if in.OldString == in.NewString {
		return fmt.Errorf("no changes to make: old_string and new_string are exactly the same")
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}
	resolved, resolveErr := tools.ResolvePath(in.FilePath)
	if resolveErr != nil {
		return &models.PermissionDecision{
			Behavior: models.PermDeny,
			Message:  resolveErr.Error(),
		}, nil
	}
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  fmt.Sprintf("Edit file: %s", resolved),
	}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return false }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	// Resolve symlinks and validate path. This replaces the simple filepath.Clean
	// with full symlink resolution to prevent symlink-based bypass attacks.
	resolved, resolveErr := tools.ResolvePath(in.FilePath)
	if resolveErr != nil {
		return &models.ToolResult{
			Content: resolveErr.Error(),
			IsError: true,
		}, nil
	}
	in.FilePath = resolved

	if !filepath.IsAbs(in.FilePath) {
		return &models.ToolResult{
			Content: fmt.Sprintf("file_path must be absolute, got: %s", in.FilePath),
			IsError: true,
		}, nil
	}

	// Special case: empty old_string means "create new file" (no cache check needed).
	if in.OldString == "" {
		return t.createFile(in.FilePath, in.NewString, tctx)
	}

	// ── Cache gate ───────────────────────────────────────────────────────────
	if tctx == nil || tctx.FileState == nil {
		return &models.ToolResult{
			Content: "You must first read the file before editing it (no FileStateCache available).",
			IsError: true,
		}, nil
	}

	state, cached := tctx.FileState.Get(in.FilePath)
	if !cached {
		return &models.ToolResult{
			Content: fmt.Sprintf("You must first read the file before editing it: %s", in.FilePath),
			IsError: true,
		}, nil
	}

	// Partial view guard: if the model only saw auto-modified content (e.g.,
	// CLAUDE.md with injected sections), the cached content diverges from disk
	// and edits cannot be trusted.
	if state.IsPartialView != nil && *state.IsPartialView {
		return &models.ToolResult{
			Content: fmt.Sprintf(
				"Cannot edit %s: the file was displayed with modified content. Read it again before editing.",
				in.FilePath,
			),
			IsError: true,
		}, nil
	}

	// ── Read file from disk ──────────────────────────────────────────────────
	rawBytes, err := os.ReadFile(in.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.ToolResult{
				Content: fmt.Sprintf("File does not exist: %s", in.FilePath),
				IsError: true,
			}, nil
		}
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	fileContent := string(rawBytes)

	// ── Staleness guard ──────────────────────────────────────────────────────
	info, err := os.Stat(in.FilePath)
	if err != nil {
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	fileMtime := info.ModTime().UnixMilli()

	if fileMtime > state.Timestamp {
		// Full-read entries (Offset=nil, Limit=nil) allow a content-comparison
		// bypass: if the content is actually unchanged, the mtime advance is a
		// false positive (common on Windows with cloud sync / antivirus).
		isFullRead := state.Offset == nil && state.Limit == nil
		if !(isFullRead && fileContent == state.Content) {
			return &models.ToolResult{
				Content: fmt.Sprintf(
					"File has been modified since last read, either by the user or by a linter. "+
						"Read it again before attempting to write it: %s",
					in.FilePath,
				),
				IsError: true,
			}, nil
		}
	}

	// ── Find old_string (exact, then fuzzy) ───────────────────────────────────
	actualOld, found := findActualString(fileContent, in.OldString)
	if !found {
		return &models.ToolResult{
			Content: fmt.Sprintf(
				"String to replace not found in file.\nString: %s",
				in.OldString,
			),
			IsError: true,
		}, nil
	}

	// ── Uniqueness check ──────────────────────────────────────────────────────
	matches := strings.Count(fileContent, actualOld)
	if matches > 1 && !in.ReplaceAll {
		return &models.ToolResult{
			Content: fmt.Sprintf(
				"Found %d matches of the string to replace, but replace_all is false. "+
					"To replace all occurrences, set replace_all to true. "+
					"To replace only one occurrence, provide more context to uniquely identify it.\n"+
					"String: %s",
				matches, in.OldString,
			),
			IsError: true,
		}, nil
	}

	// ── Apply replacement ─────────────────────────────────────────────────────
	var updatedContent string
	if in.ReplaceAll {
		updatedContent = strings.ReplaceAll(fileContent, actualOld, in.NewString)
	} else {
		updatedContent = strings.Replace(fileContent, actualOld, in.NewString, 1)
	}

	// ── Generate diff for the result message ──────────────────────────────────
	diff := generateDiff(fileContent, updatedContent, in.FilePath)

	// ── Write to disk ─────────────────────────────────────────────────────────
	if err := os.MkdirAll(filepath.Dir(in.FilePath), 0o755); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Creating parent dirs: %s", err), IsError: true}, nil
	}
	if err := os.WriteFile(in.FilePath, []byte(updatedContent), 0o644); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Writing file: %s", err), IsError: true}, nil
	}

	// ── Update cache (Offset=nil, Limit=nil signals full-file knowledge) ──────
	if tctx != nil && tctx.FileState != nil {
		var newMtime int64
		if newInfo, statErr := os.Stat(in.FilePath); statErr == nil {
			newMtime = newInfo.ModTime().UnixMilli()
		}
		tctx.FileState.Set(in.FilePath, tools.FileState{
			Content:   updatedContent,
			Timestamp: newMtime,
			Offset:    nil,
			Limit:     nil,
		})
	}

	// ── LSP file sync (best-effort) ──────────────────────────────────────────
	if tctx != nil && tctx.LSPManager != nil {
		_ = tctx.LSPManager.ChangeFile(ctx, in.FilePath, updatedContent)
		_ = tctx.LSPManager.SaveFile(ctx, in.FilePath)
	}

	// ── Result message ────────────────────────────────────────────────────────
	var msg string
	if in.ReplaceAll {
		msg = fmt.Sprintf("The file %s has been updated. All occurrences were successfully replaced.", in.FilePath)
	} else {
		msg = fmt.Sprintf("The file %s has been updated successfully.", in.FilePath)
	}
	if diff != "" {
		msg += "\n" + diff
	}

	return &models.ToolResult{Content: msg}, nil
}

// createFile handles the empty old_string case: creates (or overwrites an empty) file.
// No cache check is required because the model is not replacing existing content.
func (t *Tool) createFile(filePath, newContent string, tctx *tools.ToolContext) (*models.ToolResult, error) {
	// Check if file already exists with content — refuse to overwrite
	if existing, err := os.ReadFile(filePath); err == nil && strings.TrimSpace(string(existing)) != "" {
		return &models.ToolResult{
			Content: fmt.Sprintf("Cannot create new file — %s already exists with content.", filePath),
			IsError: true,
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Creating parent dirs: %s", err), IsError: true}, nil
	}
	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Writing file: %s", err), IsError: true}, nil
	}

	// Seed cache entry so the next Edit on this file works without a Read.
	if tctx != nil && tctx.FileState != nil {
		info, _ := os.Stat(filePath)
		var mtime int64
		if info != nil {
			mtime = info.ModTime().UnixMilli()
		}
		tctx.FileState.Set(filePath, tools.FileState{
			Content:   newContent,
			Timestamp: mtime,
		})
	}

	return &models.ToolResult{
		Content: fmt.Sprintf("The file %s has been created successfully.", filePath),
	}, nil
}

// findActualString locates searchString within fileContent using progressively
// looser matching strategies:
//  1. Exact substring match
//  2. Whitespace-normalized match: strip trailing whitespace from each line of
//     both file and search string, then find the corresponding original text
//
// Returns (actualOldString, true) on match, ("", false) on no match.
// The returned actualOldString is a string that literally appears in fileContent
// and can be used directly as the target of a strings.Replace call.
func findActualString(fileContent, searchString string) (string, bool) {
	// 1. Exact match
	if strings.Contains(fileContent, searchString) {
		return searchString, true
	}

	// 2. Normalize both and try a line-by-line search.
	// Strip trailing whitespace from each line so the model's output (which
	// often omits trailing spaces) can match a file that retains them.
	normFile := stripTrailingWhitespaceLines(fileContent)
	normSearch := stripTrailingWhitespaceLines(searchString)

	// Quick containment check in normalized space before the O(n·m) line scan.
	if !strings.Contains(normFile, normSearch) {
		return "", false
	}

	// Map from normalized match back to original text.
	// Because we only strip TRAILING whitespace, every line's prefix is
	// identical in both versions, so line indices align 1:1.
	normFileLines := strings.Split(normFile, "\n")
	normSearchLines := strings.Split(normSearch, "\n")
	origFileLines := strings.Split(fileContent, "\n")

	searchLen := len(normSearchLines)
	for startLine := 0; startLine <= len(normFileLines)-searchLen; startLine++ {
		match := true
		for j, searchLine := range normSearchLines {
			if normFileLines[startLine+j] != searchLine {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		endLine := startLine + searchLen
		if endLine > len(origFileLines) {
			continue
		}
		actual := strings.Join(origFileLines[startLine:endLine], "\n")
		// Final sanity check: the reconstructed actual must appear in the original.
		if strings.Contains(fileContent, actual) {
			return actual, true
		}
	}

	return "", false
}

// stripTrailingWhitespaceLines removes trailing spaces and tabs from every
// line in s, preserving newline characters.
func stripTrailingWhitespaceLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// generateDiff produces a compact unified-style diff between oldContent and
// newContent, showing changed lines with 3 lines of context on each side.
// Returns an empty string if the contents are identical.
func generateDiff(oldContent, newContent, filePath string) string {
	if oldContent == newContent {
		return ""
	}

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	const contextLines = 3

	// Locate changed region (first and last differing line)
	first, last := -1, -1
	minLen := len(oldLines)
	if len(newLines) < minLen {
		minLen = len(newLines)
	}

	for i := 0; i < minLen; i++ {
		if oldLines[i] != newLines[i] {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	// Account for lines only in one version
	if len(oldLines) != len(newLines) {
		if first == -1 {
			first = minLen
		}
		last = max(len(oldLines), len(newLines)) - 1
	}
	if first == -1 {
		return "" // no diff found
	}

	// Context window
	ctxStart := first - contextLines
	if ctxStart < 0 {
		ctxStart = 0
	}
	oldEnd := last + contextLines + 1
	if oldEnd > len(oldLines) {
		oldEnd = len(oldLines)
	}
	newEnd := last + contextLines + 1
	if newEnd > len(newLines) {
		newEnd = len(newLines)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", filePath, filePath)
	fmt.Fprintf(&sb, "@@ -%d +%d @@\n", ctxStart+1, ctxStart+1)

	for i := ctxStart; i < oldEnd || i < newEnd; i++ {
		switch {
		case i >= len(oldLines):
			fmt.Fprintf(&sb, "+%s\n", newLines[i])
		case i >= len(newLines):
			fmt.Fprintf(&sb, "-%s\n", oldLines[i])
		case oldLines[i] == newLines[i]:
			fmt.Fprintf(&sb, " %s\n", oldLines[i])
		default:
			fmt.Fprintf(&sb, "-%s\n", oldLines[i])
			fmt.Fprintf(&sb, "+%s\n", newLines[i])
		}
	}

	return sb.String()
}

// max returns the larger of a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
