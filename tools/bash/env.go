package bash

import (
	"os"
	"strings"
)

// dangerousEnvVars are environment variables that can inject arbitrary code
// into bash before any command runs. These MUST be stripped from the process
// environment to prevent sandbox bypass.
var dangerousEnvVars = map[string]bool{
	"BASH_ENV":       true, // sourced by non-interactive bash before commands
	"ENV":            true, // sourced by POSIX sh
	"CDPATH":         true, // can redirect cd to unexpected directories
	"GLOBIGNORE":     true, // alters glob behavior
	"PROMPT_COMMAND": true, // executed before each prompt (interactive shells)
	"SHELLOPTS":      true, // can enable unexpected shell options
	"BASHOPTS":       true, // can enable unexpected bash options
}

// safePassthroughVars are environment variables that are safe to inherit from
// the parent process. This is the allowlist — only these variables pass through.
var safePassthroughVars = map[string]bool{
	// Core POSIX
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
	"TMPDIR": true, "TEMP": true, "TMP": true,

	// Locale
	"LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_CTYPE": true,
	"LC_TIME": true, "LC_MESSAGES": true, "LC_COLLATE": true,
	"LC_NUMERIC": true, "LC_MONETARY": true, "CHARSET": true,

	// Terminal
	"TERM": true, "COLORTERM": true, "NO_COLOR": true, "FORCE_COLOR": true,
	"TZ": true, "COLUMNS": true, "LINES": true,

	// Colors / display
	"LS_COLORS": true, "LSCOLORS": true, "GREP_COLOR": true, "GREP_COLORS": true,
	"GCC_COLORS": true, "TIME_STYLE": true, "BLOCK_SIZE": true, "BLOCKSIZE": true,

	// Go
	"GOPATH": true, "GOROOT": true, "GOBIN": true, "GOCACHE": true,
	"GOMODCACHE": true, "GOEXPERIMENT": true, "GOOS": true, "GOARCH": true,
	"CGO_ENABLED": true, "GO111MODULE": true, "GOFLAGS": true,
	"GOTOOLCHAIN": true, "GOPROXY": true, "GONOSUMCHECK": true, "GONOSUMDB": true,
	"GOPRIVATE": true, "GONOPROXY": true,

	// Rust
	"RUST_BACKTRACE": true, "RUST_LOG": true, "CARGO_HOME": true, "RUSTUP_HOME": true,

	// Node
	"NODE_ENV": true, "NODE_PATH": true, "NPM_CONFIG_PREFIX": true,
	"NVM_DIR": true, "VOLTA_HOME": true,

	// Python
	"PYTHONUNBUFFERED": true, "PYTHONDONTWRITEBYTECODE": true,
	"PYTHONPATH": true, "VIRTUAL_ENV": true, "CONDA_PREFIX": true,
	"CONDA_DEFAULT_ENV": true, "PYENV_ROOT": true,

	// Pytest
	"PYTEST_DISABLE_PLUGIN_AUTOLOAD": true, "PYTEST_DEBUG": true,

	// Java
	"JAVA_HOME": true, "MAVEN_HOME": true, "GRADLE_HOME": true,

	// API keys (needed for tool use)
	"ANTHROPIC_API_KEY": true,

	// Editor
	"EDITOR": true, "VISUAL": true, "PAGER": true,

	// SSH / Git
	"SSH_AUTH_SOCK": true, "GIT_AUTHOR_NAME": true, "GIT_AUTHOR_EMAIL": true,
	"GIT_COMMITTER_NAME": true, "GIT_COMMITTER_EMAIL": true,
	"GIT_SSH_COMMAND": true,

	// XDG
	"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_CACHE_HOME": true,
	"XDG_RUNTIME_DIR": true, "XDG_STATE_HOME": true,

	// macOS
	"APPLE_SSH_ADD_BEHAVIOR": true, "COMMAND_MODE": true,

	// Homebrew
	"HOMEBREW_PREFIX": true, "HOMEBREW_CELLAR": true, "HOMEBREW_REPOSITORY": true,

	// Docker
	"DOCKER_HOST": true, "DOCKER_CONFIG": true,

	// Misc build tools
	"CC": true, "CXX": true, "CFLAGS": true, "CXXFLAGS": true, "LDFLAGS": true,
	"PKG_CONFIG_PATH": true, "CMAKE_PREFIX_PATH": true,
}

// SanitizedEnv returns a filtered copy of the current process environment.
// It allowlists safe variables and explicitly strips dangerous ones like
// BASH_ENV that can inject code before commands execute.
// Variables prefixed with "BASH_FUNC_" (exported bash functions) are also stripped.
func SanitizedEnv() []string {
	return sanitizeEnvFrom(os.Environ())
}

// sanitizeEnvFrom filters the given environment slice. Separated for testability.
func sanitizeEnvFrom(environ []string) []string {
	result := make([]string, 0, len(environ))
	for _, entry := range environ {
		eqIdx := strings.IndexByte(entry, '=')
		if eqIdx <= 0 {
			continue
		}
		name := entry[:eqIdx]

		// Strip exported bash functions (BASH_FUNC_name%%)
		if strings.HasPrefix(name, "BASH_FUNC_") {
			continue
		}

		// Strip explicitly dangerous vars
		if dangerousEnvVars[name] {
			continue
		}

		// Only pass through allowlisted vars
		if safePassthroughVars[name] {
			result = append(result, entry)
		}
	}
	return result
}
