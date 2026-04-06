package agent

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// safeSlugPattern matches safe worktree slugs: alphanumeric, underscore, hyphen only.
var safeSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validateSlug returns an error if slug contains path separators, dots, or
// other characters that could be used for path traversal.
func validateSlug(slug string) error {
	if !safeSlugPattern.MatchString(slug) {
		return fmt.Errorf("invalid worktree slug %q: must match [a-zA-Z0-9_-]+", slug)
	}
	return nil
}

// CreateWorktree creates a new git worktree at .forge/worktrees/{slug} with a
// new branch named forge-{slug}. Returns the worktree path and branch name.
//
// Equivalent to: git -C <repoRoot> worktree add .forge/worktrees/{slug} -b forge-{slug}
func CreateWorktree(repoRoot, slug string) (worktreePath, branch string, err error) {
	if err := validateSlug(slug); err != nil {
		return "", "", err
	}
	worktreePath = filepath.Join(repoRoot, ".forge", "worktrees", slug)
	branch = "forge-" + slug

	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", worktreePath, "-b", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return worktreePath, branch, nil
}

// CleanupWorktree removes the git worktree at worktreePath when hasChanges is
// false. When hasChanges is true the worktree is preserved and nil is returned,
// allowing the caller to report the path back to the user.
//
// The removal is run with -C worktreePath so git can resolve the main repo
// through the worktree's .git pointer file regardless of the caller's cwd.
func CleanupWorktree(worktreePath string, hasChanges bool) error {
	// Validate the path doesn't contain traversal segments.
	cleanPath := filepath.Clean(worktreePath)
	for _, part := range strings.Split(filepath.ToSlash(cleanPath), "/") {
		if part == ".." {
			return fmt.Errorf("invalid worktree path %q: contains traversal segments", worktreePath)
		}
	}
	if hasChanges {
		return nil
	}
	cmd := exec.Command("git", "-C", worktreePath, "worktree", "remove", worktreePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// HasWorktreeChanges returns true when the worktree at path has any uncommitted
// or untracked changes (i.e. "git status --porcelain" produces output).
func HasWorktreeChanges(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
