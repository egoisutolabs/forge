package bash

import (
	"bytes"
	"sync"
	"time"
)

// ProgressEvent carries a snapshot of command execution state, fired
// approximately every second while the command runs.
// Mirrors the bash_progress data shape from Claude Code's BashTool.tsx.
type ProgressEvent struct {
	Output     string // full stdout+stderr captured so far (empty during progress ticks; populated by caller from final result if needed)
	TotalLines int    // number of newlines written so far
	TotalBytes int    // byte length of output so far
	ElapsedMs  int64  // milliseconds since command started
}

// safeBuffer is a bytes.Buffer protected by a mutex for concurrent
// read/write from the command output and the progress goroutine.
// It also tracks the newline count incrementally to avoid rescanning
// the full buffer on every progress tick.
type safeBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	lineCount int
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.lineCount += bytes.Count(p, []byte{'\n'})
	return sb.buf.Write(p)
}

// String returns a copy of the current buffer contents, safe for concurrent use.
func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// Stats returns the current byte length and newline count without copying the
// buffer. Used by the progress poller to avoid an O(n) allocation every tick.
func (sb *safeBuffer) Stats() (byteLen, lineCount int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len(), sb.lineCount
}

// startProgressPoller launches a goroutine that calls onProgress every ~1 second
// until done is closed. It returns a channel that is closed once the goroutine
// has fully exited — callers must receive from it before reading any values
// written inside onProgress, to avoid data races.
func startProgressPoller(buf *safeBuffer, start time.Time, done <-chan struct{}, onProgress func(ProgressEvent)) <-chan struct{} {
	finished := make(chan struct{})
	go func() {
		defer close(finished)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Use Stats() to avoid copying the full output buffer on every tick.
				// Callers that need the full output should read ExecResult.Stdout.
				byteLen, lineCount := buf.Stats()
				onProgress(ProgressEvent{
					TotalLines: lineCount,
					TotalBytes: byteLen,
					ElapsedMs:  time.Since(start).Milliseconds(),
				})
			}
		}
	}()
	return finished
}
