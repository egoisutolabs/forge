package tui

import (
	"strings"
	"testing"
	"time"
)

// ---- Code block extraction tests ----

func TestExtractCodeBlocks_Empty(t *testing.T) {
	blocks := extractCodeBlocks("")
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractCodeBlocks_NoFences(t *testing.T) {
	blocks := extractCodeBlocks("just some text\nwith newlines\n")
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractCodeBlocks_SingleBlock(t *testing.T) {
	md := "before\n```go\nfunc main() {}\n```\nafter\n"
	blocks := extractCodeBlocks(md)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Lang != "go" {
		t.Fatalf("expected lang=go, got %q", blocks[0].Lang)
	}
	if strings.TrimSpace(blocks[0].Code) != "func main() {}" {
		t.Fatalf("unexpected code: %q", blocks[0].Code)
	}
}

func TestExtractCodeBlocks_MultipleBlocks(t *testing.T) {
	md := "text\n```python\nprint('hello')\n```\nmiddle\n```javascript\nconsole.log('hi')\n```\nend\n"
	blocks := extractCodeBlocks(md)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Lang != "python" {
		t.Fatalf("expected first lang=python, got %q", blocks[0].Lang)
	}
	if blocks[1].Lang != "javascript" {
		t.Fatalf("expected second lang=javascript, got %q", blocks[1].Lang)
	}
}

func TestExtractCodeBlocks_NoLang(t *testing.T) {
	md := "text\n```\nsome code\n```\n"
	blocks := extractCodeBlocks(md)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Lang != "" {
		t.Fatalf("expected empty lang, got %q", blocks[0].Lang)
	}
}

// ---- Syntax highlighting tests ----

func TestHighlightCode_Go(t *testing.T) {
	code := `func main() {
	fmt.Println("hello")
}`
	result := highlightCode(code, "go")
	// Should contain ANSI escape codes
	if !strings.Contains(result, "\x1b[") {
		t.Fatal("expected ANSI escape codes in highlighted output")
	}
	// Should still contain the original tokens
	if !strings.Contains(stripANSI(result), "func") {
		t.Fatal("expected 'func' keyword in output")
	}
	if !strings.Contains(stripANSI(result), "main") {
		t.Fatal("expected 'main' in output")
	}
}

func TestHighlightCode_Python(t *testing.T) {
	code := `def hello():
    print("world")`
	result := highlightCode(code, "python")
	if !strings.Contains(result, "\x1b[") {
		t.Fatal("expected ANSI escape codes")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "def") {
		t.Fatal("expected 'def' in output")
	}
}

func TestHighlightCode_UnknownLang(t *testing.T) {
	code := "some code"
	result := highlightCode(code, "nonexistentlang1234")
	// Should not panic, should return something
	if result == "" {
		t.Fatal("expected non-empty output for unknown lang")
	}
}

func TestHighlightCode_EmptyCode(t *testing.T) {
	result := highlightCode("", "go")
	// Should not panic
	_ = result
}

func TestRenderCodeBlock_WithLang(t *testing.T) {
	code := "x := 1"
	result := renderCodeBlock(code, "go", 80)
	if result == "" {
		t.Fatal("expected non-empty rendered code block")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "go") {
		t.Fatal("expected language badge 'go' in output")
	}
}

func TestRenderCodeBlock_NoLang(t *testing.T) {
	result := renderCodeBlock("echo hello", "", 80)
	if result == "" {
		t.Fatal("expected non-empty output")
	}
}

// ---- Diff detection tests ----

func TestIsDiffContent_UnifiedDiff(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
`
	if !isDiffContent(diff) {
		t.Fatal("expected unified diff to be detected")
	}
}

func TestIsDiffContent_PlusMinusLines(t *testing.T) {
	diff := `+added line 1
+added line 2
-removed line 1
 context line
+added line 3
`
	if !isDiffContent(diff) {
		t.Fatal("expected +/- diff to be detected")
	}
}

func TestIsDiffContent_GitDiff(t *testing.T) {
	diff := `diff --git a/file.go b/file.go
index abc123..def456 100644
--- a/file.go
+++ b/file.go
`
	if !isDiffContent(diff) {
		t.Fatal("expected git diff header to be detected")
	}
}

func TestIsDiffContent_NotDiff(t *testing.T) {
	text := `This is just some regular text.
It has multiple lines.
But no diff markers.
`
	if isDiffContent(text) {
		t.Fatal("expected regular text not to be detected as diff")
	}
}

func TestIsDiffContent_SinglePlusLine(t *testing.T) {
	text := "+just one plus line\nother text\n"
	if isDiffContent(text) {
		t.Fatal("single +/- line should not trigger diff detection")
	}
}

// ---- Diff rendering tests ----

func TestRenderDiff_Colors(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 context
-removed
+added
`
	result := renderDiff(diff, 80)
	if result == "" {
		t.Fatal("expected non-empty diff output")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "+added") {
		t.Fatal("expected added line in diff output")
	}
	if !strings.Contains(stripped, "-removed") {
		t.Fatal("expected removed line in diff output")
	}
	if !strings.Contains(stripped, "context") {
		t.Fatal("expected context line in diff output")
	}
}

func TestRenderDiff_EmptyContent(t *testing.T) {
	result := renderDiff("", 80)
	// Should not panic
	_ = result
}

// ---- Collapsible tool result tests ----

func TestFormatToolSummary_Success(t *testing.T) {
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Read",
		Content:  "package main\nimport fmt\nfunc main() {}",
	}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "⏺") {
		t.Fatal("expected ⏺ icon")
	}
	if !strings.Contains(stripped, "Read") {
		t.Fatal("expected tool name in summary")
	}
}

func TestFormatToolSummary_Error(t *testing.T) {
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Bash",
		Content:  "permission denied",
		IsError:  true,
	}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	if !strings.Contains(stripped, "⏺") {
		t.Fatal("expected ⏺ icon")
	}
	if !strings.Contains(stripped, "Bash") {
		t.Fatal("expected tool name in error summary")
	}
}

func TestFormatToolSummary_LongContent(t *testing.T) {
	// Bash uses 2-line/160-char truncation; use >160 chars to test truncation
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Bash",
		Content:  strings.Repeat("a", 200) + "\nline2",
	}
	summary := formatToolSummary(msg)
	stripped := stripANSI(summary)
	// Should be truncated
	if strings.Contains(stripped, strings.Repeat("a", 200)) {
		t.Fatal("expected long content to be truncated")
	}
	if !strings.Contains(stripped, "...") {
		t.Fatal("expected truncation marker")
	}
}

func TestRenderToolResultCollapsed(t *testing.T) {
	msg := DisplayMessage{
		Role:      "tool",
		ToolName:  "Read",
		Content:   "file contents here",
		Collapsed: true,
	}
	result := renderToolResultCollapsed(msg)
	if result == "" {
		t.Fatal("expected non-empty collapsed result")
	}
	// Should be single line (no newlines in the middle)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for collapsed tool, got %d", len(lines))
	}
}

func TestRenderToolResultExpanded(t *testing.T) {
	msg := DisplayMessage{
		Role:      "tool",
		ToolName:  "Read",
		Content:   "line1\nline2\nline3",
		Collapsed: false,
	}
	result := renderToolResultExpanded(msg, 80)
	if result == "" {
		t.Fatal("expected non-empty expanded result")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "│") {
		t.Fatal("expected left border │ in expanded tool result")
	}
}

func TestRenderToolResultExpanded_Empty(t *testing.T) {
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Bash",
		Content:  "",
	}
	result := renderToolResultExpanded(msg, 80)
	if result == "" {
		t.Fatal("expected non-empty result even with empty content")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "no output") {
		t.Fatal("expected '(no output)' message for empty bash result")
	}
}

func TestRenderToolResultExpanded_LongOutput(t *testing.T) {
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "line content here")
	}
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Bash",
		Content:  strings.Join(lines, "\n"),
	}
	result := renderToolResultExpanded(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "more lines") {
		t.Fatal("expected 'more lines' indicator for long output")
	}
}

func TestRenderToolResultExpanded_DiffContent(t *testing.T) {
	diff := "--- a/file.go\n+++ b/file.go\n@@ -1,2 +1,3 @@\n context\n-old\n+new\n"
	msg := DisplayMessage{
		Role:     "tool",
		ToolName: "Edit",
		Content:  diff,
	}
	result := renderToolResultExpanded(msg, 80)
	stripped := stripANSI(result)
	// Should contain diff lines in expanded result
	if !strings.Contains(stripped, "+new") {
		t.Fatal("expected added line in expanded diff tool result")
	}
	if !strings.Contains(stripped, "-old") {
		t.Fatal("expected removed line in expanded diff tool result")
	}
}

// ---- Collapse toggle tests ----

func TestToggleLastToolCollapse(t *testing.T) {
	messages := []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
		{Role: "tool", ToolName: "Read", Collapsed: true},
	}
	if !toggleLastToolCollapse(messages) {
		t.Fatal("expected toggle to return true")
	}
	if messages[2].Collapsed {
		t.Fatal("expected last tool to be expanded after toggle")
	}

	// Toggle back
	if !toggleLastToolCollapse(messages) {
		t.Fatal("expected toggle to return true")
	}
	if !messages[2].Collapsed {
		t.Fatal("expected last tool to be collapsed after second toggle")
	}
}

func TestToggleLastToolCollapse_NoTools(t *testing.T) {
	messages := []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	if toggleLastToolCollapse(messages) {
		t.Fatal("expected toggle to return false when no tool messages")
	}
}

func TestToggleLastToolCollapse_MultipleTool(t *testing.T) {
	messages := []DisplayMessage{
		{Role: "tool", ToolName: "Read", Collapsed: true},
		{Role: "assistant", Content: "text"},
		{Role: "tool", ToolName: "Bash", Collapsed: true},
	}
	toggleLastToolCollapse(messages)
	// Should toggle the last one (Bash), not the first (Read)
	if messages[2].Collapsed {
		t.Fatal("expected last tool (Bash) to be expanded")
	}
	if !messages[0].Collapsed {
		t.Fatal("expected first tool (Read) to remain collapsed")
	}
}

// ---- Tool progress rendering tests ----

func TestRenderActiveTools_Empty(t *testing.T) {
	result := renderActiveTools(nil, "⠋", 80)
	if result != "" {
		t.Fatal("expected empty string for no active tools")
	}
}

func TestRenderActiveTools_Single(t *testing.T) {
	tools := []ActiveToolInfo{
		{Name: "Bash", ID: "t1", StartTime: time.Now().Add(-3 * time.Second)},
	}
	result := renderActiveTools(tools, "⠋", 80)
	if result == "" {
		t.Fatal("expected non-empty tool progress")
	}
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Running command") {
		t.Fatal("expected tool verb in output")
	}
	if !strings.Contains(stripped, "s") {
		t.Fatal("expected elapsed time in output")
	}
}

func TestRenderActiveTools_Multiple(t *testing.T) {
	tools := []ActiveToolInfo{
		{Name: "Bash", ID: "t1", StartTime: time.Now().Add(-5 * time.Second)},
		{Name: "Read", ID: "t2", StartTime: time.Now().Add(-1 * time.Second)},
		{Name: "Grep", ID: "t3", StartTime: time.Now().Add(-2 * time.Second)},
	}
	result := renderActiveTools(tools, "⠋", 80)
	stripped := stripANSI(result)
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), stripped)
	}
}

func TestRenderActiveTools_MaxVisible(t *testing.T) {
	var tools []ActiveToolInfo
	for i := 0; i < 8; i++ {
		tools = append(tools, ActiveToolInfo{
			Name:      "Tool",
			ID:        string(rune('a' + i)),
			StartTime: time.Now(),
		})
	}
	result := renderActiveTools(tools, "⠋", 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "3 more tools") {
		t.Fatal("expected overflow indicator for >5 tools")
	}
}

// ---- Tool verb tests ----

func TestToolVerb(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Bash", "Running command..."},
		{"Read", "Reading file..."},
		{"Edit", "Editing file..."},
		{"Write", "Writing file..."},
		{"Grep", "Searching..."},
		{"Glob", "Finding files..."},
		{"Agent", "Running agent..."},
		{"WebFetch", "Fetching URL..."},
		{"WebSearch", "Searching web..."},
		{"Unknown", "Running Unknown..."},
	}
	for _, c := range cases {
		got := toolVerb(c.name)
		if got != c.want {
			t.Errorf("toolVerb(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// ---- Duration formatting tests ----

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "0.5s"},
		{3200 * time.Millisecond, "3.2s"},
		{59 * time.Second, "59.0s"},
		{65 * time.Second, "1m05s"},
		{125 * time.Second, "2m05s"},
	}
	for _, c := range cases {
		got := formatDuration(c.d)
		if got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// ---- Markdown with highlighting integration tests ----

func TestRenderMarkdownWithHighlighting_NoCodeBlocks(t *testing.T) {
	content := "Just some **bold** text and a list:\n- item 1\n- item 2\n"
	result := renderMarkdownWithHighlighting(content, 80)
	if result == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestRenderMarkdownWithHighlighting_WithCodeBlock(t *testing.T) {
	content := "Here's some code:\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\nAfter the code.\n"
	result := renderMarkdownWithHighlighting(content, 80)
	if result == "" {
		t.Fatal("expected non-empty output")
	}
	// Should contain syntax-highlighted output (ANSI codes)
	if !strings.Contains(result, "\x1b[") {
		t.Fatal("expected ANSI codes from syntax highlighting")
	}
}

func TestRenderMarkdownWithHighlighting_Empty(t *testing.T) {
	result := renderMarkdownWithHighlighting("", 80)
	if result != "" {
		t.Fatal("expected empty output for empty content")
	}
}

// ---- Message rendering integration tests ----

func TestRenderMessage_User(t *testing.T) {
	msg := DisplayMessage{Role: "user", Content: "hello world"}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "> You") {
		t.Fatal("expected '> You' label in user message")
	}
	if !strings.Contains(stripped, "hello world") {
		t.Fatal("expected user content in output")
	}
}

func TestRenderMessage_Assistant(t *testing.T) {
	msg := DisplayMessage{Role: "assistant", Content: "hello **world**"}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Forge") {
		t.Fatal("expected 'Forge' label in assistant message")
	}
}

func TestRenderMessage_ToolCollapsed(t *testing.T) {
	msg := DisplayMessage{
		Role:      "tool",
		ToolName:  "Read",
		Content:   "file content here\nline2\nline3",
		Collapsed: true,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "⏺") {
		t.Fatal("expected ⏺ icon in collapsed tool")
	}
	if !strings.Contains(stripped, "Read") {
		t.Fatal("expected tool name in collapsed tool")
	}
	// Should be compact — no expanded content
	if strings.Contains(stripped, "│") {
		t.Fatal("collapsed tool should not contain border │")
	}
}

func TestRenderMessage_ToolExpanded(t *testing.T) {
	msg := DisplayMessage{
		Role:      "tool",
		ToolName:  "Bash",
		Content:   "exit 0\nline2",
		Collapsed: false,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "⏺") {
		t.Fatal("expected ⏺ icon in expanded tool")
	}
	if !strings.Contains(stripped, "│") {
		t.Fatal("expected border │ in expanded tool")
	}
}

func TestRenderMessage_Error(t *testing.T) {
	msg := DisplayMessage{Role: "error", Content: "something went wrong"}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Error") {
		t.Fatal("expected 'Error' in error message")
	}
	if !strings.Contains(stripped, "something went wrong") {
		t.Fatal("expected error content in output")
	}
	// Should have border
	if !strings.Contains(stripped, "╭") {
		t.Fatal("expected bordered error message")
	}
}

// ---- LangFromFilePath tests ----

func TestLangFromFilePath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"app.js", "javascript"},
		{"mod.ts", "typescript"},
		{"lib.rs", "rust"},
		{"Gemfile.rb", "ruby"},
		{"Main.java", "java"},
		{"run.sh", "bash"},
		{"config.yaml", "yaml"},
		{"data.json", "json"},
		{"README.md", "markdown"},
		{"query.sql", "sql"},
		{"style.css", "css"},
		{"page.html", "html"},
		{"settings.toml", "toml"},
		{"noext", ""},
		{"file.xyz", "xyz"},
	}
	for _, c := range cases {
		got := LangFromFilePath(c.path)
		if got != c.want {
			t.Errorf("LangFromFilePath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// ---- Scroll behavior tests ----

func TestUserScrolledUp_PreservesPosition(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 100, 20)

	// Add enough messages to overflow viewport
	for i := 0; i < 30; i++ {
		m.messages = append(m.messages, DisplayMessage{
			Role:    "assistant",
			Content: "line of content that is long enough to matter",
		})
	}
	m.refreshViewport()

	// Simulate user scrolling up
	m.userScrolledUp = true

	// New content arrives — should NOT snap to bottom
	m.messages = append(m.messages, DisplayMessage{
		Role:    "assistant",
		Content: "new streaming content",
	})
	m.refreshViewport()

	// The user should still be scrolled up (not forced to bottom)
	if !m.userScrolledUp {
		t.Fatal("expected userScrolledUp to remain true after refreshViewport")
	}
}

func TestUserScrolledUp_ResetOnSubmit(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 100, 20)
	m.userScrolledUp = true

	_ = m.submitPrompt("hello")

	if m.userScrolledUp {
		t.Fatal("expected userScrolledUp to be false after submitPrompt")
	}
}
