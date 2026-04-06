package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SplashInfo holds the data displayed on the startup splash screen.
type SplashInfo struct {
	Version      string
	Model        string
	Cwd          string
	CommandCount int
	ToolCount    int
	SkillNames   []string // e.g. ["/commit", "/review", "/forge"]
}

// hammerArt is a braille-art Mjolnir used as the Forge icon.
var hammerArt = []string{
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⠞⢁⣴⣦⡀⠀⠀⠀⠀⠀⠀⠀⠀`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡠⠊⢀⣴⣿⣿⣿⣿⣦⡀⠀⠀⠀⠀⠀⠀`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠎⢀⣴⣿⣿⣿⣿⣿⣿⣿⣿⣦⡀⠀⠀⠀⠀`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⢴⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣦⡀⠀⠀`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣦⡀`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠟⢁`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⠘⢦⡀⠙⢿⣿⣿⣿⣿⣿⣿⠟⠁⡴⠋`,
	`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠐⠛⠓⠀⠉⠀⠀⠙⢿⣿⣿⠟⠁⡠⠊⠀⠀`,
	`⠀⠀⠀⠀⠀⠀⠀⠐⠛⠛⠛⠁⠀⠀⠀⠀⠀⠀⠙⠁⠐⠊⠀⠀⠀⠀`,
	`⠀⠀⠀⠀⠀⣠⣶⣶⣶⡖⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`,
	`⠀⠀⠀⢀⠀⢤⣤⡤⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`,
	`⠀⢰⣦⡈⠳⠄⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`,
	`⠀⠈⠻⠿⠆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`,
}

// renderSplash renders the startup splash screen shown when no messages exist.
func renderSplash(info SplashInfo, width int, theme Theme) string {
	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AccentColor))
	boldAccent := accentStyle.Bold(true)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor)).
		Faint(true)
	faintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor)).
		Faint(true)

	// Build the info lines (right side of hammer art)
	titleLine := boldAccent.Render("Forge") + " " + dimStyle.Render(info.Version)

	var metaParts []string
	if info.Model != "" {
		metaParts = append(metaParts, abbreviateModel(info.Model))
	}
	cwd := shortenHome(info.Cwd)
	metaParts = append(metaParts, cwd)
	metaLine := dimStyle.Render(strings.Join(metaParts, " · "))

	var skillsLine string
	if len(info.SkillNames) > 0 {
		maxShow := 5
		shown := info.SkillNames
		suffix := ""
		if len(shown) > maxShow {
			shown = shown[:maxShow]
			suffix = ", ..."
		}
		skillsLine = dimStyle.Render(
			fmt.Sprintf("%d skills loaded: %s%s", len(info.SkillNames), strings.Join(shown, ", "), suffix))
	}

	var countsLine string
	if info.CommandCount > 0 || info.ToolCount > 0 {
		var parts []string
		if info.CommandCount > 0 {
			parts = append(parts, fmt.Sprintf("%d commands", info.CommandCount))
		}
		if info.ToolCount > 0 {
			parts = append(parts, fmt.Sprintf("%d tools", info.ToolCount))
		}
		countsLine = dimStyle.Render(strings.Join(parts, " · "))
	}

	hintLine := faintStyle.Render("? for shortcuts · / for commands · @ for mentions")

	// Compose right-side info lines
	infoLines := []string{titleLine, metaLine}
	if skillsLine != "" {
		infoLines = append(infoLines, skillsLine)
	}
	if countsLine != "" {
		infoLines = append(infoLines, countsLine)
	}
	infoLines = append(infoLines, "")
	infoLines = append(infoLines, hintLine)

	// Render hammer art in accent color
	artLines := make([]string, len(hammerArt))
	for i, line := range hammerArt {
		artLines[i] = accentStyle.Render(line)
	}

	// Side-by-side: hammer art on left, info on right
	gap := "  "

	// Pad both sides to same height
	maxLines := len(artLines)
	if len(infoLines) > maxLines {
		maxLines = len(infoLines)
	}
	artPad := strings.Repeat(" ", lipgloss.Width(accentStyle.Render(hammerArt[0])))
	for len(artLines) < maxLines {
		artLines = append(artLines, artPad)
	}
	for len(infoLines) < maxLines {
		infoLines = append(infoLines, "")
	}

	var sb strings.Builder
	sb.WriteString("\n")
	for i := 0; i < maxLines; i++ {
		sb.WriteString("  ") // left margin
		sb.WriteString(artLines[i])
		sb.WriteString(gap)
		sb.WriteString(infoLines[i])
		sb.WriteString("\n")
	}

	return sb.String()
}

// SplashScreen bundles the info and theme needed to render the splash.
type SplashScreen struct {
	Info  SplashInfo
	Theme Theme
}

// shortenHome replaces the user's home directory prefix with "~".
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return path
}
