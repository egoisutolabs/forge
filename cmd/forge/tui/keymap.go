package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the key bindings for the TUI.
type KeyMap struct {
	Submit      key.Binding
	Quit        key.Binding
	Cancel      key.Binding
	ScrollUp    key.Binding
	ScrollDn    key.Binding
	PageUp      key.Binding
	PageDn      key.Binding
	YesPerms    key.Binding
	NoPerms     key.Binding
	HistoryUp   key.Binding
	HistoryDn   key.Binding
	TabComplete key.Binding
	RevComplete key.Binding
	ShowHelp    key.Binding
}

// DefaultKeyMap returns the standard key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send message"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "scroll up"),
		),
		ScrollDn: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDn: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		YesPerms: key.NewBinding(
			key.WithKeys("y", "Y"),
			key.WithHelp("y", "approve"),
		),
		NoPerms: key.NewBinding(
			key.WithKeys("n", "N"),
			key.WithHelp("n", "deny"),
		),
		HistoryUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "previous history"),
		),
		HistoryDn: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "next history"),
		),
		TabComplete: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "autocomplete"),
		),
		RevComplete: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "autocomplete prev"),
		),
		ShowHelp: key.NewBinding(
			key.WithKeys("ctrl+/"),
			key.WithHelp("ctrl+/", "toggle shortcuts"),
		),
	}
}

// ShortcutHelp returns the list of shortcuts for the help overlay.
func ShortcutHelp() []struct{ Key, Action string } {
	return []struct{ Key, Action string }{
		{"Enter", "Send message"},
		{"Alt+Enter", "Insert newline"},
		{"Ctrl+C", "Cancel / Quit"},
		{"Esc", "Cancel current operation"},
		{"↑ / ↓", "History (when input empty)"},
		{"PgUp / PgDn", "Scroll conversation"},
		{"/", "Slash commands"},
		{"Tab", "Autocomplete command"},
		{"Ctrl+A / Ctrl+E", "Start / End of line"},
		{"Ctrl+K", "Kill to end of line"},
		{"Ctrl+U", "Kill entire line"},
		{"Ctrl+W", "Kill word backward"},
		{"Ctrl+Y", "Yank (paste killed text)"},
		{"Ctrl+Z", "Restore stashed draft"},
		{"Ctrl+D", "Open diff viewer"},
		{"Ctrl+M", "Model picker"},
		{"Ctrl+T", "Toggle agents panel"},
		{"Ctrl+/", "Toggle this help"},
	}
}
