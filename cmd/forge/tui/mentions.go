package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charmbracelet/lipgloss"
)

// MentionItem represents a single selectable item in the mention popup.
type MentionItem struct {
	Label    string // display name
	Value    string // inserted text
	Category string // "Files", "Skills", "Agents"
	Icon     string // 📄, ⚡, 🤖
}

// MentionSource provides search results for a given category.
type MentionSource interface {
	Search(query string) []MentionItem
	Category() string
}

// MentionPopup manages the @ mention dropdown state.
type MentionPopup struct {
	active   bool
	query    string // text after @
	items    []MentionItem
	selected int
	sources  []MentionSource
}

// NewMentionPopup creates a MentionPopup with the given sources.
func NewMentionPopup(sources ...MentionSource) *MentionPopup {
	return &MentionPopup{
		sources: sources,
	}
}

// MaxMentionVisible is the maximum number of items shown in the mention dropdown.
const MaxMentionVisible = 8

// Show activates the mention popup with the given query.
func (m *MentionPopup) Show(query string) {
	m.query = query
	m.items = m.search(query)
	m.selected = 0
	m.active = len(m.items) > 0
}

// Hide closes the mention popup.
func (m *MentionPopup) Hide() {
	m.active = false
	m.selected = 0
	m.query = ""
	m.items = nil
}

// Active returns whether the mention popup is currently shown.
func (m *MentionPopup) Active() bool {
	return m.active
}

// Query returns the current search query.
func (m *MentionPopup) Query() string {
	return m.query
}

// Items returns the current filtered items.
func (m *MentionPopup) Items() []MentionItem {
	return m.items
}

// Next moves the selection cursor forward (wraps around).
func (m *MentionPopup) Next() {
	if len(m.items) == 0 {
		return
	}
	m.selected = (m.selected + 1) % len(m.items)
}

// Prev moves the selection cursor backward (wraps around).
func (m *MentionPopup) Prev() {
	if len(m.items) == 0 {
		return
	}
	m.selected = (m.selected - 1 + len(m.items)) % len(m.items)
}

// Selected returns the currently highlighted item, or nil if none.
func (m *MentionPopup) Selected() *MentionItem {
	if !m.active || len(m.items) == 0 {
		return nil
	}
	return &m.items[m.selected]
}

// SelectedIndex returns the cursor position.
func (m *MentionPopup) SelectedIndex() int {
	return m.selected
}

// Update recalculates the filtered list based on new query text.
func (m *MentionPopup) Update(query string) {
	m.query = query
	m.items = m.search(query)
	if m.selected >= len(m.items) {
		m.selected = 0
	}
	m.active = len(m.items) > 0
}

// FilteredCount returns the number of matched items.
func (m *MentionPopup) FilteredCount() int {
	return len(m.items)
}

// search queries all sources and aggregates results.
func (m *MentionPopup) search(query string) []MentionItem {
	var results []MentionItem
	for _, src := range m.sources {
		items := src.Search(query)
		results = append(results, items...)
	}
	return results
}

// Render draws the mention popup dropdown.
func (m *MentionPopup) Render(width int, theme Theme) string {
	if !m.active || len(m.items) == 0 {
		return ""
	}

	// Calculate visible window
	visible := m.items
	startIdx := 0
	if len(visible) > MaxMentionVisible {
		startIdx = 0
		if m.selected >= MaxMentionVisible {
			startIdx = m.selected - MaxMentionVisible + 1
		}
		end := startIdx + MaxMentionVisible
		if end > len(visible) {
			end = len(visible)
			startIdx = end - MaxMentionVisible
		}
		visible = visible[startIdx : startIdx+MaxMentionVisible]
	}

	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sb strings.Builder
	lastCategory := ""
	for i, item := range visible {
		// Render category header when category changes
		if item.Category != lastCategory {
			if lastCategory != "" {
				// blank separator between categories
			}
			header := theme.AutocompleteSelectedStyle.Render(item.Category)
			sb.WriteString("  " + header + "\n")
			lastCategory = item.Category
		}

		actualIdx := startIdx + i
		label := item.Icon + " " + item.Label

		// Truncate label to fit
		maxLabel := innerWidth - 4
		if maxLabel > 0 && len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		if actualIdx == m.selected {
			line := theme.AutocompleteSelectedStyle.Render("> " + label)
			sb.WriteString(line)
		} else {
			line := theme.AutocompleteStyle.Render("  " + label)
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
	}

	// Scroll indicator
	if len(m.items) > MaxMentionVisible {
		indicator := theme.AutocompleteDimStyle.Render(
			"  ↕ " + strings.Repeat("·", 3) + " more")
		sb.WriteString(indicator + "\n")
	}

	content := strings.TrimRight(sb.String(), "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.Config.DimColor)).
		Width(width - 4).
		Render(content)

	return box + "\n"
}

// --- Concrete MentionSource implementations ---

// FileMentionSource searches for files in the working directory.
type FileMentionSource struct {
	Cwd string // working directory to search
}

func (f *FileMentionSource) Category() string { return "Files" }

func (f *FileMentionSource) Search(query string) []MentionItem {
	cwd := f.Cwd
	if cwd == "" {
		cwd = "."
	}

	query = strings.ToLower(query)

	// Build glob pattern — prefer source files over docs
	pattern := "**/*"
	if query != "" {
		pattern = "**/*" + query + "*"
	}
	// Also exclude common non-source directories
	skipDirs := map[string]bool{
		"docs": true, "reviews": true, "bin": true,
	}

	fsys := os.DirFS(cwd)
	matches, err := doublestar.Glob(fsys, pattern)
	if err != nil {
		return nil
	}

	var items []MentionItem
	for _, match := range matches {
		// Skip hidden directories and common noise
		if shouldSkipPath(match) {
			continue
		}
		// Skip docs/reviews directories to prioritize source code
		parts := strings.Split(match, string(filepath.Separator))
		skipThis := false
		for _, p := range parts {
			if skipDirs[p] {
				skipThis = true
				break
			}
		}
		if skipThis {
			continue
		}

		// Check it's a file, not a directory
		info, err := os.Stat(filepath.Join(cwd, match))
		if err != nil || info.IsDir() {
			continue
		}

		items = append(items, MentionItem{
			Label:    match,
			Value:    match,
			Category: "Files",
			Icon:     FileIcon(match),
		})
		if len(items) >= 10 {
			break
		}
	}
	return items
}

// shouldSkipPath returns true for paths that should be excluded from file search.
func shouldSkipPath(path string) bool {
	parts := strings.Split(path, string(filepath.Separator))
	for _, p := range parts {
		if strings.HasPrefix(p, ".") {
			return true
		}
		switch p {
		case "node_modules", "vendor", "__pycache__", "dist", "build":
			return true
		}
	}
	return false
}

// SkillMentionSource searches available skills/commands.
type SkillMentionSource struct {
	Registry *CommandRegistry
}

func (s *SkillMentionSource) Category() string { return "Skills" }

func (s *SkillMentionSource) Search(query string) []MentionItem {
	if s.Registry == nil {
		return nil
	}

	query = strings.ToLower(query)
	var items []MentionItem
	for _, cmd := range s.Registry.Commands() {
		if cmd.Hidden {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(cmd.Name), query) {
			items = append(items, MentionItem{
				Label:    "/" + cmd.Name + " — " + cmd.Description,
				Value:    "/" + cmd.Name,
				Category: "Skills",
				Icon:     "⚡",
			})
		}
		if len(items) >= 10 {
			break
		}
	}
	return items
}

// AgentMentionSource searches available agent definitions.
type AgentMentionSource struct {
	Dirs []string // directories to scan for agent definitions
}

func (a *AgentMentionSource) Category() string { return "Agents" }

func (a *AgentMentionSource) Search(query string) []MentionItem {
	query = strings.ToLower(query)
	var items []MentionItem
	seen := make(map[string]bool)

	for _, dir := range a.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Only include .md agent definition files, skip .output and UUID files
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			base := strings.TrimSuffix(name, ".md")
			if seen[base] {
				continue
			}
			if query == "" || strings.Contains(strings.ToLower(base), query) {
				seen[base] = true
				items = append(items, MentionItem{
					Label:    base,
					Value:    base,
					Category: "Agents",
					Icon:     "🤖",
				})
			}
			if len(items) >= 10 {
				break
			}
		}
	}
	return items
}

// agentDirs returns the default directories to scan for agent definitions.
func agentDirs(cwd string) []string {
	var dirs []string
	if cwd != "" {
		dirs = append(dirs, filepath.Join(cwd, ".forge", "agents"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".forge", "agents"))
	}
	return dirs
}
