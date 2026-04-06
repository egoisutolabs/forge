package skills

import "testing"

func TestBundledRegistry_NotNil(t *testing.T) {
	r := BundledRegistry()
	if r == nil {
		t.Fatal("BundledRegistry() returned nil")
	}
}

func TestBundledRegistry_HasCommit(t *testing.T) {
	r := BundledRegistry()
	s := r.Lookup("commit")
	if s == nil {
		t.Fatal("bundled 'commit' skill not found")
	}
	if s.Description == "" {
		t.Error("commit skill should have a description")
	}
	if s.Prompt == nil {
		t.Error("commit skill should have a Prompt function")
	}
}

func TestBundledRegistry_HasReview(t *testing.T) {
	r := BundledRegistry()
	s := r.Lookup("review")
	if s == nil {
		t.Fatal("bundled 'review' skill not found")
	}
}

func TestBundledRegistry_CommitSource(t *testing.T) {
	r := BundledRegistry()
	s := r.Lookup("commit")
	if s == nil {
		t.Fatal("commit skill not found")
	}
	if s.Source != "bundled" {
		t.Errorf("Source = %q, want %q", s.Source, "bundled")
	}
}

func TestBundledRegistry_CommitPromptNonEmpty(t *testing.T) {
	r := BundledRegistry()
	s := r.Lookup("commit")
	if s == nil {
		t.Fatal("commit skill not found")
	}
	if p := s.Prompt(""); p == "" {
		t.Error("commit prompt should not be empty")
	}
}

func TestBundledRegistry_ReviewPromptWithArgs(t *testing.T) {
	r := BundledRegistry()
	s := r.Lookup("review")
	if s == nil {
		t.Fatal("review skill not found")
	}
	p := s.Prompt("main.go")
	if p == "" {
		t.Error("review prompt should not be empty")
	}
}

func TestRegisterBundledSkill_SetsSource(t *testing.T) {
	// Register into globalRegistry, verify source is set.
	before := globalRegistry.Len()
	RegisterBundledSkill(&Skill{Name: "test-bundled-skill-xyz", Prompt: func(string) string { return "" }})
	after := globalRegistry.Len()
	if after != before+1 {
		t.Errorf("Len went from %d to %d, expected +1", before, after)
	}
	s := globalRegistry.Lookup("test-bundled-skill-xyz")
	if s == nil {
		t.Fatal("registered skill not found")
	}
	if s.Source != "bundled" {
		t.Errorf("Source = %q, want bundled", s.Source)
	}
}
