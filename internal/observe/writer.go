package observe

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const (
	channelBuffer = 4096 // buffer before backpressure
	maxOutputLen  = 4096 // truncate tool output in events
	maxPromptLen  = 2048 // truncate agent prompts in events
)

// Writer is the async JSONL log writer.
type Writer struct {
	ch        chan Event
	file      *os.File
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewWriter opens (or creates) the log file at path and starts the
// background writer goroutine.
func NewWriter(path string) (*Writer, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}

	w := &Writer{
		ch:   make(chan Event, channelBuffer),
		file: f,
	}
	w.wg.Add(1)
	go w.drain()
	return w, nil
}

// Write sends an event to the background goroutine. Non-blocking: if the
// channel is full the event is dropped silently.
func (w *Writer) Write(e Event) {
	select {
	case w.ch <- e:
	default:
	}
}

// Close flushes remaining events and closes the file.
func (w *Writer) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.ch)
		w.wg.Wait()
		err = w.file.Close()
	})
	return err
}

func (w *Writer) drain() {
	defer w.wg.Done()
	bw := bufio.NewWriterSize(w.file, 32*1024) // 32KB write buffer
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for e := range w.ch {
		_ = enc.Encode(e)
	}
	_ = bw.Flush() // flush remaining on shutdown
}

// logsDir returns ~/.forge/logs/, creating it if necessary.
func logsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".forge", "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
