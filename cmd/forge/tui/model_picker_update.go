package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/egoisutolabs/forge/internal/provider"
)

// ModelPickerSelectedMsg is sent when the user selects a model from the picker.
type ModelPickerSelectedMsg struct {
	Model string
}

// updateModelPicker handles keyboard input when the model picker is active.
func (m AppModel) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "esc":
		m.modelPicker = nil
		return m, nil

	case msg.String() == "enter":
		if sel := m.modelPicker.SelectedModel(); sel != "" {
			m.modelPicker = nil
			return m, m.selectModel(sel)
		}
		m.modelPicker = nil
		return m, nil

	case msg.String() == "up":
		m.modelPicker.Prev()
		return m, nil

	case msg.String() == "down":
		m.modelPicker.Next()
		return m, nil

	case msg.String() == "backspace":
		m.modelPicker.Backspace()
		return m, nil

	case msg.Type == tea.KeyRunes:
		for _, r := range msg.Runes {
			m.modelPicker.TypeRune(r)
		}
		return m, nil
	}

	return m, nil
}

// selectModel switches the active model, updates status, and records usage.
func (m *AppModel) selectModel(name string) tea.Cmd {
	m.bridge.SetModel(name)
	m.status.Model = name
	m.addSystemMessage(fmt.Sprintf("\n  Model switched to: %s\n", name))

	// Record usage in background.
	return func() tea.Msg {
		_ = provider.RecordUsage(name)
		return nil
	}
}
