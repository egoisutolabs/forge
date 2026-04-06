package skills

import "testing"

func TestBundledRegistry_HasAllCommands(t *testing.T) {
	r := BundledRegistry()

	commands := []string{"cost", "clear", "model", "models", "compact", "help", "history", "quit"}
	for _, name := range commands {
		s := r.Lookup(name)
		if s == nil {
			t.Errorf("bundled %q skill not found", name)
			continue
		}
		if s.Source != "bundled" {
			t.Errorf("%q: Source = %q, want %q", name, s.Source, "bundled")
		}
		if !s.UserInvocable {
			t.Errorf("%q: UserInvocable = false, want true", name)
		}
		if s.Description == "" {
			t.Errorf("%q: Description is empty", name)
		}
		if s.Prompt == nil {
			t.Errorf("%q: Prompt is nil", name)
		}
	}
}

func TestCostSkill_Prompt(t *testing.T) {
	s := BundledRegistry().Lookup("cost")
	if s == nil {
		t.Fatal("cost skill not found")
	}
	p := s.Prompt("")
	if p == "" {
		t.Error("cost prompt should not be empty")
	}
}

func TestModelSkill_PromptWithArgs(t *testing.T) {
	s := BundledRegistry().Lookup("model")
	if s == nil {
		t.Fatal("model skill not found")
	}
	noArgs := s.Prompt("")
	if noArgs == "" {
		t.Error("model prompt with no args should not be empty")
	}
	withArgs := s.Prompt("claude-opus-4-6")
	if withArgs == "" {
		t.Error("model prompt with args should not be empty")
	}
	if noArgs == withArgs {
		t.Error("model prompt should differ based on args")
	}
}

func TestHelpSkill_Prompt(t *testing.T) {
	s := BundledRegistry().Lookup("help")
	if s == nil {
		t.Fatal("help skill not found")
	}
	p := s.Prompt("")
	if p == "" {
		t.Error("help prompt should not be empty")
	}
}

func TestBundledRegistry_TotalCount(t *testing.T) {
	r := BundledRegistry()
	// Should have at least the 9 bundled skills: commit, review, cost, clear, model, compact, help, history, quit
	if r.Len() < 9 {
		t.Errorf("BundledRegistry has %d skills, want at least 9", r.Len())
	}
}
