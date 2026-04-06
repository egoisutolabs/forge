package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ServerConfig defines how to start and communicate with one language server.
type ServerConfig struct {
	// Name is a human-readable identifier (e.g., "gopls", "typescript", "pyright").
	Name string

	// Command is the binary to execute.
	Command string

	// Args are command-line arguments.
	Args []string

	// Extensions maps file extensions to LSP language IDs.
	// Key: extension with dot (e.g., ".go"). Value: LSP languageId (e.g., "go").
	Extensions map[string]string

	// DetectFiles are filenames whose presence in the workspace root
	// indicates this language is used.
	DetectFiles []string

	// InitOptions are passed as initializationOptions in the initialize request.
	InitOptions map[string]any

	// Settings are sent via workspace/didChangeConfiguration after initialization.
	Settings map[string]any

	// Env are additional environment variables for the server process.
	Env map[string]string

	// StartupTimeout is how long to wait for the initialize handshake.
	// Default: 30 seconds.
	StartupTimeout time.Duration

	// MaxCrashes is the number of crashes before giving up on auto-restart.
	// Default: 3.
	MaxCrashes int
}

// DefaultConfigs returns configs for the supported language ecosystems,
// ordered by priority: gopls, typescript-language-server, pyright, pylsp.
func DefaultConfigs() []ServerConfig {
	return []ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Args:    []string{"serve"},
			Extensions: map[string]string{
				".go":  "go",
				".mod": "go.mod",
				".sum": "go.sum",
			},
			DetectFiles: []string{"go.mod", "go.sum"},
			Settings: map[string]any{
				"gopls": map[string]any{
					"staticcheck":        true,
					"completeUnimported": true,
					"usePlaceholders":    false,
				},
			},
			StartupTimeout: 30 * time.Second,
			MaxCrashes:     3,
		},
		{
			Name:    "typescript",
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			Extensions: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
				".js":  "javascript",
				".jsx": "javascriptreact",
				".mts": "typescript",
				".cts": "typescript",
				".mjs": "javascript",
				".cjs": "javascript",
			},
			DetectFiles: []string{"package.json", "tsconfig.json", "jsconfig.json"},
			InitOptions: map[string]any{
				"preferences": map[string]any{
					"includeInlayParameterNameHints": "none",
				},
			},
			StartupTimeout: 30 * time.Second,
			MaxCrashes:     3,
		},
		{
			Name:    "pyright",
			Command: "pyright-langserver",
			Args:    []string{"--stdio"},
			Extensions: map[string]string{
				".py":  "python",
				".pyi": "python",
			},
			DetectFiles: []string{
				"pyproject.toml", "requirements.txt", "setup.py",
				"setup.cfg", "Pipfile", "pyrightconfig.json",
			},
			Settings: map[string]any{
				"python": map[string]any{
					"analysis": map[string]any{
						"autoSearchPaths":        true,
						"useLibraryCodeForTypes": true,
						"diagnosticMode":         "openFilesOnly",
					},
				},
			},
			StartupTimeout: 30 * time.Second,
			MaxCrashes:     3,
		},
		{
			Name:    "pylsp",
			Command: "pylsp",
			Args:    []string{},
			Extensions: map[string]string{
				".py":  "python",
				".pyi": "python",
			},
			DetectFiles: []string{
				"pyproject.toml", "requirements.txt", "setup.py",
			},
			StartupTimeout: 30 * time.Second,
			MaxCrashes:     3,
		},
	}
}

// DetectConfigs scans workDir for indicator files and returns matching configs.
// Only returns configs whose Command binary exists on PATH.
// Deduplicates by extension coverage (e.g., pyright wins over pylsp).
func DetectConfigs(workDir string) []ServerConfig {
	all := DefaultConfigs()
	var result []ServerConfig

	seen := map[string]bool{}
	for _, cfg := range all {
		if !hasAnyFile(workDir, cfg.DetectFiles) {
			continue
		}
		if _, err := exec.LookPath(cfg.Command); err != nil {
			continue
		}
		// Deduplicate: if a prior config already covers all these extensions, skip.
		dominated := true
		for ext := range cfg.Extensions {
			if !seen[ext] {
				dominated = false
			}
		}
		if dominated {
			continue
		}
		for ext := range cfg.Extensions {
			seen[ext] = true
		}
		result = append(result, cfg)
	}
	return result
}

// hasAnyFile returns true if any of the given filenames exist in dir.
func hasAnyFile(dir string, files []string) bool {
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}
