package bash

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ── Output file permissions (SEC-8) ──────────────────────────────────────────

func TestStartBackground_OutputFile_0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not meaningful on Windows")
	}

	task, err := StartBackground("echo perm-test", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	waitForStatus(task, 5*time.Second)

	info, err := os.Stat(task.OutputFile)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("output file mode = %04o, want 0600", mode)
	}
}

// waitForStatus polls until task reaches a non-running status or timeout.
func waitForStatus(task *BackgroundTask, timeout time.Duration) BackgroundTaskStatus {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s := task.Status()
		if s != StatusRunning {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	return task.Status()
}

func TestStartBackground_RunsAndWritesToFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	task, err := StartBackground("echo hello background", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	status := waitForStatus(task, 5*time.Second)
	if status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", status)
	}

	data, err := os.ReadFile(task.OutputFile)
	if err != nil {
		t.Fatalf("cannot read output file: %v", err)
	}
	if !strings.Contains(string(data), "hello background") {
		t.Errorf("output file = %q, want to contain 'hello background'", string(data))
	}
}

func TestStartBackground_CanBeStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	task, err := StartBackground("sleep 60", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	// Let it start
	time.Sleep(150 * time.Millisecond)

	if task.Status() != StatusRunning {
		t.Fatalf("expected StatusRunning before stop, got %s", task.Status())
	}

	task.Stop()

	status := waitForStatus(task, 5*time.Second)
	if status != StatusKilled {
		t.Errorf("expected StatusKilled after Stop(), got %s", status)
	}
}

func TestStartBackground_SizeWatchdogKillsOversizedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	// Override limits for testing — restore after.
	// Use Load/Store so concurrent watchdog goroutines from other tests
	// don't race with this test's writes (fixes the data race flagged by -race).
	origMax := maxTaskOutputBytes.Load()
	origInterval := watchdogInterval.Load()
	maxTaskOutputBytes.Store(50) // tiny limit
	watchdogInterval.Store(int64(50 * time.Millisecond))
	defer func() {
		maxTaskOutputBytes.Store(origMax)
		watchdogInterval.Store(origInterval)
	}()

	// yes(1) generates infinite "y\n" output, easily blows past 50 bytes.
	task, err := StartBackground("yes", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() {
		task.Stop()
		os.Remove(task.OutputFile)
	})

	status := waitForStatus(task, 10*time.Second)
	if status != StatusKilled {
		t.Errorf("expected watchdog to kill task, got %s", status)
	}
}

func TestTaskRegistry_TracksMultipleTasks(t *testing.T) {
	reg := &TaskRegistry{tasks: make(map[string]*BackgroundTask)}

	task1 := &BackgroundTask{TaskID: "id-1", Command: "echo 1", status: StatusRunning}
	task2 := &BackgroundTask{TaskID: "id-2", Command: "echo 2", status: StatusCompleted}

	reg.Add(task1)
	reg.Add(task2)

	list := reg.List()
	if len(list) != 2 {
		t.Errorf("expected 2 tasks in registry, got %d", len(list))
	}

	got1, ok := reg.Get("id-1")
	if !ok || got1.TaskID != "id-1" {
		t.Error("failed to retrieve task1 from registry")
	}

	got2, ok := reg.Get("id-2")
	if !ok || got2.TaskID != "id-2" {
		t.Error("failed to retrieve task2 from registry")
	}

	_, ok = reg.Get("id-nonexistent")
	if ok {
		t.Error("expected not found for nonexistent task")
	}
}

func TestStartBackground_CompletedTaskHasExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	task, err := StartBackground("exit 42", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	status := waitForStatus(task, 5*time.Second)
	if status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", status)
	}

	if task.ExitCode() != 42 {
		t.Errorf("expected exit code 42, got %d", task.ExitCode())
	}
}

func TestGlobalRegistry_StopByID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	task, err := StartBackground("sleep 60", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	time.Sleep(150 * time.Millisecond)

	stopped := globalRegistry.Stop(task.TaskID)
	if !stopped {
		t.Error("expected Stop() to return true for running task")
	}

	status := waitForStatus(task, 5*time.Second)
	if status != StatusKilled {
		t.Errorf("expected StatusKilled, got %s", status)
	}
}

func TestGlobalRegistry_StopNonexistent(t *testing.T) {
	stopped := globalRegistry.Stop("does-not-exist")
	if stopped {
		t.Error("expected Stop() to return false for nonexistent task")
	}
}

func TestStartBackground_RegisteredInGlobalRegistry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	task, err := StartBackground("echo registered", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	got, ok := globalRegistry.Get(task.TaskID)
	if !ok {
		t.Fatal("task not found in global registry")
	}
	if got.TaskID != task.TaskID {
		t.Errorf("registry returned wrong task: got %s, want %s", got.TaskID, task.TaskID)
	}
}
