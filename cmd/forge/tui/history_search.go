package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HistorySearch provides a Ctrl+R fuzzy search overlay for input history.
// It filters history entries as the user types, with arrow navigation
// and Enter to select.
type HistorySearch struct {
	active  bool
	query   string
	results []string // filtered results (newest first)
	cursor  int      // index into results
	history *History
}

// NewHistorySearch creates a HistorySearch backed by the given history.
func NewHistorySearch(history *History) *HistorySearch {
	return &HistorySearch{
		history: history,
	}
}

// Open activates the search overlay with an empty query.
func (hs *HistorySearch) Open() {
	hs.active = true
	hs.query = ""
	hs.cursor = 0
	hs.filter()
}

// Close hides the search overlay and resets state.
func (hs *HistorySearch) Close() {
	hs.active = false
	hs.query = ""
	hs.results = nil
	hs.cursor = 0
}

// Active returns true if the search overlay is open.
func (hs *HistorySearch) Active() bool {
	return hs.active
}

// Query returns the current search query.
func (hs *HistorySearch) Query() string {
	return hs.query
}

// SetQuery updates the search query and re-filters results.
func (hs *HistorySearch) SetQuery(q string) {
	hs.query = q
	hs.cursor = 0
	hs.filter()
}

// TypeChar appends a character to the query and re-filters.
func (hs *HistorySearch) TypeChar(ch rune) {
	hs.query += string(ch)
	hs.cursor = 0
	hs.filter()
}

// Backspace removes the last character from the query.
func (hs *HistorySearch) Backspace() {
	if len(hs.query) > 0 {
		hs.query = hs.query[:len(hs.query)-1]
		hs.cursor = 0
		hs.filter()
	}
}

// Next moves the cursor to the next (older) result.
func (hs *HistorySearch) Next() {
	if len(hs.results) == 0 {
		return
	}
	hs.cursor = (hs.cursor + 1) % len(hs.results)
}

// Prev moves the cursor to the previous (newer) result.
func (hs *HistorySearch) Prev() {
	if len(hs.results) == 0 {
		return
	}
	hs.cursor = (hs.cursor - 1 + len(hs.results)) % len(hs.results)
}

// Selected returns the currently highlighted entry, or empty string if none.
func (hs *HistorySearch) Selected() string {
	if len(hs.results) == 0 || hs.cursor >= len(hs.results) {
		return ""
	}
	return hs.results[hs.cursor]
}

// ResultCount returns the number of matching entries.
func (hs *HistorySearch) ResultCount() int {
	return len(hs.results)
}

// MaxSearchResults is the maximum number of results shown in the overlay.
const MaxSearchResults = 10

// filter recalculates results based on the current query.
// Results are ordered newest-first (reverse of history entries).
func (hs *HistorySearch) filter() {
	hs.results = nil
	entries := hs.history.Entries()
	lower := strings.ToLower(hs.query)

	// Iterate newest-first
	for i := len(entries) - 1; i >= 0; i-- {
		if hs.query == "" || fuzzyContains(strings.ToLower(entries[i]), lower) {
			hs.results = append(hs.results, entries[i])
			if len(hs.results) >= MaxSearchResults {
				break
			}
		}
	}
}

// fuzzyContains checks if haystack contains all characters of needle in order.
func fuzzyContains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hi := 0
	for ni := 0; ni < len(needle); ni++ {
		found := false
		for hi < len(haystack) {
			if haystack[hi] == needle[ni] {
				hi++
				found = true
				break
			}
			hi++
		}
		if !found {
			return false
		}
	}
	return true
}

// Render draws the history search overlay.
func (hs *HistorySearch) Render(width int, theme Theme) string {
	if !hs.active {
		return ""
	}

	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor)).
		Faint(true)
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AccentColor)).
		Bold(true)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AssistantColor))

	var sb strings.Builder

	// Header with search query
	header := dimStyle.Render("bck-i-search: ") + normalStyle.Render(hs.query) + dimStyle.Render("_")
	sb.WriteString(header)
	sb.WriteByte('\n')

	if len(hs.results) == 0 {
		sb.WriteString(dimStyle.Render("  (no matches)"))
		sb.WriteByte('\n')
	} else {
		for i, entry := range hs.results {
			// Truncate long entries
			display := entry
			if len(display) > innerWidth-4 {
				display = display[:innerWidth-7] + "..."
			}

			if i == hs.cursor {
				sb.WriteString(selectedStyle.Render("> " + display))
			} else {
				sb.WriteString(normalStyle.Render("  " + display))
			}
			sb.WriteByte('\n')
		}
	}

	content := strings.TrimRight(sb.String(), "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.Config.DimColor)).
		Width(width - 4).
		Render(content)

	return box + "\n"
}
