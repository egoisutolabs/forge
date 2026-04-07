// Package orchestrator implements the forge 5-phase development pipeline.
// This file contains the state machine: ForgeState, FeatureEntry, PhaseState
// and their Load/Save/mutation methods, replacing the bash forge-state script.
package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

// PhaseStatus represents the lifecycle state of a single pipeline phase.
type PhaseStatus string

const (
	StatusNull    PhaseStatus = ""        // not yet started
	StatusRunning PhaseStatus = "running" // agent is executing
	StatusDone    PhaseStatus = "done"    // phase completed successfully
	StatusFail    PhaseStatus = "fail"    // phase failed; eligible for retry
	StatusBlocked PhaseStatus = "blocked" // waiting for user input
)

// PhaseState holds the status and completion timestamp of a single phase.
type PhaseState struct {
	Status      PhaseStatus `json:"status"`
	CompletedAt *time.Time  `json:"completed_at"`
}

// FeatureEntry tracks the full lifecycle of one feature slug.
type FeatureEntry struct {
	StartedAt time.Time              `json:"started_at"`
	Mode      string                 `json:"mode"` // "direct" or "github"
	Phases    map[string]*PhaseState `json:"phases"`
	Retries   int                    `json:"retries"`
}

// ForgeState is the top-level state document stored at .forge/state.json.
type ForgeState struct {
	Active   string                   `json:"active"`
	Features map[string]*FeatureEntry `json:"features"`
}

// PhaseOrder is the canonical 5-phase ordering used for resume and validation.
var PhaseOrder = []string{"plan", "prepare", "test", "implement", "verify"}

// Load reads and parses the state file at path. Creates an empty state when the
// file does not exist or is empty. Uses a shared (read) file lock.
func Load(path string) (*ForgeState, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("state: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("state: flock shared: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("state: stat: %w", err)
	}
	if info.Size() == 0 {
		return &ForgeState{Features: make(map[string]*FeatureEntry)}, nil
	}

	var s ForgeState
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("state: decode %s: %w", path, err)
	}
	if s.Features == nil {
		s.Features = make(map[string]*FeatureEntry)
	}
	migrateState(&s)
	return &s, nil
}

// Save writes the state to path with an exclusive file lock.
func (s *ForgeState) Save(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("state: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("state: flock exclusive: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("state: truncate: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("state: seek: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("state: encode: %w", err)
	}
	return nil
}

// Init creates a feature entry for slug and marks it active.
// Returns the conflicting active slug when another feature has incomplete phases.
// Returns ("", nil) on success.
func (s *ForgeState) Init(slug, mode string) (conflict string, err error) {
	if s.Active != "" && s.Active != slug {
		if existing, ok := s.Features[s.Active]; ok && !featureComplete(existing) {
			return s.Active, nil
		}
	}
	if _, exists := s.Features[slug]; !exists {
		phases := make(map[string]*PhaseState, len(PhaseOrder))
		for _, p := range PhaseOrder {
			phases[p] = &PhaseState{Status: StatusNull}
		}
		s.Features[slug] = &FeatureEntry{
			StartedAt: time.Now().UTC(),
			Mode:      mode,
			Phases:    phases,
		}
	}
	s.Active = slug
	return "", nil
}

// SetPhase updates the status of a phase. Auto-populates CompletedAt when
// status is StatusDone and clears it otherwise.
func (s *ForgeState) SetPhase(slug, phase string, status PhaseStatus) error {
	entry, ok := s.Features[slug]
	if !ok {
		return fmt.Errorf("state: feature %q not found", slug)
	}
	ps, ok := entry.Phases[phase]
	if !ok {
		ps = &PhaseState{}
		entry.Phases[phase] = ps
	}
	ps.Status = status
	if status == StatusDone {
		now := time.Now().UTC()
		ps.CompletedAt = &now
	} else {
		ps.CompletedAt = nil
	}
	return nil
}

// GetPhase returns the current status of a phase. Returns StatusNull when the
// phase has no recorded status.
func (s *ForgeState) GetPhase(slug, phase string) (PhaseStatus, error) {
	entry, ok := s.Features[slug]
	if !ok {
		return StatusNull, fmt.Errorf("state: feature %q not found", slug)
	}
	ps, ok := entry.Phases[phase]
	if !ok {
		return StatusNull, nil
	}
	return ps.Status, nil
}

// Resume returns the first phase name whose status is not StatusDone.
// Returns "" when all canonical phases are done (feature complete).
func (s *ForgeState) Resume(slug string) (string, error) {
	entry, ok := s.Features[slug]
	if !ok {
		return "", fmt.Errorf("state: feature %q not found", slug)
	}
	for _, p := range PhaseOrder {
		ps, ok := entry.Phases[p]
		if !ok || ps.Status != StatusDone {
			return p, nil
		}
	}
	return "", nil // all done
}

// ActiveFeature returns the slug of the currently active feature.
func (s *ForgeState) ActiveFeature() string {
	return s.Active
}

// Remove deletes a feature and reassigns active to the first remaining
// incomplete feature. If none exist, active is cleared.
func (s *ForgeState) Remove(slug string) {
	delete(s.Features, slug)
	if s.Active != slug {
		return
	}
	s.Active = ""
	for k, v := range s.Features {
		if !featureComplete(v) {
			s.Active = k
			return
		}
	}
}

// featureComplete returns true when all canonical phases are StatusDone.
func featureComplete(entry *FeatureEntry) bool {
	for _, p := range PhaseOrder {
		ps, ok := entry.Phases[p]
		if !ok || ps.Status != StatusDone {
			return false
		}
	}
	return true
}

// legacyToPhase maps old 9-phase names to their 5-phase equivalents.
var legacyToPhase = map[string]string{
	"discover":       "plan",
	"explore":        "plan",
	"design":         "plan",
	"design-discuss": "plan",
	"architect":      "plan",
	"handoff":        "prepare",
	"spike":          "prepare",
}

// migrateState converts any legacy 9-phase entries to the canonical 5-phase schema.
func migrateState(s *ForgeState) {
	for _, entry := range s.Features {
		for old, newPhase := range legacyToPhase {
			ps, ok := entry.Phases[old]
			if !ok {
				continue
			}
			existing := entry.Phases[newPhase]
			// Promote legacy phase if new phase slot is empty or less advanced.
			if existing == nil || (ps.Status == StatusDone && existing.Status != StatusDone) {
				entry.Phases[newPhase] = ps
			}
			delete(entry.Phases, old)
		}
		// Ensure all canonical phases exist.
		for _, p := range PhaseOrder {
			if _, ok := entry.Phases[p]; !ok {
				entry.Phases[p] = &PhaseState{Status: StatusNull}
			}
		}
	}
}
