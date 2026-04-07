package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
)

// contextCaptureTool captures the ToolContext it receives for inspection.
type contextCaptureTool struct {
	captured *ToolContext
}

func (t *contextCaptureTool) Name() string        { return "Capture" }
func (t *contextCaptureTool) Description() string { return "captures context" }
func (t *contextCaptureTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *contextCaptureTool) Execute(_ context.Context, _ json.RawMessage, tctx *ToolContext) (*models.ToolResult, error) {
	t.captured = tctx
	return &models.ToolResult{Content: "captured"}, nil
}
func (t *contextCaptureTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *contextCaptureTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *contextCaptureTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *contextCaptureTool) IsReadOnly(_ json.RawMessage) bool        { return true }

func TestToolContext_FlowsThroughExecutor(t *testing.T) {
	captureTool := &contextCaptureTool{}

	tctx := &ToolContext{
		Cwd:            "/test/dir",
		Model:          "test-model",
		Tools:          []Tool{captureTool},
		Permissions:    permissions.NewDefaultContext("/test/dir"),
		FileState:      NewFileStateCache(100, 25*1024*1024),
		AbortCtx:       context.Background(),
		GlobMaxResults: 200,
	}

	blocks := []models.Block{
		{Type: models.BlockToolUse, ID: "t1", Name: "Capture", Input: json.RawMessage(`{}`)},
	}

	ExecuteToolBlocks(context.Background(), blocks, tctx.Tools, tctx)

	if captureTool.captured == nil {
		t.Fatal("tool did not receive ToolContext")
	}
	if captureTool.captured.Cwd != "/test/dir" {
		t.Errorf("Cwd = %q, want /test/dir", captureTool.captured.Cwd)
	}
	if captureTool.captured.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", captureTool.captured.Model)
	}
	if captureTool.captured.FileState == nil {
		t.Error("FileState is nil, expected cache instance")
	}
	if captureTool.captured.GlobMaxResults != 200 {
		t.Errorf("GlobMaxResults = %d, want 200", captureTool.captured.GlobMaxResults)
	}
}

func TestToolContext_FileStatePersistsAcrossCalls(t *testing.T) {
	// Simulate Read populating cache, then Edit checking it
	cache := NewFileStateCache(100, 25*1024*1024)

	// Simulate FileReadTool setting cache
	cache.Set("/project/main.go", FileState{
		Content:   "package main\n",
		Timestamp: 1000,
	})

	// Simulate FileEditTool checking cache
	state, ok := cache.Get("/project/main.go")
	if !ok {
		t.Fatal("FileEditTool should find file in cache after Read")
	}
	if state.IsPartialView != nil && *state.IsPartialView {
		t.Error("normal Read should not set IsPartialView")
	}

	// Simulate auto-injected file (CLAUDE.md)
	partial := true
	cache.Set("/project/.claude/CLAUDE.md", FileState{
		Content:       "raw disk bytes",
		Timestamp:     2000,
		IsPartialView: &partial,
	})

	// Edit should reject partial view
	injected, _ := cache.Get("/project/.claude/CLAUDE.md")
	if injected.IsPartialView == nil || !*injected.IsPartialView {
		t.Error("auto-injected file should be marked as partial view")
	}
}
