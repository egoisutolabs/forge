package lsp

import (
	"testing"
)

func TestDocumentTrackerIsOpen(t *testing.T) {
	dt := NewDocumentTracker()

	if dt.IsOpen("/tmp/test.go") {
		t.Error("expected file not to be open initially")
	}
}

func TestDocumentTrackerVersion(t *testing.T) {
	dt := NewDocumentTracker()

	v := dt.Version("/tmp/test.go")
	if v != 0 {
		t.Errorf("version of untracked file = %d, want 0", v)
	}
}

func TestNewDocumentTracker(t *testing.T) {
	dt := NewDocumentTracker()
	if dt == nil {
		t.Fatal("NewDocumentTracker returned nil")
	}
	if dt.docs == nil {
		t.Fatal("docs map is nil")
	}
}
