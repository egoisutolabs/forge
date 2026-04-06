package engine

import (
	"testing"

	"github.com/egoisutolabs/forge/models"
)

func TestNew_DefaultMaxTurns(t *testing.T) {
	qe := New(Config{Model: "claude-sonnet-4-6", Cwd: "/tmp"})
	if qe.config.MaxTurns != 100 {
		t.Errorf("expected default maxTurns=100, got %d", qe.config.MaxTurns)
	}
}

func TestNew_CustomMaxTurns(t *testing.T) {
	qe := New(Config{Model: "claude-sonnet-4-6", MaxTurns: 50, Cwd: "/tmp"})
	if qe.config.MaxTurns != 50 {
		t.Errorf("expected maxTurns=50, got %d", qe.config.MaxTurns)
	}
}

func TestNew_HasPermissions(t *testing.T) {
	qe := New(Config{Cwd: "/tmp"})
	if qe.Permissions() == nil {
		t.Fatal("expected non-nil permissions context")
	}
	if qe.Permissions().Mode != models.ModeDefault {
		t.Errorf("expected default mode, got %v", qe.Permissions().Mode)
	}
}

func TestNew_EmptyMessages(t *testing.T) {
	qe := New(Config{Cwd: "/tmp"})
	if len(qe.Messages()) != 0 {
		t.Errorf("expected 0 messages, got %d", len(qe.Messages()))
	}
}
