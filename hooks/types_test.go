package hooks

import (
	"encoding/json"
	"testing"
)

func TestHookResult_ContinueDefaultsTrue(t *testing.T) {
	var r HookResult
	if err := json.Unmarshal([]byte(`{}`), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !r.Continue {
		t.Error("expected Continue to default to true when absent from JSON")
	}
}

func TestHookResult_ContinueFalseExplicit(t *testing.T) {
	var r HookResult
	if err := json.Unmarshal([]byte(`{"continue":false}`), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Continue {
		t.Error("expected Continue to be false when explicitly set")
	}
}

func TestHookResult_ContinueTrueExplicit(t *testing.T) {
	var r HookResult
	if err := json.Unmarshal([]byte(`{"continue":true}`), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !r.Continue {
		t.Error("expected Continue to be true when explicitly set")
	}
}

func TestHookResult_AllFields(t *testing.T) {
	raw := `{"continue":false,"decision":"deny","system_message":"blocked","reason":"unsafe","updated_input":{"x":1}}`
	var r HookResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Continue {
		t.Error("expected Continue=false")
	}
	if r.Decision != "deny" {
		t.Errorf("expected Decision=deny, got %q", r.Decision)
	}
	if r.SystemMessage != "blocked" {
		t.Errorf("expected SystemMessage=blocked, got %q", r.SystemMessage)
	}
	if r.Reason != "unsafe" {
		t.Errorf("expected Reason=unsafe, got %q", r.Reason)
	}
	if string(r.UpdatedInput) != `{"x":1}` {
		t.Errorf("unexpected UpdatedInput: %s", r.UpdatedInput)
	}
}
