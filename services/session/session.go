// Package session implements persistence of conversation state to disk.
//
// Sessions are stored as JSON files under ~/.forge/sessions/{id}.json.
// This allows conversations to be resumed across CLI invocations.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoisutolabs/forge/models"
	"github.com/google/uuid"
)

// sanitizeID validates that a session ID is safe for use in file paths.
func sanitizeID(id string) error {
	if id == "" {
		return fmt.Errorf("session: empty ID")
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("session: ID must not contain path separators")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("session: ID must not contain '..'")
	}
	if filepath.Base(id) != id {
		return fmt.Errorf("session: ID contains invalid path components")
	}
	return nil
}

// Session represents a persisted conversation.
type Session struct {
	ID         string            `json:"id"`
	Model      string            `json:"model"`
	Cwd        string            `json:"cwd"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	TotalUsage models.Usage      `json:"total_usage"`
	Messages   []*models.Message `json:"messages"`
}

// New creates a new session with a fresh UUID.
func New(model, cwd string) *Session {
	now := time.Now()
	return &Session{
		ID:        uuid.NewString(),
		Model:     model,
		Cwd:       cwd,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  make([]*models.Message, 0),
	}
}

// sessionsDir returns the directory where sessions are stored.
// If dir is empty, defaults to ~/.forge/sessions.
func sessionsDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".forge", "sessions"), nil
}

// sessionPath returns the full path for a session file.
func sessionPath(dir, id string) (string, error) {
	if err := sanitizeID(id); err != nil {
		return "", err
	}
	d, err := sessionsDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, id+".json"), nil
}

// Save persists the session to disk as JSON.
// dir overrides the default sessions directory; pass "" for the default.
func Save(s *Session, dir string) error {
	d, err := sessionsDir(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return fmt.Errorf("session: cannot create sessions directory: %w", err)
	}

	s.UpdatedAt = time.Now()

	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("session: marshal error: %w", err)
	}

	p, _ := sessionPath(dir, s.ID)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("session: write error: %w", err)
	}
	return nil
}

// Load reads a session from disk by ID.
// dir overrides the default sessions directory; pass "" for the default.
func Load(id, dir string) (*Session, error) {
	p, err := sessionPath(dir, id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("session: read error: %w", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("session: unmarshal error: %w", err)
	}
	return &s, nil
}

// List returns sessions sorted by UpdatedAt (most recent first).
// dir overrides the default sessions directory; pass "" for the default.
func List(dir string) ([]*Session, error) {
	d, err := sessionsDir(dir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session: list error: %w", err)
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		s, err := Load(id, dir)
		if err != nil {
			continue // skip corrupt files
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// AutoSave updates a session's messages and usage, then persists it.
// This is intended to be called after each turn.
func AutoSave(s *Session, messages []*models.Message, usage models.Usage, dir string) error {
	s.Messages = messages
	s.TotalUsage = usage
	return Save(s, dir)
}
