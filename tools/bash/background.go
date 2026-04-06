package bash

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// maxTaskOutputBytes is the output file size limit before the watchdog kills the process.
// Set to 5 GB, matching Claude Code's MAX_TASK_OUTPUT_BYTES.
// Stored as an atomic int64 so tests can safely reduce it while the watchdog goroutine
// runs concurrently (the race detector flags plain int64 reads/writes across goroutines).
var maxTaskOutputBytes atomic.Int64

// watchdogInterval is how often the size watchdog checks the output file.
// Stored as an atomic int64 (nanoseconds) so tests can safely override it concurrently.
var watchdogInterval atomic.Int64

func init() {
	maxTaskOutputBytes.Store(5 * 1024 * 1024 * 1024)
	watchdogInterval.Store(int64(5 * time.Second))
}

// BackgroundTaskStatus represents the lifecycle state of a background task.
type BackgroundTaskStatus string

const (
	StatusRunning   BackgroundTaskStatus = "running"
	StatusCompleted BackgroundTaskStatus = "completed"
	StatusKilled    BackgroundTaskStatus = "killed"
)

// BackgroundTask represents a command running in the background.
// The command's combined stdout+stderr is written to OutputFile.
type BackgroundTask struct {
	TaskID     string
	Command    string
	OutputFile string
	StartTime  time.Time

	mu       sync.Mutex
	status   BackgroundTaskStatus
	exitCode int
	cmd      *exec.Cmd // set after process starts; nil before
}

// Status returns the current lifecycle state of the task.
func (t *BackgroundTask) Status() BackgroundTaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// ExitCode returns the process exit code. Only meaningful once status is StatusCompleted.
func (t *BackgroundTask) ExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitCode
}

// PID returns the OS process ID, or 0 if the process has not started yet.
func (t *BackgroundTask) PID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cmd == nil || t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

// Stop kills the background task. Safe to call from any goroutine.
// If the process has not started yet, it marks the task killed so run() will
// kill it as soon as it starts.
func (t *BackgroundTask) Stop() {
	t.mu.Lock()
	if t.status != StatusRunning {
		t.mu.Unlock()
		return
	}
	t.status = StatusKilled
	cmd := t.cmd
	t.mu.Unlock()

	if cmd != nil {
		killProcessGroup(cmd)
	}
}

// run is executed in a goroutine by StartBackground.
func (t *BackgroundTask) run(opts ExecOptions) {
	f, err := os.OpenFile(t.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.mu.Lock()
		if t.status != StatusKilled {
			t.status = StatusKilled
			t.exitCode = 1
		}
		t.mu.Unlock()
		return
	}
	defer f.Close()

	cmd := exec.Command("bash", "-c", t.Command)
	cmd.Env = SanitizedEnv()
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}

	if err := cmd.Start(); err != nil {
		t.mu.Lock()
		if t.status != StatusKilled {
			t.status = StatusKilled
			t.exitCode = 126
		}
		t.mu.Unlock()
		return
	}

	// Register the cmd so Stop() can kill it.
	t.mu.Lock()
	if t.status == StatusKilled {
		// Stop() was called before we could register the cmd — kill now.
		t.mu.Unlock()
		killProcessGroup(cmd)
		cmd.Wait() //nolint:errcheck
		return
	}
	t.cmd = cmd
	t.mu.Unlock()

	// Size watchdog: kill the process if the output file grows beyond the limit.
	// This prevents a stuck write loop from filling the disk (mirrors the 768GB
	// incident mentioned in Claude Code's ShellCommand.ts).
	watchdogDone := make(chan struct{})
	go t.sizeWatchdog(watchdogDone)

	waitErr := cmd.Wait()
	close(watchdogDone)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.status == StatusKilled {
		// Killed by Stop() or the watchdog — don't overwrite.
		return
	}

	t.status = StatusCompleted
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			t.exitCode = exitErr.ExitCode()
		} else {
			t.exitCode = 1
		}
	}
}

// sizeWatchdog polls the output file size and kills the process if it exceeds
// maxTaskOutputBytes. It exits when done is closed (i.e. the process finishes).
func (t *BackgroundTask) sizeWatchdog(done <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(watchdogInterval.Load()))
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			info, err := os.Stat(t.OutputFile)
			if err != nil {
				continue
			}
			if info.Size() >= maxTaskOutputBytes.Load() {
				t.Stop()
				return
			}
		}
	}
}

// TaskRegistry is a thread-safe store of background tasks indexed by TaskID.
type TaskRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*BackgroundTask
}

// globalRegistry is the package-level registry used by StartBackground.
var globalRegistry = &TaskRegistry{
	tasks: make(map[string]*BackgroundTask),
}

// Add inserts a task into the registry. Overwrites any existing entry with the same ID.
func (r *TaskRegistry) Add(task *BackgroundTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.TaskID] = task
}

// Get returns the task with the given ID, or (nil, false) if not found.
func (r *TaskRegistry) Get(taskID string) (*BackgroundTask, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[taskID]
	return t, ok
}

// Stop finds a task by ID and calls Stop() on it.
// Returns true if the task was found (regardless of whether it was running).
func (r *TaskRegistry) Stop(taskID string) bool {
	t, ok := r.Get(taskID)
	if !ok {
		return false
	}
	t.Stop()
	return true
}

// List returns a snapshot of all tasks currently in the registry.
func (r *TaskRegistry) List() []*BackgroundTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*BackgroundTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		out = append(out, t)
	}
	return out
}

// StartBackground spawns command in the background, writing its combined
// stdout+stderr to ~/.forge/tasks/{taskId}.output.
// It returns immediately; the caller can poll task.Status() to check progress.
func StartBackground(command string, opts ExecOptions) (*BackgroundTask, error) {
	taskID := uuid.New().String()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home directory: %w", err)
	}

	taskDir := filepath.Join(homeDir, ".forge", "tasks")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create task directory %s: %w", taskDir, err)
	}

	outputFile := filepath.Join(taskDir, taskID+".output")

	task := &BackgroundTask{
		TaskID:     taskID,
		Command:    command,
		OutputFile: outputFile,
		StartTime:  time.Now(),
		status:     StatusRunning,
	}

	globalRegistry.Add(task)

	go task.run(opts)

	return task, nil
}
