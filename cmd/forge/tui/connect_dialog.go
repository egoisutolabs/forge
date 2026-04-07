package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/egoisutolabs/forge/internal/auth"
	"github.com/egoisutolabs/forge/internal/config"
)

// connectStep tracks the multi-step connect dialog flow.
type connectStep int

const (
	connectStepProvider connectStep = iota
	connectStepCustomID
	connectStepCustomURL
	connectStepKey
	connectStepDone
)

// otherProviderID is the sentinel value for the "Other (custom provider)" option.
const otherProviderID = "__other__"

// ConnectDialog is a multi-step huh-based dialog for connecting a provider.
// Steps: select provider → (if Other) enter ID → enter base URL → enter API key → done.
type ConnectDialog struct {
	step      connectStep
	provider  string
	customID  string
	customURL string
	apiKey    string
	form      *huh.Form
	theme     Theme
	cfg       *config.Config
}

// providerDisplayNames maps provider IDs to human-friendly labels.
var providerDisplayNames = map[string]string{
	"anthropic":  "Anthropic",
	"openai":     "OpenAI",
	"openrouter": "OpenRouter",
	"groq":       "Groq",
	"google":     "Google",
	"mistral":    "Mistral",
	"xai":        "xAI",
	"deepinfra":  "DeepInfra",
}

// displayName returns the human-friendly name for a provider.
func displayName(provider string) string {
	if name, ok := providerDisplayNames[provider]; ok {
		return name
	}
	return provider
}

// NewConnectDialog creates the first step of the connect dialog (provider selection).
func NewConnectDialog(theme Theme, cfg *config.Config) *ConnectDialog {
	cd := &ConnectDialog{
		step:  connectStepProvider,
		theme: theme,
		cfg:   cfg,
	}

	allProviders := auth.AllKnownProviders(cfg)
	opts := make([]huh.Option[string], 0, len(allProviders)+1)
	for _, p := range allProviders {
		label := displayName(p)
		src := auth.GetAuthSource(p, cfg)
		if src != auth.SourceNone {
			label += "  (connected)"
		}
		opts = append(opts, huh.NewOption(label, p))
	}
	// Add "Other" option for custom providers.
	opts = append(opts, huh.NewOption("Other (custom provider)", otherProviderID))

	cd.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a provider to connect").
				Options(opts...).
				Height(len(opts) + 4).
				Value(&cd.provider),
		),
	).WithTheme(connectTheme(theme))

	return cd
}

// advanceToCustomID transitions to the custom provider ID input step.
func (cd *ConnectDialog) advanceToCustomID() {
	cd.step = connectStepCustomID
	cd.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter a unique provider ID").
				Description("This will be used as the provider key in config.json").
				Placeholder("myprovider").
				Value(&cd.customID).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("provider ID cannot be empty")
					}
					if strings.Contains(s, " ") {
						return fmt.Errorf("provider ID cannot contain spaces")
					}
					return nil
				}),
		),
	).WithTheme(connectTheme(cd.theme))
}

// advanceToCustomURL transitions to the base URL input step.
func (cd *ConnectDialog) advanceToCustomURL() {
	cd.step = connectStepCustomURL
	cd.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Enter base URL for %s", cd.customID)).
				Description("OpenAI-compatible API endpoint").
				Placeholder("https://api.example.com/v1").
				Value(&cd.customURL).
				Validate(func(s string) error {
					s = strings.TrimSpace(s)
					if s == "" {
						return fmt.Errorf("base URL cannot be empty")
					}
					if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
						return fmt.Errorf("URL must start with http:// or https://")
					}
					return nil
				}),
		),
	).WithTheme(connectTheme(cd.theme))
}

// advanceToKeyInput transitions to the API key input step.
func (cd *ConnectDialog) advanceToKeyInput() {
	cd.step = connectStepKey
	cd.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Enter API key for %s", displayName(cd.provider))).
				Placeholder("sk-...").
				EchoMode(huh.EchoModePassword).
				Value(&cd.apiKey),
		),
	).WithTheme(connectTheme(cd.theme))
}

// save persists the API key and returns a success/error message.
func (cd *ConnectDialog) save() string {
	key := strings.TrimSpace(cd.apiKey)
	provName := cd.provider
	if provName == otherProviderID {
		provName = cd.customID
	}

	if key == "" && !cd.isCustomProvider() {
		return fmt.Sprintf("\n  Cancelled — no key entered for %s.\n", displayName(provName))
	}

	// For custom providers, save to config.json.
	if cd.isCustomProvider() {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Sprintf("\n  Error: could not determine home directory: %v\n", err)
		}

		if err := config.SaveCustomProvider(home, cd.customID, "", cd.customURL, key); err != nil {
			return fmt.Sprintf("\n  Error saving custom provider: %v\n", err)
		}

		if key != "" {
			if err := auth.SetAPIKey(cd.customID, key); err != nil {
				return fmt.Sprintf("\n  Error saving key: %v\n", err)
			}
		}

		msg := fmt.Sprintf("\n  ✓ Custom provider %q configured\n    Base URL: %s\n    Config saved to ~/.forge/config.json\n", cd.customID, cd.customURL)
		return msg
	}

	// Standard provider — save to auth.json.
	if err := auth.SetAPIKey(provName, key); err != nil {
		return fmt.Sprintf("\n  Error saving key for %s: %v\n", displayName(provName), err)
	}

	return fmt.Sprintf("\n  ✓ Connected to %s\n    API key saved to ~/.forge/auth.json\n", displayName(provName))
}

// isCustomProvider returns true if the user selected the "Other" option.
func (cd *ConnectDialog) isCustomProvider() bool {
	return cd.provider == otherProviderID
}

// updateConnectDialog forwards messages to the active connect dialog and handles step transitions.
func (m AppModel) updateConnectDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Escape cancels at any step
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.connectDialog = nil
		m.addSystemMessage("\n  Connect cancelled.\n")
		return m, nil
	}

	form, cmd := m.connectDialog.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.connectDialog.form = f
	}

	if m.connectDialog.form.State == huh.StateCompleted {
		switch m.connectDialog.step {
		case connectStepProvider:
			if m.connectDialog.isCustomProvider() {
				// Custom provider flow: ID → URL → Key
				m.connectDialog.advanceToCustomID()
			} else {
				// Standard provider flow: Key
				m.connectDialog.advanceToKeyInput()
			}
			initCmd := m.connectDialog.form.Init()
			return m, initCmd

		case connectStepCustomID:
			m.connectDialog.customID = strings.ToLower(strings.TrimSpace(m.connectDialog.customID))
			m.connectDialog.advanceToCustomURL()
			initCmd := m.connectDialog.form.Init()
			return m, initCmd

		case connectStepCustomURL:
			m.connectDialog.customURL = strings.TrimRight(strings.TrimSpace(m.connectDialog.customURL), "/")
			m.connectDialog.advanceToKeyInput()
			initCmd := m.connectDialog.form.Init()
			return m, initCmd

		case connectStepKey:
			// Save and show result
			result := m.connectDialog.save()
			m.connectDialog = nil
			m.suppressInput()
			m.addSystemMessage(result)
			return m, nil
		}
	} else if m.connectDialog.form.State == huh.StateAborted {
		m.connectDialog = nil
		m.suppressInput()
		m.addSystemMessage("\n  Connect cancelled.\n")
		return m, nil
	}

	return m, cmd
}

// connectTheme creates a huh theme for the connect dialog.
func connectTheme(theme Theme) *huh.Theme {
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

	// Style the text input
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	t.Focused.TextInput.Text = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15"))

	return t
}
