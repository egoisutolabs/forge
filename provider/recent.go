package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const maxRecentModels = 5

// recentModelsPath returns ~/.forge/state/recent_models.json.
func recentModelsPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".forge", "state", "recent_models.json")
}

// RecordUsage adds model to the front of the recent-models list, removing
// duplicates and capping at maxRecentModels.
func RecordUsage(model string) error {
	return RecordUsageTo(recentModelsPath(), model)
}

// RecordUsageTo is like RecordUsage but writes to an explicit path (for testing).
func RecordUsageTo(path, model string) error {
	existing := getRecentFrom(path)

	// Remove existing occurrence.
	filtered := make([]string, 0, len(existing))
	for _, m := range existing {
		if m != model {
			filtered = append(filtered, m)
		}
	}

	// Prepend.
	result := append([]string{model}, filtered...)
	if len(result) > maxRecentModels {
		result = result[:maxRecentModels]
	}

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// GetRecent returns the most recently used model names (newest first).
func GetRecent() []string {
	return getRecentFrom(recentModelsPath())
}

// GetRecentFrom reads recent models from an explicit path (for testing).
func GetRecentFrom(path string) []string {
	return getRecentFrom(path)
}

func getRecentFrom(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var models []string
	if err := json.Unmarshal(data, &models); err != nil {
		return nil
	}
	return models
}
