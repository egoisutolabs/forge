package bash

import (
	"os"
	"path/filepath"
	"testing"
)

// ── containsUNCPath ───────────────────────────────────────────────────────────

func TestContainsUNCPath_Detected(t *testing.T) {
	cases := []string{
		`\\server\share`,       // Windows-style UNC
		`\\\\server\\share`,    // escaped form in shell
		`\\hostname\c$`,        // admin share
		`cd \\fileserver\data`, // inside a command
	}
	for _, cmd := range cases {
		if !containsUNCPath(cmd) {
			t.Errorf("expected UNC path detected in: %q", cmd)
		}
	}
}

func TestContainsUNCPath_NotDetected(t *testing.T) {
	cases := []string{
		"ls -la",
		"echo hello",
		`sed 's/foo/bar/g'`,
		"git status",
		`cat file.txt`,
		`echo "hello world"`,
	}
	for _, cmd := range cases {
		if containsUNCPath(cmd) {
			t.Errorf("expected no UNC path in: %q", cmd)
		}
	}
}

// ── isBareGitRepo ─────────────────────────────────────────────────────────────

func TestIsBareGitRepo_True(t *testing.T) {
	// Create a temporary directory that looks like a bare git repo.
	dir := t.TempDir()
	for _, name := range []string{"HEAD", "objects", "refs", "hooks"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0755); err != nil {
			// HEAD is usually a file; try creating it as a file too.
			if err2 := os.WriteFile(path, []byte("ref: refs/heads/main\n"), 0644); err2 != nil {
				t.Fatalf("setup failed: %v", err2)
			}
		}
	}

	if !isBareGitRepo(dir) {
		t.Errorf("expected %q to be detected as a bare git repo", dir)
	}
}

func TestIsBareGitRepo_False_MissingEntries(t *testing.T) {
	// Directory with only some git entries — not a bare repo.
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "objects"), 0755)
	os.Mkdir(filepath.Join(dir, "refs"), 0755)
	// No HEAD, no hooks → not a bare repo.

	if isBareGitRepo(dir) {
		t.Errorf("expected %q NOT to be detected as a bare git repo", dir)
	}
}

func TestIsBareGitRepo_False_Empty(t *testing.T) {
	dir := t.TempDir()
	if isBareGitRepo(dir) {
		t.Errorf("empty dir should not be a bare git repo")
	}
}

func TestIsBareGitRepo_False_Nonexistent(t *testing.T) {
	if isBareGitRepo("/does/not/exist/at/all") {
		t.Error("non-existent dir should not be a bare git repo")
	}
}

// ── isOutputFlag ──────────────────────────────────────────────────────────────

func TestIsOutputFlag_Detected(t *testing.T) {
	flags := []string{
		"-o",
		"--output",
		"-O",
		"--output-document",
		"--output-file",
		"--output=file.txt",
		"--output-document=dump.html",
		"--output-file=log.txt",
	}
	for _, flag := range flags {
		if !isOutputFlag(flag) {
			t.Errorf("expected output flag: %q", flag)
		}
	}
}

func TestIsOutputFlag_NotDetected(t *testing.T) {
	notFlags := []string{
		"-v",
		"--verbose",
		"-L",
		"--location",
		"--silent",
		"-s",
		"file.txt",
		"",
		"--outputx",
	}
	for _, flag := range notFlags {
		if isOutputFlag(flag) {
			t.Errorf("expected NOT output flag: %q", flag)
		}
	}
}
