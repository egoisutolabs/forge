package grep

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// hasFlagValue returns true when flag is immediately followed by value in args.
func hasFlagValue(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

// ── buildRgArgs ───────────────────────────────────────────────────────────────

func TestBuildRgArgs_DefaultsFilesWithMatches(t *testing.T) {
	in := toolInput{Pattern: "foo"}
	args, _ := buildRgArgs(in, "/tmp")

	if !hasFlag(args, "--hidden") {
		t.Error("expected --hidden flag")
	}
	if !hasFlag(args, "-l") {
		t.Error("expected -l for files_with_matches mode")
	}
	if hasFlag(args, "-c") {
		t.Error("unexpected -c flag in files_with_matches mode")
	}
	if hasFlag(args, "-n") {
		t.Error("unexpected -n flag in files_with_matches mode")
	}
	if !hasFlagValue(args, "--max-columns", "500") {
		t.Error("expected --max-columns 500")
	}
	if !hasFlag(args, "foo") {
		t.Error("expected pattern 'foo' in args")
	}
}

func TestBuildRgArgs_VCSExclusions(t *testing.T) {
	in := toolInput{Pattern: "x"}
	args, _ := buildRgArgs(in, "/tmp")

	for _, dir := range vcsDirectories {
		if !hasFlagValue(args, "--glob", "!"+dir) {
			t.Errorf("expected --glob !%s in args", dir)
		}
	}
}

func TestBuildRgArgs_ContentMode(t *testing.T) {
	in := toolInput{Pattern: "foo", OutputMode: "content"}
	args, _ := buildRgArgs(in, "/tmp")

	// No -l or -c in content mode
	if hasFlag(args, "-l") {
		t.Error("unexpected -l in content mode")
	}
	if hasFlag(args, "-c") {
		t.Error("unexpected -c in content mode")
	}
	// Line numbers on by default in content mode
	if !hasFlag(args, "-n") {
		t.Error("expected -n in content mode (default)")
	}
}

func TestBuildRgArgs_ContentMode_NoLineNums(t *testing.T) {
	f := false
	in := toolInput{Pattern: "foo", OutputMode: "content", ShowLineNums: &f}
	args, _ := buildRgArgs(in, "/tmp")
	if hasFlag(args, "-n") {
		t.Error("unexpected -n when show_line_numbers=false")
	}
}

func TestBuildRgArgs_CountMode(t *testing.T) {
	in := toolInput{Pattern: "foo", OutputMode: "count"}
	args, _ := buildRgArgs(in, "/tmp")

	if !hasFlag(args, "-c") {
		t.Error("expected -c for count mode")
	}
	if hasFlag(args, "-l") {
		t.Error("unexpected -l in count mode")
	}
}

func TestBuildRgArgs_CaseInsensitive(t *testing.T) {
	tr := true
	in := toolInput{Pattern: "foo", CaseInsens: &tr}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlag(args, "-i") {
		t.Error("expected -i for case insensitive")
	}
}

func TestBuildRgArgs_Multiline(t *testing.T) {
	tr := true
	in := toolInput{Pattern: "foo", Multiline: &tr}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlag(args, "-U") {
		t.Error("expected -U for multiline")
	}
	if !hasFlag(args, "--multiline-dotall") {
		t.Error("expected --multiline-dotall for multiline")
	}
}

func TestBuildRgArgs_ContextLines_C(t *testing.T) {
	n := 3
	in := toolInput{Pattern: "foo", OutputMode: "content", ContextC: &n}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "-C", "3") {
		t.Error("expected -C 3")
	}
}

func TestBuildRgArgs_ContextLines_Context(t *testing.T) {
	// context field takes precedence over -C
	c := 5
	cc := 3
	in := toolInput{Pattern: "foo", OutputMode: "content", Context: &c, ContextC: &cc}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "-C", "5") {
		t.Error("expected -C 5 (context field takes precedence)")
	}
}

func TestBuildRgArgs_ContextLines_AB(t *testing.T) {
	b := 2
	a := 4
	in := toolInput{Pattern: "foo", OutputMode: "content", ContextB: &b, ContextA: &a}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "-B", "2") {
		t.Error("expected -B 2")
	}
	if !hasFlagValue(args, "-A", "4") {
		t.Error("expected -A 4")
	}
}

func TestBuildRgArgs_ContextLines_IgnoredInNonContentMode(t *testing.T) {
	n := 3
	in := toolInput{Pattern: "foo", OutputMode: "files_with_matches", ContextC: &n}
	args, _ := buildRgArgs(in, "/tmp")
	// Context flags should be ignored for non-content modes
	if hasFlagValue(args, "-C", "3") {
		t.Error("context flag should be ignored in files_with_matches mode")
	}
}

func TestBuildRgArgs_TypeFilter(t *testing.T) {
	in := toolInput{Pattern: "foo", Type: "go"}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "--type", "go") {
		t.Error("expected --type go")
	}
}

func TestBuildRgArgs_GlobFilter_Simple(t *testing.T) {
	in := toolInput{Pattern: "foo", Glob: "*.go"}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "--glob", "*.go") {
		t.Error("expected --glob *.go")
	}
}

func TestBuildRgArgs_GlobFilter_CommaSeparated(t *testing.T) {
	in := toolInput{Pattern: "foo", Glob: "*.go,*.ts"}
	args, _ := buildRgArgs(in, "/tmp")
	if !hasFlagValue(args, "--glob", "*.go") {
		t.Error("expected --glob *.go")
	}
	if !hasFlagValue(args, "--glob", "*.ts") {
		t.Error("expected --glob *.ts")
	}
}

func TestBuildRgArgs_GlobFilter_BracePattern(t *testing.T) {
	in := toolInput{Pattern: "foo", Glob: "*.{ts,tsx}"}
	args, _ := buildRgArgs(in, "/tmp")
	// Brace patterns must NOT be split on comma
	if !hasFlagValue(args, "--glob", "*.{ts,tsx}") {
		t.Error("expected brace pattern kept intact: *.{ts,tsx}")
	}
	// Must not create *.{ts or tsx} as separate globs
	for i, a := range args {
		if a == "--glob" && i+1 < len(args) {
			v := args[i+1]
			if v == "*.{ts" || v == "tsx}" {
				t.Errorf("brace pattern was incorrectly split: %q", v)
			}
		}
	}
}

func TestBuildRgArgs_PatternStartingWithDash(t *testing.T) {
	in := toolInput{Pattern: "-foo"}
	args, _ := buildRgArgs(in, "/tmp")
	// Pattern must appear as the value of -e (not standalone).
	if !hasFlagValue(args, "-e", "-foo") {
		t.Error("expected -e <pattern> for dash-prefixed pattern")
	}
	// Pattern must NOT appear without a preceding -e (i.e. not as a positional arg).
	for i, a := range args {
		if a == "-foo" && (i == 0 || args[i-1] != "-e") {
			t.Error("dash-prefixed pattern appears standalone (not after -e)")
		}
	}
}

func TestBuildRgArgs_SearchPath_Default(t *testing.T) {
	in := toolInput{Pattern: "foo"}
	_, searchPath := buildRgArgs(in, "/my/cwd")
	if searchPath != "/my/cwd" {
		t.Errorf("expected search path /my/cwd, got %q", searchPath)
	}
}

func TestBuildRgArgs_SearchPath_Absolute(t *testing.T) {
	in := toolInput{Pattern: "foo", Path: "/absolute/path"}
	_, searchPath := buildRgArgs(in, "/my/cwd")
	if searchPath != "/absolute/path" {
		t.Errorf("expected search path /absolute/path, got %q", searchPath)
	}
}

func TestBuildRgArgs_SearchPath_Relative(t *testing.T) {
	in := toolInput{Pattern: "foo", Path: "subdir"}
	_, searchPath := buildRgArgs(in, "/my/cwd")
	if searchPath != "/my/cwd/subdir" {
		t.Errorf("expected search path /my/cwd/subdir, got %q", searchPath)
	}
}

// ── applyHeadLimit ────────────────────────────────────────────────────────────

func TestApplyHeadLimit_NilLimit_UsesDefault(t *testing.T) {
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = "line"
	}
	result, truncated := applyHeadLimit(lines, nil, 0)
	if len(result) != defaultHeadLimit {
		t.Errorf("expected %d lines, got %d", defaultHeadLimit, len(result))
	}
	if !truncated {
		t.Error("expected truncated=true when 300 > 250")
	}
}

func TestApplyHeadLimit_ZeroLimit_Unlimited(t *testing.T) {
	lines := make([]string, 300)
	zero := 0
	result, truncated := applyHeadLimit(lines, &zero, 0)
	if len(result) != 300 {
		t.Errorf("expected all 300 lines, got %d", len(result))
	}
	if truncated {
		t.Error("expected truncated=false for unlimited (limit=0)")
	}
}

func TestApplyHeadLimit_WithOffset(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	limit := 3
	result, _ := applyHeadLimit(lines, &limit, 2)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if result[0] != "c" {
		t.Errorf("expected first result to be 'c', got %q", result[0])
	}
}

func TestApplyHeadLimit_NoTruncation(t *testing.T) {
	lines := []string{"a", "b", "c"}
	limit := 10
	result, truncated := applyHeadLimit(lines, &limit, 0)
	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if truncated {
		t.Error("expected truncated=false when result fits in limit")
	}
}

func TestApplyHeadLimit_OffsetBeyondEnd(t *testing.T) {
	lines := []string{"a", "b"}
	limit := 10
	result, _ := applyHeadLimit(lines, &limit, 100)
	if len(result) != 0 {
		t.Errorf("expected 0 lines when offset > len, got %d", len(result))
	}
}

// ── relativizePath ────────────────────────────────────────────────────────────

func TestRelativizePath(t *testing.T) {
	tests := []struct {
		abs, cwd, want string
	}{
		{"/a/b/c.go", "/a/b", "c.go"},
		{"/a/b/c/d.go", "/a/b", "c/d.go"},
		{"/a/b/c.go", "/x/y", "/a/b/c.go"}, // not under cwd → keep absolute
		{"/a/b/c.go", "/a/b/", "c.go"},     // trailing slash on cwd
	}
	for _, tt := range tests {
		got := relativizePath(tt.abs, tt.cwd)
		if got != tt.want {
			t.Errorf("relativizePath(%q, %q) = %q, want %q", tt.abs, tt.cwd, got, tt.want)
		}
	}
}

// ── JSON input deserialization ────────────────────────────────────────────────

func TestToolInput_JSONDeserialization(t *testing.T) {
	raw := `{
		"pattern": "foo",
		"output_mode": "content",
		"-B": 2,
		"-A": 3,
		"-C": 5,
		"-n": true,
		"-i": false,
		"head_limit": 100,
		"offset": 10,
		"multiline": true
	}`
	var in toolInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if in.Pattern != "foo" {
		t.Errorf("pattern: got %q, want 'foo'", in.Pattern)
	}
	if in.OutputMode != "content" {
		t.Errorf("output_mode: got %q", in.OutputMode)
	}
	if in.ContextB == nil || *in.ContextB != 2 {
		t.Errorf("-B: want 2, got %v", in.ContextB)
	}
	if in.ContextA == nil || *in.ContextA != 3 {
		t.Errorf("-A: want 3, got %v", in.ContextA)
	}
	if in.ContextC == nil || *in.ContextC != 5 {
		t.Errorf("-C: want 5, got %v", in.ContextC)
	}
	if in.ShowLineNums == nil || !*in.ShowLineNums {
		t.Errorf("-n: want true, got %v", in.ShowLineNums)
	}
	if in.CaseInsens == nil || *in.CaseInsens {
		t.Errorf("-i: want false, got %v", in.CaseInsens)
	}
	if in.HeadLimit == nil || *in.HeadLimit != 100 {
		t.Errorf("head_limit: want 100, got %v", in.HeadLimit)
	}
	if in.Offset == nil || *in.Offset != 10 {
		t.Errorf("offset: want 10, got %v", in.Offset)
	}
	if in.Multiline == nil || !*in.Multiline {
		t.Errorf("multiline: want true, got %v", in.Multiline)
	}
}

// ── ValidateInput ─────────────────────────────────────────────────────────────

func TestValidateInput_Valid(t *testing.T) {
	tool := &Tool{}
	raw, _ := json.Marshal(toolInput{Pattern: "foo"})
	if err := tool.ValidateInput(raw); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInput_EmptyPattern(t *testing.T) {
	tool := &Tool{}
	raw, _ := json.Marshal(toolInput{Pattern: ""})
	if err := tool.ValidateInput(raw); err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestValidateInput_InvalidJSON(t *testing.T) {
	tool := &Tool{}
	if err := tool.ValidateInput(json.RawMessage(`{bad json}`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestValidateInput_InvalidOutputMode(t *testing.T) {
	tool := &Tool{}
	raw, _ := json.Marshal(map[string]string{"pattern": "foo", "output_mode": "bad"})
	if err := tool.ValidateInput(raw); err == nil {
		t.Error("expected error for invalid output_mode")
	}
}

// ── Tool interface constants ───────────────────────────────────────────────────

func TestTool_AlwaysReadOnly(t *testing.T) {
	tool := &Tool{}
	inputs := []json.RawMessage{
		mustMarshal(toolInput{Pattern: "foo"}),
		mustMarshal(toolInput{Pattern: "bar", OutputMode: "content"}),
	}
	for _, raw := range inputs {
		if !tool.IsReadOnly(raw) {
			t.Errorf("IsReadOnly should always be true")
		}
	}
}

func TestTool_AlwaysConcurrencySafe(t *testing.T) {
	tool := &Tool{}
	raw := mustMarshal(toolInput{Pattern: "foo"})
	if !tool.IsConcurrencySafe(raw) {
		t.Error("IsConcurrencySafe should always be true")
	}
}

func TestTool_Name(t *testing.T) {
	tool := &Tool{}
	if tool.Name() != "Grep" {
		t.Errorf("Name() = %q, want 'Grep'", tool.Name())
	}
}

func TestTool_InputSchema_Valid(t *testing.T) {
	tool := &Tool{}
	schema := tool.InputSchema()
	var m map[string]any
	if err := json.Unmarshal(schema, &m); err != nil {
		t.Errorf("InputSchema() is not valid JSON: %v", err)
	}
	if m["type"] != "object" {
		t.Error("expected type=object in input schema")
	}
}

// ── Integration test (requires rg) ───────────────────────────────────────────

func TestIntegration_RgSearch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed, skipping integration test")
	}

	// Search this very test file for a unique string
	tool := &Tool{}
	in := toolInput{
		Pattern:    "INTEGRATION_TEST_MARKER_12345",
		OutputMode: "content",
		Path:       ".", // relative — resolved by tctx.Cwd
	}
	raw, _ := json.Marshal(in)

	// Create a temp file with the marker
	dir := t.TempDir()
	markerFile := dir + "/marker.txt"
	if err := writeFile(markerFile, "line1\nINTEGRATION_TEST_MARKER_12345\nline3\n"); err != nil {
		t.Fatal(err)
	}

	in.Path = markerFile
	raw, _ = json.Marshal(in)

	result, err := tool.Execute(t.Context(), raw, &tools.ToolContext{Cwd: dir})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "INTEGRATION_TEST_MARKER_12345") {
		t.Errorf("expected marker in output, got: %s", result.Content)
	}
}

func TestIntegration_NoMatches(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed, skipping integration test")
	}

	dir := t.TempDir()
	writeFile(dir+"/test.txt", "hello world\n") //nolint:errcheck

	tool := &Tool{}
	in := toolInput{Pattern: "UNLIKELY_PATTERN_XYZ987654"}
	raw, _ := json.Marshal(in)

	result, err := tool.Execute(t.Context(), raw, &tools.ToolContext{Cwd: dir})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No") {
		t.Errorf("expected 'No files found' or similar, got: %s", result.Content)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
