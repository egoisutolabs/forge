package skill

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/skills"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

func tctxWithSkill(s *skills.Skill) *tools.ToolContext {
	r := skills.NewRegistry()
	if s != nil {
		r.Register(s)
	}
	return &tools.ToolContext{Skills: r}
}

func basicSkill(name string) *skills.Skill {
	return &skills.Skill{
		Name:        name,
		Description: "test skill",
		Prompt:      func(args string) string { return "prompt for " + name + " args=" + args },
	}
}

// ─── interface compliance ─────────────────────────────────────────────────────

func TestSkillTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestSkillTool_Name(t *testing.T) {
	if got := (&Tool{}).Name(); got != "Skill" {
		t.Errorf("Name() = %q, want %q", got, "Skill")
	}
}

func TestSkillTool_IsConcurrencySafe(t *testing.T) {
	if (&Tool{}).IsConcurrencySafe(nil) {
		t.Error("SkillTool should NOT be concurrency-safe")
	}
}

func TestSkillTool_IsReadOnly(t *testing.T) {
	if (&Tool{}).IsReadOnly(nil) {
		t.Error("SkillTool should NOT be read-only")
	}
}

// ─── ValidateInput ────────────────────────────────────────────────────────────

func TestValidateInput_MissingSkill(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{})); err == nil {
		t.Error("expected error for missing skill field")
	}
}

func TestValidateInput_EmptySkill(t *testing.T) {
	in := mustJSON(t, map[string]any{"skill": ""})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for empty skill name")
	}
}

func TestValidateInput_SlashOnly(t *testing.T) {
	in := mustJSON(t, map[string]any{"skill": "/"})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for slash-only skill name")
	}
}

func TestValidateInput_ValidName(t *testing.T) {
	in := mustJSON(t, map[string]any{"skill": "commit"})
	if err := (&Tool{}).ValidateInput(in); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInput_ValidWithLeadingSlash(t *testing.T) {
	in := mustJSON(t, map[string]any{"skill": "/commit"})
	if err := (&Tool{}).ValidateInput(in); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── CheckPermissions ─────────────────────────────────────────────────────────

func TestCheckPermissions_NilRegistry_Allows(t *testing.T) {
	in := mustJSON(t, map[string]any{"skill": "commit"})
	dec, err := (&Tool{}).CheckPermissions(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Errorf("expected PermAllow when registry is nil, got %q", dec.Behavior)
	}
}

func TestCheckPermissions_UnknownSkill_Denies(t *testing.T) {
	tctx := tctxWithSkill(nil) // empty registry
	in := mustJSON(t, map[string]any{"skill": "unknown-skill"})
	dec, err := (&Tool{}).CheckPermissions(in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermDeny {
		t.Errorf("expected PermDeny for unknown skill, got %q", dec.Behavior)
	}
}

func TestCheckPermissions_SafeSkill_Allows(t *testing.T) {
	s := &skills.Skill{
		Name:         "safe",
		AllowedTools: []string{"Read", "Glob"},
		Prompt:       func(string) string { return "prompt" },
	}
	tctx := tctxWithSkill(s)
	in := mustJSON(t, map[string]any{"skill": "safe"})
	dec, err := (&Tool{}).CheckPermissions(in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Errorf("expected PermAllow for safe skill, got %q", dec.Behavior)
	}
}

func TestCheckPermissions_UnsafeSkill_Asks(t *testing.T) {
	s := &skills.Skill{
		Name:   "unsafe",
		Prompt: func(string) string { return "prompt" },
		// No AllowedTools restriction → potentially write ops
	}
	tctx := tctxWithSkill(s)
	in := mustJSON(t, map[string]any{"skill": "unsafe"})
	dec, _ := (&Tool{}).CheckPermissions(in, tctx)
	if dec.Behavior != models.PermAsk {
		t.Errorf("expected PermAsk for unrestricted skill, got %q", dec.Behavior)
	}
}

func TestCheckPermissions_LeadingSlashStripped(t *testing.T) {
	tctx := tctxWithSkill(basicSkill("commit"))
	in := mustJSON(t, map[string]any{"skill": "/commit"})
	dec, _ := (&Tool{}).CheckPermissions(in, tctx)
	// should not deny (would deny if slash wasn't stripped)
	if dec.Behavior == models.PermDeny {
		t.Error("leading slash should be stripped before registry lookup")
	}
}

// ─── Execute ─────────────────────────────────────────────────────────────────

func TestExecute_NilRegistry(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{"skill": "commit"}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when registry is nil")
	}
}

func TestExecute_UnknownSkill(t *testing.T) {
	tctx := tctxWithSkill(nil)
	result, _ := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{"skill": "nope"}), tctx)
	if !result.IsError {
		t.Error("expected IsError=true for unknown skill")
	}
}

func TestExecute_Success(t *testing.T) {
	tctx := tctxWithSkill(basicSkill("commit"))
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{"skill": "commit"}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if out["success"] != true {
		t.Error("expected success=true")
	}
	if out["status"] != "inline" {
		t.Errorf("status = %v, want inline", out["status"])
	}
	if out["commandName"] != "commit" {
		t.Errorf("commandName = %v, want commit", out["commandName"])
	}
	if out["prompt"] == "" {
		t.Error("prompt should not be empty")
	}
}

func TestExecute_ArgsPassedToPrompt(t *testing.T) {
	tctx := tctxWithSkill(basicSkill("review"))
	in := mustJSON(t, map[string]any{"skill": "review", "args": "main.go"})
	result, _ := (&Tool{}).Execute(context.Background(), in, tctx)

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck
	prompt, _ := out["prompt"].(string)
	if prompt == "" || !contains(prompt, "main.go") {
		t.Errorf("prompt should contain 'main.go', got %q", prompt)
	}
}

func TestExecute_LeadingSlashStripped(t *testing.T) {
	tctx := tctxWithSkill(basicSkill("commit"))
	result, _ := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{"skill": "/commit"}), tctx)
	if result.IsError {
		t.Errorf("leading slash should be stripped, got error: %s", result.Content)
	}
}

func TestExecute_AllowedToolsInOutput(t *testing.T) {
	s := &skills.Skill{
		Name:         "restricted",
		AllowedTools: []string{"Read", "Glob"},
		Prompt:       func(string) string { return "do stuff" },
	}
	tctx := tctxWithSkill(s)
	result, _ := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{"skill": "restricted"}), tctx)

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck
	tools, ok := out["allowedTools"]
	if !ok {
		t.Error("output should include allowedTools")
	}
	if tools == nil {
		t.Error("allowedTools should not be nil")
	}
}

// ─── isSkillSafe ──────────────────────────────────────────────────────────────

func TestIsSkillSafe_EmptyTools(t *testing.T) {
	if isSkillSafe(nil) {
		t.Error("empty AllowedTools should NOT be safe")
	}
}

func TestIsSkillSafe_AllReadOnly(t *testing.T) {
	if !isSkillSafe([]string{"Read", "Glob", "Grep"}) {
		t.Error("read-only tools should be safe")
	}
}

func TestIsSkillSafe_ContainsWriteTool(t *testing.T) {
	if isSkillSafe([]string{"Read", "Bash"}) {
		t.Error("list containing Bash should NOT be safe")
	}
}

// ─── normalizeSkillName ───────────────────────────────────────────────────────

func TestNormalizeSkillName_StripSlash(t *testing.T) {
	if got := normalizeSkillName("/commit"); got != "commit" {
		t.Errorf("normalizeSkillName(%q) = %q, want %q", "/commit", got, "commit")
	}
}

func TestNormalizeSkillName_TrimSpace(t *testing.T) {
	if got := normalizeSkillName("  commit  "); got != "commit" {
		t.Errorf("normalizeSkillName(%q) = %q", "  commit  ", got)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
