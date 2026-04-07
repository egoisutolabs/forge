package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// markdownRenderer caches a glamour terminal renderer to avoid creating one per
// renderMarkdown call. Re-created lazily when the wrap width changes.
var (
	mdRenderer      *glamour.TermRenderer
	mdRendererWidth int
	mdRendererMu    sync.Mutex
)

// renderCache caches expensive rendered output (syntax highlighting, hyperlinks)
// keyed by content+width. This avoids re-highlighting all messages on every frame.
var renderCache = struct {
	mu      sync.Mutex
	entries map[renderCacheKey]string
}{entries: make(map[renderCacheKey]string)}

type renderCacheKey struct {
	content string
	width   int
}

const maxRenderCacheEntries = 256

func getCachedRender(content string, width int) (string, bool) {
	renderCache.mu.Lock()
	defer renderCache.mu.Unlock()
	v, ok := renderCache.entries[renderCacheKey{content, width}]
	return v, ok
}

func putCachedRender(content string, width int, rendered string) {
	renderCache.mu.Lock()
	defer renderCache.mu.Unlock()
	if len(renderCache.entries) >= maxRenderCacheEntries {
		// Evict all on overflow — simple but effective since old messages
		// rarely change and the cache refills quickly.
		renderCache.entries = make(map[renderCacheKey]string, maxRenderCacheEntries/2)
	}
	renderCache.entries[renderCacheKey{content, width}] = rendered
}

// DisplayMessage is a single rendered message in the conversation.
type DisplayMessage struct {
	Role    string // "user", "assistant", "tool", "error"
	Content string
	// Tool-specific fields
	ToolName  string
	ToolID    string
	Detail    string // human-readable: file path, command, URL, query, etc.
	IsError   bool
	Collapsed bool // whether tool result is collapsed (default true for completed tools)

	// Transcript metadata
	Timestamp        time.Time // when the message was created
	Model            string    // model used for this assistant message (e.g. "claude-sonnet-4-6")
	Thinking         string    // thinking/reasoning content (assistant messages only)
	ThinkingExpanded bool      // whether the thinking block is expanded
	ShowTimestamps   bool      // whether to render timestamp for this message
}

// StatusInfo holds the current status bar data.
type StatusInfo struct {
	Model          string
	InputTokens    int
	OutTokens      int
	CostUSD        float64
	Session        string
	Processing     bool
	BackgroundAgts int // count of background sub-agents running
}

// ToolGroup represents a group of consecutive tool results of the same type.
type ToolGroup struct {
	ToolName  string           // e.g. "Read", "Grep", "Glob"
	Messages  []DisplayMessage // the grouped messages
	StartIdx  int              // index of first message in original slice
	Collapsed bool             // whether the group summary is collapsed
}

// groupToolLabel returns the display label for a tool group.
func groupToolLabel(toolName string, count int) string {
	switch toolName {
	case "Read":
		return fmt.Sprintf("Read %d files", count)
	case "Grep":
		return fmt.Sprintf("Searched %d patterns", count)
	case "Glob":
		return fmt.Sprintf("Globbed %d patterns", count)
	default:
		return fmt.Sprintf("%s ×%d", toolName, count)
	}
}

// renderToolGroup renders a grouped set of consecutive tool results.
// When collapsed (default), shows a single summary line.
// When expanded, shows each tool result individually.
func renderToolGroup(group ToolGroup, width int) string {
	icon := "⏺"
	hasError := false
	for _, msg := range group.Messages {
		if msg.IsError {
			hasError = true
			break
		}
	}
	iconStyle := ToolIconSuccessStyle
	if hasError {
		iconStyle = ToolIconErrorStyle
	}

	label := groupToolLabel(group.ToolName, len(group.Messages))
	header := "  " + iconStyle.Render(icon) + " " + ToolNameStyle.Render(label)

	if group.Collapsed {
		return header
	}

	// Expanded: show each tool result as a collapsed one-liner beneath the group header
	var sb strings.Builder
	sb.WriteString(header)
	for _, msg := range group.Messages {
		sb.WriteByte('\n')
		sb.WriteString("    " + formatToolSummary(msg))
	}
	return sb.String()
}

// groupableToolType returns true if this tool type should be grouped when consecutive.
func groupableToolType(name string) bool {
	switch name {
	case "Read", "Grep", "Glob":
		return true
	default:
		return false
	}
}

// renderConversation renders all display messages into a single string.
// collapseAnim may be nil when no collapse animation is active.
// splash may be nil; if provided and messages is empty, the splash screen is shown.
// unseenDividerIdx is the message index where the "N new messages" divider should appear (-1 for none).
// unseenCount is the number of new messages since the divider.
func renderConversation(messages []DisplayMessage, width int, collapseAnim *CollapseAnimation, splash *SplashScreen, unseenDividerIdx int, unseenCount int) string {
	if len(messages) == 0 {
		if splash != nil {
			return renderSplash(splash.Info, width, splash.Theme)
		}
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
		intro := dim.Render("  Start a conversation — type a message and press Enter.")
		hint := dim.Render("  Type / for commands  ·  Ctrl+/ for shortcuts")
		return "\n" + intro + "\n" + hint + "\n"
	}

	var sb strings.Builder
	i := 0
	for i < len(messages) {
		// Insert unseen message divider at the right position
		if unseenDividerIdx >= 0 && i == unseenDividerIdx && unseenCount > 0 {
			sb.WriteString(renderUnseenDivider(unseenCount, width))
			sb.WriteByte('\n')
		}

		msg := messages[i]

		// Check for groupable consecutive tool results
		if msg.Role == "tool" && groupableToolType(msg.ToolName) {
			groupEnd := i + 1
			for groupEnd < len(messages) &&
				messages[groupEnd].Role == "tool" &&
				messages[groupEnd].ToolName == msg.ToolName {
				groupEnd++
			}

			if groupEnd-i >= 2 {
				// We have a group of 2+ consecutive same-type tool results
				group := ToolGroup{
					ToolName:  msg.ToolName,
					Messages:  messages[i:groupEnd],
					StartIdx:  i,
					Collapsed: true, // groups default to collapsed summary
				}
				sb.WriteString(renderToolGroup(group, width))
				sb.WriteByte('\n')
				i = groupEnd
				continue
			}
		}

		renderMsg := msg
		// During collapse animation, always render expanded and clip
		if collapseAnim != nil && collapseAnim.MsgIndex == i {
			renderMsg.Collapsed = false
		}
		rendered := renderMessage(renderMsg, width)
		if collapseAnim != nil && collapseAnim.MsgIndex == i {
			rendered = clipToLines(rendered, maxInt(1, int(collapseAnim.Height)))
		}
		sb.WriteString(rendered)
		sb.WriteByte('\n')
		i++
	}

	// Handle divider at the very end (after all messages)
	if unseenDividerIdx >= 0 && unseenDividerIdx >= len(messages) && unseenCount > 0 {
		sb.WriteString(renderUnseenDivider(unseenCount, width))
		sb.WriteByte('\n')
	}

	return sb.String()
}

// renderUnseenDivider renders the "━━�� N new messages ━━━" line.
func renderUnseenDivider(count int, width int) string {
	label := fmt.Sprintf(" %d new messages ", count)
	if count == 1 {
		label = " 1 new message "
	}
	labelLen := len(label)
	sideLen := (width - labelLen - 4) / 2 // -4 for padding
	if sideLen < 3 {
		sideLen = 3
	}
	rule := strings.Repeat("━", sideLen)
	line := rule + label + rule
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Faint(true).
		Render("  " + line)
}

// renderMessage renders a single display message.
func renderMessage(msg DisplayMessage, width int) string {
	switch msg.Role {
	case "user":
		label := UserStyle.Render("  > You")
		if msg.ShowTimestamps && !msg.Timestamp.IsZero() {
			label += "  " + renderTimestamp(msg.Timestamp)
		}
		body := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			PaddingLeft(2).
			Width(width - 4).
			Render(msg.Content)
		return label + "\n" + body

	case "assistant":
		label := AssistantStyle.Bold(true).Render("  Forge")
		if msg.Model != "" {
			modelTag := lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Faint(true).
				Render(" (" + abbreviateModel(msg.Model) + ")")
			label += modelTag
		}
		if msg.ShowTimestamps && !msg.Timestamp.IsZero() {
			label += "  " + renderTimestamp(msg.Timestamp)
		}

		var sb strings.Builder
		sb.WriteString(label)

		// Thinking block (before main content)
		if msg.Thinking != "" {
			sb.WriteByte('\n')
			sb.WriteString(renderThinkingBlock(msg.Thinking, msg.ThinkingExpanded, width))
		}

		// Main content with hyperlinks — cached to avoid re-highlighting
		// on every frame (expensive chroma + regex work).
		body, ok := getCachedRender(msg.Content, width-4)
		if !ok {
			body = renderMarkdownWithHighlighting(msg.Content, width-4)
			body = RenderHyperlinks(body)
			if msg.Content != "" {
				putCachedRender(msg.Content, width-4, body)
			}
		}
		sb.WriteByte('\n')
		sb.WriteString(body)

		return sb.String()

	case "tool":
		if msg.Collapsed {
			return renderToolResultCollapsed(msg)
		}
		return renderToolResultExpanded(msg, width)

	case "system":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Faint(true).
			Render(msg.Content)

	case "error":
		errBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("9")).
			Padding(0, 1).
			Width(width - 6).
			Render(ErrorStyle.Render("✗ Error: " + msg.Content))
		return "  " + errBox

	default:
		return msg.Content
	}
}

// getMarkdownRenderer returns a cached glamour renderer for the given width.
// A new renderer is created only when the width changes.
func getMarkdownRenderer(width int) *glamour.TermRenderer {
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()
	if mdRenderer != nil && mdRendererWidth == width {
		return mdRenderer
	}
	// Use DarkStyle instead of AutoStyle to avoid OSC 11 terminal color
	// queries that leak ANSI responses into stdin as gibberish text.
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil
	}
	mdRenderer = r
	mdRendererWidth = width
	return r
}

// renderMarkdown renders markdown content using glamour.
func renderMarkdown(content string, width int) string {
	if content == "" {
		return ""
	}

	r := getMarkdownRenderer(width)
	if r == nil {
		return lipgloss.NewStyle().PaddingLeft(2).Width(width).Render(content)
	}

	rendered, err := r.Render(content)
	if err != nil {
		return lipgloss.NewStyle().PaddingLeft(2).Width(width).Render(content)
	}

	return rendered
}

// renderToolStatus renders the active tool spinners (simple string-based, for backward compat).
func renderToolStatus(activeTools []string, spinnerFrame string) string {
	if len(activeTools) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, name := range activeTools {
		sb.WriteString(SpinnerStyle.Render("  " + spinnerFrame + " "))
		sb.WriteString(ToolStyle.Render(toolVerb(name)))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// renderActiveToolsDetailed renders the tool progress area with details and elapsed times.
func renderActiveToolsDetailed(tools []ActiveToolInfo, spinnerFrame string, width int) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	maxVisible := 5
	for i, tool := range tools {
		if i >= maxVisible {
			remaining := len(tools) - maxVisible
			sb.WriteString(faintStyle.Render(
				fmt.Sprintf("  ... and %d more tools running", remaining)))
			sb.WriteByte('\n')
			break
		}

		elapsed := time.Since(tool.StartTime)
		elapsedStr := formatDuration(elapsed)

		verb := toolVerbDetailed(tool.Name, tool.Detail)
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

// toolVerbDetailed returns a display verb for a tool with optional detail.
func toolVerbDetailed(name, detail string) string {
	switch name {
	case "Bash":
		if detail != "" {
			return "Running " + detail + "…"
		}
		return "Running command…"
	case "Read":
		if detail != "" {
			return "Reading " + detail + "…"
		}
		return "Reading file…"
	case "Edit":
		if detail != "" {
			return "Editing " + detail + "…"
		}
		return "Editing file…"
	case "Write":
		if detail != "" {
			return "Writing " + detail + "…"
		}
		return "Writing file…"
	case "Grep":
		if detail != "" {
			return "Searching for " + detail + "…"
		}
		return "Searching…"
	case "Glob":
		if detail != "" {
			return "Finding files matching " + detail + "…"
		}
		return "Finding files…"
	case "Agent":
		if detail != "" {
			return "Agent: " + detail + "…"
		}
		return "Running agent…"
	case "WebFetch":
		if detail != "" {
			return "Fetching " + detail + "…"
		}
		return "Fetching URL…"
	case "WebSearch":
		if detail != "" {
			return "Searching " + detail + "…"
		}
		return "Searching web…"
	default:
		return "Running " + name + "…"
	}
}

// renderStatusBar renders the bottom status line.
func renderStatusBar(status StatusInfo, width int) string {
	if width <= 0 {
		return ""
	}

	parts := []string{}

	if status.Model != "" {
		model := abbreviateModel(status.Model)
		parts = append(parts, StatusKeyStyle.Render(model))
	}

	if status.InputTokens > 0 || status.OutTokens > 0 {
		tokens := fmt.Sprintf("in:%d out:%d", status.InputTokens, status.OutTokens)
		parts = append(parts, StatusBarStyle.Render(tokens))
	}

	if status.CostUSD > 0 {
		cost := fmt.Sprintf("$%.4f", status.CostUSD)
		parts = append(parts, StatusBarStyle.Render(cost))
	}

	if status.BackgroundAgts > 0 {
		bgStr := fmt.Sprintf("%d bg", status.BackgroundAgts)
		parts = append(parts, StatusBarStyle.Render(bgStr))
	}

	right := ""
	if status.Processing {
		right = SpinnerStyle.Render("processing…")
	} else if status.Session != "" {
		right = StatusBarStyle.Render("session:" + status.Session[:min(8, len(status.Session))])
	}

	left := strings.Join(parts, StatusBarStyle.Render("  ·  "))

	// Pad between left and right
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	gap := width - leftLen - rightLen - 4
	if gap < 1 {
		gap = 1
	}
	pad := strings.Repeat(" ", gap)

	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color("0")).
		Render("  " + left + pad + right + "  ")
}

// abbreviateModel shortens model names for the status bar.
func abbreviateModel(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.Contains(model, "opus-4-6") || strings.Contains(model, "opus-4.6"):
		return "opus-4.6"
	case strings.Contains(model, "sonnet-4-6") || strings.Contains(model, "sonnet-4.6"):
		return "sonnet-4.6"
	case strings.Contains(model, "haiku-4-5") || strings.Contains(model, "haiku-4.5"):
		return "haiku-4.5"
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	default:
		if len(model) > 16 {
			return model[:16] + "…"
		}
		return model
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
