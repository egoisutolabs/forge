// Package tasks implements the Task tracking system — Create, Get, List, Update,
// Stop, and Output tools backed by a simple JSON-on-disk store.
//
// Each task is stored as {id}.json inside ~/.forge/tasks/{listID}/.
// A .highwatermark file tracks the monotonically-increasing ID counter.
// All writes are protected with syscall.Flock to allow concurrent agent access.
package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// safeIDPattern matches safe task IDs: alphanumeric, underscore, hyphen only.
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validateTaskID returns an error if id contains path separators, dots, or
// other characters that could be used for path traversal.
func validateTaskID(id string) error {
	if !safeIDPattern.MatchString(id) {
		return fmt.Errorf("tasks: invalid task ID %q: must match [a-zA-Z0-9_-]+", id)
	}
	return nil
}

// Task status values.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusKilled     = "killed"
	StatusDeleted    = "deleted" // sentinel: delete the file rather than persist
)

// defaultListID is the list used when the caller does not specify one.
const defaultListID = "default"

// Task is a single tracked unit of work.
type Task struct {
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Owner       string         `json:"owner,omitempty"`
	Blocks      []string       `json:"blocks"`
	BlockedBy   []string       `json:"blockedBy"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// TaskStore manages task persistence under a single directory.
type TaskStore struct {
	dir string // absolute path to the list directory
}

// NewTaskStore creates a TaskStore rooted at ~/.forge/tasks/{listID}/.
// It creates the directory (and its parents) if they do not exist.
func NewTaskStore(listID string) (*TaskStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("tasks: get home dir: %w", err)
	}
	dir := filepath.Join(home, ".forge", "tasks", listID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("tasks: create dir %s: %w", dir, err)
	}
	// Ensure tight permissions even if the directory pre-existed with looser mode.
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf("tasks: chmod dir %s: %w", dir, err)
	}
	return &TaskStore{dir: dir}, nil
}

// NewTaskStoreFromDir creates a TaskStore rooted at an already-existing directory.
// Intended for testing — callers supply a t.TempDir() path.
func NewTaskStoreFromDir(dir string) *TaskStore {
	return &TaskStore{dir: dir}
}

// hwPath returns the absolute path to the highwatermark file.
func (s *TaskStore) hwPath() string {
	return filepath.Join(s.dir, ".highwatermark")
}

// taskPath returns the absolute path to the JSON file for a task ID.
// It validates the ID to prevent path traversal.
func (s *TaskStore) taskPath(id string) (string, error) {
	if err := validateTaskID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+".json"), nil
}

// NextID atomically reads and increments the .highwatermark file,
// returning the next available task ID as a decimal string (e.g. "1", "2", …).
func (s *TaskStore) NextID() (string, error) {
	f, err := os.OpenFile(s.hwPath(), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return "", fmt.Errorf("tasks: open highwatermark: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return "", fmt.Errorf("tasks: lock highwatermark: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	buf := make([]byte, 32)
	n, _ := f.Read(buf)
	raw := strings.TrimSpace(string(buf[:n]))

	cur := 0
	if raw != "" {
		cur, err = strconv.Atoi(raw)
		if err != nil {
			return "", fmt.Errorf("tasks: parse highwatermark %q: %w", raw, err)
		}
	}
	next := cur + 1

	if err := f.Truncate(0); err != nil {
		return "", fmt.Errorf("tasks: truncate highwatermark: %w", err)
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(next)), 0); err != nil {
		return "", fmt.Errorf("tasks: write highwatermark: %w", err)
	}
	return strconv.Itoa(next), nil
}

// SaveTask serialises task to {id}.json under an exclusive file lock.
func (s *TaskStore) SaveTask(task *Task) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("tasks: marshal task %s: %w", task.ID, err)
	}

	path, err := s.taskPath(task.ID)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("tasks: open task file %s: %w", path, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("tasks: lock task file %s: %w", path, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("tasks: truncate task file %s: %w", path, err)
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		return fmt.Errorf("tasks: write task %s: %w", task.ID, err)
	}
	return nil
}

// LoadTask reads and deserialises {id}.json.
// Returns (nil, nil) when the file does not exist.
func (s *TaskStore) LoadTask(id string) (*Task, error) {
	path, err := s.taskPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("tasks: read task %s: %w", id, err)
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("tasks: unmarshal task %s: %w", id, err)
	}
	return &task, nil
}

// ListTasks returns all tasks in the store sorted by numeric ID ascending.
// Corrupted or unreadable task files are silently skipped.
func (s *TaskStore) ListTasks() ([]*Task, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("tasks: read dir: %w", err)
	}
	var tasks []*Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		task, err := s.LoadTask(id)
		if err != nil || task == nil {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		ni, _ := strconv.Atoi(tasks[i].ID)
		nj, _ := strconv.Atoi(tasks[j].ID)
		return ni < nj
	})
	return tasks, nil
}

// DeleteTask removes {id}.json from disk. A not-found error is silently ignored.
func (s *TaskStore) DeleteTask(id string) error {
	path, err := s.taskPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("tasks: delete task %s: %w", id, err)
	}
	return nil
}

// containsString reports whether ss contains s.
func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// appendUnique appends s to ss if it is not already present.
func appendUnique(ss []string, s string) []string {
	if containsString(ss, s) {
		return ss
	}
	return append(ss, s)
}

// removeString returns a copy of ss without occurrences of s.
func removeString(ss []string, s string) []string {
	out := ss[:0:0]
	for _, v := range ss {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
