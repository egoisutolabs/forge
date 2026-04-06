package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const cookieDir = ".forge/browser"

// sanitizeSession validates that a session name is safe for use in file paths.
func sanitizeSession(session string) error {
	if strings.ContainsAny(session, "/\\") {
		return fmt.Errorf("session name must not contain path separators")
	}
	if strings.Contains(session, "..") {
		return fmt.Errorf("session name must not contain '..'")
	}
	if session != "" && filepath.Base(session) != session {
		return fmt.Errorf("session name contains invalid path components")
	}
	return nil
}

// cookiePath returns the filesystem path for a session's cookie file.
func cookiePath(session string) (string, error) {
	if err := sanitizeSession(session); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, cookieDir, fmt.Sprintf("cookies-%s.json", session)), nil
}

// saveCookies persists cookies to disk for later session restoration.
func saveCookies(session string, cookies []Cookie) error {
	if len(cookies) == 0 {
		return nil
	}
	path, err := cookiePath(session)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create cookie dir: %w", err)
	}
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cookies: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// loadCookies reads saved cookies from disk. Returns nil, nil if no file exists.
func loadCookies(session string) ([]Cookie, error) {
	path, err := cookiePath(session)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}
	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("unmarshal cookies: %w", err)
	}
	return cookies, nil
}
