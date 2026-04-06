package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
)

func newTool() *Tool { return &Tool{} }

type runnerCall struct {
	name string
	args []string
}

type stubRunner struct {
	calls []runnerCall
	run   func(name string, args ...string) ([]byte, error)
}

func (s *stubRunner) CombinedOutput(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if s.run != nil {
		return s.run(name, args...)
	}
	return nil, nil
}

func TestName(t *testing.T) {
	if newTool().Name() != "Browser" {
		t.Fatal("Name() must return Browser")
	}
}

func TestValidateInput_Open(t *testing.T) {
	if err := newTool().ValidateInput(json.RawMessage(`{"action":"open","url":"https://example.com"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInput_GetTitle(t *testing.T) {
	if err := newTool().ValidateInput(json.RawMessage(`{"action":"get","get":"title"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInput_GetTextRequiresSelector(t *testing.T) {
	err := newTool().ValidateInput(json.RawMessage(`{"action":"get","get":"text"}`))
	if err == nil {
		t.Fatal("expected selector validation error")
	}
}

func TestValidateInput_UnknownAction(t *testing.T) {
	err := newTool().ValidateInput(json.RawMessage(`{"action":"explode"}`))
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestValidateInput_DownloadRequiresPath(t *testing.T) {
	err := newTool().ValidateInput(json.RawMessage(`{"action":"download","selector":"#dl"}`))
	if err == nil {
		t.Fatal("expected path validation error")
	}
}

func TestValidateInput_FindRequiresFields(t *testing.T) {
	err := newTool().ValidateInput(json.RawMessage(`{"action":"find","locator":"role","value":"button"}`))
	if err == nil {
		t.Fatal("expected find_action validation error")
	}
}

func TestValidateInput_TabSwitchRequiresIndex(t *testing.T) {
	err := newTool().ValidateInput(json.RawMessage(`{"action":"tab","tab_action":"switch"}`))
	if err == nil {
		t.Fatal("expected tab_index validation error")
	}
}

func TestCheckPermissions_ReadOnlyActionsAllow(t *testing.T) {
	dec, err := newTool().CheckPermissions(json.RawMessage(`{"action":"snapshot"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Fatalf("got %q, want allow", dec.Behavior)
	}
}

func TestCheckPermissions_OpenAsks(t *testing.T) {
	dec, err := newTool().CheckPermissions(json.RawMessage(`{"action":"open","url":"https://example.com"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAsk {
		t.Fatalf("got %q, want ask", dec.Behavior)
	}
}

func TestCheckPermissions_TabListAllows(t *testing.T) {
	dec, err := newTool().CheckPermissions(json.RawMessage(`{"action":"tab","tab_action":"list"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Fatalf("got %q, want allow", dec.Behavior)
	}
}

func TestCheckPermissions_TabNewAsks(t *testing.T) {
	dec, err := newTool().CheckPermissions(json.RawMessage(`{"action":"tab","tab_action":"new"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAsk {
		t.Fatalf("got %q, want ask", dec.Behavior)
	}
}

func TestIsReadOnly(t *testing.T) {
	if !newTool().IsReadOnly(json.RawMessage(`{"action":"snapshot"}`)) {
		t.Fatal("snapshot should be read-only")
	}
	if !newTool().IsReadOnly(json.RawMessage(`{"action":"tab","tab_action":"list"}`)) {
		t.Fatal("tab list should be read-only")
	}
	if newTool().IsReadOnly(json.RawMessage(`{"action":"tab","tab_action":"new"}`)) {
		t.Fatal("tab new should not be read-only")
	}
	if newTool().IsReadOnly(json.RawMessage(`{"action":"click","selector":"#x"}`)) {
		t.Fatal("click should not be read-only")
	}
}

func TestBuildArgsSnapshotDefaults(t *testing.T) {
	args, err := buildArgs(toolInput{Action: "snapshot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.Join(args, " ")
	if got != "snapshot -i -c" {
		t.Fatalf("got %q, want %q", got, "snapshot -i -c")
	}
}

func TestBuildArgsGetAttr(t *testing.T) {
	args, err := buildArgs(toolInput{Action: "get", Get: "attr", AttrName: "href", Selector: "@e2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.Join(args, " ")
	if got != "get attr href @e2" {
		t.Fatalf("got %q, want %q", got, "get attr href @e2")
	}
}

func TestBuildArgsFind(t *testing.T) {
	args, err := buildArgs(toolInput{
		Action:     "find",
		Locator:    "role",
		Value:      "button",
		FindAction: "click",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(args, " "); got != "find role button click" {
		t.Fatalf("got %q, want %q", got, "find role button click")
	}
}

func TestBuildArgsTabSwitch(t *testing.T) {
	idx := 2
	args, err := buildArgs(toolInput{Action: "tab", TabAction: "switch", TabIndex: &idx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(args, " "); got != "tab 2" {
		t.Fatalf("got %q, want %q", got, "tab 2")
	}
}

func TestExecute_UsesPersistentDefaultSession(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, args ...string) ([]byte, error) {
			if len(args) > 2 && args[2] == "snapshot" {
				return []byte("- link \"More information\" [ref=e2]"), nil
			}
			return []byte("ok"), nil
		},
	}
	tool := &Tool{
		Runner:  runner,
		Command: "agent-browser",
		NewBackend: func(session string) (Backend, error) {
			return NewCLIBackend("agent-browser", session, runner), nil
		},
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"open","url":"https://example.com"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner called %d times, want 2", len(runner.calls))
	}
	if got, want := runner.calls[0].args[1], runner.calls[1].args[1]; got != want {
		t.Fatalf("sessions differ: %q vs %q", got, want)
	}
}

func TestExecute_CommandNotFound(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, _ ...string) ([]byte, error) {
			return nil, &execErrorNotFound{}
		},
	}
	tool := &Tool{
		Runner: runner,
		NewBackend: func(session string) (Backend, error) {
			return NewCLIBackend("agent-browser", session, runner), nil
		},
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content, "snapshot") {
		t.Fatalf("unexpected message: %s", result.Content)
	}
}

func TestExecute_ActionError(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, _ ...string) ([]byte, error) {
			return nil, errors.New("boom")
		},
	}
	tool := &Tool{
		Runner: runner,
		NewBackend: func(session string) (Backend, error) {
			return NewCLIBackend("agent-browser", session, runner), nil
		},
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"click","selector":"#go"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(result.Content, "boom") {
		t.Fatalf("unexpected message: %s", result.Content)
	}
}

type execErrorNotFound struct{}

func (*execErrorNotFound) Error() string { return "executable file not found in $PATH" }
func (*execErrorNotFound) Unwrap() error { return exec.ErrNotFound }
