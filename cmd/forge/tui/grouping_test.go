package tui

import (
	"strings"
	"testing"
)

// ---- Tool grouping tests ----

func TestGroupableToolType(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Read", true},
		{"Grep", true},
		{"Glob", true},
		{"Bash", false},
		{"Edit", false},
		{"Write", false},
		{"Agent", false},
	}
	for _, c := range cases {
		got := groupableToolType(c.name)
		if got != c.want {
			t.Errorf("groupableToolType(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGroupToolLabel(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  string
	}{
		{"Read", 5, "Read 5 files"},
		{"Grep", 3, "Searched 3 patterns"},
		{"Glob", 2, "Globbed 2 patterns"},
		{"Other", 4, "Other ×4"},
	}
	for _, c := range cases {
		got := groupToolLabel(c.name, c.count)
		if got != c.want {
			t.Errorf("groupToolLabel(%q, %d) = %q, want %q", c.name, c.count, got, c.want)
		}
	}
}

func TestRenderToolGroup_Collapsed(t *testing.T) {
	group := ToolGroup{
		ToolName: "Read",
		Messages: []DisplayMessage{
			{Role: "tool", ToolName: "Read", Content: "file1 content"},
			{Role: "tool", ToolName: "Read", Content: "file2 content"},
			{Role: "tool", ToolName: "Read", Content: "file3 content"},
		},
		Collapsed: true,
	}

	result := renderToolGroup(group, 80)
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "Read 3 files") {
		t.Fatalf("expected 'Read 3 files' in collapsed group, got:\n%s", stripped)
	}
	// Should be compact — a single line
	lines := strings.Split(strings.TrimSpace(stripped), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for collapsed group, got %d: %q", len(lines), stripped)
	}
}

func TestRenderToolGroup_Expanded(t *testing.T) {
	group := ToolGroup{
		ToolName: "Read",
		Messages: []DisplayMessage{
			{Role: "tool", ToolName: "Read", Content: "file1 content"},
			{Role: "tool", ToolName: "Read", Content: "file2 content"},
		},
		Collapsed: false,
	}

	result := renderToolGroup(group, 80)
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "Read 2 files") {
		t.Fatalf("expected 'Read 2 files' header, got:\n%s", stripped)
	}
	// Should have multiple lines (header + individual summaries)
	lines := strings.Split(strings.TrimSpace(stripped), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines for expanded group, got %d", len(lines))
	}
}

func TestRenderToolGroup_ErrorInGroup(t *testing.T) {
	group := ToolGroup{
		ToolName: "Read",
		Messages: []DisplayMessage{
			{Role: "tool", ToolName: "Read", Content: "ok"},
			{Role: "tool", ToolName: "Read", Content: "failed", IsError: true},
		},
		Collapsed: true,
	}

	result := renderToolGroup(group, 80)
	// Should still render without error
	if result == "" {
		t.Fatal("expected non-empty result for group with errors")
	}
}

func TestRenderConversation_ConsecutiveReadsGrouped(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "read some files"},
		{Role: "tool", ToolName: "Read", Content: "file1", Collapsed: true},
		{Role: "tool", ToolName: "Read", Content: "file2", Collapsed: true},
		{Role: "tool", ToolName: "Read", Content: "file3", Collapsed: true},
		{Role: "assistant", Content: "Done."},
	}

	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	stripped := stripANSI(out)

	if !strings.Contains(stripped, "Read 3 files") {
		t.Fatalf("expected grouped read summary 'Read 3 files', got:\n%s", stripped)
	}
}

func TestRenderConversation_MixedToolsNotGrouped(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "tool", ToolName: "Read", Content: "file1", Collapsed: true},
		{Role: "tool", ToolName: "Grep", Content: "pattern1", Collapsed: true},
		{Role: "tool", ToolName: "Read", Content: "file2", Collapsed: true},
	}

	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	stripped := stripANSI(out)

	// Since they alternate, no grouping should occur
	if strings.Contains(stripped, "Read 2 files") {
		t.Fatal("expected mixed tools NOT to be grouped")
	}
}

func TestRenderConversation_SingleToolNotGrouped(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "tool", ToolName: "Read", Content: "file1", Collapsed: true},
		{Role: "assistant", Content: "Done."},
	}

	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	stripped := stripANSI(out)

	// Single tool results should not be grouped
	if strings.Contains(stripped, "Read 1 files") {
		t.Fatal("single tool should not be grouped")
	}
}

func TestRenderConversation_ConsecutiveGrepsGrouped(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "tool", ToolName: "Grep", Content: "result1", Collapsed: true},
		{Role: "tool", ToolName: "Grep", Content: "result2", Collapsed: true},
	}

	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	stripped := stripANSI(out)

	if !strings.Contains(stripped, "Searched 2 patterns") {
		t.Fatalf("expected grouped grep summary, got:\n%s", stripped)
	}
}

// ---- Unseen message divider tests ----

func TestRenderUnseenDivider(t *testing.T) {
	result := renderUnseenDivider(5, 80)
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "5 new messages") {
		t.Fatalf("expected '5 new messages', got: %q", stripped)
	}
	if !strings.Contains(stripped, "━") {
		t.Fatalf("expected horizontal rule chars, got: %q", stripped)
	}
}

func TestRenderUnseenDivider_SingleMessage(t *testing.T) {
	result := renderUnseenDivider(1, 80)
	stripped := stripANSI(result)

	if !strings.Contains(stripped, "1 new message") {
		t.Fatalf("expected '1 new message', got: %q", stripped)
	}
	// Should NOT say "messages" (plural)
	if strings.Contains(stripped, "messages") {
		t.Fatal("expected singular 'message' for count=1")
	}
}

func TestRenderConversation_UnseenDividerShown(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "assistant", Content: "second answer"},
		{Role: "tool", ToolName: "Bash", Content: "output", Collapsed: true},
	}

	// Divider at index 2, with 2 new messages
	out := renderConversation(msgs, 80, nil, nil, 2, 2)
	stripped := stripANSI(out)

	if !strings.Contains(stripped, "2 new messages") {
		t.Fatalf("expected unseen divider with '2 new messages', got:\n%s", stripped)
	}
}

func TestRenderConversation_NoDividerWhenInactive(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}

	// Divider index -1 means no divider
	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	stripped := stripANSI(out)

	if strings.Contains(stripped, "new message") {
		t.Fatal("expected no divider when unseenDividerIdx is -1")
	}
}

func TestTrackUnseenMessage_NotScrolledUp(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 100, 40)
	m.userScrolledUp = false
	m.unseenDividerIdx = -1
	m.unseenCount = 0

	m.messages = append(m.messages, DisplayMessage{Role: "assistant", Content: "new"})
	(&m).trackUnseenMessage()

	if m.unseenDividerIdx != -1 {
		t.Fatal("expected no divider when not scrolled up")
	}
	if m.unseenCount != 0 {
		t.Fatal("expected count=0 when not scrolled up")
	}
}

func TestTrackUnseenMessage_ScrolledUp(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 100, 40)
	m.userScrolledUp = true
	m.unseenDividerIdx = -1
	m.unseenCount = 0

	m.messages = append(m.messages, DisplayMessage{Role: "assistant", Content: "first unseen"})
	(&m).trackUnseenMessage()

	if m.unseenDividerIdx != 0 {
		t.Fatalf("expected divider at index 0, got %d", m.unseenDividerIdx)
	}
	if m.unseenCount != 1 {
		t.Fatalf("expected count=1, got %d", m.unseenCount)
	}

	// Second unseen message
	m.messages = append(m.messages, DisplayMessage{Role: "tool", ToolName: "Read", Content: "data"})
	(&m).trackUnseenMessage()

	if m.unseenDividerIdx != 0 {
		t.Fatalf("expected divider still at index 0, got %d", m.unseenDividerIdx)
	}
	if m.unseenCount != 2 {
		t.Fatalf("expected count=2, got %d", m.unseenCount)
	}
}

func TestClearUnseenDivider(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 100, 40)
	m.unseenDividerIdx = 5
	m.unseenCount = 3

	(&m).clearUnseenDivider()

	if m.unseenDividerIdx != -1 {
		t.Fatalf("expected divider cleared to -1, got %d", m.unseenDividerIdx)
	}
	if m.unseenCount != 0 {
		t.Fatalf("expected count cleared to 0, got %d", m.unseenCount)
	}
}
