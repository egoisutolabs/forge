package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordUsage_And_GetRecent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent_models.json")

	if err := RecordUsageTo(path, "model-a"); err != nil {
		t.Fatal(err)
	}
	got := GetRecentFrom(path)
	if len(got) != 1 || got[0] != "model-a" {
		t.Errorf("after first record: %v, want [model-a]", got)
	}

	if err := RecordUsageTo(path, "model-b"); err != nil {
		t.Fatal(err)
	}
	got = GetRecentFrom(path)
	if len(got) != 2 || got[0] != "model-b" || got[1] != "model-a" {
		t.Errorf("after second record: %v, want [model-b model-a]", got)
	}
}

func TestRecordUsage_DeduplicatesAndReorders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent_models.json")

	for _, m := range []string{"a", "b", "c"} {
		RecordUsageTo(path, m)
	}
	// Re-record "a" — should move to front.
	RecordUsageTo(path, "a")
	got := GetRecentFrom(path)
	if len(got) != 3 || got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Errorf("after re-record: %v, want [a c b]", got)
	}
}

func TestRecordUsage_LimitsToMax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent_models.json")

	for i := 0; i < 7; i++ {
		RecordUsageTo(path, "model-"+string(rune('a'+i)))
	}
	got := GetRecentFrom(path)
	if len(got) != maxRecentModels {
		t.Errorf("recent count = %d, want %d", len(got), maxRecentModels)
	}
	// Most recent should be first.
	if got[0] != "model-g" {
		t.Errorf("most recent = %q, want model-g", got[0])
	}
}

func TestGetRecent_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent_models.json")

	got := GetRecentFrom(path)
	if got != nil {
		t.Errorf("empty file should return nil, got %v", got)
	}
}

func TestGetRecent_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent_models.json")
	os.WriteFile(path, []byte("not json"), 0o644)

	got := GetRecentFrom(path)
	if got != nil {
		t.Errorf("invalid json should return nil, got %v", got)
	}
}

func TestRecordUsage_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "recent_models.json")

	if err := RecordUsageTo(path, "model-a"); err != nil {
		t.Fatalf("RecordUsageTo should create parent dirs: %v", err)
	}
	got := GetRecentFrom(path)
	if len(got) != 1 || got[0] != "model-a" {
		t.Errorf("got %v, want [model-a]", got)
	}
}
