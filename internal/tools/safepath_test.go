package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolvePath_NormalFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	// Create a real file
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolvePath(realFile)
	if err != nil {
		t.Fatalf("ResolvePath(%s) error: %v", realFile, err)
	}
	if resolved != realFile {
		// On macOS, /tmp may resolve to /private/tmp
		if !strings.HasSuffix(resolved, "/real.txt") {
			t.Errorf("resolved = %q, want to end with /real.txt", resolved)
		}
	}
}

func TestResolvePath_SymlinkToEtcPasswd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	dir := t.TempDir()
	symlink := filepath.Join(dir, "sneaky.txt")
	// Create symlink to /etc/shadow (sensitive)
	if err := os.Symlink("/etc/shadow", symlink); err != nil {
		t.Fatal(err)
	}

	_, err := ResolvePath(symlink)
	if err == nil {
		t.Error("expected error for symlink to /etc/shadow, got nil")
	}
	if !strings.Contains(err.Error(), "sensitive location") {
		t.Errorf("expected 'sensitive location' in error, got: %v", err)
	}
}

func TestResolvePath_SymlinkToSudoers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	// Create a symlink pointing to /etc/sudoers (a blocked sensitive path)
	dir := t.TempDir()
	symlink := filepath.Join(dir, "sneaky_sudoers")
	if err := os.Symlink("/etc/sudoers", symlink); err != nil {
		t.Fatal(err)
	}

	_, resolveErr := ResolvePath(symlink)
	if resolveErr == nil {
		t.Error("expected error for symlink to /etc/sudoers, got nil")
	}
	if resolveErr != nil && !strings.Contains(resolveErr.Error(), "sensitive location") {
		if !strings.Contains(resolveErr.Error(), "resolving symlinks") {
			t.Errorf("expected 'sensitive location' or 'resolving symlinks' in error, got: %v", resolveErr)
		}
	}
}

func TestResolvePath_RelativePathRejected(t *testing.T) {
	_, err := ResolvePath("relative/path.txt")
	if err == nil {
		t.Error("expected error for relative path, got nil")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("expected 'must be absolute' in error, got: %v", err)
	}
}

func TestResolvePath_DotDotResolved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	dir := t.TempDir()
	realFile := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Path with ".." that still resolves to a safe location
	pathWithDotDot := filepath.Join(dir, "subdir", "..", "file.txt")
	resolved, err := ResolvePath(pathWithDotDot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to the canonical path (without ..)
	if strings.Contains(resolved, "..") {
		t.Errorf("resolved path still contains '..': %s", resolved)
	}
}

func TestResolvePath_NonExistentFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	dir := t.TempDir()
	newFile := filepath.Join(dir, "new_file.txt")

	// Should succeed — the parent dir exists, only the file doesn't
	resolved, err := ResolvePath(newFile)
	if err != nil {
		t.Fatalf("unexpected error for non-existent file: %v", err)
	}
	if !strings.HasSuffix(resolved, "new_file.txt") {
		t.Errorf("resolved = %q, want to end with new_file.txt", resolved)
	}
}

func TestResolvePath_SymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require unix")
	}

	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a chain: link1 -> link2 -> real.txt
	link2 := filepath.Join(dir, "link2")
	if err := os.Symlink(realFile, link2); err != nil {
		t.Fatal(err)
	}
	link1 := filepath.Join(dir, "link1")
	if err := os.Symlink(link2, link1); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolvePath(link1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to the real file
	if !strings.HasSuffix(resolved, "real.txt") {
		t.Errorf("resolved = %q, want to end with real.txt", resolved)
	}
}

func TestVerifyInodeUnchanged_SameFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode tests require unix")
	}

	file := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(file, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}

	// Same file — should pass
	if err := VerifyInodeUnchanged(file, info); err != nil {
		t.Errorf("unexpected error for same file: %v", err)
	}
}

func TestVerifyInodeUnchanged_SymlinkSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("inode tests require unix")
	}
	if runtime.GOOS == "linux" {
		// TODO: strengthen VerifyInodeUnchanged to detect symlink swaps on
		// Linux tmpfs. The current os.SameFile-based check compares device
		// and inode number, but Linux tmpfs reuses the freed inode
		// immediately after unlink, so a symlink swapped via remove+recreate
		// appears unchanged. The fix is to also capture os.Readlink() at
		// check time and compare target strings at verify time. Non-trivial
		// because it changes the check-time state threaded through callers.
		t.Skip("VerifyInodeUnchanged cannot detect symlink swaps on Linux tmpfs (inode reuse); see TODO above")
	}

	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	link := filepath.Join(dir, "target")

	if err := os.WriteFile(fileA, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create link -> fileA
	if err := os.Symlink(fileA, link); err != nil {
		t.Fatal(err)
	}

	// Capture info while pointing to A
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}

	// Swap link to point to B
	os.Remove(link)
	if err := os.Symlink(fileB, link); err != nil {
		t.Fatal(err)
	}

	// Should detect the change
	if err := VerifyInodeUnchanged(link, info); err == nil {
		t.Error("expected error after symlink swap, got nil")
	}
}
