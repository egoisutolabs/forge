package lsp

import (
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	cfg := ServerConfig{
		Name:           "test-server",
		Command:        "echo",
		Args:           []string{"hello"},
		MaxCrashes:     5,
		StartupTimeout: 10 * time.Second,
	}

	s := NewServer(cfg)
	if s.state != StateStopped {
		t.Errorf("initial state = %d, want StateStopped(%d)", s.state, StateStopped)
	}
	if s.maxCrashes != 5 {
		t.Errorf("maxCrashes = %d, want 5", s.maxCrashes)
	}
	if s.config.Name != "test-server" {
		t.Errorf("config.Name = %q, want %q", s.config.Name, "test-server")
	}
}

func TestNewServerDefaultMaxCrashes(t *testing.T) {
	cfg := ServerConfig{
		Name:    "test-server",
		Command: "echo",
	}

	s := NewServer(cfg)
	if s.maxCrashes != 3 {
		t.Errorf("default maxCrashes = %d, want 3", s.maxCrashes)
	}
}

func TestServerState(t *testing.T) {
	s := NewServer(ServerConfig{Name: "test", Command: "echo"})
	if s.State() != StateStopped {
		t.Errorf("State() = %d, want StateStopped", s.State())
	}
}

func TestServerConfig(t *testing.T) {
	cfg := ServerConfig{
		Name:    "gopls",
		Command: "gopls",
		Args:    []string{"serve"},
	}
	s := NewServer(cfg)
	got := s.Config()
	if got.Name != "gopls" {
		t.Errorf("Config().Name = %q, want %q", got.Name, "gopls")
	}
	if len(got.Args) != 1 || got.Args[0] != "serve" {
		t.Errorf("Config().Args = %v, want [serve]", got.Args)
	}
}

func TestServerStateConstants(t *testing.T) {
	if StateStopped != 0 {
		t.Errorf("StateStopped = %d, want 0", StateStopped)
	}
	if StateStarting != 1 {
		t.Errorf("StateStarting = %d, want 1", StateStarting)
	}
	if StateRunning != 2 {
		t.Errorf("StateRunning = %d, want 2", StateRunning)
	}
	if StateStopping != 3 {
		t.Errorf("StateStopping = %d, want 3", StateStopping)
	}
	if StateError != 4 {
		t.Errorf("StateError = %d, want 4", StateError)
	}
}
