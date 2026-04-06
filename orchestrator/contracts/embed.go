// Package contracts embeds the forge artifact contract markdown files.
// These are injected into phase-worker system prompts at compile time so that
// workers always have the authoritative contract format without file I/O.
package contracts

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed *.md
var contractFS embed.FS

// ContractFor returns the full content of the embedded contract file named
// `name` (without the .md extension), e.g. "common-rules".
// Returns an empty string when the contract is not found.
func ContractFor(name string) string {
	data, err := contractFS.ReadFile(name + ".md")
	if err != nil {
		return ""
	}
	return string(data)
}

// All returns a map of every embedded contract (name → content).
func All() map[string]string {
	result := make(map[string]string)
	entries, err := fs.ReadDir(contractFS, ".")
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		data, err := contractFS.ReadFile(e.Name())
		if err != nil {
			continue
		}
		result[name] = string(data)
	}
	return result
}
