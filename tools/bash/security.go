package bash

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// safeEnvVars are environment variables safe to prefix commands with.
// Derived from Claude Code's SAFE_ENV_VARS in readOnlyValidation.ts.
var safeEnvVars = map[string]bool{
	// Go
	"GOEXPERIMENT": true, "GOOS": true, "GOARCH": true, "CGO_ENABLED": true, "GO111MODULE": true,
	// Rust
	"RUST_BACKTRACE": true, "RUST_LOG": true,
	// Node
	"NODE_ENV": true,
	// Python
	"PYTHONUNBUFFERED": true, "PYTHONDONTWRITEBYTECODE": true,
	// Pytest
	"PYTEST_DISABLE_PLUGIN_AUTOLOAD": true, "PYTEST_DEBUG": true,
	// API
	"ANTHROPIC_API_KEY": true,
	// Locale
	"LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true, "LC_TIME": true, "CHARSET": true,
	// Terminal
	"TERM": true, "COLORTERM": true, "NO_COLOR": true, "FORCE_COLOR": true, "TZ": true,
	// Colors
	"LS_COLORS": true, "LSCOLORS": true, "GREP_COLOR": true, "GREP_COLORS": true, "GCC_COLORS": true,
	// Display
	"TIME_STYLE": true, "BLOCK_SIZE": true, "BLOCKSIZE": true,
}

// safeEnvValuePattern matches safe env var values (no shell metacharacters).
var safeEnvValuePattern = regexp.MustCompile(`^[A-Za-z0-9_./:\-]+$`)

// gitInternalPatterns detect paths that are git-internal (hooks, objects, refs, HEAD).
// Writing to these can enable sandbox escape via git hook injection.
var gitInternalPatterns = []string{
	"HEAD",
	"objects",
	"refs",
	"hooks",
}

// maxSubcommandsForSecurityCheck limits compound command analysis.
// Beyond this, fall back to 'ask'.
const maxSubcommandsForSecurityCheck = 50

// containsUnquotedExpansion checks for unquoted variable expansion, globs,
// and command substitution. This is the key defense against parser differential
// attacks where the shell interprets tokens differently than our validator.
//
// Exact port of Claude Code's containsUnquotedExpansion().
func containsUnquotedExpansion(command string) bool {
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escaped {
			escaped = false
			continue
		}

		// Backslash only works outside single quotes
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		// Skip everything inside single quotes
		if inSingle {
			continue
		}

		// $ expansion (expands in both double-quoted and unquoted contexts)
		if ch == '$' && i+1 < len(command) {
			next := command[i+1]
			if isShellVarChar(next) {
				return true
			}
			// $( command substitution
			if next == '(' {
				return true
			}
			// ${ parameter expansion
			if next == '{' {
				return true
			}
		}

		// Backtick substitution
		if ch == '`' {
			return true
		}

		// Globs only expand outside ALL quotes
		if inDouble {
			continue
		}

		if ch == '?' || ch == '*' || ch == '[' || ch == ']' {
			return true
		}
	}
	return false
}

// isShellVarChar returns true if ch can start/continue a shell variable name.
func isShellVarChar(ch byte) bool {
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	// Special shell variables: $@, $*, $#, $?, $!, $$, $-, $_
	return ch == '_' || ch == '@' || ch == '*' || ch == '#' ||
		ch == '?' || ch == '!' || ch == '$' || ch == '-'
}

// hasBraceExpansion checks for shell brace expansion like {a,b} or {1..5}.
func hasBraceExpansion(token string) bool {
	if !strings.Contains(token, "{") {
		return false
	}
	return strings.Contains(token, ",") || strings.Contains(token, "..")
}

// isGitInternalPath checks if a normalized path matches git-internal patterns.
func isGitInternalPath(path string) bool {
	p := strings.TrimPrefix(path, "./")
	p = strings.TrimPrefix(p, "/")
	for _, pattern := range gitInternalPatterns {
		if p == pattern || strings.HasPrefix(p, pattern+"/") {
			return true
		}
	}
	return false
}

// commandWritesToGitInternalPaths checks if any subcommand in a compound command
// writes to git-internal paths (hooks/, objects/, refs/, HEAD).
// This prevents attacks like: mkdir hooks && echo evil > hooks/pre-commit && git status
func commandWritesToGitInternalPaths(command string) bool {
	subs := splitCompoundCommand(command)
	for _, sub := range subs {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}

		// Check for output redirections to git-internal paths
		if idx := strings.Index(sub, ">"); idx >= 0 {
			target := strings.TrimSpace(sub[idx+1:])
			target = strings.TrimPrefix(target, ">") // handle >>
			target = strings.TrimSpace(target)
			if isGitInternalPath(target) {
				return true
			}
		}

		// Check write commands: mkdir, touch, cp, mv targeting git-internal paths
		parts := strings.Fields(sub)
		if len(parts) < 2 {
			continue
		}
		base := parts[0]
		if base == "mkdir" || base == "touch" || base == "cp" || base == "mv" || base == "ln" {
			for _, arg := range parts[1:] {
				if !strings.HasPrefix(arg, "-") && isGitInternalPath(arg) {
					return true
				}
			}
		}
	}
	return false
}

// compoundCommandHasCd checks if any subcommand is a cd command.
func compoundCommandHasCd(command string) bool {
	subs := splitCompoundCommand(command)
	for _, sub := range subs {
		parts := strings.Fields(strings.TrimSpace(sub))
		if len(parts) > 0 && parts[0] == "cd" {
			return true
		}
	}
	return false
}

// commandHasGit checks if any subcommand uses git.
func commandHasGit(command string) bool {
	subs := splitCompoundCommand(command)
	for _, sub := range subs {
		parts := strings.Fields(strings.TrimSpace(sub))
		if len(parts) > 0 && parts[0] == "git" {
			return true
		}
	}
	return false
}

// stripSafeEnvVars strips safe environment variable assignments from the front
// of a command. e.g., "LANG=C TERM=xterm ls" → "ls"
func stripSafeEnvVars(command string) string {
	parts := strings.Fields(command)
	i := 0
	for i < len(parts) {
		eq := strings.IndexByte(parts[i], '=')
		if eq <= 0 {
			break
		}
		varName := parts[i][:eq]
		varValue := parts[i][eq+1:]

		// Variable name must be valid identifier
		if !isValidEnvVarName(varName) {
			break
		}
		// Must be a known safe env var
		if !safeEnvVars[varName] {
			break
		}
		// Value must not contain shell metacharacters
		if !safeEnvValuePattern.MatchString(varValue) {
			break
		}
		i++
	}
	if i >= len(parts) {
		return ""
	}
	return strings.Join(parts[i:], " ")
}

func isValidEnvVarName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, ch := range name {
		if i == 0 && !unicode.IsLetter(ch) && ch != '_' {
			return false
		}
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return false
		}
	}
	return true
}

// sedIsDangerous checks if a sed command has dangerous operations.
// It is the IsDangerous callback for the sed entry in commandAllowlist.
//
// Uses the full containsDangerousOperations denylist (ported from TypeScript's
// sedValidation.ts) for expression-level validation, and additionally blocks the
// -i / --in-place flags which are outside the scope of expression checking.
func sedIsDangerous(tokens []string) bool {
	// Block in-place flag (-i, -i.bak, --in-place).
	for _, t := range tokens {
		if t == "-i" || strings.HasPrefix(t, "-i") || t == "--in-place" {
			return true
		}
	}

	// Check expressions provided via -e / --expression.
	for i, t := range tokens {
		if (t == "-e" || t == "--expression") && i+1 < len(tokens) {
			if containsDangerousOperations(tokens[i+1]) {
				return true
			}
		}
	}

	// If no -e flag, the first non-flag token is the sed expression.
	for _, t := range tokens {
		if !strings.HasPrefix(t, "-") && t != "" {
			if containsDangerousOperations(t) {
				return true
			}
			break
		}
	}

	return false
}

// ── Additional security helpers ───────────────────────────────────────────────

// uncPathRe matches Windows UNC path patterns in command strings.
// Looks for two or more consecutive backslashes followed by a hostname character,
// which indicates \\server\share style paths.
var uncPathRe = regexp.MustCompile(`\\{2,}[A-Za-z0-9]`)

// containsUNCPath reports whether the command contains a Windows UNC path
// pattern (\\host\share). Such paths can be used to access remote SMB shares.
func containsUNCPath(command string) bool {
	return uncPathRe.MatchString(command)
}

// isBareGitRepo reports whether cwd is a bare git repository by checking for
// the presence of the four required git-internal entries: HEAD, objects, refs,
// and hooks. Running git commands inside a bare repo without proper sandboxing
// can allow hook-injection attacks.
func isBareGitRepo(cwd string) bool {
	required := []string{"HEAD", "objects", "refs", "hooks"}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(cwd, name)); err != nil {
			return false
		}
	}
	return true
}

// rawAwkIsDangerous checks the raw awk command string for dangerous awk constructs
// that cannot be reliably detected via tokenized inspection alone. This is
// needed because strings.Fields incorrectly splits shell-quoted awk programs
// (e.g. '{print $1}' → ['{print', '$1}']), making per-token pattern checks
// unreliable for patterns that span the artificial token boundaries.
func rawAwkIsDangerous(command string) bool {
	patterns := []string{
		"system(",   // shell execution: system("cmd")
		`| "`, `|"`, // pipe to double-quoted command: cmd | "sort"
		`| '`, `|'`, // pipe to single-quoted command: cmd | 'sort'
		`> "`, `>> "`, // redirect to double-quoted file: print > "out.txt"
		`> '`, `>> '`, // redirect to single-quoted file: print > 'out.txt'
	}
	for _, p := range patterns {
		if strings.Contains(command, p) {
			return true
		}
	}
	return false
}

// isOutputFlag reports whether token is a flag that directs curl or wget to
// write output to a file (-o / --output for curl, -O / --output-document for
// wget). These flags can be used to write arbitrary files and warrant scrutiny.
func isOutputFlag(token string) bool {
	switch token {
	case "-o", "--output", "-O", "--output-document", "--output-file":
		return true
	}
	if strings.HasPrefix(token, "--output=") ||
		strings.HasPrefix(token, "--output-document=") ||
		strings.HasPrefix(token, "--output-file=") {
		return true
	}
	return false
}
