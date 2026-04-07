package tui

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// urlRe matches http/https URLs in text.
var urlRe = regexp.MustCompile(`https?://[^\s<>\[\]()'"` + "`" + `]+[^\s<>\[\]()'",.:;!?` + "`" + `]`)

// supportsHyperlinks returns true if the terminal supports OSC 8 hyperlinks.
func supportsHyperlinks() bool {
	// iTerm2 always supports OSC 8
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		return true
	}
	// WezTerm supports OSC 8
	if os.Getenv("TERM_PROGRAM") == "WezTerm" {
		return true
	}
	// Kitty supports OSC 8
	if strings.Contains(os.Getenv("TERM"), "kitty") {
		return true
	}
	// Windows Terminal supports OSC 8
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	// VS Code terminal supports OSC 8
	if os.Getenv("TERM_PROGRAM") == "vscode" {
		return true
	}
	// Ghostty supports OSC 8
	if os.Getenv("TERM_PROGRAM") == "ghostty" {
		return true
	}
	return false
}

// osc8Link wraps a URL in an OSC 8 hyperlink escape sequence.
func osc8Link(url, display string) string {
	return "\x1b]8;;" + url + "\x1b\\" + display + "\x1b]8;;\x1b\\"
}

// linkStyle is the fallback style for URLs when OSC 8 is not supported.
var linkStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("12")).
	Underline(true)

// RenderHyperlinks replaces URLs in text with clickable OSC 8 links (if supported)
// or underlined blue text (fallback).
func RenderHyperlinks(text string) string {
	if text == "" {
		return text
	}

	urls := urlRe.FindAllStringIndex(text, -1)
	if len(urls) == 0 {
		return text
	}

	osc8 := supportsHyperlinks()

	var sb strings.Builder
	sb.Grow(len(text) + len(urls)*64)

	last := 0
	for _, loc := range urls {
		sb.WriteString(text[last:loc[0]])
		url := text[loc[0]:loc[1]]
		if osc8 {
			sb.WriteString(osc8Link(url, url))
		} else {
			sb.WriteString(linkStyle.Render(url))
		}
		last = loc[1]
	}
	sb.WriteString(text[last:])

	return sb.String()
}

// DetectURLs returns all URLs found in text.
func DetectURLs(text string) []string {
	return urlRe.FindAllString(text, -1)
}
