package tasks

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// --- helpers ---

func newStore(t *testing.T) *TaskStore {
	t.Helper()
	return NewTaskStoreFromDir(t.TempDir())
}

func makeTask(id, subject, status string) *Task {
	now := time.Now()
	return &Task{
		ID:          id,
		Subject:     subject,
		Status:      status,
		Description: "test description",
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// --- NewTaskStoreFromDir ---

func TestTaskStore_NewFromDir(t *testing.T) {
	dir := t.TempDir()
	s := NewTaskStoreFromDir(dir)
	if s.dir != dir {
		t.Errorf("expected dir %q, got %q", dir, s.dir)
	}
}

// --- NewTaskStore ---

func TestNewTaskStore_CreatesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s, err := NewTaskStore("testlist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".forge", "tasks", "testlist")
	if s.dir != expected {
		t.Errorf("expected dir %q, got %q", expected, s.dir)
	}
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		t.Error("directory was not created")
	}
}

// --- NextID ---

func TestTaskStore_NextID_StartsAtOne(t *testing.T) {
	s := newStore(t)
	id, err := s.NextID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "1" {
		t.Errorf("expected id '1', got %q", id)
	}
}

func TestTaskStore_NextID_Increments(t *testing.T) {
	s := newStore(t)
	for i := 1; i <= 5; i++ {
		id, err := s.NextID()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if id != strconv.Itoa(i) {
			t.Errorf("iteration %d: expected %q, got %q", i, strconv.Itoa(i), id)
		}
	}
}

func TestTaskStore_NextID_Concurrent(t *testing.T) {
	s := newStore(t)
	const n = 20
	ids := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := s.NextID()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("expected %d unique IDs, got %d", n, len(seen))
	}
}

// --- SaveTask / LoadTask ---

func TestTaskStore_SaveAndLoad(t *testing.T) {
	s := newStore(t)
	task := makeTask("1", "Do the thing", StatusPending)
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	got, err := s.LoadTask("1")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.ID != "1" || got.Subject != "Do the thing" || got.Status != StatusPending {
		t.Errorf("loaded task mismatch: %+v", got)
	}
}

func TestTaskStore_LoadTask_NotFound(t *testing.T) {
	s := newStore(t)
	task, err := s.LoadTask("99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestTaskStore_SaveTask_Overwrites(t *testing.T) {
	s := newStore(t)
	task := makeTask("1", "Original", StatusPending)
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	task.Subject = "Updated"
	task.Status = StatusInProgress
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask (overwrite): %v", err)
	}
	got, _ := s.LoadTask("1")
	if got.Subject != "Updated" || got.Status != StatusInProgress {
		t.Errorf("expected updated task, got %+v", got)
	}
}

func TestTaskStore_SaveTask_PreservesMetadata(t *testing.T) {
	s := newStore(t)
	task := makeTask("1", "With metadata", StatusPending)
	task.Metadata = map[string]any{"key": "value", "num": float64(42)}
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	got, _ := s.LoadTask("1")
	if got.Metadata["key"] != "value" {
		t.Errorf("expected metadata key='value', got %v", got.Metadata["key"])
	}
	if got.Metadata["num"] != float64(42) {
		t.Errorf("expected metadata num=42, got %v", got.Metadata["num"])
	}
}

// --- ListTasks ---

func TestTaskStore_ListTasks_Empty(t *testing.T) {
	s := newStore(t)
	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty list, got %d tasks", len(tasks))
	}
}

func TestTaskStore_ListTasks_SortedByID(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"3", "1", "10", "2"} {
		if err := s.SaveTask(makeTask(id, "task-"+id, StatusPending)); err != nil {
			t.Fatalf("SaveTask %s: %v", id, err)
		}
	}
	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	want := []string{"1", "2", "3", "10"}
	for i, want := range want {
		if tasks[i].ID != want {
			t.Errorf("position %d: expected ID %q, got %q", i, want, tasks[i].ID)
		}
	}
}

func TestTaskStore_ListTasks_SkipsNonJSON(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTask(makeTask("1", "real task", StatusPending)); err != nil {
		t.Fatal(err)
	}
	// Write a non-JSON file that should be ignored.
	os.WriteFile(filepath.Join(s.dir, ".highwatermark"), []byte("1"), 0644)
	os.WriteFile(filepath.Join(s.dir, "notes.txt"), []byte("ignore me"), 0644)

	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

// --- DeleteTask ---

func TestTaskStore_DeleteTask(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTask(makeTask("1", "to delete", StatusPending)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTask("1"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	task, _ := s.LoadTask("1")
	if task != nil {
		t.Error("expected nil after delete")
	}
}

func TestTaskStore_DeleteTask_NotFoundIsOK(t *testing.T) {
	s := newStore(t)
	if err := s.DeleteTask("99"); err != nil {
		t.Errorf("expected no error for non-existent task, got: %v", err)
	}
}

// --- Task ID validation (path traversal) ---

func TestTaskStore_RejectsTraversalID(t *testing.T) {
	s := newStore(t)
	task := makeTask("../../etc/passwd", "evil", StatusPending)
	if err := s.SaveTask(task); err == nil {
		t.Fatal("expected error for traversal task ID")
	}
}

func TestTaskStore_RejectsSlashInID(t *testing.T) {
	s := newStore(t)
	task := makeTask("foo/bar", "evil", StatusPending)
	if err := s.SaveTask(task); err == nil {
		t.Fatal("expected error for slash in task ID")
	}
}

func TestTaskStore_RejectsDotInID(t *testing.T) {
	s := newStore(t)
	task := makeTask("foo.bar", "evil", StatusPending)
	if err := s.SaveTask(task); err == nil {
		t.Fatal("expected error for dot in task ID")
	}
}

func TestTaskStore_LoadRejectsTraversalID(t *testing.T) {
	s := newStore(t)
	_, err := s.LoadTask("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal task ID in LoadTask")
	}
}

func TestTaskStore_DeleteRejectsTraversalID(t *testing.T) {
	s := newStore(t)
	err := s.DeleteTask("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal task ID in DeleteTask")
	}
}

func TestTaskStore_NormalIDWorks(t *testing.T) {
	s := newStore(t)
	task := makeTask("42", "normal task", StatusPending)
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("unexpected error for normal ID: %v", err)
	}
	got, err := s.LoadTask("42")
	if err != nil {
		t.Fatalf("unexpected error loading normal ID: %v", err)
	}
	if got == nil || got.ID != "42" {
		t.Errorf("expected task with ID 42, got %v", got)
	}
}

func TestTaskStore_HyphenUnderscoreIDWorks(t *testing.T) {
	s := newStore(t)
	task := makeTask("task-1_a", "hyphen-underscore task", StatusPending)
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("unexpected error for hyphen-underscore ID: %v", err)
	}
}

// --- file permission tests ---

func TestTaskStore_FilePermissions(t *testing.T) {
	s := newStore(t)
	task := makeTask("1", "perm test", StatusPending)
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	path, _ := s.taskPath("1")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("task file permissions = %o, want 0600", perm)
	}
}

func TestNewTaskStore_DirPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	s, err := NewTaskStore("permtest")
	if err != nil {
		t.Fatalf("NewTaskStore: %v", err)
	}
	info, err := os.Stat(s.dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("task dir permissions = %o, want 0700", perm)
	}
}

// --- helper functions ---

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b", "c"}, "b") {
		t.Error("expected true")
	}
	if containsString([]string{"a", "b"}, "z") {
		t.Error("expected false")
	}
}

func TestAppendUnique(t *testing.T) {
	got := appendUnique([]string{"a", "b"}, "b")
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
	got = appendUnique([]string{"a", "b"}, "c")
	if len(got) != 3 || got[2] != "c" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestRemoveString(t *testing.T) {
	got := removeString([]string{"a", "b", "c", "b"}, "b")
	if len(got) != 2 {
		t.Errorf("expected 2, got %d: %v", len(got), got)
	}
	for _, v := range got {
		if v == "b" {
			t.Error("b should have been removed")
		}
	}
}
