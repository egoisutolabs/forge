package tui

import (
	"strings"
	"testing"
	"time"
)

// ---- formatFileSize tests ----

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{500, "500B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{1572864, "1.5MB"},
		{1073741824, "1.0GB"},
	}
	for _, c := range cases {
		got := formatFileSize(c.bytes)
		if got != c.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

// ---- truncateBashOutput tests ----

func TestTruncateBashOutput_Empty(t *testing.T) {
	got := truncateBashOutput("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestTruncateBashOutput_Short(t *testing.T) {
	got := truncateBashOutput("hello")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateBashOutput_TwoLines(t *testing.T) {
	got := truncateBashOutput("line1\nline2")
	// Two lines should be joined with space
	if got != "line1 line2" {
		t.Errorf("expected 'line1 line2', got %q", got)
	}
}

func TestTruncateBashOutput_ThreeLines(t *testing.T) {
	got := truncateBashOutput("line1\nline2\nline3\nline4")
	// Should only use first 2 lines
	if strings.Contains(got, "line3") || strings.Contains(got, "line4") {
		t.Errorf("expected only first 2 lines, got %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("expected first 2 lines present, got %q", got)
	}
}

func TestTruncateBashOutput_LongChars(t *testing.T) {
	long := strings.Repeat("a", 200)
	got := truncateBashOutput(long)
	if len(got) > 160 {
		t.Errorf("expected <= 160 chars, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected truncation marker '...'")
	}
}

func TestTruncateBashOutput_TwoLongLines(t *testing.T) {
	line1 := strings.Repeat("a", 100)
	line2 := strings.Repeat("b", 100)
	got := truncateBashOutput(line1 + "\n" + line2)
	if len(got) > 160 {
		t.Errorf("expected <= 160 chars, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected truncation marker")
	}
}

// ---- countContentLines tests ----

func TestCountContentLines(t *testing.T) {
	cases := []struct {
		content string
		want    int
	}{
		{"", 0},
		{"one line", 1},
		{"line1\nline2", 2},
		{"line1\nline2\nline3", 3},
	}
	for _, c := range cases {
		got := countContentLines(c.content)
		if got != c.want {
			t.Errorf("countContentLines(%q) = %d, want %d", c.content, got, c.want)
		}
	}
}

// ---- countDiffChanges tests ----

func TestCountDiffChanges(t *testing.T) {
	diff := "--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n context\n-old line\n+new line 1\n+new line 2\n"
	added, removed := countDiffChanges(diff)
	if added != 2 {
		t.Errorf("expected 2 added, got %d", added)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestCountDiffChanges_Empty(t *testing.T) {
	added, removed := countDiffChanges("")
	if added != 0 || removed != 0 {
		t.Errorf("expected 0/0, got %d/%d", added, removed)
	}
}

func TestCountDiffChanges_OnlyAdds(t *testing.T) {
	diff := "+line1\n+line2\n+line3\n"
	added, removed := countDiffChanges(diff)
	if added != 3 {
		t.Errorf("expected 3 added, got %d", added)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

// ---- countSearchResults tests ----

func TestCountSearchResults(t *testing.T) {
	content := "Result 1\nResult 2\nResult 3\n"
	got := countSearchResults(content)
	if got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestCountSearchResults_WithHeaders(t *testing.T) {
	content := "# Header\nResult 1\nResult 2\n"
	got := countSearchResults(content)
	if got != 2 {
		t.Errorf("expected 2 (excluding header), got %d", got)
	}
}

func TestCountSearchResults_Empty(t *testing.T) {
	got := countSearchResults("")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// ---- Per-tool collapsed detail tests: Bash ----

func TestBashCollapsedDetail_NoOutput(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: ""}
	got := toolCollapsedDetail(msg)
	if got != "(no output)" {
		t.Errorf("expected '(no output)', got %q", got)
	}
}

func TestBashCollapsedDetail_WithOutput(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: "hello world"}
	got := toolCollapsedDetail(msg)
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestBashCollapsedDetail_Truncated(t *testing.T) {
	long := strings.Repeat("x", 200)
	msg := DisplayMessage{ToolName: "Bash", Content: long}
	got := toolCollapsedDetail(msg)
	if len(got) > 160 {
		t.Errorf("expected truncated output, got len %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected truncation marker")
	}
}

func TestBashCollapsedDetail_MultiLine(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: "line1\nline2\nline3\nline4\nline5"}
	got := toolCollapsedDetail(msg)
	// Should join first 2 lines, not include line3+
	if strings.Contains(got, "line3") {
		t.Errorf("expected only first 2 lines, got %q", got)
	}
	if !strings.Contains(got, "line1") {
		t.Errorf("expected line1, got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: Read ----

func TestReadCollapsedDetail_WithPath(t *testing.T) {
	msg := DisplayMessage{ToolName: "Read", Content: "line1\nline2\nline3", Detail: "main.go"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "3 lines") {
		t.Errorf("expected '3 lines', got %q", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected 'main.go', got %q", got)
	}
	if !strings.Contains(got, "\u21a9") {
		t.Errorf("expected expand hint ↩, got %q", got)
	}
}

func TestReadCollapsedDetail_NoPath(t *testing.T) {
	msg := DisplayMessage{ToolName: "Read", Content: "line1\nline2"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "2 lines") {
		t.Errorf("expected '2 lines', got %q", got)
	}
	if !strings.Contains(got, "expand") {
		t.Errorf("expected expand hint, got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: Edit ----

func TestEditCollapsedDetail_MustReadFirst(t *testing.T) {
	msg := DisplayMessage{ToolName: "Edit", Content: "Error: must read file first", IsError: true}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "must read") {
		t.Errorf("expected 'must read' message, got %q", got)
	}
}

func TestEditCollapsedDetail_Diff(t *testing.T) {
	diff := "--- a/f.go\n+++ b/f.go\n@@ -1 +1,2 @@\n-old\n+new1\n+new2\n"
	msg := DisplayMessage{ToolName: "Edit", Content: diff, Detail: "f.go"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "+2") {
		t.Errorf("expected '+2' in detail, got %q", got)
	}
	if !strings.Contains(got, "-1") {
		t.Errorf("expected '-1' in detail, got %q", got)
	}
	if !strings.Contains(got, "f.go") {
		t.Errorf("expected file path, got %q", got)
	}
}

func TestEditCollapsedDetail_NoDiff(t *testing.T) {
	msg := DisplayMessage{ToolName: "Edit", Content: "some content", Detail: "file.go"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "file.go") {
		t.Errorf("expected file path, got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: Write ----

func TestWriteCollapsedDetail(t *testing.T) {
	msg := DisplayMessage{ToolName: "Write", Content: "line1\nline2\nline3\nline4\nline5", Detail: "out.txt"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "+5 lines") {
		t.Errorf("expected '+5 lines', got %q", got)
	}
	if !strings.Contains(got, "out.txt") {
		t.Errorf("expected 'out.txt', got %q", got)
	}
}

func TestWriteCollapsedDetail_NoPath(t *testing.T) {
	msg := DisplayMessage{ToolName: "Write", Content: "single line"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "+1 lines") {
		t.Errorf("expected '+1 lines', got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: WebFetch ----

func TestWebFetchCollapsedDetail(t *testing.T) {
	content := strings.Repeat("x", 2048)
	msg := DisplayMessage{ToolName: "WebFetch", Content: content, Detail: "https://example.com"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "KB") {
		t.Errorf("expected size in KB, got %q", got)
	}
	if !strings.Contains(got, "received") {
		t.Errorf("expected 'received', got %q", got)
	}
	if !strings.Contains(got, "example.com") {
		t.Errorf("expected URL in detail, got %q", got)
	}
}

func TestWebFetchCollapsedDetail_Small(t *testing.T) {
	msg := DisplayMessage{ToolName: "WebFetch", Content: "small content"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "B") {
		t.Errorf("expected size in bytes, got %q", got)
	}
	if !strings.Contains(got, "received") {
		t.Errorf("expected 'received', got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: WebSearch ----

func TestWebSearchCollapsedDetail_WithResults(t *testing.T) {
	content := "Result 1\nResult 2\nResult 3\n"
	msg := DisplayMessage{ToolName: "WebSearch", Content: content, Detail: "golang testing"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "Found") {
		t.Errorf("expected 'Found' in detail, got %q", got)
	}
	if !strings.Contains(got, "3 results") {
		t.Errorf("expected '3 results', got %q", got)
	}
	if !strings.Contains(got, "golang testing") {
		t.Errorf("expected query in detail, got %q", got)
	}
}

func TestWebSearchCollapsedDetail_NoResults(t *testing.T) {
	msg := DisplayMessage{ToolName: "WebSearch", Content: "", Detail: "nonexistent query"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "nonexistent query") {
		t.Errorf("expected query as fallback, got %q", got)
	}
}

// ---- Per-tool collapsed detail tests: Agent ----

func TestAgentCollapsedDetail(t *testing.T) {
	msg := DisplayMessage{ToolName: "Agent", Content: "Starting task\nDoing work\nCompleted analysis"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "Completed analysis") {
		t.Errorf("expected last line, got %q", got)
	}
}

func TestAgentCollapsedDetail_Empty(t *testing.T) {
	msg := DisplayMessage{ToolName: "Agent", Content: ""}
	got := toolCollapsedDetail(msg)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestAgentCollapsedDetail_LongLastLine(t *testing.T) {
	lastLine := strings.Repeat("z", 100)
	msg := DisplayMessage{ToolName: "Agent", Content: "line1\n" + lastLine}
	got := toolCollapsedDetail(msg)
	if len(got) > 63 { // 60 + "..."
		t.Errorf("expected truncated last line, got len %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected truncation marker")
	}
}

// ---- Per-tool collapsed detail tests: TaskOutput ----

func TestTaskOutputCollapsedDetail_StillRunning(t *testing.T) {
	msg := DisplayMessage{ToolName: "TaskOutput", Content: "task is still running"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "still running") {
		t.Errorf("expected 'still running', got %q", got)
	}
}

func TestTaskOutputCollapsedDetail_InProgress(t *testing.T) {
	msg := DisplayMessage{ToolName: "TaskOutput", Content: "operation in progress"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "still running") {
		t.Errorf("expected 'still running' for in-progress content, got %q", got)
	}
}

func TestTaskOutputCollapsedDetail_NotReady(t *testing.T) {
	msg := DisplayMessage{ToolName: "TaskOutput", Content: ""}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "not ready") {
		t.Errorf("expected 'not ready', got %q", got)
	}
}

func TestTaskOutputCollapsedDetail_WithContent(t *testing.T) {
	msg := DisplayMessage{ToolName: "TaskOutput", Content: "Task completed successfully"}
	got := toolCollapsedDetail(msg)
	if !strings.Contains(got, "Task completed") {
		t.Errorf("expected content in detail, got %q", got)
	}
}

// ---- Default collapsed detail tests ----

func TestDefaultCollapsedDetail_Empty(t *testing.T) {
	msg := DisplayMessage{ToolName: "Unknown", Content: ""}
	got := toolCollapsedDetail(msg)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestDefaultCollapsedDetail_Short(t *testing.T) {
	msg := DisplayMessage{ToolName: "Unknown", Content: "some output"}
	got := toolCollapsedDetail(msg)
	if got != "some output" {
		t.Errorf("expected 'some output', got %q", got)
	}
}

func TestDefaultCollapsedDetail_Long(t *testing.T) {
	long := strings.Repeat("x", 100)
	msg := DisplayMessage{ToolName: "Unknown", Content: long}
	got := toolCollapsedDetail(msg)
	if len(got) > 63 {
		t.Errorf("expected truncated, got len %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected truncation marker")
	}
}

func TestDefaultCollapsedDetail_MultiLine(t *testing.T) {
	msg := DisplayMessage{ToolName: "Unknown", Content: "first line\nsecond line"}
	got := toolCollapsedDetail(msg)
	if strings.Contains(got, "second") {
		t.Errorf("expected only first line, got %q", got)
	}
	if got != "first line" {
		t.Errorf("expected 'first line', got %q", got)
	}
}

// ---- Expanded body tests ----

func TestBashExpandedBody_NoOutput(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: ""}
	got := toolExpandedBody(msg, 80)
	if !strings.Contains(got, "no output") {
		t.Errorf("expected '(no output)', got %q", got)
	}
}

func TestBashExpandedBody_WithOutput(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: "some output"}
	got := toolExpandedBody(msg, 80)
	if got != "" {
		t.Errorf("expected empty (use default), got %q", got)
	}
}

func TestAgentExpandedBody_Short(t *testing.T) {
	msg := DisplayMessage{ToolName: "Agent", Content: "line1\nline2\nline3"}
	got := toolExpandedBody(msg, 80)
	if got != "" {
		t.Errorf("expected empty (use default) for short agent output, got %q", got)
	}
}

func TestAgentExpandedBody_ManyLines(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "progress message")
	}
	msg := DisplayMessage{ToolName: "Agent", Content: strings.Join(lines, "\n")}
	got := toolExpandedBody(msg, 80)
	if !strings.Contains(got, "earlier") {
		t.Errorf("expected 'earlier' indicator, got %q", got)
	}
	if !strings.Contains(got, "10") {
		t.Errorf("expected skipped count in output, got %q", got)
	}
	// Should contain last 10 lines
	outLines := strings.Split(strings.TrimSpace(got), "\n")
	// First line is the "... N earlier" indicator, rest are content
	if len(outLines) != 11 {
		t.Errorf("expected 11 lines (1 indicator + 10 content), got %d", len(outLines))
	}
}

func TestAgentExpandedBody_Empty(t *testing.T) {
	msg := DisplayMessage{ToolName: "Agent", Content: ""}
	got := toolExpandedBody(msg, 80)
	if got != "" {
		t.Errorf("expected empty for empty agent, got %q", got)
	}
}

func TestDefaultExpandedBody(t *testing.T) {
	msg := DisplayMessage{ToolName: "Read", Content: "file content"}
	got := toolExpandedBody(msg, 80)
	if got != "" {
		t.Errorf("expected empty (use default) for Read, got %q", got)
	}
}

// ---- toolActiveVerb tests ----

func TestToolActiveVerb_BashRunning(t *testing.T) {
	tool := ActiveToolInfo{
		Name:      "Bash",
		Detail:    "npm test",
		StartTime: time.Now(),
	}
	got := toolActiveVerb(tool)
	if !strings.Contains(got, "Running") {
		t.Errorf("expected 'Running', got %q", got)
	}
	if !strings.Contains(got, "npm test") {
		t.Errorf("expected command detail, got %q", got)
	}
}

func TestToolActiveVerb_BashWaiting(t *testing.T) {
	tool := ActiveToolInfo{
		Name:      "Bash",
		Detail:    "npm test",
		StartTime: time.Now().Add(-15 * time.Second),
	}
	got := toolActiveVerb(tool)
	if !strings.Contains(got, "Waiting") {
		t.Errorf("expected 'Waiting' after timeout, got %q", got)
	}
	if !strings.Contains(got, "npm test") {
		t.Errorf("expected command detail, got %q", got)
	}
}

func TestToolActiveVerb_BashWaiting_NoDetail(t *testing.T) {
	tool := ActiveToolInfo{
		Name:      "Bash",
		StartTime: time.Now().Add(-15 * time.Second),
	}
	got := toolActiveVerb(tool)
	if !strings.Contains(got, "Waiting") {
		t.Errorf("expected 'Waiting', got %q", got)
	}
}

func TestToolActiveVerb_NonBash(t *testing.T) {
	tool := ActiveToolInfo{
		Name:      "Read",
		Detail:    "main.go",
		StartTime: time.Now(),
	}
	got := toolActiveVerb(tool)
	// Should delegate to toolVerbDetailed
	if !strings.Contains(got, "Reading") {
		t.Errorf("expected 'Reading', got %q", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected detail, got %q", got)
	}
}

// ---- Integration: formatToolSummary with per-tool detail ----

func TestFormatToolSummary_BashNoOutput(t *testing.T) {
	msg := DisplayMessage{ToolName: "Bash", Content: ""}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "no output") {
		t.Errorf("expected '(no output)' in bash summary, got %q", stripped)
	}
}

func TestFormatToolSummary_ReadWithPath(t *testing.T) {
	msg := DisplayMessage{ToolName: "Read", Content: "a\nb\nc", Detail: "main.go"}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "main.go") {
		t.Errorf("expected file path in summary, got %q", stripped)
	}
	if !strings.Contains(stripped, "3 lines") {
		t.Errorf("expected line count in summary, got %q", stripped)
	}
}

func TestFormatToolSummary_EditDiff(t *testing.T) {
	diff := "-old\n+new\n"
	msg := DisplayMessage{ToolName: "Edit", Content: diff, Detail: "app.go"}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "app.go") {
		t.Errorf("expected file path, got %q", stripped)
	}
	if !strings.Contains(stripped, "+1/-1") {
		t.Errorf("expected change stats, got %q", stripped)
	}
}

func TestFormatToolSummary_WriteLines(t *testing.T) {
	msg := DisplayMessage{ToolName: "Write", Content: "a\nb\nc", Detail: "out.txt"}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "+3 lines") {
		t.Errorf("expected '+3 lines', got %q", stripped)
	}
}

func TestFormatToolSummary_WebFetchSize(t *testing.T) {
	content := strings.Repeat("x", 5120)
	msg := DisplayMessage{ToolName: "WebFetch", Content: content, Detail: "https://example.com"}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "KB") {
		t.Errorf("expected size in summary, got %q", stripped)
	}
}
