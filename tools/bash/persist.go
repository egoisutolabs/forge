package bash

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// MaxResultSizeChars is the output length threshold above which the full
	// result is persisted to disk and the model receives a preview instead.
	// Mirrors Claude Code's DEFAULT_MAX_RESULT_SIZE_CHARS = 50_000.
	MaxResultSizeChars = 50_000

	// MaxPersistedSize is the maximum bytes written to disk for a single
	// tool result. Output larger than this is truncated before writing.
	MaxPersistedSize = 64 * 1024 * 1024 // 64 MB

	// PreviewSizeBytes is the number of bytes from the start of the persisted
	// output returned inline to the model as a preview.
	PreviewSizeBytes = 2_000
)

// PersistOutput saves large command output to ~/.forge/tool-results/{toolUseId}.txt
// and returns the file path and a preview of the first PreviewSizeBytes bytes.
//
// If output is within MaxResultSizeChars, no file is written and both returned
// strings are empty (caller should use the original output as-is).
//
// If output exceeds MaxPersistedSize it is truncated to MaxPersistedSize before
// writing to avoid unbounded disk usage.
func PersistOutput(output string, toolUseId string) (persistedPath string, preview string, err error) {
	if len(output) <= MaxResultSizeChars {
		return "", "", nil
	}

	// Truncate to cap before writing.
	data := output
	if len(data) > MaxPersistedSize {
		data = data[:MaxPersistedSize]
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolving home dir: %w", err)
	}

	dir := filepath.Join(home, ".forge", "tool-results")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("creating tool-results dir: %w", err)
	}
	// Ensure tight permissions even if the directory pre-existed with looser mode.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("chmod tool-results dir: %w", err)
	}

	path := filepath.Join(dir, toolUseId+".txt")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return "", "", fmt.Errorf("writing tool result: %w", err)
	}

	p := data
	if len(p) > PreviewSizeBytes {
		p = p[:PreviewSizeBytes]
	}

	return path, p, nil
}
