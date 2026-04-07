package tui

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchResult represents a single ripgrep match.
type SearchResult struct {
	File    string // file path
	Line    int    // line number
	Content string // matched line content (trimmed)
}

// GlobalSearchDialog manages the Ctrl+F ripgrep search overlay.
type GlobalSearchDialog struct {
	query      string
	results    []SearchResult
	selected   int
	totalFound int
	cwd        string
	searching  bool // true while an rg process is running
	cancelFn   context.CancelFunc
}

// maxSearchResults caps the number of results displayed.
const maxSearchResults = 100

// maxSearchVisible is the max items shown at once.
const maxSearchVisible = 15

// searchDebounceMs is the debounce delay before kicking off rg.
const searchDebounceMs = 100

// globalSearchGeneration is a global atomic counter for race protection.
var globalSearchGeneration int64

// globalSearchDoneMsg delivers rg search results.
type globalSearchDoneMsg struct {
	results    []SearchResult
	total      int
	generation int64
	err        error
}

// globalSearchDebounceMsg triggers an rg search after debounce.
type globalSearchDebounceMsg struct {
	query      string
	generation int64
}

// NewGlobalSearchDialog creates a GlobalSearchDialog for the given cwd.
func NewGlobalSearchDialog(cwd string) *GlobalSearchDialog {
	return &GlobalSearchDialog{cwd: cwd}
}

// Open resets the dialog state.
func (d *GlobalSearchDialog) Open() {
	d.query = ""
	d.results = nil
	d.selected = 0
	d.totalFound = 0
	d.searching = false
	d.cancelFn = nil
}

// Close cancels any in-flight search and resets state.
func (d *GlobalSearchDialog) Close() {
	if d.cancelFn != nil {
		d.cancelFn()
		d.cancelFn = nil
	}
	d.query = ""
	d.results = nil
	d.selected = 0
	d.searching = false
}

// SelectedResult returns the currently highlighted search result.
func (d *GlobalSearchDialog) SelectedResult() *SearchResult {
	if d.selected >= len(d.results) {
		return nil
	}
	return &d.results[d.selected]
}

// SelectedPath returns the file path of the currently highlighted result.
func (d *GlobalSearchDialog) SelectedPath() string {
	r := d.SelectedResult()
	if r == nil {
		return ""
	}
	return r.File
}

// Next moves the selection cursor down (wraps).
func (d *GlobalSearchDialog) Next() {
	if len(d.results) == 0 {
		return
	}
	d.selected = (d.selected + 1) % len(d.results)
}

// Prev moves the selection cursor up (wraps).
func (d *GlobalSearchDialog) Prev() {
	if len(d.results) == 0 {
		return
	}
	d.selected = (d.selected - 1 + len(d.results)) % len(d.results)
}

// TypeRune appends a character to the query and schedules a debounced search.
func (d *GlobalSearchDialog) TypeRune(ch rune) tea.Cmd {
	d.query += string(ch)
	return d.scheduleSearch()
}

// Backspace removes the last rune and schedules a debounced search.
func (d *GlobalSearchDialog) Backspace() tea.Cmd {
	if len(d.query) == 0 {
		return nil
	}
	runes := []rune(d.query)
	d.query = string(runes[:len(runes)-1])
	if d.query == "" {
		d.cancelPrevious()
		d.results = nil
		d.totalFound = 0
		d.selected = 0
		d.searching = false
		return nil
	}
	return d.scheduleSearch()
}

// scheduleSearch returns a debounced tea.Cmd that will trigger the rg search.
func (d *GlobalSearchDialog) scheduleSearch() tea.Cmd {
	gen := atomic.AddInt64(&globalSearchGeneration, 1)
	query := d.query
	return tea.Tick(time.Duration(searchDebounceMs)*time.Millisecond, func(t time.Time) tea.Msg {
		return globalSearchDebounceMsg{query: query, generation: gen}
	})
}

// StartSearch cancels any previous search and starts a new rg process.
// Called when a debounce message arrives with a matching generation.
func (d *GlobalSearchDialog) StartSearch(query string) tea.Cmd {
	d.cancelPrevious()
	d.searching = true

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	d.cancelFn = cancel

	gen := atomic.LoadInt64(&globalSearchGeneration)
	cwd := d.cwd

	return func() tea.Msg {
		results, total, err := runRipgrep(ctx, cwd, query)
		return globalSearchDoneMsg{
			results:    results,
			total:      total,
			generation: gen,
			err:        err,
		}
	}
}

// HandleSearchDone processes rg results if the generation matches.
func (d *GlobalSearchDialog) HandleSearchDone(msg globalSearchDoneMsg) {
	if msg.generation != atomic.LoadInt64(&globalSearchGeneration) {
		return // stale
	}
	d.searching = false
	if msg.err != nil {
		// Context cancellation is expected — not an error
		if ctx := context.Canceled; msg.err == ctx {
			return
		}
		d.results = nil
		d.totalFound = 0
		return
	}
	d.results = msg.results
	d.totalFound = msg.total
	d.selected = 0
}

// cancelPrevious cancels any in-flight rg process.
func (d *GlobalSearchDialog) cancelPrevious() {
	if d.cancelFn != nil {
		d.cancelFn()
		d.cancelFn = nil
	}
}

// Render draws the global search overlay.
func (d *GlobalSearchDialog) Render(width, vpHeight int, theme Theme) string {
	innerWidth := width - 8
	if innerWidth < 30 {
		innerWidth = 30
	}

	var sb strings.Builder

	// Header with match count
	header := theme.HeaderStyle.Render("Global Search")
	if d.searching {
		sb.WriteString(header + theme.AutocompleteDimStyle.Render(" (searching…)"))
	} else if d.query != "" {
		sb.WriteString(header + theme.AutocompleteDimStyle.Render(
			fmt.Sprintf(" (%d matches)", d.totalFound)))
	} else {
		sb.WriteString(header)
	}
	sb.WriteByte('\n')

	// Search input
	cursor := "█"
	sb.WriteString(theme.AutocompleteSelectedStyle.Render("🔍 " + d.query + cursor))
	sb.WriteByte('\n')
	sb.WriteString(theme.AutocompleteDimStyle.Render(strings.Repeat("─", innerWidth)))
	sb.WriteByte('\n')

	linesUsed := 3

	// Results list
	if len(d.results) == 0 {
		if d.query != "" && !d.searching {
			sb.WriteString(theme.AutocompleteDimStyle.Render("No matches found"))
			sb.WriteByte('\n')
			linesUsed++
		}
	} else {
		maxVis := vpHeight - 7
		if maxVis < 5 {
			maxVis = 5
		}
		if maxVis > maxSearchVisible {
			maxVis = maxSearchVisible
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

		for i, result := range visible {
			actualIdx := startIdx + i
			icon := FileIcon(result.File)
			location := fmt.Sprintf("%s:%d", result.File, result.Line)
			content := strings.TrimSpace(result.Content)

			// Truncate content to fit on one line
			maxContent := innerWidth - len(location) - 6
			if maxContent > 0 && len(content) > maxContent {
				content = content[:maxContent-1] + "…"
			}

			line := icon + " " + location
			if content != "" {
				line += "  " + content
			}

			maxLine := innerWidth - 4
			if maxLine > 0 && len(line) > maxLine {
				line = line[:maxLine-1] + "…"
			}

			if actualIdx == d.selected {
				sb.WriteString(theme.AutocompleteSelectedStyle.Render("> " + line))
			} else {
				sb.WriteString(theme.AutocompleteStyle.Render("  " + line))
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
	remaining := vpHeight - linesUsed - 4
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

// runRipgrep executes rg and parses its output.
// Returns results (up to maxSearchResults), total match count, and any error.
func runRipgrep(ctx context.Context, cwd, pattern string) ([]SearchResult, int, error) {
	if pattern == "" {
		return nil, 0, nil
	}

	cmd := exec.CommandContext(ctx, "rg",
		"--line-number",
		"--no-heading",
		"--color=never",
		"--max-count=200", // cap per-file matches
		"--glob=!.git",
		"--glob=!node_modules",
		"--glob=!vendor",
		"--glob=!__pycache__",
		"--glob=!dist",
		"--glob=!build",
		pattern,
	)
	cmd.Dir = cwd

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}

	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}

	var results []SearchResult
	total := 0
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		line := scanner.Text()
		result, ok := parseRgLine(line)
		if !ok {
			continue
		}
		total++
		if len(results) < maxSearchResults {
			results = append(results, result)
		}
	}

	// Wait for rg to finish; exit code 1 means "no matches" — not an error.
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		// rg exits with 1 when no matches found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, 0, nil
		}
		return results, total, nil // partial results OK
	}

	return results, total, nil
}

// parseRgLine parses a single rg output line in "file:line:content" format.
func parseRgLine(line string) (SearchResult, bool) {
	// Format: file:line:content — use strings.Cut for single-pass parsing.
	file, rest, ok := strings.Cut(line, ":")
	if !ok || file == "" {
		return SearchResult{}, false
	}
	lineStr, content, ok := strings.Cut(rest, ":")
	if !ok {
		return SearchResult{}, false
	}
	lineNum, err := strconv.Atoi(lineStr)
	if err != nil {
		return SearchResult{}, false
	}
	return SearchResult{
		File:    file,
		Line:    lineNum,
		Content: content,
	}, true
}
