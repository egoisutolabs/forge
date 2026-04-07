// Package log provides debug logging for Forge.
// Enable with FORGE_DEBUG=1 environment variable.
// Logs go to ~/.forge/debug.log (not stderr, which would mess up the TUI).
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	enabled bool
	file    *os.File
	mu      sync.Mutex
)

func init() {
	if os.Getenv("FORGE_DEBUG") == "1" || os.Getenv("FORGE_DEBUG") == "true" {
		enabled = true
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".forge")
		os.MkdirAll(dir, 0755)
		path := filepath.Join(dir, "debug.log")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			file = f
			fmt.Fprintf(f, "\n=== Forge session started at %s ===\n", time.Now().Format(time.RFC3339))
		}
	}
}

// Debug logs a message if FORGE_DEBUG is enabled.
func Debug(format string, args ...any) {
	if !enabled || file == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(file, "[%s] %s\n", ts, fmt.Sprintf(format, args...))
}

// Close flushes and closes the debug log file. Safe to call multiple times.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
}

// Enabled returns true if debug logging is active.
func Enabled() bool {
	return enabled
}
