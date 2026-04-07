// Package grep implements the GrepTool — search file contents with ripgrep.
//
// This is the Go port of Claude Code's GrepTool (GrepTool.ts).
// Key behaviours:
//   - Shells out to the rg (ripgrep) binary
//   - Three output modes: content, files_with_matches (default), count
//   - VCS directories excluded automatically (.git, .svn, .hg, .bzr, .jj, .sl)
//   - Line length capped at 500 chars (--max-columns 500)
//   - Default head_limit of 250; offset for pagination
//   - Always read-only and concurrency-safe
package grep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// vcsDirectories are version-control directories excluded from every search.
// Mirrors Claude Code's VCS_DIRECTORIES_TO_EXCLUDE.
var vcsDirectories = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

// defaultHeadLimit is the result cap when head_limit is not specified.
// 250 is generous for exploratory searches while preventing context bloat.
const defaultHeadLimit = 250

// ── Input schema ──────────────────────────────────────────────────────────────

// toolInput is the JSON-decoded input for GrepTool.
//
// Field names starting with '-' (e.g. -B, -A, -C, -n, -i) match Claude Code's
// GrepTool schema exactly. In Go's encoding/json, the special "omit" sentinel is
// exactly `json:"-"` (a bare dash); `json:"-B,omitempty"` names the field "-B".
type toolInput struct {
	Pattern      string `json:"pattern"`
	Path         string `json:"path,omitempty"`
	Glob         string `json:"glob,omitempty"`
	OutputMode   string `json:"output_mode,omitempty"` // "content" | "files_with_matches" | "count"
	ContextB     *int   `json:"-B,omitempty"`          // lines before each match
	ContextA     *int   `json:"-A,omitempty"`          // lines after each match
	ContextC     *int   `json:"-C,omitempty"`          // alias for context
	Context      *int   `json:"context,omitempty"`     // lines before+after (takes precedence over -C)
	ShowLineNums *bool  `json:"-n,omitempty"`          // show line numbers (content mode, default true)
	CaseInsens   *bool  `json:"-i,omitempty"`          // case-insensitive
	Type         string `json:"type,omitempty"`        // rg --type filter
	HeadLimit    *int   `json:"head_limit,omitempty"`  // nil→250, 0→unlimited
	Offset       *int   `json:"offset,omitempty"`      // skip first N results
	Multiline    *bool  `json:"multiline,omitempty"`   // -U --multiline-dotall
}

// ── Tool ──────────────────────────────────────────────────────────────────────

// Tool implements the tools.Tool interface for grep/ripgrep searching.
type Tool struct{}

func (t *Tool) Name() string { return "Grep" }
func (t *Tool) Description() string {
	return "Plain-text and regex file search with ripgrep (rg). Use AstGrep first for structural code search, and use Grep when you need raw text or regex matching."
}

func (t *Tool) SearchHint() string {
	return "plain text regex ripgrep rg fallback literal string log config markdown json yaml search"
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The regular expression pattern to search for in file contents"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in (rg PATH). Defaults to current working directory."
			},
			"glob": {
				"type": "string",
				"description": "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\") - maps to rg --glob"
			},
			"output_mode": {
				"type": "string",
				"enum": ["content", "files_with_matches", "count"],
				"description": "Output mode: \"content\" shows matching lines, \"files_with_matches\" shows file paths (default), \"count\" shows match counts."
			},
			"-B": {"type": "number", "description": "Lines before each match (content mode only)"},
			"-A": {"type": "number", "description": "Lines after each match (content mode only)"},
			"-C": {"type": "number", "description": "Alias for context lines before+after"},
			"context": {"type": "number", "description": "Lines before+after each match (takes precedence over -C)"},
			"-n": {"type": "boolean", "description": "Show line numbers (content mode, default true)"},
			"-i": {"type": "boolean", "description": "Case insensitive search"},
			"type": {"type": "string", "description": "File type filter (rg --type). E.g. go, py, js."},
			"head_limit": {"type": "number", "description": "Max results (default 250, 0=unlimited)"},
			"offset": {"type": "number", "description": "Skip first N results for pagination"},
			"multiline": {"type": "boolean", "description": "Enable multiline mode (-U --multiline-dotall)"}
		},
		"required": ["pattern"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return errors.New("pattern is required and cannot be empty")
	}
	if in.OutputMode != "" {
		switch in.OutputMode {
		case "content", "files_with_matches", "count":
			// valid
		default:
			return fmt.Errorf("invalid output_mode %q: must be content, files_with_matches, or count", in.OutputMode)
		}
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	// GrepTool is always read-only — auto-approve.
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

// Execute runs ripgrep with the provided input and returns formatted results.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: "invalid input: " + err.Error(), IsError: true}, nil
	}

	cwd := "."
	if tctx != nil && tctx.Cwd != "" {
		cwd = tctx.Cwd
	}

	args, searchPath := buildRgArgs(in, cwd)
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 1:
				// Exit code 1 = no matches found (not an error).
				out = []byte{}
			case 2:
				// Exit code 2 = real error.
				return &models.ToolResult{
					Content: "rg error: " + string(exitErr.Stderr),
					IsError: true,
				}, nil
			default:
				return &models.ToolResult{
					Content: fmt.Sprintf("rg exited with code %d: %s", exitErr.ExitCode(), exitErr.Stderr),
					IsError: true,
				}, nil
			}
		} else if errors.Is(err, exec.ErrNotFound) {
			return &models.ToolResult{
				Content: "ripgrep (rg) is not installed or not in PATH",
				IsError: true,
			}, nil
		} else {
			return &models.ToolResult{Content: "rg error: " + err.Error(), IsError: true}, nil
		}
	}

	return formatOutput(in, string(out), cwd), nil
}

// ── Core logic ────────────────────────────────────────────────────────────────

// buildRgArgs constructs the ripgrep argument list from the tool input.
// Returns (args_without_search_path, resolved_search_path).
// The caller must append the search path before passing to exec.Command.
func buildRgArgs(in toolInput, cwd string) (args []string, searchPath string) {
	// Resolve search path.
	searchPath = cwd
	if in.Path != "" {
		if filepath.IsAbs(in.Path) {
			searchPath = in.Path
		} else {
			searchPath = filepath.Join(cwd, in.Path)
		}
	}

	args = []string{"--hidden"}

	// Exclude VCS directories.
	for _, dir := range vcsDirectories {
		args = append(args, "--glob", "!"+dir)
	}

	// Cap line length to prevent base64/minified content from cluttering output.
	args = append(args, "--max-columns", "500")

	// Multiline mode (only when explicitly requested).
	if in.Multiline != nil && *in.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}

	// Case insensitive.
	if in.CaseInsens != nil && *in.CaseInsens {
		args = append(args, "-i")
	}

	// Output mode.
	mode := outputMode(in)
	switch mode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	case "content":
		// Line numbers default to on in content mode.
		showLineNums := true
		if in.ShowLineNums != nil {
			showLineNums = *in.ShowLineNums
		}
		if showLineNums {
			args = append(args, "-n")
		}
		// Context lines: context / -C field takes precedence over -B / -A.
		if in.Context != nil {
			args = append(args, "-C", strconv.Itoa(*in.Context))
		} else if in.ContextC != nil {
			args = append(args, "-C", strconv.Itoa(*in.ContextC))
		} else {
			if in.ContextB != nil {
				args = append(args, "-B", strconv.Itoa(*in.ContextB))
			}
			if in.ContextA != nil {
				args = append(args, "-A", strconv.Itoa(*in.ContextA))
			}
		}
	}

	// Pattern — use -e when the pattern starts with '-' to prevent rg from
	// interpreting it as a flag.
	if strings.HasPrefix(in.Pattern, "-") {
		args = append(args, "-e", in.Pattern)
	} else {
		args = append(args, in.Pattern)
	}

	// Type filter.
	if in.Type != "" {
		args = append(args, "--type", in.Type)
	}

	// Glob filter — split on whitespace; preserve brace patterns intact.
	if in.Glob != "" {
		for _, rawPattern := range strings.Fields(in.Glob) {
			if strings.Contains(rawPattern, "{") && strings.Contains(rawPattern, "}") {
				// Brace patterns (e.g. *.{ts,tsx}) must not be split on comma.
				args = append(args, "--glob", rawPattern)
			} else {
				for _, p := range strings.Split(rawPattern, ",") {
					if p != "" {
						args = append(args, "--glob", p)
					}
				}
			}
		}
	}

	return args, searchPath
}

// outputMode returns the effective output mode (defaults to "files_with_matches").
func outputMode(in toolInput) string {
	if in.OutputMode != "" {
		return in.OutputMode
	}
	return "files_with_matches"
}

// applyHeadLimit slices items according to offset and head_limit.
//
// Rules (matching Claude Code's applyHeadLimit):
//   - limit == nil → use defaultHeadLimit
//   - *limit == 0  → unlimited (return everything after offset)
//   - *limit > 0   → cap at that many items
//
// Returns (sliced, truncated) where truncated is true only when items were dropped.
func applyHeadLimit(items []string, limit *int, offset int) (result []string, truncated bool) {
	// Clamp offset.
	if offset > len(items) {
		offset = len(items)
	}
	after := items[offset:]

	// Explicit 0 = unlimited.
	if limit != nil && *limit == 0 {
		return after, false
	}

	effective := defaultHeadLimit
	if limit != nil {
		effective = *limit
	}

	if len(after) <= effective {
		return after, false
	}
	return after[:effective], true
}

// formatOutput converts raw rg stdout into a ToolResult.
func formatOutput(in toolInput, rawOutput, cwd string) *models.ToolResult {
	mode := outputMode(in)

	// Split output into non-empty lines.
	var lines []string
	for _, line := range strings.Split(rawOutput, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	offset := 0
	if in.Offset != nil {
		offset = *in.Offset
	}

	switch mode {
	case "content":
		return formatContentOutput(in, lines, cwd, offset)
	case "count":
		return formatCountOutput(in, lines, cwd, offset)
	default: // files_with_matches
		return formatFilesOutput(in, lines, cwd, offset)
	}
}

// formatContentOutput handles the "content" output mode.
// Converts absolute paths in each line to relative paths, applies pagination,
// and appends a pagination note when truncation occurred.
func formatContentOutput(in toolInput, lines []string, cwd string, offset int) *models.ToolResult {
	limited, truncated := applyHeadLimit(lines, in.HeadLimit, offset)

	// Convert absolute paths in each line: "/abs/path:rest" → "rel/path:rest"
	rel := make([]string, len(limited))
	for i, line := range limited {
		if colonIdx := strings.IndexByte(line, ':'); colonIdx > 0 {
			filePart := line[:colonIdx]
			rest := line[colonIdx:]
			rel[i] = relativizePath(filePart, cwd) + rest
		} else {
			rel[i] = line
		}
	}

	var content string
	if len(rel) == 0 {
		content = "No matches found"
	} else {
		content = strings.Join(rel, "\n")
	}

	if truncated {
		content += "\n\n" + formatLimitInfo(in.HeadLimit, offset)
	}
	return &models.ToolResult{Content: content}
}

// formatCountOutput handles the "count" output mode.
// Lines from rg are "path:count". We sum totals and format a summary.
func formatCountOutput(in toolInput, lines []string, cwd string, offset int) *models.ToolResult {
	limited, truncated := applyHeadLimit(lines, in.HeadLimit, offset)

	var totalMatches int
	relLines := make([]string, 0, len(limited))
	for _, line := range limited {
		colonIdx := strings.LastIndexByte(line, ':')
		if colonIdx > 0 {
			filePart := line[:colonIdx]
			countStr := line[colonIdx+1:]
			if n, err := strconv.Atoi(countStr); err == nil {
				totalMatches += n
			}
			relLines = append(relLines, relativizePath(filePart, cwd)+":"+line[colonIdx+1:])
		} else {
			relLines = append(relLines, line)
		}
	}

	var content string
	if len(relLines) == 0 {
		content = "No matches found"
	} else {
		files := len(relLines)
		fileWord := "file"
		if files != 1 {
			fileWord = "files"
		}
		matchWord := "occurrence"
		if totalMatches != 1 {
			matchWord = "occurrences"
		}
		summary := fmt.Sprintf("Found %d total %s across %d %s.", totalMatches, matchWord, files, fileWord)
		if truncated {
			summary += " " + formatLimitInfo(in.HeadLimit, offset)
		}
		content = strings.Join(relLines, "\n") + "\n\n" + summary
	}
	return &models.ToolResult{Content: content}
}

// formatFilesOutput handles the "files_with_matches" output mode.
// Sorts results by modification time (most recently modified first),
// applies pagination, and converts paths to relative.
func formatFilesOutput(in toolInput, lines []string, cwd string, offset int) *models.ToolResult {
	if len(lines) == 0 {
		return &models.ToolResult{Content: "No files found"}
	}

	// Sort by mtime descending, with alphabetical tiebreaker.
	type fileEntry struct {
		path  string
		mtime int64
	}
	entries := make([]fileEntry, len(lines))
	for i, p := range lines {
		var mtime int64
		if info, err := os.Stat(p); err == nil {
			mtime = info.ModTime().UnixMilli()
		}
		entries[i] = fileEntry{path: p, mtime: mtime}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].mtime != entries[j].mtime {
			return entries[i].mtime > entries[j].mtime // newer first
		}
		return entries[i].path < entries[j].path // alphabetical tiebreaker
	})

	sorted := make([]string, len(entries))
	for i, e := range entries {
		sorted[i] = e.path
	}

	limited, truncated := applyHeadLimit(sorted, in.HeadLimit, offset)

	// Relativize paths.
	rel := make([]string, len(limited))
	for i, p := range limited {
		rel[i] = relativizePath(p, cwd)
	}

	n := len(rel)
	fileWord := "file"
	if n != 1 {
		fileWord = "files"
	}

	header := fmt.Sprintf("Found %d %s", n, fileWord)
	if truncated {
		header += " " + formatLimitInfo(in.HeadLimit, offset)
	}
	return &models.ToolResult{Content: header + "\n" + strings.Join(rel, "\n")}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// relativizePath converts an absolute file path to a path relative to cwd.
// If the path is not under cwd, it is returned unchanged.
func relativizePath(absPath, cwd string) string {
	// Clean the cwd to remove trailing slashes.
	cwd = filepath.Clean(cwd)
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	// filepath.Rel returns paths starting with ".." if not under cwd.
	if strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}

// formatLimitInfo produces the pagination note appended to truncated results.
func formatLimitInfo(headLimit *int, offset int) string {
	effective := defaultHeadLimit
	if headLimit != nil && *headLimit > 0 {
		effective = *headLimit
	}
	parts := []string{fmt.Sprintf("limit: %d", effective)}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("offset: %d", offset))
	}
	return "[Showing results with pagination = " + strings.Join(parts, ", ") + "]"
}
