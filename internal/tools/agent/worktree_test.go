package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if git is not available in PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

// initGitRepo initialises a fresh git repository with a single commit in dir.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@forge.test")
	run("config", "user.name", "Forge Test")
	// Write an initial file so we can commit.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
}

// ── CreateWorktree ─────────────────────────────────────────────────────────────

func TestCreateWorktree_CreatesDirectoryAndBranch(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	slug := "test-slug"
	worktreePath, branch, err := CreateWorktree(repoRoot, slug)
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Validate returned values.
	wantPath := filepath.Join(repoRoot, ".forge", "worktrees", slug)
	if worktreePath != wantPath {
		t.Errorf("worktreePath = %q, want %q", worktreePath, wantPath)
	}
	wantBranch := "forge-" + slug
	if branch != wantBranch {
		t.Errorf("branch = %q, want %q", branch, wantBranch)
	}

	// The worktree directory must exist and contain a git file.
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree directory not created: %v", err)
	}
	gitFile := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitFile); err != nil {
		t.Errorf("worktree .git file missing: %v", err)
	}
}

func TestCreateWorktree_InvalidRepo_ReturnsError(t *testing.T) {
	requireGit(t)
	noRepo := t.TempDir() // not a git repo
	_, _, err := CreateWorktree(noRepo, "any-slug")
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

func TestCreateWorktree_BranchNameContainsSlug(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, branch, err := CreateWorktree(repoRoot, "my-feature")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if !strings.Contains(branch, "my-feature") {
		t.Errorf("branch %q does not contain slug", branch)
	}
}

// ── Slug validation (path traversal) ──────────────────────────────────────────

func TestCreateWorktree_RejectsTraversalSlug(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "../../tmp/evil")
	if err == nil {
		t.Fatal("expected error for traversal slug")
	}
}

func TestCreateWorktree_RejectsSlashInSlug(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "foo/bar")
	if err == nil {
		t.Fatal("expected error for slash in slug")
	}
}

func TestCreateWorktree_RejectsDotSegmentSlug(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "..")
	if err == nil {
		t.Fatal("expected error for .. slug")
	}
}

func TestCreateWorktree_NormalSlugWorks(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "agent-1")
	if err != nil {
		t.Fatalf("unexpected error for normal slug: %v", err)
	}
}

func TestValidateSlug_AcceptsValidSlugs(t *testing.T) {
	for _, slug := range []string{"agent-1", "my_task", "ABC123", "a"} {
		if err := validateSlug(slug); err != nil {
			t.Errorf("validateSlug(%q) unexpected error: %v", slug, err)
		}
	}
}

func TestValidateSlug_RejectsInvalidSlugs(t *testing.T) {
	for _, slug := range []string{"../../tmp/evil", "foo/bar", "..", "a.b", ""} {
		if err := validateSlug(slug); err == nil {
			t.Errorf("validateSlug(%q) should have returned error", slug)
		}
	}
}

// ── HasWorktreeChanges ─────────────────────────────────────────────────────────

func TestHasWorktreeChanges_CleanRepo_ReturnsFalse(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	changed, err := HasWorktreeChanges(repoRoot)
	if err != nil {
		t.Fatalf("HasWorktreeChanges: %v", err)
	}
	if changed {
		t.Error("clean repo should report no changes")
	}
}

func TestHasWorktreeChanges_UntrackedFile_ReturnsTrue(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	// Add an untracked file.
	if err := os.WriteFile(filepath.Join(repoRoot, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := HasWorktreeChanges(repoRoot)
	if err != nil {
		t.Fatalf("HasWorktreeChanges: %v", err)
	}
	if !changed {
		t.Error("repo with untracked file should report changes")
	}
}

func TestHasWorktreeChanges_ModifiedTrackedFile_ReturnsTrue(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	// Modify the tracked README.
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := HasWorktreeChanges(repoRoot)
	if err != nil {
		t.Fatalf("HasWorktreeChanges: %v", err)
	}
	if !changed {
		t.Error("repo with modified file should report changes")
	}
}

func TestHasWorktreeChanges_NotAGitRepo_ReturnsError(t *testing.T) {
	requireGit(t)
	dir := t.TempDir() // not a git repo
	_, err := HasWorktreeChanges(dir)
	if err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

// ── CleanupWorktree ────────────────────────────────────────────────────────────

func TestCleanupWorktree_HasChangesTrue_KeepsWorktree(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "cleanup-keep")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	worktreePath := filepath.Join(repoRoot, ".forge", "worktrees", "cleanup-keep")

	// CleanupWorktree with hasChanges=true must keep the directory.
	if err := CleanupWorktree(worktreePath, true); err != nil {
		t.Fatalf("CleanupWorktree(hasChanges=true): %v", err)
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Errorf("worktree should still exist after cleanup with changes: %v", err)
	}
}

func TestCleanupWorktree_HasChangesFalse_RemovesWorktree(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	_, _, err := CreateWorktree(repoRoot, "cleanup-remove")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	worktreePath := filepath.Join(repoRoot, ".forge", "worktrees", "cleanup-remove")

	// CleanupWorktree with hasChanges=false should remove the directory.
	if err := CleanupWorktree(worktreePath, false); err != nil {
		t.Fatalf("CleanupWorktree(hasChanges=false): %v", err)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("worktree should be removed after cleanup with no changes")
	}
}

// TestCreateAndDetectChanges is an end-to-end test: create worktree, write a
// file in it, then verify HasWorktreeChanges detects it.
func TestCreateAndDetectChanges(t *testing.T) {
	requireGit(t)
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	worktreePath, _, err := CreateWorktree(repoRoot, "e2e")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Initially clean.
	changed, err := HasWorktreeChanges(worktreePath)
	if err != nil {
		t.Fatalf("HasWorktreeChanges (clean): %v", err)
	}
	if changed {
		t.Error("fresh worktree should be clean")
	}

	// Write a file.
	if err := os.WriteFile(filepath.Join(worktreePath, "work.txt"), []byte("work"), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err = HasWorktreeChanges(worktreePath)
	if err != nil {
		t.Fatalf("HasWorktreeChanges (dirty): %v", err)
	}
	if !changed {
		t.Error("worktree with new file should report changes")
	}
}
