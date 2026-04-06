package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/egoisutolabs/forge/tools"
)

// AskUserRequestMsg is sent when the AskUser tool needs to present questions
// to the user via the TUI. The engine goroutine blocks on ResponseCh.
type AskUserRequestMsg struct {
	Questions  []tools.AskQuestion
	ResponseCh chan AskUserResponse
}

// AskUserResponse carries answers back to the engine goroutine.
type AskUserResponse struct {
	Answers map[string]string
	Err     error
}

// AskUserForm wraps huh forms for presenting AskUser tool questions.
type AskUserForm struct {
	form       *huh.Form
	questions  []tools.AskQuestion
	responseCh chan AskUserResponse

	// Value targets for single-select questions
	selectValues []string
	// Value targets for multi-select questions
	multiValues [][]string
}

// NewAskUserForm creates a huh form for AskUser tool questions.
// Each question becomes a huh.Select or huh.MultiSelect field.
func NewAskUserForm(questions []tools.AskQuestion, theme Theme) *AskUserForm {
	af := &AskUserForm{
		questions:    questions,
		selectValues: make([]string, len(questions)),
		multiValues:  make([][]string, len(questions)),
	}

	var fields []huh.Field
	for i, q := range questions {
		opts := make([]huh.Option[string], len(q.Options))
		for j, o := range q.Options {
			label := o.Label
			if o.Description != "" {
				label += " — " + o.Description
			}
			opts[j] = huh.NewOption(label, o.Label)
		}

		if q.MultiSelect {
			field := huh.NewMultiSelect[string]().
				Key(q.Question).
				Title(q.Header).
				Description(q.Question).
				Options(opts...).
				Height(len(q.Options) + 2).
				Value(&af.multiValues[i])
			fields = append(fields, field)
		} else {
			field := huh.NewSelect[string]().
				Key(q.Question).
				Title(q.Header).
				Description(q.Question).
				Options(opts...).
				Height(len(q.Options) + 2).
				Value(&af.selectValues[i])
			fields = append(fields, field)
		}
	}

	af.form = huh.NewForm(
		huh.NewGroup(fields...),
	).WithTheme(askUserTheme(theme))

	return af
}

// collectAnswers gathers answers from the form values into a map.
func (af *AskUserForm) collectAnswers() map[string]string {
	answers := make(map[string]string, len(af.questions))
	for i, q := range af.questions {
		if q.MultiSelect {
			answers[q.Question] = strings.Join(af.multiValues[i], ", ")
		} else {
			answers[q.Question] = af.selectValues[i]
		}
	}
	return answers
}

// updateAskForm forwards messages to the huh AskUser form and handles completion.
func (m AppModel) updateAskForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.askForm.responseCh <- AskUserResponse{
			Err: errUserCancelled,
		}
		m.askForm = nil
		return m, nil
	}

	form, cmd := m.askForm.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.askForm.form = f
	}

	if m.askForm.form.State == huh.StateCompleted {
		m.askForm.responseCh <- AskUserResponse{
			Answers: m.askForm.collectAnswers(),
		}
		m.askForm = nil
		m.suppressInput()
	} else if m.askForm.form.State == huh.StateAborted {
		m.askForm.responseCh <- AskUserResponse{
			Err: errUserCancelled,
		}
		m.askForm = nil
		m.suppressInput()
	}

	return m, cmd
}

// askUserTheme creates a huh theme for AskUser forms.
func askUserTheme(theme Theme) *huh.Theme {
	t := huh.ThemeCharm()

	t.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Config.AccentColor))

	t.Focused.Description = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	t.Focused.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Config.AccentColor)).
		Padding(1, 2)

	t.Focused.SelectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AccentColor)).
		Bold(true)

	t.Focused.UnselectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	return t
}

// errUserCancelled is a sentinel error for when the user cancels an AskUser form.
var errUserCancelled = &userCancelledError{}

type userCancelledError struct{}

func (e *userCancelledError) Error() string { return "user cancelled" }
