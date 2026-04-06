package orchestrator

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/hooks"
)

// ForgeHooks returns HooksSettings that block destructive commands and
// sensitive writes, and auto-formats edited Go files.
//
// The orchestrator calls this once at startup and merges the result into
// the ToolContext.Hooks before spawning phase workers. The three hooks use
// the "forge-internal:" prefix so that hooks/executor.go routes them to
// compiled Go functions rather than spawning subprocesses.
func ForgeHooks() hooks.HooksSettings {
	hooks.RegisterInternalHook("block-destructive", blockDestructiveCommand)
	hooks.RegisterInternalHook("block-sensitive-write", blockSensitiveWrite)
	hooks.RegisterInternalHook("format-edited-file", formatEditedFile)

	return hooks.HooksSettings{
		hooks.HookEventPreToolUse: []hooks.HookMatcher{
			{
				Matcher: "Bash",
				Hooks: []hooks.HookConfig{{
					Command: "forge-internal:block-destructive",
				}},
			},
			{
				Matcher: "Write",
				Hooks: []hooks.HookConfig{{
					Command: "forge-internal:block-sensitive-write",
				}},
			},
		},
		hooks.HookEventPostToolUse: []hooks.HookMatcher{
			{
				Matcher: "Write|Edit",
				Hooks: []hooks.HookConfig{{
					Command: "forge-internal:format-edited-file",
				}},
			},
		},
	}
}

// destructivePatterns is the list of command substrings that forge treats as
// too dangerous to execute inside a phase worker.
var destructivePatterns = []string{
	"rm -rf /",
	"rm -rf ~",
	"rm -rf .",
	"git clean -fdx",
	"git checkout .",
	"git reset --hard",
	"DROP DATABASE",
	"DROP TABLE",
	"TRUNCATE TABLE",
	"> /dev/sda",
	"mkfs.",
	"dd if=",
	"chmod -R 777 /",
}

// blockDestructiveCommand is a PreToolUse hook for Bash.
// It denies any command that contains a known destructive pattern.
func blockDestructiveCommand(input hooks.HookInput) (*hooks.HookResult, error) {
	var cmdInput struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input.ToolInput, &cmdInput); err != nil {
		return &hooks.HookResult{Continue: true}, nil
	}
	for _, pattern := range destructivePatterns {
		if strings.Contains(cmdInput.Command, pattern) {
			return &hooks.HookResult{
				Continue: false,
				Decision: "deny",
				Reason:   "Destructive command blocked by forge safeguards: " + pattern,
			}, nil
		}
	}
	return &hooks.HookResult{Continue: true}, nil
}

// sensitivePathPatterns lists file path fragments that indicate sensitive
// content. Matches against both the full path and the base filename.
var sensitivePathPatterns = []string{
	".env",
	".pem",
	".key",
	"id_rsa",
	"id_ed25519",
	"id_dsa",
	"credentials.json",
	"secrets.json",
	".aws/credentials",
	".ssh/",
	"service-account.json",
	"token.json",
}

// blockSensitiveWrite is a PreToolUse hook for Write.
// It denies writes to files that match sensitive path patterns.
func blockSensitiveWrite(input hooks.HookInput) (*hooks.HookResult, error) {
	var writeInput struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input.ToolInput, &writeInput); err != nil {
		return &hooks.HookResult{Continue: true}, nil
	}

	path := writeInput.FilePath
	base := filepath.Base(path)

	for _, pattern := range sensitivePathPatterns {
		if strings.Contains(path, pattern) || base == pattern ||
			strings.HasSuffix(base, pattern) {
			return &hooks.HookResult{
				Continue: false,
				Decision: "deny",
				Reason:   "Sensitive file write blocked by forge safeguards: " + pattern,
			}, nil
		}
	}
	return &hooks.HookResult{Continue: true}, nil
}

// formatEditedFile is a PostToolUse hook for Write and Edit.
// It runs gofmt -w on .go files after they are written or edited.
// Formatting is best-effort: errors are silently ignored so that a format
// failure never blocks the agent.
func formatEditedFile(input hooks.HookInput) (*hooks.HookResult, error) {
	var fileInput struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input.ToolInput, &fileInput); err != nil {
		return &hooks.HookResult{Continue: true}, nil
	}

	if strings.HasSuffix(fileInput.FilePath, ".go") {
		// Best-effort: ignore errors (file may not exist yet, gofmt may not be in PATH).
		_ = exec.Command("gofmt", "-w", fileInput.FilePath).Run()
	}
	return &hooks.HookResult{Continue: true}, nil
}
