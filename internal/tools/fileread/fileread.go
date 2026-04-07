// Package fileread implements the FileReadTool — the Go port of Claude Code's
// FileReadTool. It reads text files with optional line-range control, prepends
// cat -n style line numbers, and updates the FileStateCache so downstream
// FileEditTool and FileWriteTool can gate their mutations on a prior read.
package fileread

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// DefaultLineLimit is the maximum number of lines returned when the caller
// does not specify a limit. Mirrors Claude Code's default of 2000 lines.
const DefaultLineLimit = 2000

// binaryExtensions is the set of file extensions that cannot be read as text.
// This matches Claude Code's BINARY_EXTENSIONS in constants/files.ts.
// PDF is included here; the Go port does not natively render PDFs.
var binaryExtensions = map[string]bool{
	// Images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".webp": true, ".tiff": true, ".tif": true,
	// Videos
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
	".wmv": true, ".flv": true, ".m4v": true, ".mpeg": true, ".mpg": true,
	// Audio
	".mp3": true, ".wav": true, ".ogg": true, ".flac": true, ".aac": true,
	".m4a": true, ".wma": true, ".aiff": true, ".opus": true,
	// Archives
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".7z": true,
	".rar": true, ".xz": true, ".z": true, ".tgz": true, ".iso": true,
	// Executables / binaries
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".o": true, ".a": true, ".obj": true, ".lib": true, ".app": true,
	".msi": true, ".deb": true, ".rpm": true,
	// Documents
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	// Fonts
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	// Bytecode / VM artifacts
	".pyc": true, ".pyo": true, ".class": true, ".jar": true, ".war": true,
	".ear": true, ".node": true, ".wasm": true, ".rlib": true,
	// Database files
	".sqlite": true, ".sqlite3": true, ".db": true, ".mdb": true, ".idx": true,
	// Design / 3D
	".psd": true, ".ai": true, ".eps": true, ".sketch": true, ".fig": true,
	".xd": true, ".blend": true, ".3ds": true, ".max": true,
	// Flash
	".swf": true, ".fla": true,
	// Lock / profiling data
	".lockb": true, ".dat": true, ".data": true,
}

// toolInput is the JSON schema for FileReadTool input.
type toolInput struct {
	FilePath string `json:"file_path"`
	Offset   *int   `json:"offset,omitempty"` // 1-based line number to start from
	Limit    *int   `json:"limit,omitempty"`  // number of lines to read
}

// Tool implements the Read tool — read text files with optional line-range control.
//
// This is the Go port of Claude Code's FileReadTool. Simplified behaviors:
//   - Text files only (no PDF / image / notebook rendering)
//   - Line numbers in cat -n format: %6d\t%s
//   - offset is 1-based; default 1 (start of file)
//   - limit defaults to DefaultLineLimit (2000)
//   - Updates tools.ToolContext.FileState after a successful read
type Tool struct{}

func (t *Tool) Name() string        { return "Read" }
func (t *Tool) Description() string { return "Reads a file from the local filesystem." }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The absolute path to the file to read"
			},
			"offset": {
				"type": "integer",
				"description": "The line number to start reading from (1-based). Only provide if the file is too large to read at once."
			},
			"limit": {
				"type": "integer",
				"description": "The number of lines to read. Only provide if the file is too large to read at once."
			}
		},
		"required": ["file_path"]
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
	if in.Offset != nil && *in.Offset < 1 {
		return fmt.Errorf("offset must be >= 1 (1-based line number), got %d", *in.Offset)
	}
	if in.Limit != nil && *in.Limit < 1 {
		return fmt.Errorf("limit must be >= 1, got %d", *in.Limit)
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	if len(input) > 0 {
		var in toolInput
		if err := json.Unmarshal(input, &in); err == nil && in.FilePath != "" {
			// Lexical sensitivity check first. Classification should not
			// depend on filesystem state: a path like /etc/ssl/private/*.pem
			// may exist-but-be-unreadable on Linux and not exist on macOS,
			// and we want the same PermAsk answer in both cases.
			cleaned := filepath.Clean(in.FilePath)
			if isSensitivePath(cleaned) {
				return &models.PermissionDecision{
					Behavior: models.PermAsk,
					Message:  fmt.Sprintf("read sensitive file %s", cleaned),
				}, nil
			}
			// Resolve symlinks to catch bypass attempts where a safe-looking
			// symlink points at a secret. Only then propagate a resolve error
			// as PermDeny.
			resolved, resolveErr := tools.ResolvePath(in.FilePath)
			if resolveErr != nil {
				return &models.PermissionDecision{
					Behavior: models.PermDeny,
					Message:  resolveErr.Error(),
				}, nil
			}
			if isSensitivePath(resolved) {
				return &models.PermissionDecision{
					Behavior: models.PermAsk,
					Message:  fmt.Sprintf("read sensitive file %s", resolved),
				}, nil
			}
		}
	}
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

// isSensitivePath returns true for paths that warrant user approval before reading:
// .env*, *credentials*, *secret*, *.pem, *.key, ~/.ssh/*, ~/.aws/*
func isSensitivePath(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	if strings.HasPrefix(base, ".env") ||
		strings.Contains(base, "credentials") ||
		strings.Contains(base, "secret") ||
		strings.HasSuffix(base, ".pem") ||
		strings.HasSuffix(base, ".key") {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil {
		sep := string(filepath.Separator)
		sshDir := filepath.Join(home, ".ssh")
		awsDir := filepath.Join(home, ".aws")
		if p == sshDir || strings.HasPrefix(p, sshDir+sep) {
			return true
		}
		if p == awsDir || strings.HasPrefix(p, awsDir+sep) {
			return true
		}
	}
	return false
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

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

	// Reject binary extensions
	if isBinaryExtension(in.FilePath) {
		ext := strings.ToLower(filepath.Ext(in.FilePath))
		return &models.ToolResult{
			Content: fmt.Sprintf(
				"Cannot read binary file (extension %s). Use appropriate tools for binary file analysis.",
				ext,
			),
			IsError: true,
		}, nil
	}

	// Resolve defaults
	offset := 1
	if in.Offset != nil {
		offset = *in.Offset
	}
	limit := DefaultLineLimit
	if in.Limit != nil {
		limit = *in.Limit
	}

	// Read file with line-range control
	rawContent, totalLines, mtimeMs, err := readLinesFromFile(in.FilePath, offset, limit)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.ToolResult{
				Content: fmt.Sprintf("File does not exist: %s", in.FilePath),
				IsError: true,
			}, nil
		}
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Update FileStateCache with raw (un-numbered) content and mtime.
	// FileEditTool and FileWriteTool require a prior read in the cache before
	// they will accept edits — storing here enables that gate.
	if tctx != nil && tctx.FileState != nil {
		off := offset
		lim := limit
		tctx.FileState.Set(in.FilePath, tools.FileState{
			Content:   rawContent,
			Timestamp: mtimeMs,
			Offset:    &off,
			Limit:     &lim,
		})
	}

	// Handle empty content (empty file or offset beyond end)
	if rawContent == "" {
		if totalLines == 0 {
			return &models.ToolResult{
				Content: "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>",
			}, nil
		}
		return &models.ToolResult{
			Content: fmt.Sprintf(
				"<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>",
				offset, totalLines,
			),
		}, nil
	}

	// Notify LSP server about the opened file (best-effort).
	if tctx != nil && tctx.LSPManager != nil {
		_ = tctx.LSPManager.OpenFile(ctx, in.FilePath, rawContent)
	}

	return &models.ToolResult{Content: addLineNumbers(rawContent, offset)}, nil
}

// readLinesFromFile reads up to limit lines starting from offset (1-based) of
// the file at filePath. Returns:
//   - rawContent: the selected lines joined by newlines (no trailing newline)
//   - totalLines: total number of lines in the file
//   - mtimeMs: file modification time in unix milliseconds
//   - err: any I/O error
func readLinesFromFile(filePath string, offset, limit int) (rawContent string, totalLines int, mtimeMs int64, err error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return "", 0, 0, err
	}
	mtimeMs = info.ModTime().UnixMilli()

	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()

	// lineStart is 0-based index of the first line to include
	lineStart := offset - 1
	if lineStart < 0 {
		lineStart = 0
	}

	var selected []string
	scanner := bufio.NewScanner(f)
	// Increase buffer size for very long lines
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		totalLines++
		idx := totalLines - 1 // 0-based index of this line

		if idx < lineStart {
			continue // before requested range
		}
		if len(selected) < limit {
			selected = append(selected, scanner.Text())
		}
		// Continue scanning to count totalLines — don't break early.
	}
	if err := scanner.Err(); err != nil {
		return "", 0, 0, err
	}

	return strings.Join(selected, "\n"), totalLines, mtimeMs, nil
}

// addLineNumbers prepends cat -n style line numbers to content.
// startLine is the file-relative line number of the first line (1-based).
// Format: "%6d\t%s" matching `cat -n` output.
func addLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = fmt.Sprintf("%6d\t%s", startLine+i, line)
	}
	return strings.Join(result, "\n")
}

// isBinaryExtension returns true if filePath has a known binary file extension.
func isBinaryExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return binaryExtensions[ext]
}
