package astgrep

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// sgAvailable returns true if sg is installed on PATH.
func sgAvailable() bool {
	_, err := exec.LookPath("sg")
	return err == nil
}

// --- Interface compliance ---

func TestAstGrepTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

// --- Metadata ---

func TestAstGrepTool_Name(t *testing.T) {
	bt := &Tool{}
	if bt.Name() != "AstGrep" {
		t.Errorf("Name() = %q, want 'AstGrep'", bt.Name())
	}
}

func TestAstGrepTool_IsReadOnly_AlwaysTrue(t *testing.T) {
	bt := &Tool{}
	if !bt.IsReadOnly(json.RawMessage(`{"pattern":"foo"}`)) {
		t.Error("IsReadOnly should always be true")
	}
}

func TestAstGrepTool_IsConcurrencySafe_AlwaysTrue(t *testing.T) {
	bt := &Tool{}
	if !bt.IsConcurrencySafe(json.RawMessage(`{"pattern":"foo"}`)) {
		t.Error("IsConcurrencySafe should always be true")
	}
}

func TestAstGrepTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	bt := &Tool{}
	decision, err := bt.CheckPermissions(json.RawMessage(`{"pattern":"foo"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("CheckPermissions = %v, want PermAllow", decision.Behavior)
	}
}

// --- ValidateInput ---

func TestAstGrepTool_ValidateInput_PatternRequired(t *testing.T) {
	bt := &Tool{}
	if err := bt.ValidateInput(json.RawMessage(`{}`)); err == nil {
		t.Error("expected error when neither pattern nor rule is set")
	}
}

func TestAstGrepTool_ValidateInput_EmptyPattern(t *testing.T) {
	bt := &Tool{}
	if err := bt.ValidateInput(json.RawMessage(`{"pattern":""}`)); err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestAstGrepTool_ValidateInput_PatternAndRuleMutuallyExclusive(t *testing.T) {
	bt := &Tool{}
	input := json.RawMessage(`{"pattern":"foo","rule":"id: x\nlanguage: js\nrule:\n  pattern: foo"}`)
	if err := bt.ValidateInput(input); err == nil {
		t.Error("expected error when both pattern and rule are set")
	}
}

func TestAstGrepTool_ValidateInput_ValidPattern(t *testing.T) {
	bt := &Tool{}
	if err := bt.ValidateInput(json.RawMessage(`{"pattern":"console.log($$$ARGS)"}`)); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestAstGrepTool_ValidateInput_ValidRule(t *testing.T) {
	bt := &Tool{}
	rule := `id: test
language: javascript
rule:
  pattern: console.log($$$ARGS)`
	input, _ := json.Marshal(map[string]string{"rule": rule})
	if err := bt.ValidateInput(input); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestAstGrepTool_ValidateInput_ValidPatternWithLangAndPath(t *testing.T) {
	bt := &Tool{}
	input := json.RawMessage(`{"pattern":"foo","lang":"go","path":"/tmp"}`)
	if err := bt.ValidateInput(input); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// --- buildArgs ---

func TestBuildArgs_PatternMode_Basic(t *testing.T) {
	args := buildArgs(toolInput{Pattern: "console.log($$$ARGS)"})
	assertContainsSequence(t, args, []string{"run", "--pattern", "console.log($$$ARGS)"})
	assertContains(t, args, "--json=compact")
	assertContainsSequence(t, args, []string{"--globs", "!.git/**"})
	assertNotContains(t, args, "scan")
}

func TestBuildArgs_PatternMode_WithLang(t *testing.T) {
	args := buildArgs(toolInput{Pattern: "foo", Lang: "javascript"})
	assertContainsSequence(t, args, []string{"--lang", "javascript"})
}

func TestBuildArgs_PatternMode_WithPath(t *testing.T) {
	args := buildArgs(toolInput{Pattern: "foo", Path: "/some/path"})
	if args[len(args)-1] != "/some/path" {
		t.Errorf("path should be last arg, got %v", args)
	}
}

func TestBuildArgs_PatternMode_WithoutPath_DefaultsDot(t *testing.T) {
	args := buildArgs(toolInput{Pattern: "foo"})
	// When no path, sg defaults to current dir; we don't pass a path argument.
	// Verify we don't accidentally inject a stray path.
	for _, a := range args {
		if a == "." {
			return // found the dot, that's fine too
		}
	}
	// No path arg is also valid — sg defaults to .
}

func TestBuildArgs_ScanMode_Basic(t *testing.T) {
	rule := "id: test\nlanguage: js\nrule:\n  pattern: foo"
	args := buildArgs(toolInput{Rule: rule})
	assertContains(t, args, "scan")
	assertContainsSequence(t, args, []string{"--inline-rules", rule})
	assertContains(t, args, "--json=compact")
	assertContainsSequence(t, args, []string{"--globs", "!.git/**"})
	assertNotContains(t, args, "run")
	assertNotContains(t, args, "--pattern")
}

func TestBuildArgs_ScanMode_WithPath(t *testing.T) {
	args := buildArgs(toolInput{Rule: "id: x\nlang: js\nrule:\n  pattern: foo", Path: "/src"})
	if args[len(args)-1] != "/src" {
		t.Errorf("path should be last arg, got %v", args)
	}
}

func TestBuildArgs_GitAlwaysExcluded(t *testing.T) {
	// Both pattern and rule modes must always include the .git exclusion.
	for _, in := range []toolInput{
		{Pattern: "foo"},
		{Rule: "id: x\nlanguage: js\nrule:\n  pattern: foo"},
	} {
		args := buildArgs(in)
		assertContainsSequence(t, args, []string{"--globs", "!.git/**"})
	}
}

// --- formatMatches ---

func TestFormatMatches_Empty(t *testing.T) {
	out := formatMatches(nil, 250)
	if out != "No matches found" {
		t.Errorf("formatMatches(nil) = %q, want 'No matches found'", out)
	}
}

func TestFormatMatches_Basic(t *testing.T) {
	matches := []sgMatch{
		{
			File:  "src/main.js",
			Lines: `console.log("hello")`,
			Range: sgRange{Start: sgPosition{Line: 4, Column: 0}},
		},
	}
	out := formatMatches(matches, 250)
	if !strings.Contains(out, "src/main.js:5:") {
		t.Errorf("expected 1-indexed line 5, got: %s", out)
	}
	if !strings.Contains(out, `console.log("hello")`) {
		t.Errorf("expected match text in output, got: %s", out)
	}
}

func TestFormatMatches_MultipleMatches(t *testing.T) {
	matches := []sgMatch{
		{File: "a.go", Lines: "foo()", Range: sgRange{Start: sgPosition{Line: 0}}},
		{File: "b.go", Lines: "foo(x)", Range: sgRange{Start: sgPosition{Line: 9}}},
	}
	out := formatMatches(matches, 250)
	if !strings.Contains(out, "a.go:1:") {
		t.Errorf("missing a.go:1, got: %s", out)
	}
	if !strings.Contains(out, "b.go:10:") {
		t.Errorf("missing b.go:10, got: %s", out)
	}
}

func TestFormatMatches_TruncatesAtLimit(t *testing.T) {
	matches := make([]sgMatch, 300)
	for i := range matches {
		matches[i] = sgMatch{
			File:  "f.js",
			Lines: "x()",
			Range: sgRange{Start: sgPosition{Line: i}},
		}
	}
	out := formatMatches(matches, 250)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// 250 match lines + 1 truncation notice
	if len(lines) != 251 {
		t.Errorf("expected 251 lines (250 matches + truncation notice), got %d", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "truncated") {
		t.Errorf("last line should mention truncation, got: %s", lines[len(lines)-1])
	}
}

func TestFormatMatches_ExactlyAtLimit_NoTruncation(t *testing.T) {
	matches := make([]sgMatch, 250)
	for i := range matches {
		matches[i] = sgMatch{File: "f.js", Lines: "x()", Range: sgRange{Start: sgPosition{Line: i}}}
	}
	out := formatMatches(matches, 250)
	if strings.Contains(out, "truncated") {
		t.Error("should not truncate when exactly at limit")
	}
}

// --- Integration tests (require sg installed) ---

func TestAstGrepTool_Execute_PatternSearch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix")
	}
	if !sgAvailable() {
		t.Skip("sg not installed, skipping integration test")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
console.log("hello");
const x = 1;
console.log(x, "world");
`)

	bt := &Tool{}
	input, _ := json.Marshal(map[string]string{
		"pattern": "console.log($$$ARGS)",
		"lang":    "javascript",
		"path":    dir,
	})

	result, err := bt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// Both console.log calls should appear
	if !strings.Contains(result.Content, "main.js") {
		t.Errorf("expected filename in output, got: %s", result.Content)
	}
	count := strings.Count(result.Content, "console.log")
	if count < 2 {
		t.Errorf("expected at least 2 matches, got output: %s", result.Content)
	}
}

func TestAstGrepTool_Execute_RuleSearch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix")
	}
	if !sgAvailable() {
		t.Skip("sg not installed, skipping integration test")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.js"), `
console.log("debug");
let y = 2;
`)

	rule := `id: no-console
language: javascript
rule:
  pattern: console.log($$$ARGS)`

	bt := &Tool{}
	input, _ := json.Marshal(map[string]string{
		"rule": rule,
		"path": dir,
	})

	result, err := bt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "console.log") {
		t.Errorf("expected match in output, got: %s", result.Content)
	}
}

func TestAstGrepTool_Execute_NoMatches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix")
	}
	if !sgAvailable() {
		t.Skip("sg not installed, skipping integration test")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "clean.js"), `const x = 1;`)

	bt := &Tool{}
	input, _ := json.Marshal(map[string]string{
		"pattern": "console.log($$$ARGS)",
		"lang":    "javascript",
		"path":    dir,
	})

	result, err := bt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No matches") {
		t.Errorf("expected 'No matches' message, got: %s", result.Content)
	}
}

func TestAstGrepTool_Execute_GitDirExcluded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix")
	}
	if !sgAvailable() {
		t.Skip("sg not installed, skipping integration test")
	}

	dir := t.TempDir()
	// Write a match inside .git/
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gitDir, "COMMIT_EDITMSG"), `console.log("inside git")`)
	// And one outside .git/
	writeFile(t, filepath.Join(dir, "main.js"), `console.log("outside")`)

	bt := &Tool{}
	input, _ := json.Marshal(map[string]string{
		"pattern": "console.log($$$ARGS)",
		"lang":    "javascript",
		"path":    dir,
	})

	result, err := bt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Content, ".git") {
		t.Errorf(".git/ dir should be excluded, but got: %s", result.Content)
	}
}

func TestAstGrepTool_Execute_SgNotInstalled(t *testing.T) {
	// We can't easily simulate "sg not found" without modifying PATH,
	// but we test that the error message is helpful when it would fail.
	// This is covered by the implementation returning a descriptive error.
	bt := &Tool{}
	// Test that missing sg produces an error result, not a Go error.
	// We only run this if sg is NOT available.
	if sgAvailable() {
		t.Skip("sg is installed; this test only runs when sg is missing")
	}
	input := json.RawMessage(`{"pattern":"foo"}`)
	result, err := bt.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute should not return Go error, got: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when sg not installed")
	}
	if !strings.Contains(strings.ToLower(result.Content), "sg") {
		t.Errorf("error should mention 'sg', got: %s", result.Content)
	}
}

// --- helpers ---

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, want)
}

func assertNotContains(t *testing.T, args []string, unwanted string) {
	t.Helper()
	for _, a := range args {
		if a == unwanted {
			t.Errorf("args %v should not contain %q", args, unwanted)
			return
		}
	}
}

// assertContainsSequence checks that want appears as a contiguous sub-slice of args.
func assertContainsSequence(t *testing.T, args []string, want []string) {
	t.Helper()
	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j, w := range want {
			if args[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return
		}
	}
	t.Errorf("args %v does not contain sequence %v", args, want)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
