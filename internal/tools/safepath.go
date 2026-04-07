package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sensitivePathPrefixes are absolute path prefixes that file tools should
// never read or write through symlink resolution. A symlink pointing into
// one of these directories is rejected even if the original path looked safe.
var sensitivePathPrefixes []string

func init() {
	// Only block truly dangerous system files that should never be accessed
	// via symlink bypass. Moderately sensitive paths like ~/.ssh are handled
	// by each tool's own permission checks (PermAsk) so users can approve them.
	sensitivePathPrefixes = []string{
		"/etc/shadow",
		"/etc/gshadow",
		"/etc/sudoers",
	}
}

// ResolvePath resolves a file path through symlinks and validates the result.
// It:
//  1. Cleans the path (resolves ".." etc.)
//  2. Requires the path to be absolute
//  3. Resolves symlinks via filepath.EvalSymlinks
//  4. Rejects paths that resolve to sensitive locations
//
// Returns the resolved, cleaned absolute path or an error.
func ResolvePath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path must be absolute, got: %s", path)
	}

	// First, check if the cleaned path itself is a symlink.
	// If so, read its target and check that before doing anything else.
	// This catches dangling symlinks to sensitive locations (where EvalSymlinks
	// would fail because the target doesn't exist).
	if target, err := os.Readlink(cleaned); err == nil {
		// It's a symlink — resolve the target to an absolute path.
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cleaned), target)
		}
		target = filepath.Clean(target)
		if err := checkSensitivePath(target); err != nil {
			return "", err
		}
	}

	// Resolve all symlinks to get the real path on disk.
	// This is the key defense against symlink-based bypass attacks.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// If the file doesn't exist yet (e.g., new file creation), resolve
		// as much of the parent as possible and append the filename.
		if os.IsNotExist(err) {
			dir := filepath.Dir(cleaned)
			base := filepath.Base(cleaned)
			resolvedDir, dirErr := filepath.EvalSymlinks(dir)
			if dirErr != nil {
				// Parent doesn't exist either — check the cleaned path
				// for sensitivity, then return it as-is (the actual
				// open/write will report the real error).
				if sensErr := checkSensitivePath(cleaned); sensErr != nil {
					return "", sensErr
				}
				return cleaned, nil
			}
			resolved = filepath.Join(resolvedDir, base)
		} else {
			return "", fmt.Errorf("resolving symlinks for %s: %w", path, err)
		}
	}

	// Check if the resolved path points to a sensitive location.
	if err := checkSensitivePath(resolved); err != nil {
		return "", err
	}

	return resolved, nil
}

// checkSensitivePath rejects paths that point to known-sensitive locations.
func checkSensitivePath(resolved string) error {
	for _, prefix := range sensitivePathPrefixes {
		if resolved == prefix || strings.HasPrefix(resolved, prefix+string(filepath.Separator)) {
			return fmt.Errorf("access denied: path resolves to sensitive location %s", resolved)
		}
	}
	return nil
}

// VerifyInodeUnchanged checks that the inode of the file at path hasn't changed
// since the given os.FileInfo was captured. This detects TOCTOU attacks where
// a symlink is swapped between permission check and actual I/O.
// Returns nil if the inode matches, or an error if it changed or can't be checked.
func VerifyInodeUnchanged(path string, before os.FileInfo) error {
	after, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("re-stat failed for %s: %w", path, err)
	}
	if !os.SameFile(before, after) {
		return fmt.Errorf("file identity changed between permission check and access for %s (possible symlink swap)", path)
	}
	return nil
}
