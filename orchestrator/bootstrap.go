package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvInfo captures the detected runtime environment.
type EnvInfo struct {
	// HasGit is true when the working directory is inside a git repository.
	HasGit bool
	// HasDocker is true when the docker CLI is available on PATH.
	HasDocker bool
}

// RunContext is the resolved runtime context returned by Bootstrap.Run.
// It contains everything the orchestrator needs to drive the pipeline.
type RunContext struct {
	// Slug is the URL-safe identifier derived from the feature description.
	Slug string
	// Mode is the execution mode. Always "direct" in v1.
	Mode string
	// ResumeFrom is the name of the first phase that should run (either the
	// first incomplete phase for a resume, or "plan" for a fresh start).
	ResumeFrom string
	// Cwd is the absolute working directory for this session.
	Cwd string
	// ForgeDir is the absolute path to .forge/.
	ForgeDir string
	// FeatureDir is the absolute path to .forge/features/{slug}/.
	FeatureDir string
	// StateFile is the absolute path to .forge/state.json.
	StateFile string
	// Env describes detected environment capabilities.
	Env EnvInfo
	// ExtraContext holds optional answers or extra context injected between
	// plan retries (when the plan phase returns "blocked").
	ExtraContext string
}

// Bootstrap handles the initialization phase: slug derivation, environment
// detection, state initialisation, and resume-point determination.
type Bootstrap struct {
	Cwd   string
	State *ForgeState
}

// NewBootstrap creates a Bootstrap for the given working directory and state.
func NewBootstrap(cwd string, state *ForgeState) *Bootstrap {
	return &Bootstrap{Cwd: cwd, State: state}
}

// Run derives a slug from featureDesc, detects the environment, initialises or
// resumes state for the slug, creates the feature directory, persists state,
// and returns a RunContext ready for the orchestrator to act on.
//
// If another incomplete feature is already active, Run returns an error
// containing the conflicting slug so the caller can surface it to the user.
func (b *Bootstrap) Run(_ context.Context, featureDesc string) (RunContext, error) {
	slug := Slugify(featureDesc)
	if slug == "" {
		return RunContext{}, fmt.Errorf("bootstrap: feature description produced empty slug")
	}

	env := detectEnv(b.Cwd)
	mode := "direct" // v1: direct mode only

	conflict, err := b.State.Init(slug, mode)
	if err != nil {
		return RunContext{}, fmt.Errorf("bootstrap: state init: %w", err)
	}
	if conflict != "" {
		return RunContext{}, fmt.Errorf("bootstrap: another feature is active: %q — complete or remove it first", conflict)
	}

	resumeFrom, err := b.State.Resume(slug)
	if err != nil {
		return RunContext{}, fmt.Errorf("bootstrap: resume: %w", err)
	}
	if resumeFrom == "" {
		resumeFrom = "plan"
	}

	forgeDir := filepath.Join(b.Cwd, ".forge")
	featureDir := filepath.Join(forgeDir, "features", slug)
	stateFile := filepath.Join(forgeDir, "state.json")

	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		return RunContext{}, fmt.Errorf("bootstrap: create feature dir: %w", err)
	}

	if err := b.State.Save(stateFile); err != nil {
		return RunContext{}, fmt.Errorf("bootstrap: save state: %w", err)
	}

	return RunContext{
		Slug:       slug,
		Mode:       mode,
		ResumeFrom: resumeFrom,
		Cwd:        b.Cwd,
		ForgeDir:   forgeDir,
		FeatureDir: featureDir,
		StateFile:  stateFile,
		Env:        env,
	}, nil
}

// slugRe matches runs of characters that are NOT lowercase letters or digits.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a human-readable feature description to a URL-safe slug.
// "Add User Auth" → "add-user-auth"
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// detectEnv probes the working directory and PATH for git and docker.
func detectEnv(cwd string) EnvInfo {
	env := EnvInfo{}

	// Git: check for a .git directory in cwd (fast, no subprocess).
	if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
		env.HasGit = true
	}

	// Docker: check PATH only (no subprocess required).
	if _, err := exec.LookPath("docker"); err == nil {
		env.HasDocker = true
	}

	return env
}
