package tui

import (
	"fmt"
	"strings"
	"time"
)

const (
	bashMaxLines = 2
	bashMaxChars = 160

	agentMaxExpandedLines = 10

	defaultDetailMaxChars = 60
)

// formatFileSize formats bytes into a human-readable size string.
func formatFileSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1fGB", float64(bytes)/(1024*1024*1024))
	}
}

// truncateBashOutput joins the first 2 lines and truncates to 160 chars.
func truncateBashOutput(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.SplitN(content, "\n", bashMaxLines+1)
	var preview string
	if len(lines) >= bashMaxLines {
		preview = strings.Join(lines[:bashMaxLines], " ")
	} else {
		preview = lines[0]
	}
	if len(preview) > bashMaxChars {
		preview = preview[:bashMaxChars-3] + "..."
	}
	return preview
}

// countContentLines returns the number of lines in content.
func countContentLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

// countDiffChanges counts added and removed lines in a unified diff.
func countDiffChanges(content string) (added, removed int) {
	for _, line := range strings.Split(content, "\n") {
		if len(line) == 0 {
			continue
		}
		switch {
		case line[0] == '+' && !strings.HasPrefix(line, "+++"):
			added++
		case line[0] == '-' && !strings.HasPrefix(line, "---"):
			removed++
		}
	}
	return
}

// toolCollapsedDetail returns a tool-specific detail string for the collapsed summary line.
func toolCollapsedDetail(msg DisplayMessage) string {
	switch msg.ToolName {
	case "Bash":
		return bashCollapsedDetail(msg)
	case "Read":
		return readCollapsedDetail(msg)
	case "Edit":
		return editCollapsedDetail(msg)
	case "Write":
		return writeCollapsedDetail(msg)
	case "WebFetch":
		return webFetchCollapsedDetail(msg)
	case "WebSearch":
		return webSearchCollapsedDetail(msg)
	case "Agent":
		return agentCollapsedDetail(msg)
	case "TaskOutput":
		return taskOutputCollapsedDetail(msg)
	default:
		return defaultCollapsedDetail(msg)
	}
}

func bashCollapsedDetail(msg DisplayMessage) string {
	if msg.Content == "" {
		return "(no output)"
	}
	return truncateBashOutput(msg.Content)
}

func readCollapsedDetail(msg DisplayMessage) string {
	n := countContentLines(msg.Content)
	parts := []string{}
	if msg.Detail != "" {
		parts = append(parts, msg.Detail)
	}
	parts = append(parts, fmt.Sprintf("%d lines", n))
	return strings.Join(parts, " · ") + "  \u21a9 expand"
}

func editCollapsedDetail(msg DisplayMessage) string {
	if msg.IsError && strings.Contains(strings.ToLower(msg.Content), "must read") {
		return "\u26a0 must read file first"
	}
	added, removed := countDiffChanges(msg.Content)
	parts := []string{}
	if msg.Detail != "" {
		parts = append(parts, msg.Detail)
	}
	if added > 0 || removed > 0 {
		parts = append(parts, fmt.Sprintf("+%d/-%d lines", added, removed))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

func writeCollapsedDetail(msg DisplayMessage) string {
	n := countContentLines(msg.Content)
	parts := []string{}
	if msg.Detail != "" {
		parts = append(parts, msg.Detail)
	}
	parts = append(parts, fmt.Sprintf("+%d lines", n))
	return strings.Join(parts, " · ")
}

func webFetchCollapsedDetail(msg DisplayMessage) string {
	size := formatFileSize(int64(len(msg.Content)))
	parts := []string{}
	if msg.Detail != "" {
		parts = append(parts, msg.Detail)
	}
	parts = append(parts, size+" received")
	return strings.Join(parts, " · ")
}

func webSearchCollapsedDetail(msg DisplayMessage) string {
	count := countSearchResults(msg.Content)
	if count > 0 {
		result := fmt.Sprintf("Found %d results", count)
		if msg.Detail != "" {
			return msg.Detail + " \u2192 " + result
		}
		return result
	}
	if msg.Detail != "" {
		return msg.Detail
	}
	return ""
}

func agentCollapsedDetail(msg DisplayMessage) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	last := lines[len(lines)-1]
	if len(last) > defaultDetailMaxChars {
		last = last[:defaultDetailMaxChars-3] + "..."
	}
	return last
}

func taskOutputCollapsedDetail(msg DisplayMessage) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "not ready"
	}
	lower := strings.ToLower(content)
	if strings.Contains(lower, "still running") || strings.Contains(lower, "in progress") {
		return "still running\u2026"
	}
	return defaultCollapsedDetail(DisplayMessage{Content: content})
}

func defaultCollapsedDetail(msg DisplayMessage) string {
	if msg.Content == "" {
		return ""
	}
	firstLine := strings.SplitN(msg.Content, "\n", 2)[0]
	if len(firstLine) > defaultDetailMaxChars {
		firstLine = firstLine[:defaultDetailMaxChars-3] + "..."
	}
	return firstLine
}

// countSearchResults counts non-empty, non-header lines in search output.
func countSearchResults(content string) int {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			count++
		}
	}
	return count
}

// toolExpandedBody returns tool-specific formatted body for expanded view.
// Returns "" to use default rendering.
func toolExpandedBody(msg DisplayMessage, width int) string {
	switch msg.ToolName {
	case "Bash":
		if msg.Content == "" {
			return "(no output)"
		}
		return "" // non-empty bash uses default rendering
	case "Agent":
		return agentExpandedBody(msg)
	default:
		return ""
	}
}

func agentExpandedBody(msg DisplayMessage) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > agentMaxExpandedLines {
		skipped := len(lines) - agentMaxExpandedLines
		return fmt.Sprintf("... %d earlier messages\n", skipped) +
			strings.Join(lines[len(lines)-agentMaxExpandedLines:], "\n")
	}
	return "" // short enough for default rendering
}

// toolActiveVerb returns an enhanced progress verb for an active tool,
// supporting state changes like "Waiting..." for long-running Bash commands.
func toolActiveVerb(tool ActiveToolInfo) string {
	switch tool.Name {
	case "Bash":
		elapsed := time.Since(tool.StartTime)
		if elapsed > 10*time.Second {
			if tool.Detail != "" {
				return "Waiting for " + tool.Detail + "\u2026"
			}
			return "Waiting\u2026"
		}
		if tool.Detail != "" {
			return "Running " + tool.Detail + "\u2026"
		}
		return "Running\u2026"
	default:
		return toolVerbDetailed(tool.Name, tool.Detail)
	}
}
