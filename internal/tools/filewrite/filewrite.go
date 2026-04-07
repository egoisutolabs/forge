// Package filewrite implements the FileWriteTool — the Go port of Claude Code's
// FileWriteTool. It creates or fully overwrites files, guarding against writes
// to files that the model has not yet read (or has only partially read).
package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// toolInput is the JSON input accepted by the FileWrite tool.
type toolInput struct {
	FilePath string `json:"file_path"` // absolute path
	Content  string `json:"content"`   // full file content to write
}

// Tool implements the Write tool — create or overwrite files on disk.
//
// Key behaviors (matching Claude Code's FileWriteTool):
//   - Absolute paths only
//   - Parent directories created automatically
//   - Creating a new file: no prior read required
//   - Updating an existing file: requires a matching cache entry
//     (IsPartialView=true is rejected; stale mtime is rejected)
//   - After write: updates FileStateCache with nil Offset/Limit
//   - Returns a unified diff for updates
type Tool struct{}

func (t *Tool) Name() string        { return "Write" }
func (t *Tool) Description() string { return "Write a file to the local filesystem." }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The absolute path to the file to write (must be absolute, not relative)"
			},
			"content": {
				"type": "string",
				"description": "The content to write to the file"
			}
		},
		"required": ["file_path", "content"]
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
	if !filepath.IsAbs(in.FilePath) {
		return fmt.Errorf("file_path must be absolute, got: %s", in.FilePath)
	}
	return nil
}

// CheckPermissions always returns PermAsk — writing to files requires explicit approval.
func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  "Write file",
	}, nil
}

// IsConcurrencySafe returns false — writes are not safe to run concurrently.
func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return false }

// IsReadOnly returns false — writes are never read-only.
func (t *Tool) IsReadOnly(_ json.RawMessage) bool { return false }

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

	// Detect create vs update: use Lstat (not Stat) to detect symlinks.
	info, statErr := os.Lstat(in.FilePath)
	isUpdate := statErr == nil // file exists → update; ENOENT → create

	if isUpdate {
		// Guard: require a prior read for existing files.
		if err := checkCacheGuard(tctx, in.FilePath, info); err != nil {
			return &models.ToolResult{Content: err.Error(), IsError: true}, nil
		}
	}

	// Capture original content for the diff (from cache, populated above).
	var originalContent string
	if isUpdate && tctx != nil && tctx.FileState != nil {
		if cached, ok := tctx.FileState.Get(in.FilePath); ok {
			originalContent = cached.Content
		}
	}

	// Create parent directories before writing.
	if err := os.MkdirAll(filepath.Dir(in.FilePath), 0o755); err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Cannot create parent directories for %s: %s", in.FilePath, err),
			IsError: true,
		}, nil
	}

	// TOCTOU defense: verify inode hasn't changed between permission check and write.
	// This detects symlink swaps that occur between the Lstat above and the write below.
	if isUpdate {
		if err := tools.VerifyInodeUnchanged(in.FilePath, info); err != nil {
			return &models.ToolResult{
				Content: fmt.Sprintf("Write aborted: %s", err),
				IsError: true,
			}, nil
		}
	}

	// Write the file using O_WRONLY|O_CREATE|O_TRUNC instead of os.WriteFile
	// to avoid following symlinks that may have been created after our checks.
	writeFlags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	f, err := os.OpenFile(in.FilePath, writeFlags, 0o644)
	if err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Cannot write file %s: %s", in.FilePath, err),
			IsError: true,
		}, nil
	}
	_, writeErr := f.Write([]byte(in.Content))
	closeErr := f.Close()
	if writeErr != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Cannot write file %s: %s", in.FilePath, writeErr),
			IsError: true,
		}, nil
	}
	if closeErr != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Cannot write file %s: %s", in.FilePath, closeErr),
			IsError: true,
		}, nil
	}

	// Read back the mtime immediately after write for cache freshness.
	var newMtimeMs int64
	if newInfo, err := os.Stat(in.FilePath); err == nil {
		newMtimeMs = newInfo.ModTime().UnixMilli()
	}

	// Update the FileStateCache: nil Offset/Limit signals a full, fresh read.
	if tctx != nil && tctx.FileState != nil {
		tctx.FileState.Set(in.FilePath, tools.FileState{
			Content:   in.Content,
			Timestamp: newMtimeMs,
			// Offset and Limit intentionally nil — full content is known.
		})
	}

	// ── LSP file sync (best-effort) ──────────────────────────────────────────
	if tctx != nil && tctx.LSPManager != nil {
		if isUpdate {
			_ = tctx.LSPManager.ChangeFile(ctx, in.FilePath, in.Content)
		} else {
			_ = tctx.LSPManager.OpenFile(ctx, in.FilePath, in.Content)
		}
		_ = tctx.LSPManager.SaveFile(ctx, in.FilePath)
	}

	if isUpdate {
		diff := formatUnifiedDiff(
			computeDiff(splitLines(originalContent), splitLines(in.Content)),
			filepath.Base(in.FilePath),
		)
		content := fmt.Sprintf("The file %s has been updated successfully.", in.FilePath)
		if diff != "" {
			content += "\n\n" + diff
		}
		return &models.ToolResult{Content: content}, nil
	}

	return &models.ToolResult{
		Content: fmt.Sprintf("File created successfully at: %s", in.FilePath),
	}, nil
}

// checkCacheGuard enforces that an existing file was read before being written.
// Returns a non-nil error if the write should be rejected.
func checkCacheGuard(tctx *tools.ToolContext, filePath string, diskInfo os.FileInfo) error {
	if tctx == nil || tctx.FileState == nil {
		// No cache available — treat as unread.
		return fmt.Errorf(
			"You must read the file before writing to it. Use the Read tool first: %s",
			filePath,
		)
	}

	cached, ok := tctx.FileState.Get(filePath)
	if !ok {
		return fmt.Errorf(
			"You must read the file before writing to it. Use the Read tool first: %s",
			filePath,
		)
	}

	// Reject if the model only saw a partial/auto-injected view.
	if cached.IsPartialView != nil && *cached.IsPartialView {
		return fmt.Errorf(
			"Cannot write: file was loaded as a partial view. Read it fully before writing: %s",
			filePath,
		)
	}

	// Reject if the file was modified on disk after our read (stale cache).
	currentMtime := diskInfo.ModTime().UnixMilli()
	if currentMtime > cached.Timestamp {
		return fmt.Errorf(
			"File has been modified since read, either by the user or by a linter. "+
				"Read it again before attempting to write it: %s",
			filePath,
		)
	}

	return nil
}

// splitLines splits text into lines without trailing newline.
// An empty string returns an empty slice.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// Trim one trailing newline to avoid a phantom blank line at the end.
	s = strings.TrimRight(s, "\n")
	return strings.Split(s, "\n")
}

// ─── Diff algorithm ──────────────────────────────────────────────────────────

// opKind represents a diff operation on a single line.
type opKind byte

const (
	opKeep   opKind = '='
	opInsert opKind = '+'
	opDelete opKind = '-'
)

// editEntry is one entry in a diff edit script.
type editEntry struct {
	op   opKind
	text string
}

// computeDiff returns the edit script (LCS-based) from oldLines to newLines.
// For large inputs (> 2000 total lines after stripping common prefix/suffix)
// it falls back to "delete all + insert all" which is still a correct diff.
func computeDiff(oldLines, newLines []string) []editEntry {
	m, n := len(oldLines), len(newLines)

	if m == 0 && n == 0 {
		return nil
	}
	if m == 0 {
		edits := make([]editEntry, n)
		for i, l := range newLines {
			edits[i] = editEntry{opInsert, l}
		}
		return edits
	}
	if n == 0 {
		edits := make([]editEntry, m)
		for i, l := range oldLines {
			edits[i] = editEntry{opDelete, l}
		}
		return edits
	}

	// Strip common prefix — these lines are always "keep".
	pfx := 0
	for pfx < m && pfx < n && oldLines[pfx] == newLines[pfx] {
		pfx++
	}

	// Strip common suffix.
	sfx := 0
	for sfx < m-pfx && sfx < n-pfx && oldLines[m-1-sfx] == newLines[n-1-sfx] {
		sfx++
	}

	midOld := oldLines[pfx : m-sfx]
	midNew := newLines[pfx : n-sfx]

	// Choose algorithm based on middle size.
	var midEdits []editEntry
	if len(midOld)+len(midNew) <= 2000 {
		midEdits = lcsEdits(midOld, midNew)
	} else {
		// Fallback: "delete all old middle, insert all new middle"
		midEdits = make([]editEntry, 0, len(midOld)+len(midNew))
		for _, l := range midOld {
			midEdits = append(midEdits, editEntry{opDelete, l})
		}
		for _, l := range midNew {
			midEdits = append(midEdits, editEntry{opInsert, l})
		}
	}

	// Reassemble: prefix keeps + middle edits + suffix keeps.
	result := make([]editEntry, 0, pfx+len(midEdits)+sfx)
	for _, l := range oldLines[:pfx] {
		result = append(result, editEntry{opKeep, l})
	}
	result = append(result, midEdits...)
	for _, l := range oldLines[m-sfx:] {
		result = append(result, editEntry{opKeep, l})
	}
	return result
}

// lcsEdits computes the edit script from a to b using LCS DP with O(n) rolling rows.
// Precondition: len(a)+len(b) <= 2000 (caller enforces this).
//
// Space: two int32 rows (O(n)) for the forward DP pass + an int8 direction
// table (O(m×n), 1 byte/cell vs 4 for int32 → 4× smaller than the naive table).
func lcsEdits(a, b []string) []editEntry {
	m, n := len(a), len(b)
	if m == 0 && n == 0 {
		return nil
	}

	// direction constants stored in the dir table.
	const (
		dirMatch int8 = 1 // diagonal: a[i-1]==b[j-1]
		dirUp    int8 = 2 // up: delete a[i-1]
		dirLeft  int8 = 3 // left: insert b[j-1]
	)

	// Two rolling rows replace the full (m+1)×(n+1) int32 table.
	prev := make([]int32, n+1) // dp[i-1][*]
	curr := make([]int32, n+1) // dp[i][*] being filled

	// dir[i][j] encodes the backtrack decision made at cell (i, j).
	// Allocated once as a flat (m+1)×(n+1) int8 slab.
	dir := make([][]int8, m+1)
	for i := range dir {
		dir[i] = make([]int8, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
				dir[i][j] = dirMatch
			} else if prev[j] > curr[j-1] {
				// dp[i-1][j] strictly beats dp[i][j-1] → go up (delete).
				curr[j] = prev[j]
				dir[i][j] = dirUp
			} else {
				// dp[i][j-1] >= dp[i-1][j] → go left (insert), ties favour insert
				// to match the original backtrack condition.
				curr[j] = curr[j-1]
				dir[i][j] = dirLeft
			}
		}
		// Swap rows: prev becomes the row we just computed; curr is recycled.
		prev, curr = curr, prev
		// curr[0] is always 0 (base case dp[i][0]=0) and is never written in the
		// j loop, so no explicit clear is needed.
	}

	// Backtrack using the direction table.
	result := make([]editEntry, 0, m+n)
	i, j := m, n
	for i > 0 || j > 0 {
		if i == 0 {
			result = append(result, editEntry{opInsert, b[j-1]})
			j--
		} else if j == 0 {
			result = append(result, editEntry{opDelete, a[i-1]})
			i--
		} else {
			switch dir[i][j] {
			case dirMatch:
				result = append(result, editEntry{opKeep, a[i-1]})
				i--
				j--
			case dirLeft:
				result = append(result, editEntry{opInsert, b[j-1]})
				j--
			default: // dirUp
				result = append(result, editEntry{opDelete, a[i-1]})
				i--
			}
		}
	}

	// Reverse to get forward order.
	for lo, hi := 0, len(result)-1; lo < hi; lo, hi = lo+1, hi-1 {
		result[lo], result[hi] = result[hi], result[lo]
	}
	return result
}

// ─── Unified diff formatter ───────────────────────────────────────────────────

const contextLines = 3

// formatUnifiedDiff formats an edit script as a unified diff string.
// Returns an empty string when there are no changes.
//
// Algorithm (two passes):
//  1. Find all change indices, then compute hunk ranges [start,end) in edit-
//     array coordinates by expanding each change cluster by contextLines on
//     both sides and merging overlapping windows.
//  2. Walk each hunk range, accumulate @@ line counters, and emit lines.
func formatUnifiedDiff(edits []editEntry, filename string) string {
	if len(edits) == 0 {
		return ""
	}

	// Pass 1: locate all non-keep edit indices.
	var changeIdx []int
	for i, e := range edits {
		if e.op != opKeep {
			changeIdx = append(changeIdx, i)
		}
	}
	if len(changeIdx) == 0 {
		return "" // nothing changed
	}

	// Compute hunk ranges [lo, hi) in edit-array indices.
	// Two consecutive changes belong to the same hunk when the gap between
	// them is <= 2*contextLines (so their context windows overlap or touch).
	type hunkRange struct{ lo, hi int }
	var ranges []hunkRange

	lo := changeIdx[0] - contextLines
	if lo < 0 {
		lo = 0
	}
	hi := changeIdx[0] + contextLines + 1
	if hi > len(edits) {
		hi = len(edits)
	}

	for _, ci := range changeIdx[1:] {
		newHi := ci + contextLines + 1
		if newHi > len(edits) {
			newHi = len(edits)
		}
		if ci-contextLines <= hi {
			// Overlapping or adjacent — extend current hunk.
			hi = newHi
		} else {
			ranges = append(ranges, hunkRange{lo, hi})
			lo = ci - contextLines
			if lo < 0 {
				lo = 0
			}
			hi = newHi
		}
	}
	ranges = append(ranges, hunkRange{lo, hi})

	// Pass 2: format each hunk.
	var sb strings.Builder
	sb.WriteString("--- " + filename + "\n")
	sb.WriteString("+++ " + filename + "\n")

	for _, r := range ranges {
		// Compute 1-based old/new line numbers at the start of this hunk.
		oldStart, newStart := 1, 1
		for k := 0; k < r.lo; k++ {
			if edits[k].op != opInsert {
				oldStart++
			}
			if edits[k].op != opDelete {
				newStart++
			}
		}

		// Count lines in old and new files within this hunk.
		oldCount, newCount := 0, 0
		for k := r.lo; k < r.hi; k++ {
			if edits[k].op != opInsert {
				oldCount++
			}
			if edits[k].op != opDelete {
				newCount++
			}
		}

		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
		for k := r.lo; k < r.hi; k++ {
			e := edits[k]
			switch e.op {
			case opKeep:
				sb.WriteString(" " + e.text + "\n")
			case opDelete:
				sb.WriteString("-" + e.text + "\n")
			case opInsert:
				sb.WriteString("+" + e.text + "\n")
			}
		}
	}

	return sb.String()
}
