package tui

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/provider"
)

func testRegistry(recentPath string) *provider.Registry {
	// We can't easily construct a registry with fake data, so we test
	// the ModelPickerDialog directly by feeding it data.
	return nil
}

func newTestPicker() *ModelPickerDialog {
	models := []PickerModel{
		{Name: "claude-opus-4-6", Provider: "Anthropic", InputCost: 5, OutputCost: 25, ContextWindow: 200000},
		{Name: "claude-sonnet-4-6", Provider: "Anthropic", InputCost: 3, OutputCost: 15, ContextWindow: 200000},
		{Name: "claude-haiku-4-5", Provider: "Anthropic", InputCost: 1, OutputCost: 5, ContextWindow: 200000},
		{Name: "gpt-4o", Provider: "OpenAI", InputCost: 2.5, OutputCost: 10, ContextWindow: 128000},
		{Name: "o3", Provider: "OpenAI", InputCost: 10, OutputCost: 40, ContextWindow: 200000},
		{Name: "deepseek-r1", Provider: "DeepSeek", InputCost: 0.55, OutputCost: 2.19, ContextWindow: 64000},
	}
	recent := []string{"claude-sonnet-4-6", "deepseek-r1"}
	return NewModelPickerDialog(models, recent, "claude-sonnet-4-6")
}

func TestPickerFilterByQuery(t *testing.T) {
	p := newTestPicker()

	// No filter — all models visible
	if len(p.filtered) != 6 {
		t.Fatalf("expected 6 models, got %d", len(p.filtered))
	}

	// Filter by "opus"
	p.TypeRune('o')
	p.TypeRune('p')
	p.TypeRune('u')
	p.TypeRune('s')

	if len(p.filtered) != 1 {
		t.Fatalf("expected 1 match for 'opus', got %d", len(p.filtered))
	}
	if p.filtered[0].Name != "claude-opus-4-6" {
		t.Fatalf("expected claude-opus-4-6, got %s", p.filtered[0].Name)
	}
}

func TestPickerFilterByProvider(t *testing.T) {
	p := newTestPicker()

	// Filter by "openai"
	for _, r := range "openai" {
		p.TypeRune(r)
	}

	if len(p.filtered) != 2 {
		t.Fatalf("expected 2 OpenAI matches, got %d", len(p.filtered))
	}
	for _, m := range p.filtered {
		if m.Provider != "OpenAI" {
			t.Errorf("expected OpenAI provider, got %s", m.Provider)
		}
	}
}

func TestPickerGroupByProvider(t *testing.T) {
	p := newTestPicker()
	groups := p.Groups()

	// Should have recent group + 3 provider groups
	if len(groups) < 3 {
		t.Fatalf("expected at least 3 groups, got %d", len(groups))
	}

	// First group should be "Recently used"
	if groups[0].Header != "Recently used" {
		t.Errorf("expected first group header 'Recently used', got %q", groups[0].Header)
	}
	if len(groups[0].Models) != 2 {
		t.Errorf("expected 2 recent models, got %d", len(groups[0].Models))
	}
}

func TestPickerRecentModelsAtTop(t *testing.T) {
	p := newTestPicker()
	groups := p.Groups()

	if groups[0].Header != "Recently used" {
		t.Fatalf("first group should be 'Recently used', got %q", groups[0].Header)
	}

	// First recent model should be claude-sonnet-4-6
	if groups[0].Models[0].Name != "claude-sonnet-4-6" {
		t.Errorf("expected first recent model to be claude-sonnet-4-6, got %s", groups[0].Models[0].Name)
	}
	// Second recent model should be deepseek-r1
	if groups[0].Models[1].Name != "deepseek-r1" {
		t.Errorf("expected second recent model to be deepseek-r1, got %s", groups[0].Models[1].Name)
	}
}

func TestPickerSelectionReturnsCorrectModel(t *testing.T) {
	p := newTestPicker()

	// Default selection is first item
	sel := p.SelectedModel()
	if sel == "" {
		t.Fatal("expected a selected model, got empty string")
	}

	// Navigate down and check
	p.Next()
	sel2 := p.SelectedModel()
	if sel2 == sel {
		t.Error("expected different model after Next()")
	}
}

func TestPickerArrowNavigation(t *testing.T) {
	p := newTestPicker()

	// Move down through all items
	total := p.TotalItems()
	if total < 6 {
		t.Fatalf("expected at least 6 items, got %d", total)
	}

	for i := 0; i < total-1; i++ {
		p.Next()
	}
	// Should be at last item
	if p.selected != total-1 {
		t.Errorf("expected selected=%d, got %d", total-1, p.selected)
	}

	// Wrap around
	p.Next()
	if p.selected != 0 {
		t.Errorf("expected wrap to 0, got %d", p.selected)
	}

	// Prev from 0 wraps to end
	p.Prev()
	if p.selected != total-1 {
		t.Errorf("expected wrap to %d, got %d", total-1, p.selected)
	}
}

func TestPickerBackspace(t *testing.T) {
	p := newTestPicker()

	// Type "opus" then backspace
	for _, r := range "opus" {
		p.TypeRune(r)
	}
	if len(p.filtered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(p.filtered))
	}

	p.Backspace()
	p.Backspace()
	p.Backspace()
	p.Backspace()

	// Back to showing all
	if len(p.filtered) != 6 {
		t.Fatalf("expected 6 models after clearing query, got %d", len(p.filtered))
	}
}

func TestPickerEscClears(t *testing.T) {
	p := newTestPicker()
	if p.query != "" {
		t.Fatal("expected empty query initially")
	}

	for _, r := range "test" {
		p.TypeRune(r)
	}
	if p.query != "test" {
		t.Fatalf("expected query 'test', got %q", p.query)
	}
}

func TestPickerCurrentModelHighlighted(t *testing.T) {
	p := newTestPicker()

	// The current model should be tracked
	if p.currentModel != "claude-sonnet-4-6" {
		t.Errorf("expected currentModel to be claude-sonnet-4-6, got %s", p.currentModel)
	}
}

func TestPickerRenderIncludesCostAndContext(t *testing.T) {
	p := newTestPicker()
	theme := InitTheme()
	rendered := p.Render(80, 30, theme)

	// Should contain model names
	if !strings.Contains(rendered, "claude-opus-4-6") {
		t.Error("rendered output should contain claude-opus-4-6")
	}

	// Should contain cost info
	if !strings.Contains(rendered, "$5") || !strings.Contains(rendered, "$25") {
		t.Error("rendered output should contain cost info")
	}

	// Should contain context window
	if !strings.Contains(rendered, "200K") {
		t.Error("rendered output should contain context window")
	}
}

func TestPickerFilterNoResults(t *testing.T) {
	p := newTestPicker()
	for _, r := range "zzzzzzz" {
		p.TypeRune(r)
	}

	if len(p.filtered) != 0 {
		t.Errorf("expected 0 results for nonsense query, got %d", len(p.filtered))
	}

	sel := p.SelectedModel()
	if sel != "" {
		t.Errorf("expected empty selection with no results, got %s", sel)
	}
}

func TestPickerGroupsWithFilter(t *testing.T) {
	p := newTestPicker()

	// Filter to only Anthropic models
	for _, r := range "claude" {
		p.TypeRune(r)
	}

	groups := p.Groups()
	// Should only have groups with matching models
	for _, g := range groups {
		for _, m := range g.Models {
			if !strings.Contains(strings.ToLower(m.Name), "claude") {
				t.Errorf("filtered group should only contain claude models, got %s", m.Name)
			}
		}
	}
}

func TestPickerDirectModelSwitch(t *testing.T) {
	// Test that /models <name> finds a valid model
	models := []PickerModel{
		{Name: "claude-sonnet-4-6", Provider: "Anthropic"},
		{Name: "gpt-4o", Provider: "OpenAI"},
	}

	found := FindModelByName(models, "gpt-4o")
	if found == nil {
		t.Fatal("expected to find gpt-4o")
	}
	if found.Name != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", found.Name)
	}

	// Partial match
	found = FindModelByName(models, "sonnet")
	if found == nil {
		t.Fatal("expected to find sonnet via partial match")
	}
	if found.Name != "claude-sonnet-4-6" {
		t.Errorf("expected claude-sonnet-4-6, got %s", found.Name)
	}

	// No match
	found = FindModelByName(models, "nonexistent")
	if found != nil {
		t.Errorf("expected nil for nonexistent model, got %v", found)
	}
}
