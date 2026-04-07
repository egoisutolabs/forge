package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/egoisutolabs/forge/internal/provider"
)

// PickerModel is a model entry for the picker dialog.
type PickerModel struct {
	Name          string
	Provider      string
	InputCost     float64
	OutputCost    float64
	ContextWindow int
}

// PickerGroup is a group of models under a header.
type PickerGroup struct {
	Header string
	Models []PickerModel
}

// ModelPickerDialog manages the interactive model picker overlay.
type ModelPickerDialog struct {
	query        string
	allModels    []PickerModel
	filtered     []PickerModel
	recentNames  []string
	currentModel string
	selected     int
}

// maxPickerVisible is the max items visible at once in the list.
const maxPickerVisible = 15

// NewModelPickerDialog creates a picker with the given models and recent list.
func NewModelPickerDialog(models []PickerModel, recent []string, currentModel string) *ModelPickerDialog {
	d := &ModelPickerDialog{
		allModels:    models,
		recentNames:  recent,
		currentModel: currentModel,
	}
	d.filter()
	return d
}

// NewModelPickerFromRegistry creates a picker from a provider.Registry.
func NewModelPickerFromRegistry(reg *provider.Registry, currentModel string) *ModelPickerDialog {
	available := reg.ListAvailable()
	recent := reg.RecentModels()

	var models []PickerModel
	seen := make(map[string]bool)
	for _, ap := range available {
		for _, name := range ap.Models {
			if seen[name] {
				continue
			}
			seen[name] = true
			info, _ := reg.GetModel(name)
			models = append(models, PickerModel{
				Name:          info.Name,
				Provider:      capitalizeProvider(info.Provider),
				InputCost:     info.InputCostPerMTok,
				OutputCost:    info.OutputCostPerMTok,
				ContextWindow: info.ContextWindow,
			})
		}
	}

	return NewModelPickerDialog(models, recent, currentModel)
}

// TypeRune appends a character to the search query and refilters.
func (d *ModelPickerDialog) TypeRune(ch rune) {
	d.query += string(ch)
	d.filter()
}

// Backspace removes the last rune from the query and refilters.
func (d *ModelPickerDialog) Backspace() {
	if len(d.query) == 0 {
		return
	}
	runes := []rune(d.query)
	d.query = string(runes[:len(runes)-1])
	d.filter()
}

// Next moves selection down, wrapping around.
func (d *ModelPickerDialog) Next() {
	total := d.TotalItems()
	if total == 0 {
		return
	}
	d.selected = (d.selected + 1) % total
}

// Prev moves selection up, wrapping around.
func (d *ModelPickerDialog) Prev() {
	total := d.TotalItems()
	if total == 0 {
		return
	}
	d.selected = (d.selected - 1 + total) % total
}

// SelectedModel returns the name of the currently highlighted model.
func (d *ModelPickerDialog) SelectedModel() string {
	items := d.flatItems()
	if d.selected >= len(items) {
		return ""
	}
	return items[d.selected].Name
}

// TotalItems returns the total number of selectable items.
func (d *ModelPickerDialog) TotalItems() int {
	return len(d.flatItems())
}

// Groups returns the models grouped by provider, with recent models first.
func (d *ModelPickerDialog) Groups() []PickerGroup {
	if len(d.filtered) == 0 {
		return nil
	}

	recentSet := make(map[string]bool, len(d.recentNames))
	for _, r := range d.recentNames {
		recentSet[r] = true
	}

	// Build recent group from filtered models that are in the recent list.
	var recentModels []PickerModel
	if d.query == "" {
		// Preserve recent order.
		for _, name := range d.recentNames {
			for _, m := range d.filtered {
				if m.Name == name {
					recentModels = append(recentModels, m)
					break
				}
			}
		}
	}

	// Build provider groups.
	providerOrder := []string{}
	providerMap := make(map[string][]PickerModel)
	for _, m := range d.filtered {
		if _, exists := providerMap[m.Provider]; !exists {
			providerOrder = append(providerOrder, m.Provider)
		}
		providerMap[m.Provider] = append(providerMap[m.Provider], m)
	}

	var groups []PickerGroup
	if len(recentModels) > 0 {
		groups = append(groups, PickerGroup{
			Header: "Recently used",
			Models: recentModels,
		})
	}

	for _, prov := range providerOrder {
		groups = append(groups, PickerGroup{
			Header: prov,
			Models: providerMap[prov],
		})
	}

	return groups
}

// flatItems returns all selectable models in display order (recent first, then by provider).
func (d *ModelPickerDialog) flatItems() []PickerModel {
	var items []PickerModel
	for _, g := range d.Groups() {
		items = append(items, g.Models...)
	}
	return items
}

// filter applies the search query against all models.
func (d *ModelPickerDialog) filter() {
	d.selected = 0
	if d.query == "" {
		d.filtered = make([]PickerModel, len(d.allModels))
		copy(d.filtered, d.allModels)
		return
	}

	q := strings.ToLower(d.query)
	var matched []PickerModel
	for _, m := range d.allModels {
		name := strings.ToLower(m.Name)
		prov := strings.ToLower(m.Provider)
		if strings.Contains(name, q) || strings.Contains(prov, q) {
			matched = append(matched, m)
		}
	}
	d.filtered = matched
}

// Render draws the model picker overlay.
func (d *ModelPickerDialog) Render(width, vpHeight int, theme Theme) string {
	innerWidth := width - 8
	if innerWidth < 40 {
		innerWidth = 40
	}

	var sb strings.Builder

	// Header
	header := theme.HeaderStyle.Render("Model Picker")
	sb.WriteString(header + theme.AutocompleteDimStyle.Render(
		fmt.Sprintf(" (%d models)", len(d.filtered))))
	sb.WriteByte('\n')

	// Search input
	cursor := "█"
	sb.WriteString(theme.AutocompleteSelectedStyle.Render("  Search: " + d.query + cursor))
	sb.WriteByte('\n')
	sb.WriteString(theme.AutocompleteDimStyle.Render(strings.Repeat("─", innerWidth)))
	sb.WriteByte('\n')

	linesUsed := 3
	groups := d.Groups()

	if len(d.filtered) == 0 {
		sb.WriteString(theme.AutocompleteDimStyle.Render("  No models found"))
		sb.WriteByte('\n')
		linesUsed++
	} else {
		// Flatten items for selection tracking
		flatIdx := 0

		maxVis := vpHeight - 7
		if maxVis < 5 {
			maxVis = 5
		}
		if maxVis > maxPickerVisible+10 {
			maxVis = maxPickerVisible + 10
		}

		visibleLines := 0
		for _, g := range groups {
			if visibleLines >= maxVis {
				break
			}
			// Group header
			sb.WriteString(theme.AutocompleteDimStyle.Render("  " + g.Header + ":"))
			sb.WriteByte('\n')
			visibleLines++
			linesUsed++

			for _, m := range g.Models {
				if visibleLines >= maxVis {
					break
				}

				line := d.formatModelLine(m, innerWidth)

				if flatIdx == d.selected {
					sb.WriteString(theme.AutocompleteSelectedStyle.Render("> " + line))
				} else if m.Name == d.currentModel {
					sb.WriteString(theme.AutocompleteSelectedStyle.Render("  " + line + " ●"))
				} else {
					sb.WriteString(theme.AutocompleteStyle.Render("  " + line))
				}
				sb.WriteByte('\n')
				visibleLines++
				linesUsed++
				flatIdx++
			}
		}

		total := d.TotalItems()
		if total > maxVis {
			sb.WriteString(theme.AutocompleteDimStyle.Render("  ↕ ··· more"))
			sb.WriteByte('\n')
			linesUsed++
		}
	}

	// Footer hints
	sb.WriteString(theme.AutocompleteDimStyle.Render(
		"  ↑/↓: navigate · enter: select · esc: close"))
	linesUsed++

	// Pad
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

// formatModelLine formats a single model row with aligned columns.
func (d *ModelPickerDialog) formatModelLine(m PickerModel, width int) string {
	// Format cost
	cost := formatCost(m.InputCost, m.OutputCost)

	// Format context window
	ctx := formatContextWindow(m.ContextWindow)

	// Build the line: name + provider + cost + context
	nameWidth := width - 40
	if nameWidth < 20 {
		nameWidth = 20
	}

	name := m.Name
	if len(name) > nameWidth {
		name = name[:nameWidth-1] + "…"
	}

	return fmt.Sprintf("%-*s  %-12s  %s  %s", nameWidth, name, m.Provider, cost, ctx)
}

// formatCost formats input/output cost as $in/$out.
func formatCost(input, output float64) string {
	if input == 0 && output == 0 {
		return "free       "
	}
	return fmt.Sprintf("$%g/$%g", input, output)
}

// formatContextWindow formats a context window size.
func formatContextWindow(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.0fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	if tokens == 0 {
		return ""
	}
	return fmt.Sprintf("%d", tokens)
}

// capitalizeProvider capitalizes a provider name for display.
func capitalizeProvider(name string) string {
	if name == "" {
		return ""
	}
	switch strings.ToLower(name) {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "deepseek":
		return "DeepSeek"
	case "google":
		return "Google"
	case "mistral":
		return "Mistral"
	case "groq":
		return "Groq"
	case "ollama":
		return "Ollama"
	case "meta":
		return "Meta"
	case "xai":
		return "xAI"
	default:
		return strings.ToUpper(name[:1]) + name[1:]
	}
}

// FindModelByName finds a model by exact or partial name match.
func FindModelByName(models []PickerModel, query string) *PickerModel {
	q := strings.ToLower(query)

	// Exact match first.
	for i, m := range models {
		if strings.ToLower(m.Name) == q {
			return &models[i]
		}
	}

	// Partial/substring match.
	for i, m := range models {
		if strings.Contains(strings.ToLower(m.Name), q) {
			return &models[i]
		}
	}

	return nil
}
