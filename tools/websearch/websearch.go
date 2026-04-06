// Package websearch implements the WebSearchTool using the local agent-browser
// CLI. It opens a search-engine results page in an isolated browser session,
// captures a compact accessibility snapshot, and returns the text output.
package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

type toolInput struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Tool implements the WebSearch tool using the external agent-browser CLI.
//
// The search flow is:
//  1. Create an isolated agent-browser session.
//  2. Open a DuckDuckGo search URL.
//  3. Capture a compact interactive snapshot.
//  4. Close the session.
//
// Runner and Command are injectable for tests.
type Tool struct {
	Command string
	Runner  commandRunner
}

func (t *Tool) Name() string { return "WebSearch" }

func (t *Tool) Description() string {
	return "Searches the web using the local agent-browser CLI and returns a compact text snapshot of search results. Best for discovery. Use Browser for multi-step site interaction."
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"allowed_domains": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional list of domains to restrict results to"
			},
			"blocked_domains": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional list of domains to exclude from results"
			}
		},
		"required": ["query"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.Query == "" {
		return fmt.Errorf("query is required and cannot be empty")
	}
	return nil
}

// CheckPermissions always allows — web search is considered read-only.
func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Invalid input: %s", err),
			IsError: true,
		}, nil
	}

	cmd := t.commandName()
	runner := t.commandRunner()
	session := newSessionID()
	searchURL := buildSearchURL(in.Query, in.AllowedDomains, in.BlockedDomains)

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	defer func() {
		_, _ = runner.CombinedOutput(closeCtx, cmd, "--session", session, "close")
	}()

	if _, err := runner.CombinedOutput(ctx, cmd, "--session", session, "open", searchURL); err != nil {
		return &models.ToolResult{
			Content: formatExecError("open search page", cmd, err),
			IsError: true,
		}, nil
	}

	out, err := runner.CombinedOutput(ctx, cmd, "--session", session, "snapshot", "-i", "-c", "-d", "5")
	if err != nil {
		return &models.ToolResult{
			Content: formatExecError("capture search results", cmd, err),
			IsError: true,
		}, nil
	}

	snapshot := strings.TrimSpace(string(out))
	if snapshot == "" {
		return &models.ToolResult{
			Content: fmt.Sprintf("Search ran via %s, but no snapshot output was returned.", cmd),
			IsError: true,
		}, nil
	}

	content := fmt.Sprintf("Search results for %q via agent-browser:\n\n%s", strings.TrimSpace(in.Query), snapshot)
	return &models.ToolResult{Content: content}, nil
}

func (t *Tool) commandName() string {
	if strings.TrimSpace(t.Command) == "" {
		return "agent-browser"
	}
	return t.Command
}

func (t *Tool) commandRunner() commandRunner {
	if t.Runner == nil {
		return execRunner{}
	}
	return t.Runner
}

func buildSearchURL(query string, allowedDomains, blockedDomains []string) string {
	composed := composeSearchQuery(query, allowedDomains, blockedDomains)
	return "https://duckduckgo.com/?q=" + url.QueryEscape(composed)
}

func composeSearchQuery(query string, allowedDomains, blockedDomains []string) string {
	query = strings.TrimSpace(query)

	allowed := normalizeDomains(allowedDomains)
	blocked := normalizeDomains(blockedDomains)

	var parts []string
	if len(allowed) == 1 {
		parts = append(parts, "site:"+allowed[0])
	} else if len(allowed) > 1 {
		var clauses []string
		for _, domain := range allowed {
			clauses = append(clauses, "site:"+domain)
		}
		parts = append(parts, "("+strings.Join(clauses, " OR ")+")")
	}

	if query != "" {
		parts = append(parts, query)
	}

	for _, domain := range blocked {
		parts = append(parts, "-site:"+domain)
	}

	return strings.Join(parts, " ")
}

func normalizeDomains(domains []string) []string {
	seen := make(map[string]bool, len(domains))
	var out []string
	for _, raw := range domains {
		domain := normalizeDomain(raw)
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out
}

func normalizeDomain(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(strings.ToLower(u.Hostname()))
	host = strings.Trim(host, ".")
	return host
}

func formatExecError(action, command string, err error) string {
	if isCommandNotFound(err) {
		return fmt.Sprintf(
			"%s is not installed or not on PATH. Install it with `npm install -g agent-browser` or `brew install agent-browser`, then run `agent-browser install` once.",
			command,
		)
	}
	return fmt.Sprintf("%s failed via %s: %s", action, command, err)
}

func isCommandNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return true
	}
	return errors.Is(err, exec.ErrNotFound)
}

func newSessionID() string {
	return fmt.Sprintf("forge-websearch-%d", time.Now().UnixNano())
}
