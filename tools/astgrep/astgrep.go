// Package astgrep implements the AstGrep tool — structural code search using
// ast-grep (sg). Unlike text-based grep, ast-grep understands code structure
// and can match patterns like "all console.log calls" regardless of formatting.
package astgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

const defaultMaxResults = 250

// toolInput is the JSON input accepted by the AstGrep tool.
type toolInput struct {
	// Pattern is an AST pattern string (pattern mode). Mutually exclusive with Rule.
	Pattern string `json:"pattern,omitempty"`
	// Lang is the language for pattern mode (e.g. "javascript", "go", "python").
	// Optional; sg infers from file extensions when omitted.
	Lang string `json:"lang,omitempty"`
	// Rule is an inline YAML ast-grep rule (scan mode). Mutually exclusive with Pattern.
	Rule string `json:"rule,omitempty"`
	// Path is the directory or file to search. Defaults to current directory.
	Path string `json:"path,omitempty"`
}

// sgRange holds line/column positions returned by sg's JSON output.
type sgRange struct {
	Start sgPosition `json:"start"`
	End   sgPosition `json:"end"`
}

type sgPosition struct {
	Line   int `json:"line"`   // 0-indexed
	Column int `json:"column"` // 0-indexed
}

// sgMatch is one search result from sg's --json=compact output.
type sgMatch struct {
	Text  string  `json:"text"`  // the matched AST node text
	File  string  `json:"file"`  // path to the matched file
	Lines string  `json:"lines"` // full source line(s) containing the match
	Range sgRange `json:"range"`
	// RuleID is only present in scan mode.
	RuleID string `json:"ruleId,omitempty"`
}

// Tool implements the AstGrep tool — structural code search via sg.
//
// Two modes:
//   - Pattern mode: `sg run --pattern '<pattern>' [--lang <lang>] [<path>]`
//   - Rule mode:    `sg scan --inline-rules '<yaml>' [<path>]`
//
// Always read-only and concurrency-safe; always auto-approved.
type Tool struct{}

func (t *Tool) Name() string { return "AstGrep" }
func (t *Tool) Description() string {
	return "Structural code search using AST patterns (ast-grep). Use instead of Grep when searching for code constructs: function calls (e.g. 'console.log($$$ARGS)'), class/method definitions (e.g. 'func $NAME($$$PARAMS) error'), imports (e.g. 'import $MOD from \"react\"'), JSX elements, decorators, or type annotations. Matches code structure regardless of whitespace or formatting. Use Grep only for plain-text or log-message searches."
}

func (t *Tool) SearchHint() string {
	return "preferred code search ast structural syntax aware semantic find function call import handler class method JSX SQL AST ast-grep sg"
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "AST pattern to search for (e.g. 'console.log($$$ARGS)'). Mutually exclusive with rule."
			},
			"lang": {
				"type": "string",
				"description": "Language for pattern matching (e.g. 'javascript', 'go', 'python'). Optional; inferred from file extensions when omitted."
			},
			"rule": {
				"type": "string",
				"description": "Inline YAML ast-grep rule for scan mode. Mutually exclusive with pattern."
			},
			"path": {
				"type": "string",
				"description": "Directory or file to search. Defaults to current directory."
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	pattern := strings.TrimSpace(in.Pattern)
	rule := strings.TrimSpace(in.Rule)

	if pattern == "" && rule == "" {
		return fmt.Errorf("either 'pattern' or 'rule' is required")
	}
	if pattern != "" && rule != "" {
		return fmt.Errorf("'pattern' and 'rule' are mutually exclusive; provide only one")
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	// AstGrep is always read-only — no permission check needed.
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	// Check ast-grep is available. Prefer the `ast-grep` binary name to avoid
	// a collision with shadow-utils' `sg` command on Linux (which expects a
	// Unix group as its first argument and fails with "group 'run' does not
	// exist" if fed an ast-grep subcommand).
	sgPath, err := exec.LookPath("ast-grep")
	if err != nil {
		sgPath, err = exec.LookPath("sg")
		if err != nil {
			return &models.ToolResult{
				Content: "ast-grep is not installed or not on PATH. " +
					"Install it with: cargo install ast-grep, brew install ast-grep, " +
					"or visit https://ast-grep.github.io/guide/quick-start.html",
				IsError: true,
			}, nil
		}
	}

	args := buildArgs(in)

	cmd := exec.CommandContext(ctx, sgPath, args...)

	// Capture stdout and stderr separately: stdout carries JSON, stderr carries errors.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Use the tool context's working directory if set and no explicit path given.
	if in.Path == "" && tctx != nil && tctx.Cwd != "" {
		cmd.Dir = tctx.Cwd
	}

	_ = cmd.Run() // exit code 1 = no matches; treat as non-error (parsed below)

	// sg outputs valid JSON to stdout in all non-error cases (even `[]` for no matches).
	// If stdout is empty, sg encountered a hard error — report stderr.
	if stdout.Len() == 0 {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = "sg produced no output"
		}
		return &models.ToolResult{Content: fmt.Sprintf("sg error: %s", errMsg), IsError: true}, nil
	}

	var matches []sgMatch
	if err := json.Unmarshal(stdout.Bytes(), &matches); err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Failed to parse sg output: %s\nRaw output: %s", err, stdout.String()),
			IsError: true,
		}, nil
	}

	return &models.ToolResult{Content: formatMatches(matches, defaultMaxResults)}, nil
}

// buildArgs constructs the sg command arguments for the given input.
// The .git/ directory is always excluded via --globs.
// Exported as a package-level function so tests can verify arg construction
// without running sg.
func buildArgs(in toolInput) []string {
	var args []string

	if in.Rule != "" {
		// Scan mode: uses inline YAML rule
		args = []string{
			"scan",
			"--inline-rules", in.Rule,
			"--json=compact",
			"--globs", "!.git/**",
		}
	} else {
		// Pattern mode: direct AST pattern
		args = []string{
			"run",
			"--pattern", in.Pattern,
			"--json=compact",
			"--globs", "!.git/**",
		}
		if in.Lang != "" {
			args = append(args, "--lang", in.Lang)
		}
	}

	if in.Path != "" {
		args = append(args, in.Path)
	}

	return args
}

// formatMatches renders a slice of sg matches into a human-readable string
// with file:line: text format, capped at maxResults entries.
func formatMatches(matches []sgMatch, maxResults int) string {
	if len(matches) == 0 {
		return "No matches found"
	}

	truncated := len(matches) > maxResults
	if truncated {
		matches = matches[:maxResults]
	}

	lines := make([]string, 0, len(matches)+1)
	for _, m := range matches {
		// sg reports 0-indexed lines; display 1-indexed.
		line := fmt.Sprintf("%s:%d: %s", m.File, m.Range.Start.Line+1, m.Lines)
		lines = append(lines, line)
	}

	if truncated {
		lines = append(lines, fmt.Sprintf(
			"(Results are truncated, showing first %d matches. Use a more specific path or pattern.)",
			maxResults,
		))
	}

	return strings.Join(lines, "\n")
}
