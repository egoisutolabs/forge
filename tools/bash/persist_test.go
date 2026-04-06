package bash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolResultsDir returns the expected directory for persisted tool results.
func toolResultsDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	return filepath.Join(home, ".forge", "tool-results")
}

func TestPersistOutput_SmallOutputNotPersisted(t *testing.T) {
	output := strings.Repeat("x", MaxResultSizeChars-1)
	path, preview, err := PersistOutput(output, "toolu_small")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("small output should not be persisted, got path=%q", path)
	}
	if preview != "" {
		t.Errorf("small output should return empty preview from PersistOutput, got len=%d", len(preview))
	}
	// Verify no file was written
	expectedFile := filepath.Join(toolResultsDir(t), "toolu_small.txt")
	if _, err := os.Stat(expectedFile); !os.IsNotExist(err) {
		t.Errorf("no file should exist for small output")
		os.Remove(expectedFile)
	}
}

func TestPersistOutput_LargeOutputPersistedToDisk(t *testing.T) {
	id := "toolu_large_test_001"
	output := strings.Repeat("y", MaxResultSizeChars+100)

	path, preview, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	if path == "" {
		t.Fatal("expected non-empty path for large output")
	}
	if preview == "" {
		t.Error("expected non-empty preview for large output")
	}

	// Verify file is under ~/.forge/tool-results/
	expectedPath := filepath.Join(toolResultsDir(t), id+".txt")
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}
}

func TestPersistOutput_PreviewIsFirstBytes(t *testing.T) {
	id := "toolu_preview_test"
	// Build output where the first PreviewSizeBytes chars are all 'A'
	// and the rest are 'B'
	output := strings.Repeat("A", PreviewSizeBytes) + strings.Repeat("B", MaxResultSizeChars+500)

	_, preview, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedFile := filepath.Join(toolResultsDir(t), id+".txt")
	t.Cleanup(func() { os.Remove(expectedFile) })

	if len(preview) != PreviewSizeBytes {
		t.Errorf("preview len = %d, want %d", len(preview), PreviewSizeBytes)
	}
	if !strings.HasPrefix(preview, strings.Repeat("A", PreviewSizeBytes)) {
		t.Error("preview should be the first PreviewSizeBytes bytes of output")
	}
}

func TestPersistOutput_FileContainsFullContent(t *testing.T) {
	id := "toolu_full_content"
	// 60K chars — above 50K threshold but well below 64MB cap
	output := strings.Repeat("z", 60_000)

	path, _, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading persisted file: %v", err)
	}
	if len(data) != len(output) {
		t.Errorf("file size = %d, want %d", len(data), len(output))
	}
	if string(data) != output {
		t.Error("file content does not match original output")
	}
}

func TestPersistOutput_OversizedOutputTruncated(t *testing.T) {
	id := "toolu_oversized"
	// Create output slightly larger than MaxPersistedSize
	oversized := strings.Repeat("x", MaxPersistedSize+1_000)

	path, _, err := PersistOutput(oversized, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat persisted file: %v", err)
	}
	if info.Size() != int64(MaxPersistedSize) {
		t.Errorf("file size = %d, want %d (MaxPersistedSize)", info.Size(), MaxPersistedSize)
	}
}

func TestPersistOutput_FilePermissions(t *testing.T) {
	id := "toolu_perms_test"
	output := strings.Repeat("p", MaxResultSizeChars+1)

	path, _, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("PersistOutput: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("persisted file permissions = %o, want 0600", perm)
	}
}

func TestPersistOutput_DirPermissions(t *testing.T) {
	id := "toolu_dir_perms_test"
	output := strings.Repeat("d", MaxResultSizeChars+1)

	path, _, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("PersistOutput: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("tool-results dir permissions = %o, want 0700", perm)
	}
}

func TestPersistOutput_ParentDirectoriesCreated(t *testing.T) {
	// Ensure PersistOutput succeeds even if parent dirs don't yet exist.
	// We verify this by checking that MkdirAll is called: the simplest proof
	// is that the call succeeds and the file appears in the expected directory.
	id := "toolu_mkdir_test"
	output := strings.Repeat("m", MaxResultSizeChars+1)

	path, _, err := PersistOutput(output, id)
	if err != nil {
		t.Fatalf("PersistOutput failed (parent dirs not created?): %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("parent directory %q was not created", dir)
	}
}
