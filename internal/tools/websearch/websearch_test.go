package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
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
	if newTool().Name() != "WebSearch" {
		t.Error("Name() must return WebSearch")
	}
}

func TestValidateInput_Valid(t *testing.T) {
	input := json.RawMessage(`{"query":"golang channels"}`)
	if err := newTool().ValidateInput(input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInput_WithDomains(t *testing.T) {
	input := json.RawMessage(`{
		"query": "go modules",
		"allowed_domains": ["go.dev"],
		"blocked_domains": ["spam.com"]
	}`)
	if err := newTool().ValidateInput(input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInput_EmptyQuery(t *testing.T) {
	input := json.RawMessage(`{"query":""}`)
	if err := newTool().ValidateInput(input); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestValidateInput_MissingQuery(t *testing.T) {
	input := json.RawMessage(`{}`)
	if err := newTool().ValidateInput(input); err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestCheckPermissions_AlwaysAllow(t *testing.T) {
	input := json.RawMessage(`{"query":"test"}`)
	dec, err := newTool().CheckPermissions(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Errorf("got %q, want allow", dec.Behavior)
	}
}

func TestInterfaceInvariants(t *testing.T) {
	tool := newTool()
	input := json.RawMessage(`{"query":"test"}`)
	if !tool.IsConcurrencySafe(input) {
		t.Error("WebSearch must be concurrency-safe")
	}
	if !tool.IsReadOnly(input) {
		t.Error("WebSearch must be read-only")
	}
}

func TestExecute_ReturnsSearchSnapshot(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, args ...string) ([]byte, error) {
			if len(args) > 2 && args[2] == "snapshot" {
				return []byte("- link \"Go by Example\" [ref=e1]\n- link \"The Go Programming Language\" [ref=e2]\n"), nil
			}
			return nil, nil
		},
	}
	tool := &Tool{Runner: runner, Command: "agent-browser"}
	input := json.RawMessage(`{"query":"golang channels"}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute must not return a Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if got, want := len(runner.calls), 3; got != want {
		t.Fatalf("runner called %d times, want %d", got, want)
	}
	if runner.calls[0].name != "agent-browser" {
		t.Fatalf("unexpected command name: %s", runner.calls[0].name)
	}
	if got := runner.calls[0].args[2]; got != "open" {
		t.Fatalf("first command verb = %q, want open", got)
	}
	if got := runner.calls[1].args[2]; got != "snapshot" {
		t.Fatalf("second command verb = %q, want snapshot", got)
	}
	if got := runner.calls[2].args[2]; got != "close" {
		t.Fatalf("third command verb = %q, want close", got)
	}
	if result.Content == "" || result.Content == "web search requires Anthropic API web_search tool" {
		t.Errorf("unexpected search result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Go by Example") {
		t.Fatalf("expected snapshot content, got: %s", result.Content)
	}
}

func TestInputSchema_ValidJSON(t *testing.T) {
	schema := newTool().InputSchema()
	var v map[string]any
	if err := json.Unmarshal(schema, &v); err != nil {
		t.Fatalf("InputSchema returned invalid JSON: %v", err)
	}
}

func TestComposeSearchQuery_WithDomainFilters(t *testing.T) {
	got := composeSearchQuery(
		"forge cli",
		[]string{"go.dev", "pkg.go.dev"},
		[]string{"example.com"},
	)
	want := "(site:go.dev OR site:pkg.go.dev) forge cli -site:example.com"
	if got != want {
		t.Fatalf("composeSearchQuery() = %q, want %q", got, want)
	}
}

func TestBuildSearchURL_EncodesQuery(t *testing.T) {
	got := buildSearchURL("forge cli", []string{"go.dev"}, []string{"example.com"})
	want := "https://duckduckgo.com/?q=site%3Ago.dev+forge+cli+-site%3Aexample.com"
	if got != want {
		t.Fatalf("buildSearchURL() = %q, want %q", got, want)
	}
}

func TestNormalizeDomain(t *testing.T) {
	got := normalizeDomain("HTTPS://Docs.Go.Dev/reference/")
	if got != "docs.go.dev" {
		t.Fatalf("normalizeDomain() = %q, want docs.go.dev", got)
	}
}

func TestExecute_CommandNotFound(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, _ ...string) ([]byte, error) {
			return nil, &execErrorNotFound{}
		},
	}
	tool := &Tool{Runner: runner}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"forge cli"}`), nil)
	if err != nil {
		t.Fatalf("Execute must not return a Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if want := "agent-browser is not installed"; !containsString(result.Content, want) {
		t.Fatalf("expected missing-command hint, got: %s", result.Content)
	}
}

func TestExecute_OpenError(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, args ...string) ([]byte, error) {
			if len(args) > 2 && args[2] == "open" {
				return nil, errors.New("boom")
			}
			return nil, nil
		},
	}
	tool := &Tool{Runner: runner}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"forge cli"}`), nil)
	if err != nil {
		t.Fatalf("Execute must not return a Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	if !containsString(result.Content, "open search page failed") {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

type execErrorNotFound struct{}

func (*execErrorNotFound) Error() string { return "executable file not found in $PATH" }
func (*execErrorNotFound) Unwrap() error { return exec.ErrNotFound }

func containsString(s, want string) bool {
	return strings.Contains(s, want)
}
