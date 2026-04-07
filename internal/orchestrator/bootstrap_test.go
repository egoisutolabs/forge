package orchestrator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/internal/orchestrator"
)

// ---- Slugify ----------------------------------------------------------------

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"add user auth", "add-user-auth"},
		{"Add User Auth", "add-user-auth"},
		{"implement OAuth 2.0 login", "implement-oauth-2-0-login"},
		{"  spaces  ", "spaces"},
		{"multi---dashes", "multi-dashes"},
		{"special!@#chars", "special-chars"},
		{"UPPER CASE", "upper-case"},
		{"already-a-slug", "already-a-slug"},
		{"trailing-", "trailing"},
		{"-leading", "leading"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := orchestrator.Slugify(c.input)
			if got != c.want {
				t.Errorf("Slugify(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestSlugify_Empty(t *testing.T) {
	got := orchestrator.Slugify("")
	if got != "" {
		t.Errorf("Slugify(\"\") = %q, want empty", got)
	}
}

// ---- Bootstrap.Run ----------------------------------------------------------

func newTestState() *orchestrator.ForgeState {
	return &orchestrator.ForgeState{
		Features: make(map[string]*orchestrator.FeatureEntry),
	}
}

func TestBootstrap_FreshFeature(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	rc, err := b.Run(context.Background(), "add user auth")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if rc.Slug != "add-user-auth" {
		t.Errorf("Slug = %q, want add-user-auth", rc.Slug)
	}
	if rc.Mode != "direct" {
		t.Errorf("Mode = %q, want direct", rc.Mode)
	}
	if rc.ResumeFrom != "plan" {
		t.Errorf("ResumeFrom = %q, want plan", rc.ResumeFrom)
	}
	if rc.Cwd != dir {
		t.Errorf("Cwd = %q, want %q", rc.Cwd, dir)
	}
}

func TestBootstrap_CreatesFeatureDir(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	rc, err := b.Run(context.Background(), "widget builder")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if _, err := os.Stat(rc.FeatureDir); err != nil {
		t.Errorf("FeatureDir %q not created: %v", rc.FeatureDir, err)
	}
}

func TestBootstrap_SavesStateFile(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	_, err := b.Run(context.Background(), "payment integration")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	stateFile := filepath.Join(dir, ".forge", "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("state.json not created at %q: %v", stateFile, err)
	}
}

func TestBootstrap_ResumeMidPipeline(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	slug := "resume-test"

	// Simulate plan and prepare done.
	_, err := state.Init(slug, "direct")
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	_ = state.SetPhase(slug, "plan", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "prepare", orchestrator.StatusDone)

	b := orchestrator.NewBootstrap(dir, state)
	rc, err := b.Run(context.Background(), "Resume Test")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if rc.Slug != slug {
		t.Errorf("Slug = %q, want %q", rc.Slug, slug)
	}
	if rc.ResumeFrom != "test" {
		t.Errorf("ResumeFrom = %q, want test", rc.ResumeFrom)
	}
}

func TestBootstrap_ConflictReturnsError(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()

	// Init first feature (incomplete).
	_, _ = state.Init("first-feature", "direct")

	b := orchestrator.NewBootstrap(dir, state)

	// Try to run a different feature.
	_, err := b.Run(context.Background(), "second feature")
	if err == nil {
		t.Fatal("expected error for conflicting active feature, got nil")
	}
}

func TestBootstrap_SameSlugNoConflict(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()

	// Init the same slug first.
	_, _ = state.Init("same-feature", "direct")

	b := orchestrator.NewBootstrap(dir, state)

	// Running the same feature again should not conflict.
	rc, err := b.Run(context.Background(), "same feature")
	if err != nil {
		t.Fatalf("Run() same feature error: %v", err)
	}
	if rc.Slug != "same-feature" {
		t.Errorf("Slug = %q, want same-feature", rc.Slug)
	}
}

func TestBootstrap_EnvInfo_GitDetection(t *testing.T) {
	dir := t.TempDir()
	// Create a fake .git directory.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	rc, err := b.Run(context.Background(), "git feature")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !rc.Env.HasGit {
		t.Error("EnvInfo.HasGit should be true when .git exists")
	}
}

func TestBootstrap_EnvInfo_NoGit(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	rc, err := b.Run(context.Background(), "no git feature")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if rc.Env.HasGit {
		t.Error("EnvInfo.HasGit should be false when .git does not exist")
	}
}

func TestBootstrap_EmptyDesc_Error(t *testing.T) {
	dir := t.TempDir()
	state := newTestState()
	b := orchestrator.NewBootstrap(dir, state)

	_, err := b.Run(context.Background(), "!@#$%")
	// All non-alphanumeric → empty slug → error
	if err == nil {
		t.Fatal("expected error for feature description that slugifies to empty, got nil")
	}
}
