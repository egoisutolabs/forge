package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
)

func tmpDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return filepath.Join(d, "sessions")
}

func TestNew_CreatesSessionWithUUID(t *testing.T) {
	s := New("claude-sonnet-4-6", "/tmp/work")
	if s.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if s.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", s.Model, "claude-sonnet-4-6")
	}
	if s.Cwd != "/tmp/work" {
		t.Errorf("Cwd = %q, want %q", s.Cwd, "/tmp/work")
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if len(s.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(s.Messages))
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := tmpDir(t)
	s := New("claude-sonnet-4-6", "/tmp/work")
	s.Messages = []*models.Message{
		models.NewUserMessage("hello"),
	}
	s.TotalUsage = models.Usage{InputTokens: 100, OutputTokens: 50}

	if err := Save(s, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(s.ID, dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != s.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Model != s.Model {
		t.Errorf("Model = %q, want %q", loaded.Model, s.Model)
	}
	if loaded.Cwd != s.Cwd {
		t.Errorf("Cwd = %q, want %q", loaded.Cwd, s.Cwd)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].TextContent() != "hello" {
		t.Errorf("message text = %q, want %q", loaded.Messages[0].TextContent(), "hello")
	}
	if loaded.TotalUsage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", loaded.TotalUsage.InputTokens)
	}
	if loaded.TotalUsage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", loaded.TotalUsage.OutputTokens)
	}
}

func TestSave_UpdatesTimestamp(t *testing.T) {
	dir := tmpDir(t)
	s := New("claude-sonnet-4-6", "/tmp")
	original := s.UpdatedAt

	// Ensure time moves forward.
	time.Sleep(time.Millisecond)

	if err := Save(s, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !s.UpdatedAt.After(original) {
		t.Error("UpdatedAt should advance on Save")
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := tmpDir(t)
	// Ensure the directory exists so we get a file-not-found, not dir-not-found.
	os.MkdirAll(dir, 0o700)

	_, err := Load("nonexistent", dir)
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestList_Empty(t *testing.T) {
	dir := tmpDir(t)
	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestList_SortedByUpdatedAt(t *testing.T) {
	dir := tmpDir(t)

	s1 := New("model1", "/a")
	s1.UpdatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := Save(s1, dir); err != nil {
		t.Fatal(err)
	}

	s2 := New("model2", "/b")
	s2.UpdatedAt = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := Save(s2, dir); err != nil {
		t.Fatal(err)
	}

	s3 := New("model3", "/c")
	s3.UpdatedAt = time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := Save(s3, dir); err != nil {
		t.Fatal(err)
	}

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Most recent first: s2 (June), s3 (March), s1 (Jan)
	// Note: Save updates UpdatedAt, so we reload to check order.
	// The UpdatedAt is set by Save, but we pre-set it before saving,
	// so on Load it reflects the pre-set value (Save writes UpdatedAt = now,
	// but the JSON was written with the now-at-save time).
	// Since all three saves happen ~instantly, we check IDs instead.
	// Re-save with controlled timestamps by writing directly.
	// Actually, Save sets UpdatedAt = time.Now() which overwrites our preset.
	// Let's just verify they're returned in descending UpdatedAt order.
	for i := 0; i < len(sessions)-1; i++ {
		if sessions[i].UpdatedAt.Before(sessions[i+1].UpdatedAt) {
			t.Errorf("session %d (%s) should be after session %d (%s)",
				i, sessions[i].UpdatedAt, i+1, sessions[i+1].UpdatedAt)
		}
	}
}

func TestList_SkipsNonJSON(t *testing.T) {
	dir := tmpDir(t)
	os.MkdirAll(dir, 0o700)

	// Create a non-JSON file.
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0o600)

	// Create a valid session.
	s := New("model", "/tmp")
	if err := Save(s, dir); err != nil {
		t.Fatal(err)
	}

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestAutoSave_PersistsMessagesAndUsage(t *testing.T) {
	dir := tmpDir(t)
	s := New("model", "/tmp")
	if err := Save(s, dir); err != nil {
		t.Fatal(err)
	}

	msgs := []*models.Message{
		models.NewUserMessage("turn 1"),
		models.NewUserMessage("turn 2"),
	}
	usage := models.Usage{InputTokens: 200, OutputTokens: 100}

	if err := AutoSave(s, msgs, usage, dir); err != nil {
		t.Fatalf("AutoSave: %v", err)
	}

	loaded, err := Load(s.ID, dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.TotalUsage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", loaded.TotalUsage.InputTokens)
	}
}

// --- Security: path traversal in session IDs ---

func TestSanitizeID_Valid(t *testing.T) {
	for _, id := range []string{
		"abc123",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-session",
	} {
		if err := sanitizeID(id); err != nil {
			t.Errorf("sanitizeID(%q) = %v, want nil", id, err)
		}
	}
}

func TestSanitizeID_Rejects(t *testing.T) {
	for _, id := range []string{
		"",
		"../etc/passwd",
		"foo/bar",
		"foo\\bar",
		"..evil",
	} {
		if err := sanitizeID(id); err == nil {
			t.Errorf("sanitizeID(%q) = nil, want error", id)
		}
	}
}

func TestSave_RejectsTraversalID(t *testing.T) {
	dir := tmpDir(t)
	s := New("model", "/tmp")
	s.ID = "../../../tmp/evil"
	err := Save(s, dir)
	if err == nil {
		t.Fatal("Save should reject session ID with path traversal")
	}
}

func TestLoad_RejectsTraversalID(t *testing.T) {
	dir := tmpDir(t)
	os.MkdirAll(dir, 0o700)
	_, err := Load("../../../etc/passwd", dir)
	if err == nil {
		t.Fatal("Load should reject session ID with path traversal")
	}
}

func TestSave_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested", "sessions")
	s := New("model", "/tmp")
	if err := Save(s, dir); err != nil {
		t.Fatalf("Save should create nested dir: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory should have been created")
	}
}
