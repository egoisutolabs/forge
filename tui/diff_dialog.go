package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DiffFileEntry represents a changed file in the diff dialog.
type DiffFileEntry struct {
	Path      string
	Added     int
	Removed   int
	Diff      string // raw unified diff for this file
	IsBinary  bool
	Truncated bool // true if diff was truncated (>1000 lines)
	Viewed    bool
}

// DiffDialog provides a full-screen diff viewer with file list and detail views.
// It is triggered by Ctrl+D when tool results contain diffs.
type DiffDialog struct {
	files        []DiffFileEntry
	cursor       int  // selected file index
	inDetail     bool // true = showing detail view for selected file
	detailScroll int  // scroll offset in detail view
	width        int
	height       int
	theme        Theme
}

// NewDiffDialog creates a DiffDialog from tool result messages containing diffs.
func NewDiffDialog(messages []DisplayMessage, width, height int, theme Theme) *DiffDialog {
	files := extractDiffFiles(messages)
	if len(files) == 0 {
		return nil
	}
	return &DiffDialog{
		files:  files,
		width:  width,
		height: height,
		theme:  theme,
	}
}

// extractDiffFiles scans tool results for diff content and parses them into file entries.
func extractDiffFiles(messages []DisplayMessage) []DiffFileEntry {
	var files []DiffFileEntry
	seen := make(map[string]bool)

	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		if msg.ToolName != "Edit" && msg.ToolName != "Write" {
			continue
		}
		if !isDiffContent(msg.Content) {
			continue
		}
		for _, f := range parseDiffFiles(msg.Content) {
			if !seen[f.Path] {
				seen[f.Path] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// parseDiffFiles parses unified diff content into individual file entries.
func parseDiffFiles(content string) []DiffFileEntry {
	lines := strings.Split(content, "\n")
	var files []DiffFileEntry
	var current *DiffFileEntry
	var diffLines []string

	flushCurrent := func() {
		if current != nil {
			diff := strings.Join(diffLines, "\n")
			if len(diffLines) > maxDiffLines {
				current.Truncated = true
				diff = strings.Join(diffLines[:maxDiffLines], "\n")
			}
			current.Diff = diff
			files = append(files, *current)
			current = nil
			diffLines = nil
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			flushCurrent()
			// Extract file path from "diff --git a/path b/path"
			parts := strings.Fields(line)
			path := ""
			if len(parts) >= 4 {
				path = strings.TrimPrefix(parts[3], "b/")
			}
			current = &DiffFileEntry{Path: path}
			diffLines = []string{line}
			continue
		}

		if strings.HasPrefix(line, "--- ") && current != nil && len(diffLines) <= 2 {
			diffLines = append(diffLines, line)
			continue
		}
		if strings.HasPrefix(line, "+++ ") && current != nil {
			if path := strings.TrimPrefix(line, "+++ b/"); current.Path == "" {
				current.Path = path
			}
			diffLines = append(diffLines, line)
			continue
		}

		if current != nil {
			diffLines = append(diffLines, line)
			if strings.HasPrefix(line, "Binary files") {
				current.IsBinary = true
			}
			if len(line) > 0 {
				switch line[0] {
				case '+':
					current.Added++
				case '-':
					current.Removed++
				}
			}
		}
	}
	flushCurrent()

	// If no "diff --git" headers found, treat entire content as a single file diff
	if len(files) == 0 && isDiffContent(content) {
		diffLines := strings.Split(content, "\n")
		entry := DiffFileEntry{Path: "changes", Diff: content}
		if len(diffLines) > maxDiffLines {
			entry.Truncated = true
			entry.Diff = strings.Join(diffLines[:maxDiffLines], "\n")
		}
		for _, line := range diffLines {
			if len(line) > 0 {
				switch line[0] {
				case '+':
					entry.Added++
				case '-':
					entry.Removed++
				}
			}
		}
		files = append(files, entry)
	}

	return files
}

const maxDiffLines = 1000

// HandleKey processes a key press in the diff dialog.
// Returns true if the dialog should close.
func (d *DiffDialog) HandleKey(key string) bool {
	if d.inDetail {
		return d.handleDetailKey(key)
	}
	return d.handleListKey(key)
}

func (d *DiffDialog) handleListKey(key string) bool {
	switch key {
	case "up":
		if d.cursor > 0 {
			d.cursor--
		}
	case "down":
		if d.cursor < len(d.files)-1 {
			d.cursor++
		}
	case "enter":
		if len(d.files) > 0 {
			d.files[d.cursor].Viewed = true
			d.inDetail = true
			d.detailScroll = 0
		}
	case "esc", "ctrl+d":
		return true // close dialog
	}
	return false
}

func (d *DiffDialog) handleDetailKey(key string) bool {
	switch key {
	case "esc", "backspace":
		d.inDetail = false
		d.detailScroll = 0
	case "up":
		if d.detailScroll > 0 {
			d.detailScroll--
		}
	case "down":
		d.detailScroll++
	case "pgup":
		d.detailScroll -= d.height / 2
		if d.detailScroll < 0 {
			d.detailScroll = 0
		}
	case "pgdown":
		d.detailScroll += d.height / 2
	}
	return false
}

// Render returns the full-screen diff dialog view.
func (d *DiffDialog) Render() string {
	if d.inDetail {
		return d.renderDetail()
	}
	return d.renderFileList()
}

func (d *DiffDialog) renderFileList() string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(d.theme.Config.AccentColor)).
		Render("Changed Files")

	hint := lipgloss.NewStyle().
		Faint(true).
		Render("  ↑↓ navigate · Enter view · Esc close")

	sb.WriteString("\n  " + title + hint + "\n\n")

	for i, f := range d.files {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == d.cursor {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color(d.theme.Config.AccentColor))
		}

		viewedMark := " "
		if f.Viewed {
			viewedMark = "✓"
		}

		stats := ""
		if f.IsBinary {
			stats = lipgloss.NewStyle().Faint(true).Render("(binary)")
		} else {
			addStr := DiffAddStyle.Render(fmt.Sprintf("+%d", f.Added))
			remStr := DiffRemoveStyle.Render(fmt.Sprintf("-%d", f.Removed))
			stats = addStr + " " + remStr
		}

		if f.Truncated {
			stats += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("(truncated)")
		}

		line := fmt.Sprintf("%s%s %s  %s", prefix, viewedMark, style.Render(f.Path), stats)
		sb.WriteString("  " + line + "\n")
	}

	content := sb.String()
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(d.theme.Config.AccentColor)).
		Width(d.width - 4).
		Height(d.height - 4).
		Render(content)

	return box
}

func (d *DiffDialog) renderDetail() string {
	if d.cursor >= len(d.files) {
		return ""
	}
	f := d.files[d.cursor]

	var sb strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(d.theme.Config.AccentColor)).
		Render(f.Path)

	hint := lipgloss.NewStyle().
		Faint(true).
		Render("  Esc/Backspace back · ↑↓ scroll")

	sb.WriteString("\n  " + header + hint + "\n\n")

	if f.IsBinary {
		sb.WriteString("  " + lipgloss.NewStyle().Faint(true).Render("(binary file)") + "\n")
	} else {
		diffContent := renderDiffColored(f.Diff, d.width-8)
		lines := strings.Split(diffContent, "\n")

		// Apply scroll
		if d.detailScroll >= len(lines) {
			d.detailScroll = maxInt(0, len(lines)-1)
		}
		visibleHeight := d.height - 8
		if visibleHeight < 1 {
			visibleHeight = 1
		}
		end := d.detailScroll + visibleHeight
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[d.detailScroll:end]

		for _, line := range visible {
			sb.WriteString("  " + line + "\n")
		}

		if f.Truncated {
			sb.WriteString("\n  " + lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).
				Render(fmt.Sprintf("(truncated at %d lines)", maxDiffLines)) + "\n")
		}
	}

	content := sb.String()
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(d.theme.Config.AccentColor)).
		Width(d.width - 4).
		Height(d.height - 4).
		Render(content)

	return box
}

// renderDiffColored applies syntax coloring to unified diff lines.
func renderDiffColored(diff string, width int) string {
	lines := strings.Split(diff, "\n")
	var sb strings.Builder

	for _, line := range lines {
		if len(line) == 0 {
			sb.WriteByte('\n')
			continue
		}

		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = DiffHeaderStyle.Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = DiffHunkStyle.Render(line)
		case strings.HasPrefix(line, "diff --git"):
			styled = DiffHeaderStyle.Render(line)
		case line[0] == '+':
			styled = DiffAddStyle.Render(line)
		case line[0] == '-':
			styled = DiffRemoveStyle.Render(line)
		default:
			styled = DiffContextStyle.Render(line)
		}
		sb.WriteString(styled)
		sb.WriteByte('\n')
	}

	return strings.TrimRight(sb.String(), "\n")
}

// FileCount returns the number of files in the dialog.
func (d *DiffDialog) FileCount() int {
	return len(d.files)
}

// InDetail returns true if the dialog is showing a file detail view.
func (d *DiffDialog) InDetail() bool {
	return d.inDetail
}

// Cursor returns the currently selected file index.
func (d *DiffDialog) Cursor() int {
	return d.cursor
}
