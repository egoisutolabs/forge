package tui

import "testing"

func TestShouldClearSelection_TypingClears(t *testing.T) {
	clearKeys := []string{"enter", "backspace", "ctrl+c", "ctrl+a", "a", "space", "tab"}
	for _, key := range clearKeys {
		if !shouldClearSelection(key) {
			t.Errorf("expected shouldClearSelection(%q) = true", key)
		}
	}
}

func TestShouldClearSelection_NavigationPreserves(t *testing.T) {
	preserveKeys := []string{
		"shift+up", "shift+down", "shift+left", "shift+right",
		"ctrl+shift+left", "ctrl+shift+right",
		"pgup", "pgdown",
		"up", "down", "left", "right",
		"shift+tab",
	}
	for _, key := range preserveKeys {
		if shouldClearSelection(key) {
			t.Errorf("expected shouldClearSelection(%q) = false", key)
		}
	}
}

func TestSelectionState_Lifecycle(t *testing.T) {
	s := NewSelectionState()
	if s.Active() {
		t.Fatal("expected inactive initially")
	}

	s.SetActive(true)
	if !s.Active() {
		t.Fatal("expected active after SetActive(true)")
	}

	s.Clear()
	if s.Active() {
		t.Fatal("expected inactive after Clear()")
	}
}

func TestSelectionState_ClearOnKeystrokes(t *testing.T) {
	s := NewSelectionState()
	s.SetActive(true)

	// Simulate processing a keystroke
	key := "enter"
	if shouldClearSelection(key) {
		s.Clear()
	}
	if s.Active() {
		t.Fatal("expected selection cleared after enter")
	}
}

func TestSelectionState_PreserveOnShiftArrow(t *testing.T) {
	s := NewSelectionState()
	s.SetActive(true)

	key := "shift+right"
	if shouldClearSelection(key) {
		s.Clear()
	}
	if !s.Active() {
		t.Fatal("expected selection preserved after shift+right")
	}
}
