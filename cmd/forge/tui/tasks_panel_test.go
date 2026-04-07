package tui

import (
	"strings"
	"testing"
	"time"
)

// ---- TasksPanel basic lifecycle ----

func TestTasksPanel_NewIsCollapsed(t *testing.T) {
	p := NewTasksPanel()
	if p.Expanded {
		t.Fatal("expected new panel to be collapsed")
	}
	if p.Selected != 0 {
		t.Fatalf("expected selected=0, got %d", p.Selected)
	}
}

func TestTasksPanel_Toggle(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()
	if !p.Expanded {
		t.Fatal("expected panel to be expanded after toggle")
	}
	p.Toggle()
	if p.Expanded {
		t.Fatal("expected panel to be collapsed after second toggle")
	}
}

func TestTasksPanel_Collapse(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()
	p.Collapse()
	if p.Expanded {
		t.Fatal("expected panel to be collapsed")
	}
}

// ---- Navigation ----

func TestTasksPanel_NextAgent(t *testing.T) {
	p := NewTasksPanel()
	p.NextAgent(3)
	if p.Selected != 1 {
		t.Fatalf("expected selected=1 after next, got %d", p.Selected)
	}
	p.NextAgent(3)
	if p.Selected != 2 {
		t.Fatalf("expected selected=2 after next, got %d", p.Selected)
	}
	p.NextAgent(3) // wraps
	if p.Selected != 0 {
		t.Fatalf("expected selected=0 after wrap, got %d", p.Selected)
	}
}

func TestTasksPanel_PrevAgent(t *testing.T) {
	p := NewTasksPanel()
	p.PrevAgent(3) // wraps to end
	if p.Selected != 2 {
		t.Fatalf("expected selected=2 after prev from 0, got %d", p.Selected)
	}
	p.PrevAgent(3)
	if p.Selected != 1 {
		t.Fatalf("expected selected=1 after prev, got %d", p.Selected)
	}
}

func TestTasksPanel_NextAgent_ZeroCount(t *testing.T) {
	p := NewTasksPanel()
	p.NextAgent(0) // should not panic
	if p.Selected != 0 {
		t.Fatalf("expected selected=0 with zero agents, got %d", p.Selected)
	}
}

func TestTasksPanel_ClampSelected(t *testing.T) {
	p := NewTasksPanel()
	p.Selected = 5
	p.ClampSelected(3)
	if p.Selected != 2 {
		t.Fatalf("expected selected=2 after clamp, got %d", p.Selected)
	}
	p.ClampSelected(0)
	if p.Selected != 0 {
		t.Fatalf("expected selected=0 after clamp with zero agents, got %d", p.Selected)
	}
}

// ---- Rendering ----

func TestTasksPanel_Render_CollapsedEmpty(t *testing.T) {
	p := NewTasksPanel()
	result := p.Render(nil, 80)
	if result != "" {
		t.Fatal("expected empty string for collapsed panel")
	}
}

func TestTasksPanel_Render_ExpandedNoAgents(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()
	result := p.Render(nil, 80)
	if result != "" {
		t.Fatal("expected empty string for expanded panel with no agents")
	}
}

func TestTasksPanel_Render_ExpandedWithAgents(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()

	agents := []*BackgroundAgent{
		{Name: "test-agent", StartTime: time.Now().Add(-5 * time.Second)},
		{Name: "another-agent", StartTime: time.Now().Add(-10 * time.Second), Completed: true, CompletedAt: time.Now()},
	}

	result := p.Render(agents, 80)
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "Background Agents") {
		t.Fatal("expected panel header")
	}
	if !strings.Contains(stripped, "test-agent") {
		t.Fatal("expected first agent name")
	}
	if !strings.Contains(stripped, "another-agent") {
		t.Fatal("expected second agent name")
	}
	if !strings.Contains(stripped, "esc:close") {
		t.Fatal("expected close hint")
	}
}

func TestTasksPanel_Render_SelectionIndicator(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()
	p.Selected = 1

	agents := []*BackgroundAgent{
		{Name: "agent-a", StartTime: time.Now()},
		{Name: "agent-b", StartTime: time.Now()},
	}

	result := p.Render(agents, 80)
	stripped := stripANSI(result)

	// The selected agent should have > prefix
	if !strings.Contains(stripped, "> ") {
		t.Fatal("expected > selection indicator")
	}
}

func TestTasksPanel_Render_NarrowTerminal(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()

	agents := []*BackgroundAgent{
		{Name: "narrow-agent", StartTime: time.Now().Add(-5 * time.Second)},
	}

	result := p.Render(agents, 40) // narrow
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "narrow-agent") {
		t.Fatal("expected agent name in narrow mode")
	}
	// In narrow mode, elapsed time should NOT be shown
	if strings.Contains(stripped, "5.") {
		t.Fatal("expected no elapsed time in narrow terminal mode")
	}
}

func TestTasksPanel_Render_StopHint(t *testing.T) {
	p := NewTasksPanel()
	p.Toggle()
	p.Selected = 0

	// Running agent should show stop hint
	agents := []*BackgroundAgent{
		{Name: "running-agent", StartTime: time.Now()},
	}
	result := p.Render(agents, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "x:stop") {
		t.Fatal("expected x:stop hint for running agent")
	}

	// Completed agent should NOT show stop hint
	agents = []*BackgroundAgent{
		{Name: "done-agent", StartTime: time.Now(), Completed: true, CompletedAt: time.Now()},
	}
	result = p.Render(agents, 80)
	stripped = stripANSI(result)
	if strings.Contains(stripped, "x:stop") {
		t.Fatal("expected no x:stop hint for completed agent")
	}
}
