package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
)

// deferredTool implements Tool and Deferrable with ShouldDefer() = true.
type deferredTool struct{ name string }

func (d *deferredTool) Name() string                 { return d.name }
func (d *deferredTool) Description() string          { return "deferred: " + d.name }
func (d *deferredTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (d *deferredTool) ShouldDefer() bool            { return true }
func (d *deferredTool) Execute(_ context.Context, _ json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: d.name}, nil
}
func (d *deferredTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (d *deferredTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (d *deferredTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (d *deferredTool) IsReadOnly(_ json.RawMessage) bool        { return true }

// optedOutTool implements Deferrable with ShouldDefer() = false.
type optedOutTool struct{ deferredTool }

func (o *optedOutTool) ShouldDefer() bool { return false }

func TestSplitTools_Empty(t *testing.T) {
	loaded, deferred := SplitTools(nil)
	if len(loaded) != 0 || len(deferred) != 0 {
		t.Errorf("expected empty splits, got loaded=%d deferred=%d", len(loaded), len(deferred))
	}
}

func TestSplitTools_AllLoaded(t *testing.T) {
	all := []Tool{&echoTool{}, &countingTool{}}
	loaded, deferred := SplitTools(all)
	if len(loaded) != 2 {
		t.Errorf("expected 2 loaded, got %d", len(loaded))
	}
	if len(deferred) != 0 {
		t.Errorf("expected 0 deferred, got %d", len(deferred))
	}
}

func TestSplitTools_AllDeferred(t *testing.T) {
	all := []Tool{
		&deferredTool{name: "Alpha"},
		&deferredTool{name: "Beta"},
	}
	loaded, deferred := SplitTools(all)
	if len(loaded) != 0 {
		t.Errorf("expected 0 loaded, got %d", len(loaded))
	}
	if len(deferred) != 2 {
		t.Errorf("expected 2 deferred, got %d", len(deferred))
	}
}

func TestSplitTools_Mixed(t *testing.T) {
	all := []Tool{
		&echoTool{},
		&deferredTool{name: "LazyTool"},
		&countingTool{},
		&deferredTool{name: "AnotherLazy"},
	}
	loaded, deferred := SplitTools(all)
	if len(loaded) != 2 {
		t.Errorf("expected 2 loaded, got %d", len(loaded))
	}
	if len(deferred) != 2 {
		t.Errorf("expected 2 deferred, got %d", len(deferred))
	}
}

func TestSplitTools_OptedOut(t *testing.T) {
	// A tool that implements Deferrable but returns false should be loaded.
	all := []Tool{
		&optedOutTool{deferredTool{name: "OptedOut"}},
		&deferredTool{name: "ActuallyDeferred"},
	}
	loaded, deferred := SplitTools(all)
	if len(loaded) != 1 || loaded[0].Name() != "OptedOut" {
		t.Errorf("expected OptedOut in loaded, got %v", loaded)
	}
	if len(deferred) != 1 || deferred[0].Name() != "ActuallyDeferred" {
		t.Errorf("expected ActuallyDeferred in deferred, got %v", deferred)
	}
}

func TestGenerateSystemReminder_Empty(t *testing.T) {
	var s DeferredToolSet
	if got := s.GenerateSystemReminder(); got != "" {
		t.Errorf("expected empty string for empty set, got %q", got)
	}
}

func TestGenerateSystemReminder_OneItem(t *testing.T) {
	s := DeferredToolSet{&deferredTool{name: "WebFetch"}}
	got := s.GenerateSystemReminder()

	if !strings.Contains(got, "<system-reminder>") {
		t.Error("expected <system-reminder> tag")
	}
	if !strings.Contains(got, "</system-reminder>") {
		t.Error("expected </system-reminder> tag")
	}
	if !strings.Contains(got, "WebFetch") {
		t.Error("expected tool name in reminder")
	}
}

func TestGenerateSystemReminder_MultipleItems(t *testing.T) {
	s := DeferredToolSet{
		&deferredTool{name: "ToolA"},
		&deferredTool{name: "ToolB"},
		&deferredTool{name: "ToolC"},
	}
	got := s.GenerateSystemReminder()

	for _, name := range []string{"ToolA", "ToolB", "ToolC"} {
		if !strings.Contains(got, name) {
			t.Errorf("expected %q in reminder", name)
		}
	}

	// Count should appear
	if !strings.Contains(got, "3") {
		t.Errorf("expected count '3' in reminder: %s", got)
	}
}

func TestGenerateSystemReminder_Format(t *testing.T) {
	s := DeferredToolSet{
		&deferredTool{name: "Alpha"},
		&deferredTool{name: "Beta"},
	}
	got := s.GenerateSystemReminder()

	// Must start with <system-reminder> and end with </system-reminder>
	if !strings.HasPrefix(got, "<system-reminder>") {
		t.Errorf("must start with <system-reminder>, got: %s", got)
	}
	if !strings.HasSuffix(got, "</system-reminder>") {
		t.Errorf("must end with </system-reminder>, got: %s", got)
	}
}
