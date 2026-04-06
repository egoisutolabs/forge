package tui

import tea "github.com/charmbracelet/bubbletea"

// updateQuickOpen handles keyboard input when the quick open dialog is active.
// The dialog owns all keyboard input (modal).
func (m AppModel) updateQuickOpen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.quickOpen = nil
		return m, nil

	case msg.String() == "enter":
		if path := m.quickOpen.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + "@" + path + " ")
		}
		m.quickOpen = nil
		return m, nil

	case msg.String() == "tab":
		if path := m.quickOpen.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + "@" + path + " ")
		}
		m.quickOpen = nil
		return m, nil

	case msg.String() == "shift+tab":
		if path := m.quickOpen.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + path + " ")
		}
		m.quickOpen = nil
		return m, nil

	case msg.String() == "up":
		m.quickOpen.Prev()
		return m, nil

	case msg.String() == "down":
		m.quickOpen.Next()
		return m, nil

	case msg.String() == "backspace":
		m.quickOpen.Backspace()
		return m, nil

	case msg.Type == tea.KeyRunes:
		for _, r := range msg.Runes {
			m.quickOpen.TypeRune(r)
		}
		return m, nil
	}

	return m, nil
}

// updateGlobalSearch handles keyboard input when the global search dialog is active.
func (m AppModel) updateGlobalSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.globalSearch.Close()
		m.globalSearch = nil
		return m, nil

	case msg.String() == "enter":
		if path := m.globalSearch.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + "@" + path + " ")
		}
		m.globalSearch.Close()
		m.globalSearch = nil
		return m, nil

	case msg.String() == "tab":
		if path := m.globalSearch.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + "@" + path + " ")
		}
		m.globalSearch.Close()
		m.globalSearch = nil
		return m, nil

	case msg.String() == "shift+tab":
		if path := m.globalSearch.SelectedPath(); path != "" {
			m.input.SetValue(m.input.Value() + path + " ")
		}
		m.globalSearch.Close()
		m.globalSearch = nil
		return m, nil

	case msg.String() == "up":
		m.globalSearch.Prev()
		return m, nil

	case msg.String() == "down":
		m.globalSearch.Next()
		return m, nil

	case msg.String() == "backspace":
		return m, m.globalSearch.Backspace()

	case msg.Type == tea.KeyRunes:
		var cmd tea.Cmd
		for _, r := range msg.Runes {
			cmd = m.globalSearch.TypeRune(r)
		}
		return m, cmd
	}

	return m, nil
}
