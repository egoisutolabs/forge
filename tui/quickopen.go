package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// QuickOpenItem represents a file result in the quick open dialog.
type QuickOpenItem struct {
	Path  string // relative path from cwd
	Score int    // fuzzy match score (higher = better)
	Icon  string // Nerd Font icon
}

// QuickOpenDialog manages the Ctrl+O fuzzy file finder overlay.
type QuickOpenDialog struct {
	query      string
	allFiles   []string        // cached file paths from initial scan
	results    []QuickOpenItem // fuzzy-filtered, scored, and limited
	selected   int             // cursor position in results
	totalFound int             // total matches before limit
	cwd        string
	scanned    bool // true after initial file scan completes
}

// maxQuickOpenResults caps the displayed result count.
const maxQuickOpenResults = 100

// maxQuickOpenVisible is the max items visible at once in the list.
const maxQuickOpenVisible = 15

// maxScanFiles caps how many files we collect during a scan.
const maxScanFiles = 10000

// quickOpenGeneration is a global atomic counter for race protection.
var quickOpenGeneration int64

// quickOpenScanDoneMsg delivers scanned file paths from a background scan.
type quickOpenScanDoneMsg struct {
	files      []string
	generation int64
}

// NewQuickOpenDialog creates a QuickOpenDialog for the given working directory.
func NewQuickOpenDialog(cwd string) *QuickOpenDialog {
	return &QuickOpenDialog{cwd: cwd}
}

// Open resets the dialog state and starts a background file scan.
// Returns a tea.Cmd that performs the scan.
func (d *QuickOpenDialog) Open() tea.Cmd {
	d.query = ""
	d.results = nil
	d.selected = 0
	d.totalFound = 0
	d.scanned = false
	d.allFiles = nil

	gen := atomic.AddInt64(&quickOpenGeneration, 1)
	cwd := d.cwd

	return func() tea.Msg {
		files := scanFiles(cwd)
		return quickOpenScanDoneMsg{files: files, generation: gen}
	}
}

// HandleScanDone processes background scan results if the generation matches.
func (d *QuickOpenDialog) HandleScanDone(msg quickOpenScanDoneMsg) {
	if msg.generation != atomic.LoadInt64(&quickOpenGeneration) {
		return // stale
	}
	d.allFiles = msg.files
	d.scanned = true
	d.filterFiles()
}

// SelectedPath returns the path of the currently highlighted result.
func (d *QuickOpenDialog) SelectedPath() string {
	if d.selected >= len(d.results) {
		return ""
	}
	return d.results[d.selected].Path
}

// Next moves the selection cursor down (wraps).
func (d *QuickOpenDialog) Next() {
	if len(d.results) == 0 {
		return
	}
	d.selected = (d.selected + 1) % len(d.results)
}

// Prev moves the selection cursor up (wraps).
func (d *QuickOpenDialog) Prev() {
	if len(d.results) == 0 {
		return
	}
	d.selected = (d.selected - 1 + len(d.results)) % len(d.results)
}

// TypeRune appends a character to the query and refilters.
func (d *QuickOpenDialog) TypeRune(ch rune) {
	d.query += string(ch)
	d.filterFiles()
}

// Backspace removes the last rune from the query and refilters.
func (d *QuickOpenDialog) Backspace() {
	if len(d.query) == 0 {
		return
	}
	runes := []rune(d.query)
	d.query = string(runes[:len(runes)-1])
	d.filterFiles()
}

// filterFiles applies fuzzy matching on the cached file list.
func (d *QuickOpenDialog) filterFiles() {
	if d.query == "" {
		// No filter — show all files up to limit
		n := len(d.allFiles)
		if n > maxQuickOpenResults {
			n = maxQuickOpenResults
		}
		items := make([]QuickOpenItem, n)
		for i := 0; i < n; i++ {
			items[i] = QuickOpenItem{
				Path: d.allFiles[i],
				Icon: FileIcon(d.allFiles[i]),
			}
		}
		d.results = items
		d.totalFound = len(d.allFiles)
		d.selected = 0
		return
	}

	type scored struct {
		path  string
		score int
	}
	var matches []scored
	for _, f := range d.allFiles {
		if ok, score := fuzzyMatch(d.query, f); ok {
			matches = append(matches, scored{path: f, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	d.totalFound = len(matches)
	if len(matches) > maxQuickOpenResults {
		matches = matches[:maxQuickOpenResults]
	}

	items := make([]QuickOpenItem, len(matches))
	for i, m := range matches {
		items[i] = QuickOpenItem{
			Path:  m.path,
			Score: m.score,
			Icon:  FileIcon(m.path),
		}
	}
	d.results = items
	d.selected = 0
}

// Render draws the quick open overlay.
// vpHeight is the available viewport height so the dialog can fill it.
func (d *QuickOpenDialog) Render(width, vpHeight int, theme Theme) string {
	innerWidth := width - 8
	if innerWidth < 30 {
		innerWidth = 30
	}

	var sb strings.Builder

	// Header with match count
	header := theme.HeaderStyle.Render("Quick Open")
	if d.scanned {
		sb.WriteString(header + theme.AutocompleteDimStyle.Render(
			fmt.Sprintf(" (%d/%d)", len(d.results), d.totalFound)))
	} else {
		sb.WriteString(header + theme.AutocompleteDimStyle.Render(" (scanning…)"))
	}
	sb.WriteByte('\n')

	// Search input
	cursor := "█"
	sb.WriteString(theme.AutocompleteSelectedStyle.Render("🔍 " + d.query + cursor))
	sb.WriteByte('\n')
	sb.WriteString(theme.AutocompleteDimStyle.Render(strings.Repeat("─", innerWidth)))
	sb.WriteByte('\n')

	// Results list
	linesUsed := 3 // header + input + separator
	if len(d.results) == 0 {
		if d.scanned {
			sb.WriteString(theme.AutocompleteDimStyle.Render("No files found"))
			sb.WriteByte('\n')
			linesUsed++
		}
	} else {
		maxVis := vpHeight - 7 // reserve for header, input, separator, scroll indicator, hints, padding
		if maxVis < 5 {
			maxVis = 5
		}
		if maxVis > maxQuickOpenVisible {
			maxVis = maxQuickOpenVisible
		}

		visible := d.results
		startIdx := 0
		if len(visible) > maxVis {
			if d.selected >= maxVis {
				startIdx = d.selected - maxVis + 1
			}
			end := startIdx + maxVis
			if end > len(visible) {
				end = len(visible)
				startIdx = end - maxVis
			}
			visible = visible[startIdx : startIdx+maxVis]
		}

		for i, item := range visible {
			actualIdx := startIdx + i
			label := item.Icon + " " + item.Path

			maxLabel := innerWidth - 4
			if maxLabel > 0 && len(label) > maxLabel {
				label = label[:maxLabel-1] + "…"
			}

			if actualIdx == d.selected {
				sb.WriteString(theme.AutocompleteSelectedStyle.Render("> " + label))
			} else {
				sb.WriteString(theme.AutocompleteStyle.Render("  " + label))
			}
			sb.WriteByte('\n')
			linesUsed++
		}

		if len(d.results) > maxVis {
			sb.WriteString(theme.AutocompleteDimStyle.Render("↕ ··· more"))
			sb.WriteByte('\n')
			linesUsed++
		}
	}

	// Footer hints
	sb.WriteString(theme.AutocompleteDimStyle.Render(
		"enter: mention · tab: @path · shift+tab: raw path · esc: close"))
	linesUsed++

	// Pad to fill viewport height
	remaining := vpHeight - linesUsed - 4 // account for box border + padding
	for i := 0; i < remaining; i++ {
		sb.WriteByte('\n')
	}

	content := sb.String()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Config.AccentColor)).
		Padding(0, 1).
		Width(width - 4).
		Render(content)

	return box + "\n"
}

// scanFiles walks cwd and collects file paths, skipping noise directories.
func scanFiles(cwd string) []string {
	var files []string

	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return nil
		}

		if d.IsDir() {
			base := filepath.Base(path)
			switch base {
			case ".git", "node_modules", "vendor", "__pycache__", "dist", "build", ".cache", ".next":
				return filepath.SkipDir
			}
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(filepath.Base(rel), ".") {
			return nil
		}

		files = append(files, rel)
		if len(files) >= maxScanFiles {
			return filepath.SkipAll
		}
		return nil
	})

	return files
}

// fuzzyMatch checks whether all characters in pattern appear in order in str.
// Returns match success and a relevance score (higher = better match).
func fuzzyMatch(pattern, str string) (bool, int) {
	pLower := strings.ToLower(pattern)
	sLower := strings.ToLower(str)

	if pLower == "" {
		return true, 0
	}

	pi := 0
	score := 0
	lastMatchIdx := -1
	lastSlash := strings.LastIndex(sLower, "/")

	for si := 0; si < len(sLower) && pi < len(pLower); si++ {
		if sLower[si] != pLower[pi] {
			continue
		}
		// Consecutive match bonus
		if lastMatchIdx == si-1 {
			score += 5
		}
		// Word boundary bonus (start, after / . _ -)
		if si == 0 || sLower[si-1] == '/' || sLower[si-1] == '.' ||
			sLower[si-1] == '_' || sLower[si-1] == '-' {
			score += 3
		}
		// Filename portion bonus
		if si > lastSlash {
			score += 2
		}
		score++
		lastMatchIdx = si
		pi++
	}

	if pi < len(pLower) {
		return false, 0
	}

	// Prefer shorter paths
	score -= len(str) / 20

	return true, score
}
