package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SlashCommand defines a registered slash command.
type SlashCommand struct {
	Name        string   // e.g. "help" (without the leading "/")
	Description string   // shown in dropdown
	Aliases     []string // e.g. ["h", "?"]
	Hidden      bool     // internal commands not shown in autocomplete
	Handler     func(args string) tea.Cmd
}

// CommandRegistry holds all available slash commands.
type CommandRegistry struct {
	commands []SlashCommand
}

// NewCommandRegistry creates a registry with the built-in commands.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: builtinCommands(),
	}
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd SlashCommand) {
	r.commands = append(r.commands, cmd)
}

// Commands returns all registered commands.
func (r *CommandRegistry) Commands() []SlashCommand {
	return r.commands
}

// Match returns commands whose name or aliases start with the given prefix.
// Empty prefix returns all non-hidden commands.
func (r *CommandRegistry) Match(prefix string) []SlashCommand {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var results []SlashCommand
	for _, cmd := range r.commands {
		if cmd.Hidden {
			continue
		}
		if prefix == "" || strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			results = append(results, cmd)
			continue
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(strings.ToLower(alias), prefix) {
				results = append(results, cmd)
				break
			}
		}
	}
	return results
}

// Lookup finds a command by exact name or alias. Returns nil if not found.
func (r *CommandRegistry) Lookup(name string) *SlashCommand {
	name = strings.ToLower(strings.TrimSpace(name))
	for i := range r.commands {
		if strings.ToLower(r.commands[i].Name) == name {
			return &r.commands[i]
		}
		for _, alias := range r.commands[i].Aliases {
			if strings.ToLower(alias) == name {
				return &r.commands[i]
			}
		}
	}
	return nil
}

func builtinCommands() []SlashCommand {
	return []SlashCommand{
		{Name: "help", Description: "Show help and keyboard shortcuts", Aliases: []string{"h", "?"}},
		{Name: "clear", Description: "Clear conversation history", Aliases: []string{"cls"}},
		{Name: "connect", Description: "Connect an API provider"},
		{Name: "model", Description: "Switch model"},
		{Name: "compact", Description: "Toggle compact mode"},
		{Name: "cost", Description: "Show detailed cost breakdown"},
		{Name: "diff", Description: "Show all file changes this session"},
		{Name: "history", Description: "Browse conversation history"},
		{Name: "providers", Description: "Show API provider status"},
		{Name: "quit", Description: "Exit Forge", Aliases: []string{"exit", "q"}},
		{Name: "bug", Description: "Report a bug"},
		{Name: "config", Description: "Open configuration"},
	}
}

// Autocomplete manages the slash command dropdown state.
type Autocomplete struct {
	registry *CommandRegistry
	filtered []SlashCommand
	cursor   int
	visible  bool
	query    string // text after "/" being typed
}

// NewAutocomplete creates an Autocomplete backed by the given registry.
func NewAutocomplete(registry *CommandRegistry) *Autocomplete {
	return &Autocomplete{
		registry: registry,
	}
}

// MaxVisible is the maximum number of items shown in the dropdown.
const MaxVisible = 8

// Show activates the dropdown with the given query (text after "/").
func (a *Autocomplete) Show(query string) {
	a.query = query
	a.filtered = a.registry.Match(query)
	a.cursor = 0
	a.visible = len(a.filtered) > 0
}

// Hide closes the dropdown.
func (a *Autocomplete) Hide() {
	a.visible = false
	a.cursor = 0
	a.query = ""
	a.filtered = nil
}

// Visible returns whether the dropdown is currently shown.
func (a *Autocomplete) Visible() bool {
	return a.visible
}

// Next moves the selection cursor forward (wraps around).
func (a *Autocomplete) Next() {
	if len(a.filtered) == 0 {
		return
	}
	a.cursor = (a.cursor + 1) % len(a.filtered)
}

// Prev moves the selection cursor backward (wraps around).
func (a *Autocomplete) Prev() {
	if len(a.filtered) == 0 {
		return
	}
	a.cursor = (a.cursor - 1 + len(a.filtered)) % len(a.filtered)
}

// Selected returns the currently highlighted command, or nil if none.
func (a *Autocomplete) Selected() *SlashCommand {
	if !a.visible || len(a.filtered) == 0 {
		return nil
	}
	return &a.filtered[a.cursor]
}

// Update recalculates the filtered list based on new query text.
func (a *Autocomplete) Update(query string) {
	a.query = query
	a.filtered = a.registry.Match(query)
	if a.cursor >= len(a.filtered) {
		a.cursor = 0
	}
	a.visible = len(a.filtered) > 0
}

// FilteredCount returns the number of matched commands.
func (a *Autocomplete) FilteredCount() int {
	return len(a.filtered)
}

// Render draws the autocomplete dropdown.
func (a *Autocomplete) Render(width int, theme Theme) string {
	if !a.visible || len(a.filtered) == 0 {
		return ""
	}

	// Calculate visible window
	visible := a.filtered
	if len(visible) > MaxVisible {
		// Keep cursor in view
		start := 0
		if a.cursor >= MaxVisible {
			start = a.cursor - MaxVisible + 1
		}
		end := start + MaxVisible
		if end > len(visible) {
			end = len(visible)
			start = end - MaxVisible
		}
		visible = visible[start:end]
	}

	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sb strings.Builder
	for i, cmd := range visible {
		name := "/" + cmd.Name
		desc := cmd.Description

		// Determine actual index in filtered list
		actualIdx := i
		if len(a.filtered) > MaxVisible && a.cursor >= MaxVisible {
			actualIdx = i + (a.cursor - MaxVisible + 1)
		}

		// Truncate description to fit
		maxDesc := innerWidth - len(name) - 4
		if maxDesc > 0 && len(desc) > maxDesc {
			desc = desc[:maxDesc-1] + "…"
		}

		if actualIdx == a.cursor {
			line := theme.AutocompleteSelectedStyle.Render("> " + name)
			if maxDesc > 0 {
				padding := strings.Repeat(" ", maxInline(1, innerWidth-len(name)-len(desc)-2))
				line += theme.AutocompleteDimStyle.Render(padding + desc)
			}
			sb.WriteString(line)
		} else {
			line := theme.AutocompleteStyle.Render("  " + name)
			if maxDesc > 0 {
				padding := strings.Repeat(" ", maxInline(1, innerWidth-len(name)-len(desc)-2))
				line += theme.AutocompleteDimStyle.Render(padding + desc)
			}
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
	}

	content := strings.TrimRight(sb.String(), "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.Config.DimColor)).
		Width(width - 4).
		Render(content)

	return box + "\n"
}

func maxInline(a, b int) int {
	if a > b {
		return a
	}
	return b
}
