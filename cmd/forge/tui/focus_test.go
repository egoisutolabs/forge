package tui

import (
	"bytes"
	"testing"
)

func TestTerminalFocus_InitialState(t *testing.T) {
	tf := NewTerminalFocus(nil)
	if tf.State() != FocusUnknown {
		t.Fatalf("expected unknown initial state, got %v", tf.State())
	}
	if tf.IsFocused() {
		t.Fatal("should not be focused initially")
	}
	if tf.IsBlurred() {
		t.Fatal("should not be blurred initially")
	}
}

func TestTerminalFocus_HandleFocus(t *testing.T) {
	tf := NewTerminalFocus(nil)
	tf.HandleFocus()

	if tf.State() != FocusFocused {
		t.Fatalf("expected focused, got %v", tf.State())
	}
	if !tf.IsFocused() {
		t.Fatal("expected IsFocused=true")
	}
	if tf.IsBlurred() {
		t.Fatal("expected IsBlurred=false")
	}
}

func TestTerminalFocus_HandleBlur(t *testing.T) {
	tf := NewTerminalFocus(nil)
	tf.HandleBlur()

	if tf.State() != FocusBlurred {
		t.Fatalf("expected blurred, got %v", tf.State())
	}
	if tf.IsFocused() {
		t.Fatal("expected IsFocused=false")
	}
	if !tf.IsBlurred() {
		t.Fatal("expected IsBlurred=true")
	}
}

func TestTerminalFocus_BlurUpdatesAnimClock(t *testing.T) {
	clock := NewAnimClock()
	defer clock.Stop()

	tf := NewTerminalFocus(clock)

	tf.HandleBlur()
	if !clock.Blurred() {
		t.Fatal("expected clock to be blurred after terminal blur")
	}

	tf.HandleFocus()
	if clock.Blurred() {
		t.Fatal("expected clock to be unblurred after terminal focus")
	}
}

func TestTerminalFocus_FocusBlurCycle(t *testing.T) {
	clock := NewAnimClock()
	defer clock.Stop()

	tf := NewTerminalFocus(clock)

	// Focus → Blur → Focus
	tf.HandleFocus()
	if tf.State() != FocusFocused {
		t.Fatal("expected focused")
	}

	tf.HandleBlur()
	if tf.State() != FocusBlurred {
		t.Fatal("expected blurred")
	}

	tf.HandleFocus()
	if tf.State() != FocusFocused {
		t.Fatal("expected focused again")
	}
}

func TestFocusState_String(t *testing.T) {
	tests := []struct {
		state FocusState
		want  string
	}{
		{FocusUnknown, "unknown"},
		{FocusFocused, "focused"},
		{FocusBlurred, "blurred"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("FocusState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestEnableFocusReporting(t *testing.T) {
	var buf bytes.Buffer
	cmd := EnableFocusReporting(&buf)
	cmd() // execute the command

	if buf.String() != enableFocusReporting {
		t.Fatalf("expected %q, got %q", enableFocusReporting, buf.String())
	}
}

func TestDisableFocusReporting(t *testing.T) {
	var buf bytes.Buffer
	DisableFocusReporting(&buf)

	if buf.String() != disableFocusReporting {
		t.Fatalf("expected %q, got %q", disableFocusReporting, buf.String())
	}
}
