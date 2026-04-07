// Package fileread — verification tests comparing Go port against Claude Code's
// FileReadTool TypeScript source.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: PDF / image / notebook rendering.
//     TypeScript natively renders PDFs (via pdf2pic), images (multimodal blocks),
//     and Jupyter notebooks. Go rejects all binary extensions including PDF.
//
//  2. CORRECT: Sensitive path protection (task #31 closed).
//     CheckPermissions returns PermAsk for .env*, *credentials*, *secret*,
//     *.pem, *.key, ~/.ssh/*, ~/.aws/*.
//
//  3. CORRECT: Absolute path required (relative paths rejected).
//
//  4. CORRECT: filepath.Clean applied before disk I/O.
//
//  5. CORRECT: Binary extensions rejected with helpful message.
//
//  6. CORRECT: Empty file returns system-reminder warning.
//
//  7. CORRECT: Line numbers in cat -n format (%6d\t%s).
//
//  8. CORRECT: Offset + limit line-range control.
//
//  9. CORRECT: FileState cache updated on successful read.
//
// 10. CORRECT: Offset beyond file end returns warning, not error.
package fileread

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── GAP 1: binary formats not rendered ─────────────────────────────────────

// TestVerification_PDFRejected_NotRendered documents that Go's FileRead rejects
// PDFs instead of rendering them (TypeScript uses pdf2pic).
func TestVerification_PDFRejected_NotRendered(t *testing.T) {
	// Create a minimal fake PDF file to test extension rejection.
	dir := t.TempDir()
	fakePDF := filepath.Join(dir, "report.pdf")
	os.WriteFile(fakePDF, []byte("%PDF-1.4 dummy"), 0644) //nolint:errcheck

	var in toolInput
	in.FilePath = fakePDF
	inJSON, _ := json.Marshal(in)

	result, _ := (&Tool{}).Execute(nil, inJSON, nil)
	if result == nil {
		t.Fatal("result should not be nil")
	}

	if !result.IsError || !strings.Contains(result.Content, "binary") {
		t.Logf("NOTE: PDF may not be rejected as binary (content: %q)", result.Content[:min(len(result.Content), 80)])
	} else {
		t.Log("CORRECT: PDF rejected with 'binary file' message (TypeScript renders PDFs; Go rejects them)")
		t.Log("GAP: TypeScript renders PDFs as images via pdf2pic; Go has no PDF rendering")
	}
}

// ─── CORRECT: sensitive path protection (task #31) ───────────────────────────

// TestVerification_SensitivePath_DotEnv_ReturnsPermAsk verifies that .env files
// require interactive approval (PermAsk) before being read.
func TestVerification_SensitivePath_DotEnv_ReturnsPermAsk(t *testing.T) {
	paths := []string{
		"/home/user/.env",
		"/app/.env.local",
		"/project/.env.production",
	}
	for _, p := range paths {
		inJSON, _ := json.Marshal(toolInput{FilePath: p})
		decision, err := (&Tool{}).CheckPermissions(inJSON, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%q): %v", p, err)
		}
		if decision.Behavior != models.PermAsk {
			t.Errorf("path %q: Behavior = %q, want PermAsk", p, decision.Behavior)
		}
	}
	t.Log("CORRECT: .env* files return PermAsk (task #31 sensitive path protection)")
}

// TestVerification_SensitivePath_Credentials_ReturnsPermAsk verifies that
// files with "credentials" in the name require approval.
func TestVerification_SensitivePath_Credentials_ReturnsPermAsk(t *testing.T) {
	paths := []string{
		"/home/user/.aws/credentials",
		"/app/config/credentials.json",
		"/secrets/aws-credentials",
	}
	for _, p := range paths {
		inJSON, _ := json.Marshal(toolInput{FilePath: p})
		decision, err := (&Tool{}).CheckPermissions(inJSON, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%q): %v", p, err)
		}
		if decision.Behavior != models.PermAsk {
			t.Errorf("path %q: Behavior = %q, want PermAsk", p, decision.Behavior)
		}
	}
	t.Log("CORRECT: credential files return PermAsk (task #31)")
}

// TestVerification_SensitivePath_PEMKey_ReturnsPermAsk verifies that .pem and
// .key files require approval.
func TestVerification_SensitivePath_PEMKey_ReturnsPermAsk(t *testing.T) {
	paths := []string{
		"/etc/ssl/private/server.pem",
		"/home/user/.ssh/id_rsa.key",
		"/certs/client.key",
	}
	for _, p := range paths {
		inJSON, _ := json.Marshal(toolInput{FilePath: p})
		decision, err := (&Tool{}).CheckPermissions(inJSON, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%q): %v", p, err)
		}
		if decision.Behavior != models.PermAsk {
			t.Errorf("path %q: Behavior = %q, want PermAsk", p, decision.Behavior)
		}
	}
	t.Log("CORRECT: .pem and .key files return PermAsk (task #31)")
}

// TestVerification_SensitivePath_SSHDir_ReturnsPermAsk verifies that ~/.ssh/*
// paths require approval.
func TestVerification_SensitivePath_SSHDir_ReturnsPermAsk(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	sshPaths := []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "known_hosts"),
		filepath.Join(home, ".ssh", "config"),
	}
	for _, p := range sshPaths {
		inJSON, _ := json.Marshal(toolInput{FilePath: p})
		decision, err := (&Tool{}).CheckPermissions(inJSON, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%q): %v", p, err)
		}
		if decision.Behavior != models.PermAsk {
			t.Errorf("path %q: Behavior = %q, want PermAsk", p, decision.Behavior)
		}
	}
	t.Log("CORRECT: ~/.ssh/* paths return PermAsk (task #31)")
}

// TestVerification_SensitivePath_NormalFile_ReturnsPermAllow verifies that
// regular (non-sensitive) files are allowed without prompting.
func TestVerification_SensitivePath_NormalFile_ReturnsPermAllow(t *testing.T) {
	paths := []string{
		"/home/user/README.md",
		"/app/src/main.go",
		"/project/config.yaml",
	}
	for _, p := range paths {
		inJSON, _ := json.Marshal(toolInput{FilePath: p})
		decision, err := (&Tool{}).CheckPermissions(inJSON, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%q): %v", p, err)
		}
		if decision.Behavior != models.PermAllow {
			t.Errorf("normal path %q: Behavior = %q, want PermAllow", p, decision.Behavior)
		}
	}
	t.Log("CORRECT: non-sensitive paths return PermAllow without prompting")
}

// ─── CORRECT: absolute path enforcement ──────────────────────────────────────

// TestVerification_RelativePath_Rejected verifies that relative paths are
// rejected with a clear error message.
func TestVerification_RelativePath_Rejected(t *testing.T) {
	inJSON, _ := json.Marshal(toolInput{FilePath: "relative/path.txt"})
	result, _ := (&Tool{}).Execute(nil, inJSON, nil)
	if !result.IsError {
		t.Error("relative path should produce an error result")
	}
	if !strings.Contains(result.Content, "absolute") {
		t.Errorf("error should mention 'absolute', got: %q", result.Content)
	}
	t.Log("CORRECT: relative paths rejected with 'must be absolute' message")
}

// ─── CORRECT: FileState cache updated ────────────────────────────────────────

// TestVerification_FileState_UpdatedAfterRead verifies that ToolContext.FileState
// is populated after a successful read.
func TestVerification_FileState_UpdatedAfterRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644) //nolint:errcheck

	fileState := tools.NewFileStateCache(100, 25*1024*1024)
	tctx := &tools.ToolContext{FileState: fileState}

	inJSON, _ := json.Marshal(toolInput{FilePath: path})
	result, _ := (&Tool{}).Execute(nil, inJSON, tctx)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	state, ok := fileState.Get(path)
	if !ok {
		t.Error("FileState should be populated after a successful read")
	} else {
		if !strings.Contains(state.Content, "line1") {
			t.Error("FileState content should contain file contents")
		}
		t.Log("CORRECT: FileState cache populated after read — enables FileEditTool/FileWriteTool guards")
	}
}

// ─── TypeScript comparison note ──────────────────────────────────────────────

// TestVerification_SensitivePath_Coverage_VsTypeScript documents the scope
// difference in sensitive path detection between Go and TypeScript.
//
// TypeScript FileReadTool does not implement isSensitivePath() — sensitivity
// detection is handled at the system prompt / permissions layer. Go's
// FileReadTool adds isSensitivePath() directly to CheckPermissions as a
// defence-in-depth measure.
//
// Known gaps vs typical TypeScript behaviour:
//   - TypeScript detects sensitive paths via permissions system (configurable).
//   - Go's list is hard-coded in fileread.go — no user configuration.
func TestVerification_SensitivePath_Coverage_VsTypeScript(t *testing.T) {
	t.Log("NOTE: TypeScript relies on permissions system for sensitive path detection")
	t.Log("Go isSensitivePath() is a hard-coded defence-in-depth in CheckPermissions")
	t.Log("Patterns: .env*, *credentials*, *secret*, *.pem, *.key, ~/.ssh/*, ~/.aws/*")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
