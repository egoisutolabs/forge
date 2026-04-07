package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// LoadHooksFromFile reads a JSON file at path and parses it into HooksSettings.
// The JSON must be an object mapping HookEvent strings to arrays of HookMatcher.
// Returns an error if any matcher regex pattern is invalid.
func LoadHooksFromFile(path string) (HooksSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var settings HooksSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	// Validate all regex patterns at load time so misconfigured hooks surface
	// immediately rather than silently failing to match during execution.
	for event, matchers := range settings {
		for i, m := range matchers {
			if m.Matcher != "" {
				if _, err := regexp.Compile(m.Matcher); err != nil {
					return nil, fmt.Errorf("hooks[%s][%d].matcher: invalid regex %q: %w", event, i, m.Matcher, err)
				}
			}
		}
	}
	return settings, nil
}

// MergeHooks combines two HooksSettings into one by appending the matchers
// from b after those from a for each event. Both inputs are left unmodified.
func MergeHooks(a, b HooksSettings) HooksSettings {
	result := make(HooksSettings)
	for event, matchers := range a {
		result[event] = append(result[event], matchers...)
	}
	for event, matchers := range b {
		result[event] = append(result[event], matchers...)
	}
	return result
}
