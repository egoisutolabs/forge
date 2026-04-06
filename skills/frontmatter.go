package skills

import (
	"strings"
)

// Frontmatter holds the parsed YAML frontmatter fields from a skill file.
type Frontmatter struct {
	Description   string
	WhenToUse     string
	AllowedTools  []string
	UserInvocable bool // default true
	Context       ContextMode
}

// ParseFrontmatter splits a markdown document into its YAML frontmatter
// and body. The document must begin with "---\n"; if it does not, an empty
// Frontmatter and the full document as body are returned.
//
// Supported frontmatter keys:
//
//	description:   short description string
//	when-to-use:   hint for when to invoke this skill
//	allowed-tools: inline list [A, B] or multi-line with "- item" entries
//	user-invocable: true|false  (default: true)
//	context:       inline|fork  (default: inline)
func ParseFrontmatter(doc string) (fm Frontmatter, body string) {
	fm.UserInvocable = true
	fm.Context = ContextInline

	const marker = "---"
	if !strings.HasPrefix(doc, marker) {
		return fm, doc
	}

	// Skip the opening "---\n"
	rest := doc[len(marker):]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 0 && rest[0] == '\r' && len(rest) > 1 && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Find the closing "---". It may appear at the very start of rest (when
	// the frontmatter block is empty) or after a newline.
	var fmBlock, after string
	if strings.HasPrefix(rest, marker) {
		// Empty frontmatter: "---\n---\n..."
		fmBlock = ""
		after = rest[len(marker):]
	} else {
		end := strings.Index(rest, "\n"+marker)
		if end == -1 {
			// No closing marker — treat whole document as body
			return fm, doc
		}
		fmBlock = rest[:end]
		after = rest[end+1+len(marker):]
	}

	// Strip optional trailing newline from the closing "---" line.
	body = strings.TrimPrefix(after, "\r\n")
	body = strings.TrimPrefix(body, "\n")

	parseFrontmatterBlock(fmBlock, &fm)
	return fm, body
}

// parseFrontmatterBlock parses a YAML block into a Frontmatter struct.
// Handles scalar strings, booleans, inline lists, and multi-line lists.
func parseFrontmatterBlock(block string, fm *Frontmatter) {
	lines := strings.Split(block, "\n")
	var currentKey string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Strip trailing \r for Windows line endings
		line = strings.TrimRight(line, "\r")

		// Multi-line list item: "  - value"
		if strings.HasPrefix(line, "  - ") || strings.HasPrefix(line, "- ") {
			item := strings.TrimPrefix(line, "  - ")
			item = strings.TrimPrefix(item, "- ")
			item = strings.Trim(item, `"'`)
			if currentKey == "allowed-tools" {
				fm.AllowedTools = append(fm.AllowedTools, item)
			}
			continue
		}

		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}

		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		currentKey = key

		switch key {
		case "description":
			fm.Description = unquote(value)

		case "when-to-use", "whenToUse", "when_to_use":
			fm.WhenToUse = unquote(value)

		case "allowed-tools", "allowedTools", "allowed_tools":
			if strings.HasPrefix(value, "[") {
				fm.AllowedTools = parseInlineList(value)
			}
			// If value is empty, multi-line list items follow (handled above)

		case "user-invocable", "userInvocable", "user_invocable":
			fm.UserInvocable = parseBool(value, true)

		case "context":
			switch strings.ToLower(unquote(value)) {
			case "fork":
				fm.Context = ContextFork
			default:
				fm.Context = ContextInline
			}
		}
	}
}

// parseInlineList parses a YAML inline list like "[Bash, Read, Glob]".
func parseInlineList(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseBool parses a YAML boolean value, returning def on unrecognised input.
func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	}
	return def
}

// unquote strips surrounding single or double quotes from a YAML scalar value.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
