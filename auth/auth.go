// Package auth provides centralized API key management for Forge providers.
//
// Credentials are stored in ~/.forge/auth.json with 0600 file permissions.
// The GetAPIKey function checks three sources in order: auth.json, config.yaml,
// then environment variables.
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoisutolabs/forge/config"
)

// AuthStore holds API credentials for multiple providers.
type AuthStore struct {
	Providers map[string]ProviderAuth `json:"providers"`
}

// ProviderAuth holds authentication details for a single provider.
type ProviderAuth struct {
	Type   string `json:"type"` // "api_key"
	APIKey string `json:"api_key,omitempty"`
}

const (
	authFileName = "auth.json"
	dirPerm      = 0700
	filePerm     = 0600
)

var (
	storeMu   sync.Mutex
	storePath string // lazily resolved
)

// DefaultPath returns the default path to auth.json (~/.forge/auth.json).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".forge", authFileName)
	}
	return filepath.Join(home, ".forge", authFileName)
}

// SetPath overrides the auth.json path. Useful for testing.
func SetPath(path string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	storePath = path
}

func getPath() string {
	storeMu.Lock()
	defer storeMu.Unlock()
	if storePath != "" {
		return storePath
	}
	storePath = DefaultPath()
	return storePath
}

// Load reads the auth store from disk. Returns an empty store (not an error)
// if the file does not exist.
func Load() (*AuthStore, error) {
	return LoadFrom(getPath())
}

// LoadFrom reads the auth store from the given path.
func LoadFrom(path string) (*AuthStore, error) {
	store := &AuthStore{Providers: make(map[string]ProviderAuth)}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, err
	}
	if store.Providers == nil {
		store.Providers = make(map[string]ProviderAuth)
	}
	return store, nil
}

// Save writes the auth store to disk with secure permissions.
func (s *AuthStore) Save() error {
	return s.SaveTo(getPath())
}

// SaveTo writes the auth store to the given path.
func (s *AuthStore) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, filePerm)
}

// GetAPIKey retrieves an API key for the given provider, checking sources
// in precedence order: auth.json > config.yaml > environment variable.
// The cfg parameter may be nil if no config is loaded.
func GetAPIKey(provider string, cfg *config.Config) string {
	// 1. Check auth.json.
	store, err := Load()
	if err == nil {
		if pa, ok := store.Providers[provider]; ok && pa.APIKey != "" {
			return pa.APIKey
		}
	}

	// 2. Check config.yaml providers.
	if cfg != nil {
		for _, p := range cfg.Providers {
			if p.Name == provider && p.APIKey != "" {
				return p.APIKey
			}
		}
	}

	// 3. Check environment variable.
	return GetEnvKey(provider)
}

// GetAPIKeyFrom is like GetAPIKey but loads auth from a specific path.
func GetAPIKeyFrom(provider string, authPath string, cfg *config.Config) string {
	store, err := LoadFrom(authPath)
	if err == nil {
		if pa, ok := store.Providers[provider]; ok && pa.APIKey != "" {
			return pa.APIKey
		}
	}

	if cfg != nil {
		for _, p := range cfg.Providers {
			if p.Name == provider && p.APIKey != "" {
				return p.APIKey
			}
		}
	}

	return GetEnvKey(provider)
}

// SetAPIKey stores an API key for the given provider in auth.json.
func SetAPIKey(provider, key string) error {
	return SetAPIKeyIn(getPath(), provider, key)
}

// SetAPIKeyIn stores an API key at the given auth.json path.
func SetAPIKeyIn(path, provider, key string) error {
	store, err := LoadFrom(path)
	if err != nil {
		store = &AuthStore{Providers: make(map[string]ProviderAuth)}
	}

	store.Providers[provider] = ProviderAuth{
		Type:   "api_key",
		APIKey: key,
	}

	return store.SaveTo(path)
}

// HasAnyAuth returns true if any provider has a configured API key from
// any source (auth.json, config, or environment).
func HasAnyAuth(cfg *config.Config) bool {
	// Check auth.json.
	store, err := Load()
	if err == nil {
		for _, pa := range store.Providers {
			if pa.APIKey != "" {
				return true
			}
		}
	}

	// Check config providers.
	if cfg != nil {
		for _, p := range cfg.Providers {
			if p.APIKey != "" {
				return true
			}
		}
	}

	// Check environment variables.
	for _, provider := range KnownProviders() {
		if GetEnvKey(provider) != "" {
			return true
		}
	}

	return false
}

// AuthSource describes where a provider's API key was found.
type AuthSource string

const (
	SourceAuthFile AuthSource = "auth.json"
	SourceConfig   AuthSource = "config.yaml"
	SourceEnvVar   AuthSource = "env var"
	SourceNone     AuthSource = ""
)

// GetAuthSource returns where the API key for a provider is coming from.
func GetAuthSource(provider string, cfg *config.Config) AuthSource {
	store, err := Load()
	if err == nil {
		if pa, ok := store.Providers[provider]; ok && pa.APIKey != "" {
			return SourceAuthFile
		}
	}

	if cfg != nil {
		for _, p := range cfg.Providers {
			if p.Name == provider && p.APIKey != "" {
				return SourceConfig
			}
		}
	}

	if GetEnvKey(provider) != "" {
		return SourceEnvVar
	}

	return SourceNone
}
