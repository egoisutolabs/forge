package tui

import (
	"strings"
	"testing"
	"time"
)

// ---- Benchmark: renderMarkdownWithHighlighting (chroma + code fence processing) ----

func BenchmarkRenderMarkdownWithHighlighting_NoCode(b *testing.B) {
	content := "This is a simple **markdown** paragraph with some text.\n\n- item one\n- item two\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderMarkdownWithHighlighting(content, 80)
	}
}

func BenchmarkRenderMarkdownWithHighlighting_SingleCodeBlock(b *testing.B) {
	content := "Here's code:\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\nDone.\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderMarkdownWithHighlighting(content, 80)
	}
}

func BenchmarkRenderMarkdownWithHighlighting_MultipleCodeBlocks(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("Intro text.\n\n")
	for i := 0; i < 5; i++ {
		sb.WriteString("```go\nfunc example")
		sb.WriteByte(byte('A' + i))
		sb.WriteString("() {\n\tx := 1\n\ty := 2\n\treturn x + y\n}\n```\n\nMore text.\n\n")
	}
	content := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderMarkdownWithHighlighting(content, 80)
	}
}

// ---- Benchmark: clipToLines (hot path in animation frames) ----

func BenchmarkClipToLines_Short(b *testing.B) {
	s := "line1\nline2\nline3\nline4\nline5"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clipToLines(s, 3)
	}
}

func BenchmarkClipToLines_Long(b *testing.B) {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "this is a rendered line of content with ANSI codes and text"
	}
	s := strings.Join(lines, "\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clipToLines(s, 20)
	}
}

// ---- Benchmark: highlightCode (chroma tokenization + formatting) ----

func BenchmarkHighlightCode_Go(b *testing.B) {
	code := `func RunLoop(ctx context.Context, params LoopParams) (*LoopResult, error) {
	messages := make([]*Message, len(params.Messages))
	copy(messages, params.Messages)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, err := callAPI(ctx, messages)
		if err != nil {
			return nil, err
		}
		messages = append(messages, resp)
	}
}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		highlightCode(code, "go")
	}
}

// ---- Benchmark: renderConversation (full conversation rendering) ----

func BenchmarkRenderConversation_SmallChat(b *testing.B) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "Hello, how are you?"},
		{Role: "assistant", Content: "I'm doing well! How can I help you?"},
		{Role: "user", Content: "Can you explain Go interfaces?"},
		{Role: "assistant", Content: "Go interfaces are implicit contracts.\n```go\ntype Reader interface {\n\tRead(p []byte) (n int, err error)\n}\n```\n"},
		{Role: "tool", ToolName: "Read", Content: "file contents", Collapsed: true},
		{Role: "tool", ToolName: "Read", Content: "more contents", Collapsed: true},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderConversation(msgs, 100, nil, nil, -1, 0)
	}
}

func BenchmarkRenderConversation_LargeChat(b *testing.B) {
	var msgs []DisplayMessage
	for i := 0; i < 50; i++ {
		msgs = append(msgs, DisplayMessage{
			Role:    "user",
			Content: "This is user message with some content to render.",
		})
		msgs = append(msgs, DisplayMessage{
			Role:    "assistant",
			Content: "Assistant response with **markdown** and a list:\n- alpha\n- bravo\n- charlie\n",
		})
		msgs = append(msgs, DisplayMessage{
			Role:      "tool",
			ToolName:  "Read",
			Content:   "tool output here",
			Collapsed: true,
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderConversation(msgs, 100, nil, nil, -1, 0)
	}
}

// ---- Benchmark: renderActiveTools (spinner area) ----

func BenchmarkRenderActiveTools(b *testing.B) {
	tools := []ActiveToolInfo{
		{Name: "Bash", ID: "t1", Detail: "go test ./...", StartTime: time.Now().Add(-5 * time.Second)},
		{Name: "Read", ID: "t2", Detail: "/path/to/file.go", StartTime: time.Now().Add(-2 * time.Second)},
		{Name: "Grep", ID: "t3", Detail: "pattern", StartTime: time.Now().Add(-1 * time.Second)},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderActiveTools(tools, "⠋", 100)
	}
}

// ---- Benchmark: parseRgLine (ripgrep result parsing) ----

func BenchmarkParseRgLine(b *testing.B) {
	line := "src/components/App.tsx:42:  const handleClick = () => {"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseRgLine(line)
	}
}

// ---- Benchmark: renderCache hit vs miss ----

func BenchmarkRenderCache_Hit(b *testing.B) {
	content := "Cached content with **markdown** and a list:\n- item 1\n- item 2\n"
	// Prime the cache
	body := renderMarkdownWithHighlighting(content, 76)
	body = RenderHyperlinks(body)
	putCachedRender(content, 76, body)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCachedRender(content, 76)
	}
}
