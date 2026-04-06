package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Message role styles
	UserStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")). // bright blue
			Bold(true)

	AssistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")) // bright white

	ToolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // dark gray

	ToolNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // bright yellow
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")) // bright red

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Faint(true)

	StatusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")) // cyan

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")) // magenta

	// Input area
	InputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("8")).
				Padding(0, 1)

	InputFocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("12")).
				Padding(0, 1)

	// Permission prompt
	PermissionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Background(lipgloss.Color("0")).
			Bold(true).
			Padding(0, 1)

	// Separator
	SeparatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Faint(true)

	// Diff styles
	DiffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")) // green

	DiffRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")) // red

	DiffContextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")). // gray
				Faint(true)

	DiffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")). // white
			Bold(true)

	DiffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")) // cyan

	// Tool result styles
	ToolIconSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")) // green

	ToolIconErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")) // red

	ToolBorderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Faint(true)

	// Code block background
	CodeBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235"))
)
