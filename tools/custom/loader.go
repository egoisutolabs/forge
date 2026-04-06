// Package custom implements user-defined tools loaded from YAML configuration files.
//
// Custom tools are discovered from two directories (lowest to highest priority):
//   - ~/.forge/tools/    (user-global)
//   - .forge/tools/      (project-local, overrides user-global by name)
//
// Each .yaml/.yml file defines one tool: its name, description, JSON Schema for
// input, a shell command to execute, and optional flags for timeout, read_only,
// and concurrency_safe behavior.
package custom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Definition is the parsed YAML definition of a custom tool.
type Definition struct {
	Name            string         `yaml:"name"`
	Description     string         `yaml:"description"`
	InputSchema     map[string]any `yaml:"input_schema"`
	Command         string         `yaml:"command"`
	Timeout         int            `yaml:"timeout"` // seconds; default 10
	ReadOnly        bool           `yaml:"read_only"`
	ConcurrencySafe bool           `yaml:"concurrency_safe"`
	SearchHintText  string         `yaml:"search_hint"`
}

// Validate checks that all required fields are present and well-formed.
func (d *Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(d.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(d.Command) == "" {
		return fmt.Errorf("command is required")
	}
	if d.Timeout < 0 {
		return fmt.Errorf("timeout must be >= 0")
	}

	// input_schema must have "type": "object" if present.
	if d.InputSchema != nil {
		typ, _ := d.InputSchema["type"].(string)
		if typ != "object" {
			return fmt.Errorf("input_schema.type must be \"object\"")
		}
	}

	// Validate that input_schema serializes to valid JSON.
	if d.InputSchema != nil {
		if _, err := json.Marshal(d.InputSchema); err != nil {
			return fmt.Errorf("input_schema is not valid JSON: %w", err)
		}
	}

	return nil
}

// ParseDefinition reads and validates a single YAML file into a Definition.
func ParseDefinition(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var def Definition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if def.Timeout == 0 {
		def.Timeout = 10
	}
	if def.InputSchema == nil {
		def.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", filepath.Base(path), err)
	}
	return &def, nil
}

// LoadToolsDir scans dir for .yaml/.yml files and returns parsed definitions.
// Malformed files are collected as errors but do not stop loading.
func LoadToolsDir(dir string) ([]*Definition, []error) {
	var defs []*Definition
	var errs []error

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil // silently ignore missing directories
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		def, parseErr := ParseDefinition(path)
		if parseErr != nil {
			errs = append(errs, parseErr)
			return nil
		}
		defs = append(defs, def)
		return nil
	})
	return defs, errs
}

// DefaultSearchPaths returns custom tool directories in priority order
// (lowest to highest): user-global, then project-local.
func DefaultSearchPaths(cwd string) []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".forge", "tools"),
		filepath.Join(cwd, ".forge", "tools"),
	}
}

// DiscoverTools loads custom tools from the given directories (or default paths
// if none provided), deduplicates by name (later directories win), and rejects
// tools whose names collide with builtinNames.
func DiscoverTools(cwd string, builtinNames map[string]bool, dirs ...string) ([]*Tool, []error) {
	if len(dirs) == 0 {
		dirs = DefaultSearchPaths(cwd)
	}

	seen := map[string]*Definition{}
	var allErrs []error

	for _, dir := range dirs {
		defs, errs := LoadToolsDir(dir)
		allErrs = append(allErrs, errs...)
		for _, d := range defs {
			seen[d.Name] = d // later wins (project-local overrides user-global)
		}
	}

	var tools []*Tool
	for _, d := range seen {
		if builtinNames != nil && builtinNames[d.Name] {
			allErrs = append(allErrs, fmt.Errorf("custom tool %q skipped: conflicts with built-in tool", d.Name))
			continue
		}
		tools = append(tools, New(d))
	}
	return tools, allErrs
}
