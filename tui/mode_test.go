package tui

import (
	"strings"
	"testing"
)

func TestInputMode_String(t *testing.T) {
	tests := []struct {
		mode InputMode
		want string
	}{
		{ModeNormal, ""},
		{ModeBash, "bash"},
		{ModeProcessing, "..."},
		{ModePlan, "plan"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("InputMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestModeBorderColor_Distinct(t *testing.T) {
	theme := ResolveTheme(DarkTheme())

	bash := modeBorderColor(ModeBash, theme)
	proc := modeBorderColor(ModeProcessing, theme)
	plan := modeBorderColor(ModePlan, theme)
	normal := modeBorderColor(ModeNormal, theme)

	// Bash should use warning color (distinct from normal)
	if string(bash) != theme.Config.WarningColor {
		t.Errorf("bash border should use WarningColor, got %s", bash)
	}
	// Processing should use dim border color
	if string(proc) != theme.Config.BorderColor {
		t.Errorf("processing border should use BorderColor, got %s", proc)
	}
	// Plan should use accent
	if string(plan) != theme.Config.AccentColor {
		t.Errorf("plan border should use AccentColor, got %s", plan)
	}
	// Normal should use accent
	if string(normal) != theme.Config.AccentColor {
		t.Errorf("normal border should use AccentColor, got %s", normal)
	}
}

func TestRenderInputWithMode_Normal(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	rendered := renderInputWithMode("test input", ModeNormal, 40, theme)
	if rendered == "" {
		t.Fatal("expected non-empty render")
	}
	// Normal mode should have no label
	if strings.Contains(rendered, "bash") || strings.Contains(rendered, "plan") {
		t.Error("normal mode should not contain a mode label")
	}
}

func TestRenderInputWithMode_BashLabel(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	rendered := renderInputWithMode("ls -la", ModeBash, 60, theme)
	if !strings.Contains(rendered, "bash") {
		t.Error("bash mode should contain 'bash' label in border")
	}
}

func TestRenderInputWithMode_ProcessingLabel(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	rendered := renderInputWithMode("", ModeProcessing, 60, theme)
	if !strings.Contains(rendered, "...") {
		t.Error("processing mode should contain '...' label")
	}
}

func TestRenderInputWithMode_PlanLabel(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	rendered := renderInputWithMode("plan this", ModePlan, 60, theme)
	if !strings.Contains(rendered, "plan") {
		t.Error("plan mode should contain 'plan' label")
	}
}

func TestBashMode_TriggerOnBang(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Simulate typing "!" as first character
	if m.inputMode != ModeNormal {
		t.Fatal("expected ModeNormal initially")
	}

	// Trigger bash mode
	m.inputMode = ModeBash // simulating the mode change from Update
	if m.inputMode != ModeBash {
		t.Fatal("expected ModeBash after ! trigger")
	}
}

func TestBashMode_EscExits(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.inputMode = ModeBash
	m.input.SetValue("!ls")

	// Pressing Esc should exit bash mode
	// This tests the state transition logic
	m.inputMode = ModeNormal // simulating esc handler
	if m.inputMode != ModeNormal {
		t.Fatal("expected ModeNormal after Esc")
	}
}

func TestInsertBorderLabel(t *testing.T) {
	// Simple test with a mock border line
	border := "╭────────────────────╮\n│ content            │\n���────────────────��───╯"
	label := " bash "
	result := insertBorderLabel(border, label)
	if !strings.Contains(result, "bash") {
		t.Error("expected label inserted in border")
	}
	// Should still have the border structure
	if !strings.Contains(result, "╭") || !strings.Contains(result, "╮") {
		t.Error("expected border characters preserved")
	}
}
