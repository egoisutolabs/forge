package toolsearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// ---- helpers ----------------------------------------------------------------

// fakeTool is a minimal deferred tool for testing.
type fakeTool struct {
	name string
	desc string
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return f.desc }
func (f *fakeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`)
}
func (f *fakeTool) ShouldDefer() bool { return true }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: f.name}, nil
}
func (f *fakeTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (f *fakeTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (f *fakeTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (f *fakeTool) IsReadOnly(_ json.RawMessage) bool        { return true }

// loadedTool does NOT implement Deferrable — always loaded.
type loadedTool struct{ fakeTool }

func (l *loadedTool) ShouldDefer() bool { return false }

func tctxWithTools(ts ...tools.Tool) *tools.ToolContext {
	return &tools.ToolContext{Tools: ts}
}

func mustInput(query string, maxResults int) json.RawMessage {
	in := toolInput{Query: query, MaxResults: maxResults}
	b, _ := json.Marshal(in)
	return b
}

func exec(t *testing.T, tctx *tools.ToolContext, query string, maxResults int) string {
	t.Helper()
	tool := &Tool{}
	res, err := tool.Execute(context.Background(), mustInput(query, maxResults), tctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return res.Content
}

// ---- ToolSearch metadata ----------------------------------------------------

func TestToolSearch_Name(t *testing.T) {
	if (&Tool{}).Name() != ToolName {
		t.Errorf("expected name %q", ToolName)
	}
}

func TestToolSearch_IsConcurrencySafe(t *testing.T) {
	if !(&Tool{}).IsConcurrencySafe(nil) {
		t.Error("expected concurrency safe")
	}
}

func TestToolSearch_IsReadOnly(t *testing.T) {
	if !(&Tool{}).IsReadOnly(nil) {
		t.Error("expected read-only")
	}
}

func TestToolSearch_ValidateInput_EmptyQuery(t *testing.T) {
	err := (&Tool{}).ValidateInput(json.RawMessage(`{"query":""}`))
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestToolSearch_ValidateInput_Valid(t *testing.T) {
	if err := (&Tool{}).ValidateInput(json.RawMessage(`{"query":"bash"}`)); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestToolSearch_CheckPermissions(t *testing.T) {
	d, err := (&Tool{}).CheckPermissions(nil, nil)
	if err != nil || d.Behavior != models.PermAllow {
		t.Errorf("unexpected: %v %v", d, err)
	}
}

// ---- select: fast path ------------------------------------------------------

func TestToolSearch_Select_Single(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "run bash commands"},
		&fakeTool{name: "GlobTool", desc: "find files"},
	)
	out := exec(t, tctx, "select:BashTool", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool in output, got: %s", out)
	}
	if strings.Contains(out, "GlobTool") {
		t.Errorf("GlobTool should not appear in output")
	}
}

func TestToolSearch_Select_Multi(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "run bash"},
		&fakeTool{name: "GlobTool", desc: "find files"},
		&fakeTool{name: "ReadTool", desc: "read files"},
	)
	out := exec(t, tctx, "select:BashTool,GlobTool", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool in output")
	}
	if !strings.Contains(out, "GlobTool") {
		t.Errorf("expected GlobTool in output")
	}
	if strings.Contains(out, "ReadTool") {
		t.Errorf("ReadTool should not appear")
	}
}

func TestToolSearch_Select_CaseInsensitive(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:bashtool", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool (case-insensitive match), got: %s", out)
	}
}

func TestToolSearch_Select_MissingTool(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:DoesNotExist", 5)
	if out != "No matching deferred tools found" {
		t.Errorf("expected not-found message, got: %s", out)
	}
}

func TestToolSearch_Select_PartialMissing(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	// One exists, one doesn't — should return the one that exists.
	out := exec(t, tctx, "select:BashTool,DoesNotExist", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool, got: %s", out)
	}
}

func TestToolSearch_Select_LoadedToolIncluded(t *testing.T) {
	// If a tool is already loaded (not deferred), select: still finds it.
	loaded := &loadedTool{fakeTool{name: "AlwaysLoaded", desc: "always here"}}
	tctx := tctxWithTools(loaded, &fakeTool{name: "DeferredOne", desc: "deferred"})
	out := exec(t, tctx, "select:AlwaysLoaded", 5)
	if !strings.Contains(out, "AlwaysLoaded") {
		t.Errorf("expected AlwaysLoaded, got: %s", out)
	}
}

// ---- exact match fast path --------------------------------------------------

func TestToolSearch_ExactMatch(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "run bash commands"},
		&fakeTool{name: "GlobTool", desc: "find files"},
	)
	out := exec(t, tctx, "bashtool", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool in output, got: %s", out)
	}
}

// ---- keyword search ---------------------------------------------------------

func TestToolSearch_Keyword_NameMatch(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "executes shell commands"},
		&fakeTool{name: "GlobTool", desc: "pattern file matching"},
		&fakeTool{name: "FileReadTool", desc: "read file contents"},
	)
	out := exec(t, tctx, "bash", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool in output, got: %s", out)
	}
}

func TestToolSearch_Keyword_DescriptionMatch(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "ToolA", desc: "this searches for patterns"},
		&fakeTool{name: "ToolB", desc: "unrelated functionality"},
	)
	out := exec(t, tctx, "patterns", 5)
	if !strings.Contains(out, "ToolA") {
		t.Errorf("expected ToolA (desc match), got: %s", out)
	}
}

func TestToolSearch_Keyword_RequiredTerms(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "FileReadTool", desc: "read files"},
		&fakeTool{name: "FileWriteTool", desc: "write files"},
		&fakeTool{name: "BashTool", desc: "run commands"},
	)
	// +file requires "file" in name or description; "read" is optional scoring term.
	out := exec(t, tctx, "+file read", 5)
	if !strings.Contains(out, "FileReadTool") {
		t.Errorf("expected FileReadTool, got: %s", out)
	}
	// BashTool has no "file" anywhere — must be excluded.
	if strings.Contains(out, "BashTool") {
		t.Errorf("BashTool should be excluded by +file, got: %s", out)
	}
}

func TestToolSearch_Keyword_RequiredTermFiltersAll(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "run bash"},
	)
	out := exec(t, tctx, "+network", 5)
	if out != "No matching deferred tools found" {
		t.Errorf("expected no match, got: %s", out)
	}
}

func TestToolSearch_Keyword_MaxResults(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "FileReadTool", desc: "read files"},
		&fakeTool{name: "FileWriteTool", desc: "write files"},
		&fakeTool{name: "FileEditTool", desc: "edit files"},
		&fakeTool{name: "FileCopyTool", desc: "copy files"},
		&fakeTool{name: "FileDeleteTool", desc: "delete files"},
	)
	out := exec(t, tctx, "file", 2)
	// Count how many tool names appear in output — should be at most 2.
	count := 0
	for _, name := range []string{"FileReadTool", "FileWriteTool", "FileEditTool", "FileCopyTool", "FileDeleteTool"} {
		if strings.Contains(out, name) {
			count++
		}
	}
	if count > 2 {
		t.Errorf("expected at most 2 results, got %d: %s", count, out)
	}
}

func TestToolSearch_Keyword_NoMatch(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "xyzzy", 5)
	if out != "No matching deferred tools found" {
		t.Errorf("expected no match, got: %s", out)
	}
}

func TestToolSearch_Keyword_DefaultMaxResults(t *testing.T) {
	// max_results=0 should default to 5.
	tctx := tctxWithTools(
		&fakeTool{name: "FileRead", desc: "read a file"},
		&fakeTool{name: "FileWrite", desc: "write a file"},
		&fakeTool{name: "FileEdit", desc: "edit a file"},
		&fakeTool{name: "FileCopy", desc: "copy a file"},
		&fakeTool{name: "FileDelete", desc: "delete a file"},
		&fakeTool{name: "FileMove", desc: "move a file"},
	)
	out := exec(t, tctx, "file", 0) // 0 → default 5
	count := strings.Count(out, "<function>")
	if count > 5 {
		t.Errorf("expected at most 5 results with default max_results, got %d", count)
	}
}

// ---- output format ----------------------------------------------------------

func TestToolSearch_OutputContainsFunctionsBlock(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:BashTool", 5)
	if !strings.Contains(out, "<functions>") {
		t.Errorf("output must contain <functions>, got: %s", out)
	}
	if !strings.HasSuffix(out, "</functions>") {
		t.Errorf("output must end with </functions>, got: %s", out)
	}
}

func TestToolSearch_OutputContainsFunctionTag(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:BashTool", 5)
	if !strings.Contains(out, "<function>") || !strings.Contains(out, "</function>") {
		t.Errorf("expected <function> tags in output, got: %s", out)
	}
}

func TestToolSearch_OutputContainsInputSchema(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:BashTool", 5)
	if !strings.Contains(out, "input_schema") {
		t.Errorf("expected input_schema in output, got: %s", out)
	}
}

func TestToolSearch_OutputIsValidJSON(t *testing.T) {
	tctx := tctxWithTools(&fakeTool{name: "BashTool", desc: "run bash"})
	out := exec(t, tctx, "select:BashTool", 5)
	// Extract the JSON from between <function> and </function>.
	start := strings.Index(out, "<function>") + len("<function>")
	end := strings.Index(out, "</function>")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not find <function> tags in: %s", out)
	}
	raw := out[start:end]
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Errorf("function body is not valid JSON: %v\n%s", err, raw)
	}
	if v["name"] != "BashTool" {
		t.Errorf("expected name=BashTool, got %v", v["name"])
	}
}

// ---- nil / empty tctx -------------------------------------------------------

func TestToolSearch_NilTctx(t *testing.T) {
	tool := &Tool{}
	res, err := tool.Execute(context.Background(), mustInput("bash", 5), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != "No matching deferred tools found" {
		t.Errorf("expected not-found for nil tctx, got: %s", res.Content)
	}
}

func TestToolSearch_EmptyToolList(t *testing.T) {
	tctx := tctxWithTools() // no tools
	out := exec(t, tctx, "bash", 5)
	if out != "No matching deferred tools found" {
		t.Errorf("expected not-found for empty tool list, got: %s", out)
	}
}

// ---- parseNameParts ---------------------------------------------------------

func TestParseNameParts_CamelCase(t *testing.T) {
	cases := map[string][]string{
		"BashTool":     {"bash", "tool"},
		"FileReadTool": {"file", "read", "tool"},
		"Glob":         {"glob"},
		"ToolSearch":   {"tool", "search"},
	}
	for name, want := range cases {
		got := parseNameParts(name)
		if !equalSlices(got, want) {
			t.Errorf("parseNameParts(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseNameParts_SnakeCase(t *testing.T) {
	got := parseNameParts("my_tool_name")
	want := []string{"my", "tool", "name"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseNameParts_MCP(t *testing.T) {
	got := parseNameParts("mcp__slack__send_message")
	// slack, send, message
	if len(got) == 0 {
		t.Error("expected non-empty parts for MCP tool")
	}
	if !containsAll(got, "slack", "send", "message") {
		t.Errorf("expected slack/send/message in parts, got %v", got)
	}
}

// ---- helpers for tests ------------------------------------------------------

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsAll(parts []string, terms ...string) bool {
	for _, term := range terms {
		found := false
		for _, p := range parts {
			if p == term {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ---- SearchHint scoring -----------------------------------------------------

// hintTool extends fakeTool with SearchHint support.
type hintTool struct {
	fakeTool
	hint string
}

func (h *hintTool) SearchHint() string { return h.hint }

func TestToolSearch_SearchHint_BoostsScore(t *testing.T) {
	// Tool with matching hint should rank above tool with only description match.
	hinted := &hintTool{
		fakeTool: fakeTool{name: "SlackNotify", desc: "sends a message"},
		hint:     "notify slack channel post",
	}
	plain := &fakeTool{name: "EmailSend", desc: "sends a notify email message"}

	tctx := tctxWithTools(hinted, plain)
	out := exec(t, tctx, "notify", 2)

	// Both appear, but hinted tool should rank first (higher score).
	slackIdx := strings.Index(out, "SlackNotify")
	emailIdx := strings.Index(out, "EmailSend")
	if slackIdx < 0 {
		t.Errorf("expected SlackNotify in output, got: %s", out)
	}
	if emailIdx < 0 {
		t.Errorf("expected EmailSend in output, got: %s", out)
	}
	if slackIdx > emailIdx {
		t.Errorf("SearchHint tool should rank before description-only match; slack=%d email=%d", slackIdx, emailIdx)
	}
}

func TestToolSearch_SearchHint_NoHintNoBonus(t *testing.T) {
	// Tool without SearchHint interface gets no hint bonus.
	plain := &fakeTool{name: "BashTool", desc: "run bash commands"}
	tctx := tctxWithTools(plain)
	out := exec(t, tctx, "bash", 5)
	if !strings.Contains(out, "BashTool") {
		t.Errorf("expected BashTool in output, got: %s", out)
	}
}

// ---- total_deferred_tools in output -----------------------------------------

func TestToolSearch_TotalDeferredToolsInOutput(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "ToolA", desc: "a"},
		&fakeTool{name: "ToolB", desc: "b"},
		&fakeTool{name: "ToolC", desc: "c"},
	)
	out := exec(t, tctx, "select:ToolA", 5)
	if !strings.Contains(out, "total_deferred_tools: 3") {
		t.Errorf("expected 'total_deferred_tools: 3' in output, got: %s", out)
	}
}

func TestToolSearch_TotalDeferredToolsKeywordSearch(t *testing.T) {
	tctx := tctxWithTools(
		&fakeTool{name: "BashTool", desc: "run bash commands"},
		&fakeTool{name: "GlobTool", desc: "find files"},
	)
	out := exec(t, tctx, "bash", 5)
	if !strings.Contains(out, "total_deferred_tools: 2") {
		t.Errorf("expected 'total_deferred_tools: 2' in output, got: %s", out)
	}
}

func TestToolSearch_TotalDeferredTools_ExcludesLoadedTools(t *testing.T) {
	// loaded tools (ShouldDefer=false) should not count toward total_deferred_tools.
	loaded := &loadedTool{fakeTool{name: "AlwaysLoaded", desc: "loaded"}}
	deferred1 := &fakeTool{name: "DeferredA", desc: "deferred a"}
	deferred2 := &fakeTool{name: "DeferredB", desc: "deferred b"}

	tctx := tctxWithTools(loaded, deferred1, deferred2)
	out := exec(t, tctx, "select:DeferredA", 5)
	if !strings.Contains(out, "total_deferred_tools: 2") {
		t.Errorf("expected 'total_deferred_tools: 2' (loaded excluded), got: %s", out)
	}
}
