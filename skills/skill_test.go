package skills

import "testing"

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	s := &Skill{Name: "commit", Description: "make a commit"}
	r.Register(s)

	got := r.Lookup("commit")
	if got == nil {
		t.Fatal("Lookup returned nil for registered skill")
	}
	if got.Name != "commit" {
		t.Errorf("Name = %q, want %q", got.Name, "commit")
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	r := NewRegistry()
	if r.Lookup("nonexistent") != nil {
		t.Error("expected nil for unregistered skill")
	}
}

func TestRegistry_LookupStripsLeadingSlash(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Name: "commit"})
	if r.Lookup("/commit") == nil {
		t.Error("Lookup with leading slash should work")
	}
}

func TestRegistry_OverwritesExisting(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Name: "foo", Description: "first"})
	r.Register(&Skill{Name: "foo", Description: "second"})
	if r.Lookup("foo").Description != "second" {
		t.Error("second registration should overwrite first")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Name: "a"})
	r.Register(&Skill{Name: "b"})
	all := r.All()
	if len(all) != 2 {
		t.Errorf("All() = %d skills, want 2", len(all))
	}
}

func TestRegistry_Len(t *testing.T) {
	r := NewRegistry()
	if r.Len() != 0 {
		t.Error("empty registry should have Len 0")
	}
	r.Register(&Skill{Name: "x"})
	if r.Len() != 1 {
		t.Error("after registration, Len should be 1")
	}
}

func TestStripLeadingSlash(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/commit", "commit"},
		{"commit", "commit"},
		{"//double", "/double"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := stripLeadingSlash(tc.in); got != tc.want {
			t.Errorf("stripLeadingSlash(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
