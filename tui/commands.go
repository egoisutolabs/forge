package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/egoisutolabs/forge/auth"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/provider"
)

// handleSlashCommand intercepts slash commands for immediate TUI-level execution.
// Returns (cmd, true) if the command was handled, (nil, false) to fall through
// to the engine (for commands registered only as skills).
func (m *AppModel) handleSlashCommand(name, args string) (tea.Cmd, bool) {
	switch name {
	case "help", "h", "?":
		return m.cmdHelp(), true
	case "cost":
		return m.cmdCost(), true
	case "clear", "cls":
		return m.cmdClear(), true
	case "model":
		return m.cmdModel(args), true
	case "models":
		return m.cmdModels(args), true
	case "compact":
		return m.cmdCompact(), true
	case "connect":
		return m.cmdConnect(args), true
	case "providers":
		return m.cmdProviders(), true
	case "history":
		return m.cmdHistory(), true
	case "quit", "exit", "q":
		return tea.Quit, true
	default:
		return nil, false
	}
}

func (m *AppModel) addSystemMessage(content string) {
	m.messages = append(m.messages, DisplayMessage{
		Role:    "system",
		Content: content,
	})
	m.trackUnseenMessage()
	m.refreshViewport()
}

func (m *AppModel) cmdHelp() tea.Cmd {
	help := strings.Join([]string{
		"",
		"  Commands",
		"  /help        Show this help",
		"  /cost        Session cost & token usage",
		"  /model       Show or switch model",
		"  /models      Browse and switch models (Ctrl+M)",
		"  /clear       Clear conversation",
		"  /compact     Compact conversation context",
		"  /connect     Connect an API provider",
		"  /providers   Show API provider status",
		"  /history     Show input history",
		"  /commit      Create a git commit",
		"  /review      Review code changes",
		"  /forge       Run the forge pipeline",
		"  /quit        Exit Forge",
		"",
		"  Keyboard Shortcuts",
		"  Enter        Send message",
		"  Ctrl+C       Cancel / quit",
		"  Esc          Cancel current request",
		"  Up/Down      Browse message history",
		"  Tab          Cycle autocomplete / collapse tool results",
		"  /            Slash commands",
		"  @            Mention files, skills, agents",
		"",
	}, "\n")
	m.addSystemMessage(help)
	return nil
}

func (m *AppModel) cmdCost() tea.Cmd {
	usage := m.bridge.TotalUsage()
	model := m.bridge.Model()
	cost := models.CostForModel(model, usage)

	msg := fmt.Sprintf(
		"\n  Session cost: $%.4f\n  Input: %s tokens | Output: %s tokens\n  Cache read: %s tokens | Cache create: %s tokens\n",
		cost,
		formatTokens(usage.InputTokens),
		formatTokens(usage.OutputTokens),
		formatTokens(usage.CacheRead),
		formatTokens(usage.CacheCreate),
	)
	m.addSystemMessage(msg)
	return nil
}

func (m *AppModel) cmdClear() tea.Cmd {
	m.messages = nil
	m.refreshViewport()
	return nil
}

func (m *AppModel) cmdModel(args string) tea.Cmd {
	args = strings.TrimSpace(args)
	if args == "" {
		m.addSystemMessage(fmt.Sprintf("\n  Current model: %s\n", m.bridge.Model()))
		return nil
	}
	m.bridge.SetModel(args)
	m.status.Model = args
	m.addSystemMessage(fmt.Sprintf("\n  Model switched to: %s\n", args))
	return nil
}

func (m *AppModel) cmdModels(args string) tea.Cmd {
	args = strings.TrimSpace(args)

	if m.registry == nil {
		m.addSystemMessage("\n  No model registry available.\n")
		return nil
	}

	// /models <name> → direct switch without picker
	if args != "" {
		available := m.registry.ListAvailable()
		var allModels []PickerModel
		seen := make(map[string]bool)
		for _, ap := range available {
			for _, name := range ap.Models {
				if seen[name] {
					continue
				}
				seen[name] = true
				info, _ := m.registry.GetModel(name)
				allModels = append(allModels, PickerModel{
					Name:     info.Name,
					Provider: capitalizeProvider(info.Provider),
				})
			}
		}

		found := FindModelByName(allModels, args)
		if found == nil {
			m.addSystemMessage(fmt.Sprintf("\n  Model %q not found.\n", args))
			return nil
		}
		m.bridge.SetModel(found.Name)
		m.status.Model = found.Name
		m.addSystemMessage(fmt.Sprintf("\n  Model switched to: %s\n", found.Name))
		return func() tea.Msg {
			_ = provider.RecordUsage(found.Name)
			return nil
		}
	}

	// /models → open picker
	m.modelPicker = NewModelPickerFromRegistry(m.registry, m.bridge.Model())
	return nil
}

func (m *AppModel) cmdHistory() tea.Cmd {
	entries := m.history.Entries()
	if len(entries) == 0 {
		m.addSystemMessage("\n  No input history.\n")
		return nil
	}
	var sb strings.Builder
	sb.WriteString("\n  Input History\n")
	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, e))
	}
	m.addSystemMessage(sb.String())
	return nil
}

func (m *AppModel) cmdConnect(args string) tea.Cmd {
	args = strings.TrimSpace(args)
	allProviders := auth.AllKnownProviders(m.cfg)
	knownSet := make(map[string]bool, len(allProviders))
	for _, p := range allProviders {
		knownSet[p] = true
	}

	if args == "" {
		// Launch interactive connect dialog
		cd := NewConnectDialog(m.theme, m.cfg)
		m.connectDialog = cd
		return cd.form.Init()
	}

	parts := strings.SplitN(args, " ", 2)
	providerName := strings.ToLower(strings.TrimSpace(parts[0]))

	if !knownSet[providerName] {
		m.addSystemMessage(fmt.Sprintf("\n  Unknown provider %q. Run /connect to see available providers.\n", providerName))
		return nil
	}

	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		m.addSystemMessage(fmt.Sprintf("\n  Usage: /connect %s <api-key>\n", providerName))
		return nil
	}

	apiKey := strings.TrimSpace(parts[1])
	if err := auth.SetAPIKey(providerName, apiKey); err != nil {
		m.addSystemMessage(fmt.Sprintf("\n  Error saving key for %s: %v\n", providerName, err))
		return nil
	}

	m.addSystemMessage(fmt.Sprintf("\n  ✓ Connected %s — API key saved to auth.json\n", providerName))
	return nil
}

func (m *AppModel) cmdProviders() tea.Cmd {
	allProviders := auth.AllKnownProviders(m.cfg)

	var sb strings.Builder
	sb.WriteString("\n  Providers\n")
	for _, p := range allProviders {
		src := auth.GetAuthSource(p, m.cfg)
		switch src {
		case auth.SourceAuthFile:
			sb.WriteString(fmt.Sprintf("    ✓ %-12s — connected (auth.json)\n", p))
		case auth.SourceConfig:
			sb.WriteString(fmt.Sprintf("    ✓ %-12s — connected (config)\n", p))
		case auth.SourceEnvVar:
			envVar, _ := auth.EnvVarForProvider(p)
			sb.WriteString(fmt.Sprintf("    ✓ %-12s — connected (env: %s)\n", p, envVar))
		default:
			sb.WriteString(fmt.Sprintf("    ✗ %-12s — not configured\n", p))
		}
	}
	m.addSystemMessage(sb.String())
	return nil
}

func (m *AppModel) cmdCompact() tea.Cmd {
	m.addSystemMessage("  Compacting conversation...")
	m.processing = true
	m.status.Processing = true
	bridge := m.bridge
	return func() tea.Msg {
		err := bridge.Compact(context.Background())
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return CompactDoneMsg{}
	}
}

// CompactDoneMsg signals that conversation compaction is complete.
type CompactDoneMsg struct{}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
