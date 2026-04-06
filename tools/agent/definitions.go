// Package agent implements AgentTool — the Go port of Claude Code's AgentTool.
// It allows the model to spawn sub-agents to handle complex, multi-step tasks.
package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AgentDefinition describes a named agent configuration.
type AgentDefinition struct {
	Name            string
	Description     string
	Tools           []string // allowed tool names (nil/empty = inherit all)
	DisallowedTools []string // tool names to block
	Model           string   // model alias: "sonnet", "opus", "haiku", or full ID
	PermissionMode  string   // e.g. "default", "plan"
	SystemPrompt    string
	MaxTurns        int
	Background      bool // always run as background task
}

const exploreSystemPrompt = `You are a fast code exploration agent specialized for exploring codebases. Use the available tools to search for files, read code, and answer questions about the codebase. Prefer AstGrep for structural code search, and fall back to Grep for plain-text or regex matching. Be thorough but efficient. When you have found the answer, provide a clear and concise response.

Available tools: Glob (find files by pattern), AstGrep (structural code search), Grep (plain-text search), Read (read files), Bash (run commands).

When calling this agent, specify the desired thoroughness level in your prompt: "quick" for basic searches, "medium" for moderate exploration, or "very thorough" for comprehensive analysis.`

const planSystemPrompt = `You are a software architect agent for designing implementation plans. Use the available tools to explore the codebase, understand the existing architecture, and return step-by-step implementation plans. Prefer AstGrep for code-aware searches, and use Grep only when you need plain-text or regex matching.

Focus on:
- Identifying critical files that need to change
- Architectural trade-offs
- Concrete, actionable steps
- Potential risks or complications

Return a clear plan that another agent can execute.`

// BuiltInAgents returns the hardcoded built-in agent definitions.
func BuiltInAgents() []AgentDefinition {
	return []AgentDefinition{
		{
			Name:         "Explore",
			Description:  "Fast agent specialized for exploring codebases. Use this when you need to quickly find files by patterns, search code structure or keywords, or answer questions about the codebase.",
			Tools:        []string{"Read", "Glob", "AstGrep", "Grep", "Bash"},
			SystemPrompt: exploreSystemPrompt,
			MaxTurns:     30,
		},
		{
			Name:         "Plan",
			Description:  "Software architect agent for designing implementation plans. Use this when you need to plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs.",
			Tools:        []string{"Read", "Glob", "AstGrep", "Grep", "Bash"},
			SystemPrompt: planSystemPrompt,
			MaxTurns:     30,
		},
	}
}

// FindAgent returns the AgentDefinition with the given name, or nil.
// Name matching is case-insensitive.
func FindAgent(agents []AgentDefinition, name string) *AgentDefinition {
	for i := range agents {
		if strings.EqualFold(agents[i].Name, name) {
			return &agents[i]
		}
	}
	return nil
}

// LoadAgentsDir scans dir for .md files with YAML frontmatter and returns
// all agent definitions (built-ins + any found in dir).
// If dir does not exist, only built-ins are returned (no error).
func LoadAgentsDir(dir string) ([]AgentDefinition, error) {
	agents := BuiltInAgents()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return agents, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		def, err := loadAgentFile(path)
		if err != nil {
			continue // skip malformed files
		}
		agents = append(agents, def)
	}
	return agents, nil
}

// loadAgentFile parses a single agent markdown file with YAML frontmatter.
// The file must begin with a --- delimiter. The body after the closing ---
// becomes the system prompt if no system_prompt key is in the frontmatter.
func loadAgentFile(path string) (AgentDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return AgentDefinition{}, err
	}
	defer f.Close()

	var fmLines []string
	var bodyLines []string

	scanner := bufio.NewScanner(f)

	// Skip leading blank lines; first non-blank must be ---
	foundOpen := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundOpen = true
			break
		}
		if strings.TrimSpace(line) != "" {
			return AgentDefinition{}, fmt.Errorf("missing frontmatter delimiter in %s", path)
		}
	}
	if !foundOpen {
		return AgentDefinition{}, fmt.Errorf("missing frontmatter delimiter in %s", path)
	}

	// Read frontmatter until closing ---
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

	def := parseFrontmatter(fmLines)
	if def.Name == "" {
		def.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if def.SystemPrompt == "" {
		def.SystemPrompt = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	}
	return def, nil
}

// parseFrontmatter parses YAML frontmatter lines into an AgentDefinition.
// Supports scalar values and YAML list items ("- value").
func parseFrontmatter(lines []string) AgentDefinition {
	var def AgentDefinition
	var currentKey string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// YAML list item under the previous key
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			switch currentKey {
			case "tools":
				def.Tools = append(def.Tools, val)
			case "disallowed_tools", "disallowedTools":
				def.DisallowedTools = append(def.DisallowedTools, val)
			}
			continue
		}

		// key: value pair
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])
		currentKey = key

		switch key {
		case "name":
			def.Name = val
		case "description":
			def.Description = val
		case "model":
			def.Model = val
		case "permission_mode", "permissionMode":
			def.PermissionMode = val
		case "max_turns", "maxTurns":
			if n, err := strconv.Atoi(val); err == nil {
				def.MaxTurns = n
			}
		case "background":
			def.Background = strings.EqualFold(val, "true")
		case "system_prompt", "systemPrompt":
			def.SystemPrompt = val
		case "tools":
			// Inline comma-separated or start of YAML list (val empty)
			if val != "" {
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						def.Tools = append(def.Tools, t)
					}
				}
			}
		case "disallowed_tools", "disallowedTools":
			if val != "" {
				for _, t := range strings.Split(val, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						def.DisallowedTools = append(def.DisallowedTools, t)
					}
				}
			}
		}
	}
	return def
}
