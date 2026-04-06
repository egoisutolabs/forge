package glob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

const defaultMaxResults = 100

// Tool implements the Glob tool — find files matching a glob pattern.
//
// This is the Go equivalent of Claude Code's GlobTool:
//   - Accepts a pattern (** supported) and an optional base path
//   - Returns results sorted by modification time, newest first
//   - Returns paths relative to cwd to save tokens
//   - Always read-only and concurrency-safe
type Tool struct{}

// toolInput is the JSON input shape.
type toolInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *Tool) Name() string        { return "Glob" }
func (t *Tool) Description() string { return "Find files matching a glob pattern" }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The glob pattern to match files against"
			},
			"path": {
				"type": "string",
				"description": "The directory to search in. If not specified, the current working directory will be used. IMPORTANT: Omit this field to use the default directory. DO NOT enter \"undefined\" or \"null\" - simply omit it for the default behavior. Must be a valid directory path if provided."
			}
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
		return fmt.Errorf("pattern is required and cannot be empty")
	}
	if in.Path != "" {
		info, err := os.Stat(in.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", in.Path)
			}
			return fmt.Errorf("cannot access path: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", in.Path)
		}
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	// Glob is always read-only — no permission check needed.
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	cwd := ""
	maxResults := defaultMaxResults
	if tctx != nil {
		cwd = tctx.Cwd
		if tctx.GlobMaxResults > 0 {
			maxResults = tctx.GlobMaxResults
		}
	}

	// Normalize the user-supplied path to eliminate ".." traversal components.
	basePath := in.Path
	if basePath != "" {
		basePath = filepath.Clean(basePath)
	}
	if basePath == "" {
		basePath = cwd
	}
	if basePath == "" {
		var err error
		basePath, err = os.Getwd()
		if err != nil {
			return &models.ToolResult{Content: fmt.Sprintf("Cannot determine working directory: %s", err), IsError: true}, nil
		}
	}

	// Use doublestar against os.DirFS so paths returned are relative to basePath.
	fsys := os.DirFS(basePath)
	matches, err := doublestar.Glob(fsys, in.Pattern)
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid glob pattern: %s", err), IsError: true}, nil
	}

	if len(matches) == 0 {
		return &models.ToolResult{Content: "No files found"}, nil
	}

	// Stat each match to get mtime for sorting. Build (path, mtime) pairs.
	type entry struct {
		relToBase string
		mtime     int64 // Unix nano
	}
	entries := make([]entry, 0, len(matches))
	for _, rel := range matches {
		abs := filepath.Join(basePath, rel)
		info, err := os.Stat(abs)
		if err != nil {
			continue // skip files we can't stat (race: deleted between glob and stat)
		}
		if info.IsDir() {
			continue // only return files, not directories
		}
		entries = append(entries, entry{relToBase: rel, mtime: info.ModTime().UnixNano()})
	}

	// Sort newest first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mtime > entries[j].mtime
	})

	truncated := len(entries) > maxResults
	if truncated {
		entries = entries[:maxResults]
	}

	// Convert to display paths: relative to cwd when possible, else absolute.
	lines := make([]string, 0, len(entries)+1)
	for _, e := range entries {
		abs := filepath.Join(basePath, e.relToBase)
		display := abs
		if cwd != "" {
			if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
				display = rel
			}
		}
		lines = append(lines, display)
	}

	if truncated {
		lines = append(lines, "(Results are truncated. Consider using a more specific path or pattern.)")
	}

	return &models.ToolResult{Content: strings.Join(lines, "\n")}, nil
}
