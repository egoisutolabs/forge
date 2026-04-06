// Package toolsearch implements ToolSearchTool — the Go port of Claude Code's
// ToolSearchTool. It fetches full JSON schemas for deferred tools so the model
// can invoke them after an initial prompt that listed only their names.
package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// ToolName is the canonical name used in the Claude API and system prompts.
const ToolName = "ToolSearch"

const defaultMaxResults = 5

const description = `Fetches full schema definitions for deferred tools so they can be called.

Deferred tools appear by name in <system-reminder> messages. Until fetched, only the name is known — there is no parameter schema, so the tool cannot be invoked. This tool takes a query, matches it against the deferred tool list, and returns the matched tools' complete JSONSchema definitions inside a <functions> block. Once a tool's schema appears in that result, it is callable exactly like any tool defined at the top of the prompt.

Result format: each matched tool appears as one <function>{"description": "...", "name": "...", "parameters": {...}}</function> line inside the <functions> block — the same encoding as the tool list at the top of this prompt.

Query forms:
- "select:Read,Edit,Grep" — fetch these exact tools by name
- "notebook jupyter" — keyword search, up to max_results best matches
- "+slack send" — require "slack" in the name, rank by remaining terms`

// Tool implements the ToolSearch tool.
type Tool struct{}

type toolInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (t *Tool) Name() string        { return ToolName }
func (t *Tool) Description() string { return description }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Query to find deferred tools. Use \"select:<tool_name>\" for direct selection, or keywords to search."
			},
			"max_results": {
				"type": "number",
				"description": "Maximum number of results to return (default: 5)"
			}
		},
		"required": ["query"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.Query) == "" {
		return fmt.Errorf("query is required and cannot be empty")
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(_ context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	var allTools []tools.Tool
	if tctx != nil {
		allTools = tctx.Tools
	}

	_, deferred := tools.SplitTools(allTools)
	totalDeferred := len(deferred)

	withHeader := func(schemas string) string {
		return fmt.Sprintf("total_deferred_tools: %d\n\n%s", totalDeferred, schemas)
	}

	// select: fast path — direct selection by name, supports comma-separated multi-select.
	if after, ok := strings.CutPrefix(strings.ToLower(in.Query), "select:"); ok {
		names := strings.Split(in.Query[len("select:"):], ",")
		if after == "" {
			names = nil
		}
		var found []tools.Tool
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			// Deferred first, then all tools (already-loaded tool is harmless).
			if tool := findByName(deferred, name); tool != nil {
				found = append(found, tool)
			} else if tool := findByName(allTools, name); tool != nil {
				found = append(found, tool)
			}
		}
		if len(found) == 0 {
			return &models.ToolResult{Content: "No matching deferred tools found"}, nil
		}
		return &models.ToolResult{Content: withHeader(renderSchemas(found))}, nil
	}

	// Exact name match fast path (deferred first, then all).
	queryLower := strings.ToLower(strings.TrimSpace(in.Query))
	if exact := findByNameLower(deferred, queryLower); exact != nil {
		return &models.ToolResult{Content: withHeader(renderSchemas([]tools.Tool{exact}))}, nil
	}
	if exact := findByNameLower(allTools, queryLower); exact != nil {
		return &models.ToolResult{Content: withHeader(renderSchemas([]tools.Tool{exact}))}, nil
	}

	// Keyword search with scoring.
	matches := searchByKeywords(in.Query, deferred, maxResults)
	if len(matches) == 0 {
		return &models.ToolResult{Content: "No matching deferred tools found"}, nil
	}
	return &models.ToolResult{Content: withHeader(renderSchemas(matches))}, nil
}

// findByName does a case-insensitive name lookup in a tool slice.
func findByName(ts []tools.Tool, name string) tools.Tool {
	lower := strings.ToLower(name)
	for _, t := range ts {
		if strings.ToLower(t.Name()) == lower {
			return t
		}
	}
	return nil
}

// findByNameLower looks up a tool whose lowercase name equals queryLower.
func findByNameLower(ts []tools.Tool, queryLower string) tools.Tool {
	for _, t := range ts {
		if strings.ToLower(t.Name()) == queryLower {
			return t
		}
	}
	return nil
}

// renderSchemas formats matched tools as a <functions> block containing
// each tool's full JSON schema definition.
func renderSchemas(matched []tools.Tool) string {
	var b strings.Builder
	b.WriteString("<functions>\n")
	for _, t := range matched {
		entry := map[string]any{
			"name":         t.Name(),
			"description":  t.Description(),
			"input_schema": t.InputSchema(),
		}
		data, _ := json.Marshal(entry)
		b.WriteString("<function>")
		b.Write(data)
		b.WriteString("</function>\n")
	}
	b.WriteString("</functions>")
	return b.String()
}

// parseNameParts splits a tool name into lowercase search tokens.
//
//   - CamelCase → ["file", "read", "tool"]
//   - snake_case → ["my", "tool"]
//   - mcp__server__action → ["server", "action"]
func parseNameParts(name string) []string {
	if strings.HasPrefix(name, "mcp__") {
		rest := strings.TrimPrefix(name, "mcp__")
		var parts []string
		for _, seg := range strings.Split(rest, "__") {
			for _, p := range strings.Split(seg, "_") {
				if p != "" {
					parts = append(parts, strings.ToLower(p))
				}
			}
		}
		return parts
	}

	// CamelCase + underscore/hyphen split.
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for i, r := range name {
		switch {
		case r == '_' || r == '-':
			flush()
		case i > 0 && unicode.IsUpper(r):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return parts
}

type scored struct {
	tool  tools.Tool
	score int
}

// searchByKeywords performs keyword-based scoring over deferred tools.
//
// Term prefixes:
//   - "+term"  → required; tools that don't match ALL required terms are filtered
//   - "term"   → optional; contribute to score but don't gate inclusion
//
// Scoring per term:
//   - Exact name-part match: +10
//   - Partial name-part match (substring): +5
//   - Full-name substring fallback (when score is still 0): +3
//   - Description substring match: +2
func searchByKeywords(query string, deferred tools.DeferredToolSet, maxResults int) []tools.Tool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	rawTerms := strings.Fields(queryLower)
	if len(rawTerms) == 0 {
		return nil
	}

	var required, optional []string
	for _, term := range rawTerms {
		if strings.HasPrefix(term, "+") && len(term) > 1 {
			required = append(required, term[1:])
		} else {
			optional = append(optional, term)
		}
	}

	// All terms used for scoring; when there are required terms, only they
	// and optionals are scored (the TS reference behaviour).
	scoringTerms := rawTerms
	if len(required) > 0 {
		scoringTerms = append(required, optional...)
	}

	var results []scored
	for _, t := range deferred {
		nameParts := parseNameParts(t.Name())
		fullLower := strings.ToLower(t.Name())
		descLower := strings.ToLower(t.Description())

		// Filter: ALL required terms must appear in name or description.
		if len(required) > 0 {
			ok := true
			for _, req := range required {
				if !termInParts(req, nameParts) && !strings.Contains(descLower, req) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}

		var score int
		// SearchHint bonus: if the tool advertises a hint that contains a query
		// term, add +4 per matching term (on top of name/description scoring).
		var hintLower string
		if sh, ok := t.(tools.SearchHinter); ok {
			hintLower = strings.ToLower(sh.SearchHint())
		}
		for _, term := range scoringTerms {
			// Name-part scoring.
			if termExactInParts(term, nameParts) {
				score += 10
			} else if termInParts(term, nameParts) {
				score += 5
			}
			// Full-name fallback when there's no part match yet.
			if score == 0 && strings.Contains(fullLower, term) {
				score += 3
			}
			// Description match.
			if strings.Contains(descLower, term) {
				score += 2
			}
			// SearchHint match: +4 per term.
			if hintLower != "" && strings.Contains(hintLower, term) {
				score += 4
			}
		}

		if score > 0 {
			results = append(results, scored{tool: t, score: score})
		}
	}

	// Sort descending by score (stable to keep original order as tiebreaker).
	stableSort(results)

	if maxResults < len(results) {
		results = results[:maxResults]
	}

	out := make([]tools.Tool, len(results))
	for i, r := range results {
		out[i] = r.tool
	}
	return out
}

// termExactInParts returns true if any part equals term exactly.
func termExactInParts(term string, parts []string) bool {
	for _, p := range parts {
		if p == term {
			return true
		}
	}
	return false
}

// termInParts returns true if any part contains term or term contains part.
func termInParts(term string, parts []string) bool {
	for _, p := range parts {
		if strings.Contains(p, term) || strings.Contains(term, p) {
			return true
		}
	}
	return false
}

// stableSort sorts scored entries descending by score.
// Ties preserve the original insertion order (stable sort).
func stableSort(s []scored) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].score > s[j].score })
}
