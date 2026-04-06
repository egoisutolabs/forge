// Package agents embeds the forge phase-worker agent prompt definitions.
// Each .md file is a markdown document with YAML frontmatter that defines
// the agent's name, description, and model, plus a body that becomes the
// system prompt (with {{contracts.*}} placeholders resolved at load time).
package agents

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/egoisutolabs/forge/orchestrator/contracts"
)

//go:embed *.md
var agentFS embed.FS

// AgentDef is a fully resolved agent definition ready to use as a system prompt.
type AgentDef struct {
	// Name is the canonical agent identifier (e.g. "forge-plan").
	Name string

	// Description is a short human-readable summary of when to use this agent.
	Description string

	// Model is the model hint from the frontmatter ("inherit", "sonnet", etc.).
	// "inherit" (the default) means the phase runner uses the caller's current model.
	Model string

	// Prompt is the markdown body with {{contracts.*}} placeholders resolved
	// to their full embedded content.
	Prompt string
}

// LoadAgents loads all embedded .md files, parses their frontmatter, and
// resolves {{contracts.NAME}} placeholders by embedding the corresponding
// contract content. Returns an error if any file cannot be parsed.
func LoadAgents() ([]AgentDef, error) {
	var defs []AgentDef
	entries, err := fs.ReadDir(agentFS, ".")
	if err != nil {
		return nil, fmt.Errorf("agents: read embedded dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := agentFS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("agents: read %s: %w", e.Name(), err)
		}
		def, err := parseAgentDef(e.Name(), data)
		if err != nil {
			return nil, fmt.Errorf("agents: parse %s: %w", e.Name(), err)
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// FindAgent returns the AgentDef whose Name matches (case-insensitive), or nil.
// Callers should strip the .md extension before calling: use "forge-plan" not "forge-plan.md".
func FindAgent(defs []AgentDef, name string) *AgentDef {
	for i := range defs {
		if strings.EqualFold(defs[i].Name, name) {
			return &defs[i]
		}
	}
	return nil
}

// parseAgentDef parses a single agent markdown file.
// Format: YAML frontmatter (--- ... ---) followed by markdown body.
// The body becomes Prompt after {{contracts.NAME}} substitution.
func parseAgentDef(filename string, data []byte) (AgentDef, error) {
	var fmLines, bodyLines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))

	// Expect the file to open with a --- delimiter (allow leading blank lines).
	foundOpen := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundOpen = true
			break
		}
		if strings.TrimSpace(line) != "" {
			return AgentDef{}, fmt.Errorf("missing frontmatter in %s", filename)
		}
	}
	if !foundOpen {
		return AgentDef{}, fmt.Errorf("missing frontmatter delimiter in %s", filename)
	}

	// Collect frontmatter lines until the closing ---.
	inFM := true
	for scanner.Scan() {
		line := scanner.Text()
		if inFM && strings.TrimSpace(line) == "---" {
			inFM = false
			continue
		}
		if inFM {
			fmLines = append(fmLines, line)
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return AgentDef{}, fmt.Errorf("scan %s: %w", filename, err)
	}

	def := parseFrontmatter(fmLines)
	if def.Name == "" {
		def.Name = strings.TrimSuffix(filename, ".md")
	}
	if def.Model == "" {
		def.Model = "inherit"
	}

	body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
	def.Prompt = resolveContracts(body)
	return def, nil
}

// parseFrontmatter extracts name, description, and model from YAML-like lines.
func parseFrontmatter(lines []string) AgentDef {
	var def AgentDef
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		switch key {
		case "name":
			def.Name = val
		case "description":
			def.Description = val
		case "model":
			def.Model = val
		}
	}
	return def
}

// resolveContracts replaces every {{contracts.NAME}} occurrence in text with
// the embedded content of contracts/NAME.md. Unresolved placeholders (contract
// not found) are replaced with an empty string so the prompt is always clean.
func resolveContracts(text string) string {
	const prefix = "{{contracts."
	const suffix = "}}"
	for {
		start := strings.Index(text, prefix)
		if start < 0 {
			break
		}
		rest := text[start+len(prefix):]
		end := strings.Index(rest, suffix)
		if end < 0 {
			break
		}
		name := rest[:end]
		content := contracts.ContractFor(name)
		text = text[:start] + content + rest[end+len(suffix):]
	}
	return text
}
