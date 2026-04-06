package tui

import (
	"strings"
	"testing"
)

func TestTitleState_SetProcessing(t *testing.T) {
	ts := NewTitleState("/home/user/project")
	seq := ts.SetProcessing()

	if !strings.Contains(seq, "⚒ Forge - processing...") {
		t.Fatalf("expected processing title, got %q", seq)
	}
	if !strings.HasPrefix(seq, "\033]0;") {
		t.Fatal("expected ANSI OSC prefix")
	}
	if !strings.HasSuffix(seq, "\007") {
		t.Fatal("expected BEL terminator")
	}
}

func TestTitleState_SetIdle(t *testing.T) {
	ts := NewTitleState("/home/user/project")
	seq := ts.SetIdle()

	if !strings.Contains(seq, "⚒ Forge") {
		t.Fatalf("expected idle title to contain Forge, got %q", seq)
	}
}

func TestTitleState_NoRedundantUpdate(t *testing.T) {
	ts := NewTitleState("/tmp")

	seq1 := ts.SetProcessing()
	if seq1 == "" {
		t.Fatal("expected non-empty sequence on first set")
	}

	seq2 := ts.SetProcessing()
	if seq2 != "" {
		t.Fatalf("expected empty sequence on redundant set, got %q", seq2)
	}
}

func TestTitleState_TransitionProcessingToIdle(t *testing.T) {
	ts := NewTitleState("/home/user/project")

	ts.SetProcessing()
	seq := ts.SetIdle()
	if seq == "" {
		t.Fatal("expected non-empty sequence when transitioning to idle")
	}
	if !strings.Contains(seq, "⚒ Forge") {
		t.Fatalf("expected Forge in idle title, got %q", seq)
	}
}

func TestTitleState_Current(t *testing.T) {
	ts := NewTitleState("/tmp")
	ts.SetProcessing()

	if ts.Current() != "⚒ Forge - processing..." {
		t.Fatalf("expected current to be processing title, got %q", ts.Current())
	}
}

func TestFormatTitleSequence(t *testing.T) {
	seq := formatTitleSequence("Hello World")
	expected := "\033]0;Hello World\007"
	if seq != expected {
		t.Fatalf("expected %q, got %q", expected, seq)
	}
}

func TestShortenCwd_Short(t *testing.T) {
	result := shortenCwd("/tmp")
	if result != "/tmp" {
		t.Fatalf("expected /tmp, got %q", result)
	}
}

func TestShortenCwd_Empty(t *testing.T) {
	result := shortenCwd("")
	if result != "~" {
		t.Fatalf("expected ~, got %q", result)
	}
}

func TestShortenCwd_Long(t *testing.T) {
	result := shortenCwd("/home/user/projects/deep/nested/repo")
	// Should be shortened to show first + last 2 components
	if !strings.Contains(result, "nested") || !strings.Contains(result, "repo") {
		t.Fatalf("expected shortened path to contain last 2 components, got %q", result)
	}
}

func TestTitleState_SetCustom(t *testing.T) {
	ts := NewTitleState("/tmp")
	seq := ts.SetCustom("Custom Title")

	if !strings.Contains(seq, "Custom Title") {
		t.Fatalf("expected custom title in sequence, got %q", seq)
	}
	if ts.Current() != "Custom Title" {
		t.Fatalf("expected current to be Custom Title, got %q", ts.Current())
	}
}
