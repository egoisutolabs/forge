package lsp

import "os/exec"

// BinaryStatus reports whether a language server binary is available.
type BinaryStatus struct {
	Name  string
	Found bool
	Path  string
}

// DetectBinary checks if a language server binary is available on PATH.
// Returns the full path if found, empty string if not.
func DetectBinary(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// DetectAllServers returns a status report of available language servers.
func DetectAllServers() map[string]BinaryStatus {
	result := make(map[string]BinaryStatus)
	for _, cfg := range DefaultConfigs() {
		path := DetectBinary(cfg.Command)
		result[cfg.Name] = BinaryStatus{
			Name:  cfg.Name,
			Found: path != "",
			Path:  path,
		}
	}
	return result
}

// installHints maps binary names to install instructions.
var installHints = map[string]string{
	"gopls":                      "Install: go install golang.org/x/tools/gopls@latest",
	"typescript-language-server": "Install: npm install -g typescript-language-server typescript",
	"pyright-langserver":         "Install: pip install pyright  (or: npm install -g pyright)",
	"pylsp":                      "Install: pip install python-lsp-server",
}

// InstallHint returns the install instruction for a language server binary.
func InstallHint(command string) string {
	if hint, ok := installHints[command]; ok {
		return hint
	}
	return ""
}
