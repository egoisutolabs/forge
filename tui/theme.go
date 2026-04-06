package tui

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// ThemeConfig defines all color slots used by the TUI.
// Named slots are resolved to lipgloss.Color values and cached as styles.
type ThemeConfig struct {
	Name string `yaml:"name"`

	// Core palette — ANSI 256-color strings (e.g. "12", "#5f87ff")
	UserColor      string `yaml:"user_color"`
	AssistantColor string `yaml:"assistant_color"`
	ToolColor      string `yaml:"tool_color"`
	ErrorColor     string `yaml:"error_color"`
	BorderColor    string `yaml:"border_color"`
	AccentColor    string `yaml:"accent_color"`
	DimColor       string `yaml:"dim_color"`

	// Behavior
	Animations *bool `yaml:"animations"` // nil defaults to true; set false to disable

	// Extended palette
	SuccessColor    string `yaml:"success_color"`
	WarningColor    string `yaml:"warning_color"`
	SpinnerColor    string `yaml:"spinner_color"`
	StatusBgColor   string `yaml:"status_bg_color"`
	StatusTextColor string `yaml:"status_text_color"`
	CodeBgColor     string `yaml:"code_bg_color"`
}

// Theme holds resolved lipgloss styles derived from a ThemeConfig.
type Theme struct {
	Config ThemeConfig

	// Message roles
	UserStyle      lipgloss.Style
	AssistantStyle lipgloss.Style
	ToolStyle      lipgloss.Style
	ToolNameStyle  lipgloss.Style
	ErrorStyle     lipgloss.Style

	// UI chrome
	StatusBarStyle lipgloss.Style
	StatusKeyStyle lipgloss.Style
	SpinnerStyle   lipgloss.Style
	SeparatorStyle lipgloss.Style
	HeaderStyle    lipgloss.Style
	HeaderDimStyle lipgloss.Style

	// Input
	InputBorderStyle  lipgloss.Style
	InputFocusedStyle lipgloss.Style

	// Permission
	PermissionStyle lipgloss.Style

	// Autocomplete
	AutocompleteStyle         lipgloss.Style
	AutocompleteSelectedStyle lipgloss.Style
	AutocompleteDimStyle      lipgloss.Style
}

// DarkTheme returns the default dark theme matching Claude Code aesthetics.
func DarkTheme() ThemeConfig {
	return ThemeConfig{
		Name:            "dark",
		UserColor:       "12",  // bright blue
		AssistantColor:  "15",  // bright white
		ToolColor:       "8",   // dark gray
		ErrorColor:      "9",   // bright red
		BorderColor:     "8",   // dark gray
		AccentColor:     "12",  // bright blue
		DimColor:        "8",   // dark gray
		SuccessColor:    "10",  // bright green
		WarningColor:    "11",  // bright yellow
		SpinnerColor:    "5",   // magenta
		StatusBgColor:   "0",   // black
		StatusTextColor: "8",   // dim gray
		CodeBgColor:     "235", // dark gray
	}
}

// LightTheme returns a light theme variant.
func LightTheme() ThemeConfig {
	return ThemeConfig{
		Name:            "light",
		UserColor:       "4",  // blue
		AssistantColor:  "0",  // black
		ToolColor:       "8",  // gray
		ErrorColor:      "1",  // red
		BorderColor:     "7",  // light gray
		AccentColor:     "4",  // blue
		DimColor:        "7",  // light gray
		SuccessColor:    "2",  // green
		WarningColor:    "3",  // yellow
		SpinnerColor:    "5",  // magenta
		StatusBgColor:   "7",  // light gray
		StatusTextColor: "8",  // gray
		CodeBgColor:     "15", // white
	}
}

// MinimalTheme returns a stripped-down theme with fewer colors.
func MinimalTheme() ThemeConfig {
	return ThemeConfig{
		Name:            "minimal",
		UserColor:       "15", // white
		AssistantColor:  "15", // white
		ToolColor:       "8",  // gray
		ErrorColor:      "9",  // red
		BorderColor:     "8",  // gray
		AccentColor:     "15", // white
		DimColor:        "8",  // gray
		SuccessColor:    "15", // white
		WarningColor:    "15", // white
		SpinnerColor:    "15", // white
		StatusBgColor:   "0",  // black
		StatusTextColor: "8",  // gray
		CodeBgColor:     "0",  // black
	}
}

// ResolveTheme converts a ThemeConfig into a ready-to-use Theme with lipgloss styles.
func ResolveTheme(cfg ThemeConfig) Theme {
	return Theme{
		Config: cfg,

		UserStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.UserColor)).
			Bold(true),
		AssistantStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.AssistantColor)),
		ToolStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.ToolColor)),
		ToolNameStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.WarningColor)).
			Bold(true),
		ErrorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.ErrorColor)),

		StatusBarStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.StatusTextColor)).
			Faint(true),
		StatusKeyStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.AccentColor)),
		SpinnerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.SpinnerColor)),
		SeparatorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.DimColor)).
			Faint(true),
		HeaderStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.AssistantColor)).
			Bold(true),
		HeaderDimStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.DimColor)).
			Faint(true),

		InputBorderStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(cfg.BorderColor)).
			Padding(0, 1),
		InputFocusedStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(cfg.AccentColor)).
			Padding(0, 1),

		PermissionStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.WarningColor)).
			Background(lipgloss.Color(cfg.StatusBgColor)).
			Bold(true).
			Padding(0, 1),

		AutocompleteStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.AssistantColor)),
		AutocompleteSelectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.AccentColor)).
			Bold(true),
		AutocompleteDimStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(cfg.DimColor)).
			Faint(true),
	}
}

// LoadThemeFromFile loads a ThemeConfig from a YAML file.
// Returns the parsed config and nil error on success.
func LoadThemeFromFile(path string) (ThemeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ThemeConfig{}, err
	}
	var cfg ThemeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ThemeConfig{}, err
	}
	return cfg, nil
}

// DefaultThemePath returns ~/.forge/theme.yaml.
func DefaultThemePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".forge", "theme.yaml")
}

// InitTheme selects and resolves the active theme based on environment.
// Priority: FORGE_THEME env → ~/.forge/theme.yaml → dark default.
func InitTheme() Theme {
	// Check NO_COLOR for accessibility
	if os.Getenv("NO_COLOR") != "" {
		return ResolveTheme(MinimalTheme())
	}

	// Check FORGE_THEME env var
	switch os.Getenv("FORGE_THEME") {
	case "light":
		return ResolveTheme(LightTheme())
	case "minimal":
		return ResolveTheme(MinimalTheme())
	case "dark":
		return ResolveTheme(DarkTheme())
	}

	// Try loading custom theme from file
	if path := DefaultThemePath(); path != "" {
		if cfg, err := LoadThemeFromFile(path); err == nil && cfg.Name != "" {
			// Fill in missing colors from dark theme defaults
			cfg = mergeWithDefaults(cfg, DarkTheme())
			return ResolveTheme(cfg)
		}
	}

	return ResolveTheme(DarkTheme())
}

// mergeWithDefaults fills empty fields in cfg with values from defaults.
func mergeWithDefaults(cfg, defaults ThemeConfig) ThemeConfig {
	if cfg.UserColor == "" {
		cfg.UserColor = defaults.UserColor
	}
	if cfg.AssistantColor == "" {
		cfg.AssistantColor = defaults.AssistantColor
	}
	if cfg.ToolColor == "" {
		cfg.ToolColor = defaults.ToolColor
	}
	if cfg.ErrorColor == "" {
		cfg.ErrorColor = defaults.ErrorColor
	}
	if cfg.BorderColor == "" {
		cfg.BorderColor = defaults.BorderColor
	}
	if cfg.AccentColor == "" {
		cfg.AccentColor = defaults.AccentColor
	}
	if cfg.DimColor == "" {
		cfg.DimColor = defaults.DimColor
	}
	if cfg.SuccessColor == "" {
		cfg.SuccessColor = defaults.SuccessColor
	}
	if cfg.WarningColor == "" {
		cfg.WarningColor = defaults.WarningColor
	}
	if cfg.SpinnerColor == "" {
		cfg.SpinnerColor = defaults.SpinnerColor
	}
	if cfg.StatusBgColor == "" {
		cfg.StatusBgColor = defaults.StatusBgColor
	}
	if cfg.StatusTextColor == "" {
		cfg.StatusTextColor = defaults.StatusTextColor
	}
	if cfg.CodeBgColor == "" {
		cfg.CodeBgColor = defaults.CodeBgColor
	}
	return cfg
}
