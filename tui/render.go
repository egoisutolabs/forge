package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// codeFenceRe matches markdown code fences with optional language.
var codeFenceRe = regexp.MustCompile("(?m)^```(\\w*)\\s*\n([\\s\\S]*?)^```\\s*$")

// faintStyle is a cached lipgloss style to avoid per-frame allocations.
var faintStyle = lipgloss.NewStyle().Faint(true)

// CodeBlock represents a parsed code fence from markdown.
type CodeBlock struct {
	Lang       string
	Code       string
	StartIndex int
	EndIndex   int
}

// ToolSummary holds the structured summary for a collapsed tool result.
type ToolSummary struct {
	Icon     string
	ToolName string
	Target   string
	Detail   string
}

// extractCodeBlocks finds all fenced code blocks in markdown content.
func extractCodeBlocks(md string) []CodeBlock {
	matches := codeFenceRe.FindAllStringSubmatchIndex(md, -1)
	blocks := make([]CodeBlock, 0, len(matches))
	for _, m := range matches {
		lang := md[m[2]:m[3]]
		code := md[m[4]:m[5]]
		blocks = append(blocks, CodeBlock{
			Lang:       lang,
			Code:       code,
			StartIndex: m[0],
			EndIndex:   m[1],
		})
	}
	return blocks
}

// highlightCode applies chroma syntax highlighting to a code string.
// Returns the highlighted string, or the original code on any failure.
func highlightCode(code, lang string) string {
	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("dracula")
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf strings.Builder
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return code
	}

	return strings.TrimRight(buf.String(), "\n")
}

// renderCodeBlock renders a syntax-highlighted code block with background and language badge.
func renderCodeBlock(code, lang string, width int) string {
	highlighted := highlightCode(code, lang)

	// Wrap in a subtle background
	blockStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Padding(0, 1).
		Width(width)

	lines := strings.Split(highlighted, "\n")
	var sb strings.Builder
	// Language badge
	if lang != "" {
		badge := lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Faint(true).
			Render(lang)
		sb.WriteString(badge)
		sb.WriteByte('\n')
	}
	sb.WriteString(strings.Join(lines, "\n"))

	return blockStyle.Render(sb.String())
}

// renderMarkdownWithHighlighting replaces code fences in markdown with
// syntax-highlighted blocks, then renders the rest through glamour.
func renderMarkdownWithHighlighting(content string, width int) string {
	if content == "" {
		return ""
	}

	blocks := extractCodeBlocks(content)
	if len(blocks) == 0 {
		// No code fences — use standard glamour rendering
		return renderMarkdown(content, width)
	}

	// Replace code fences with highlighted versions, building result
	// forward through the string to avoid O(n*m) concatenation.
	var sb strings.Builder
	sb.Grow(len(content) + len(blocks)*256) // estimate highlighted output
	prev := 0
	for _, b := range blocks {
		sb.WriteString(content[prev:b.StartIndex])
		sb.WriteByte('\n')
		sb.WriteString(renderCodeBlock(b.Code, b.Lang, width-4))
		sb.WriteByte('\n')
		prev = b.EndIndex
	}
	sb.WriteString(content[prev:])
	result := sb.String()

	// Render the non-code parts through glamour (which will pass through
	// already-formatted ANSI sequences)
	return lipgloss.NewStyle().PaddingLeft(2).Width(width).Render(result)
}

// isDiffContent detects whether content looks like a unified diff.
func isDiffContent(content string) bool {
	lines := strings.SplitN(content, "\n", 20)
	diffMarkers := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "diff --git") {
			return true
		}
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			diffMarkers++
		}
	}
	// If multiple lines start with +/-, likely a diff
	return diffMarkers >= 3
}

// renderDiff colorizes unified diff content with green/red/gray.
func renderDiff(content string, width int) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder

	for _, line := range lines {
		if len(line) == 0 {
			sb.WriteByte('\n')
			continue
		}

		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = DiffHeaderStyle.Width(width).Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = DiffHunkStyle.Width(width).Render(line)
		case line[0] == '+':
			styled = DiffAddStyle.Width(width).Render(line)
		case line[0] == '-':
			styled = DiffRemoveStyle.Width(width).Render(line)
		default:
			styled = DiffContextStyle.Width(width).Render(line)
		}
		sb.WriteString(styled)
		sb.WriteByte('\n')
	}

	return strings.TrimRight(sb.String(), "\n")
}

// formatToolSummary creates a one-line summary for a tool result.
func formatToolSummary(msg DisplayMessage) string {
	icon := "⏺"
	iconStyle := ToolIconSuccessStyle
	if msg.IsError {
		iconStyle = ToolIconErrorStyle
	}

	detail := toolCollapsedDetail(msg)
	summary := msg.ToolName
	if detail != "" {
		summary += " — " + detail
	}

	return iconStyle.Render(icon) + " " + ToolNameStyle.Render(summary)
}

// renderToolResultCollapsed renders a one-line collapsed tool result.
func renderToolResultCollapsed(msg DisplayMessage) string {
	return "  " + formatToolSummary(msg)
}

// renderToolResultExpanded renders a full tool result with border.
func renderToolResultExpanded(msg DisplayMessage, width int) string {
	innerWidth := width - 8
	if innerWidth < 20 {
		innerWidth = 20
	}

	header := "  " + formatToolSummary(msg)

	// Check for tool-specific expanded body override
	content := toolExpandedBody(msg, innerWidth)
	if content == "" {
		content = msg.Content
	}

	if content == "" {
		return header
	}

	// Detect and render diffs specially
	if isDiffContent(content) {
		content = renderDiff(content, innerWidth)
	}

	// Add left border to expanded content
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	maxLines := 20
	for i, line := range lines {
		if i >= maxLines {
			remaining := len(lines) - maxLines
			sb.WriteString(ToolBorderStyle.Render("  │ "))
			sb.WriteString(lipgloss.NewStyle().Faint(true).Render(
				fmt.Sprintf("... %d more lines", remaining)))
			sb.WriteByte('\n')
			break
		}
		sb.WriteString(ToolBorderStyle.Render("  │ "))
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	return header + "\n" + strings.TrimRight(sb.String(), "\n")
}

// renderActiveTools renders the tool progress area with individual spinners and elapsed times.
func renderActiveTools(tools []ActiveToolInfo, spinnerFrame string, width int) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	maxVisible := 5
	for i, tool := range tools {
		if i >= maxVisible {
			remaining := len(tools) - maxVisible
			sb.WriteString(lipgloss.NewStyle().Faint(true).Render(
				fmt.Sprintf("  ... and %d more tools running", remaining)))
			sb.WriteByte('\n')
			break
		}

		elapsed := time.Since(tool.StartTime)
		elapsedStr := formatDuration(elapsed)

		verb := toolVerb(tool.Name)
		left := SpinnerStyle.Render("  "+spinnerFrame+" ") +
			ToolStyle.Render(verb)

		// Right-align elapsed time
		rightStr := faintStyle.Render(elapsedStr)
		leftWidth := lipgloss.Width(left)
		rightWidth := lipgloss.Width(rightStr)
		gap := width - leftWidth - rightWidth - 2
		if gap < 1 {
			gap = 1
		}

		sb.WriteString(left)
		sb.WriteString(strings.Repeat(" ", gap))
		sb.WriteString(rightStr)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// toolVerb returns the display verb for a tool name.
func toolVerb(name string) string {
	switch name {
	case "Bash":
		return "Running command..."
	case "Read":
		return "Reading file..."
	case "Edit":
		return "Editing file..."
	case "Write":
		return "Writing file..."
	case "Grep":
		return "Searching..."
	case "Glob":
		return "Finding files..."
	case "Agent":
		return "Running agent..."
	case "WebFetch":
		return "Fetching URL..."
	case "WebSearch":
		return "Searching web..."
	default:
		return "Running " + name + "..."
	}
}

// formatDuration formats a duration as a concise string (e.g. "3.2s", "1m04s").
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

// LangFromFilePath extracts a language hint from a file path extension.
func LangFromFilePath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return ""
	}
	ext = ext[1:] // strip leading dot
	switch ext {
	case "go":
		return "go"
	case "py":
		return "python"
	case "js":
		return "javascript"
	case "ts":
		return "typescript"
	case "tsx":
		return "typescript"
	case "jsx":
		return "javascript"
	case "rs":
		return "rust"
	case "rb":
		return "ruby"
	case "java":
		return "java"
	case "sh", "bash", "zsh":
		return "bash"
	case "yaml", "yml":
		return "yaml"
	case "json":
		return "json"
	case "md":
		return "markdown"
	case "sql":
		return "sql"
	case "css":
		return "css"
	case "html":
		return "html"
	case "xml":
		return "xml"
	case "toml":
		return "toml"
	default:
		return ext
	}
}
