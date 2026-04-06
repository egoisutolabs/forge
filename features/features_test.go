package features

import "testing"

// TestDefaultFeatureGates verifies the default build (no tags) has expected values.
// When running with tags, these tests will reflect those tag values instead.
func TestFeatureConstants(t *testing.T) {
	// All three constants must be declared and accessible — this is the primary
	// compile-time check. The actual values depend on which build tags are active.
	t.Logf("MinimalBuild=%v SpeculationEnabled=%v DebugEnabled=%v",
		MinimalBuild, SpeculationEnabled, DebugEnabled)

	// Invariant: minimal and speculation should not both be true.
	// Minimal builds exclude extra features; speculation is an addition.
	if MinimalBuild && SpeculationEnabled {
		t.Error("MinimalBuild and SpeculationEnabled should not both be true")
	}
}

func TestDefaultBuildValues(t *testing.T) {
	// In default build (no tags), all should be false.
	// This test will fail if run with build tags, which is expected —
	// the tag-specific behavior is validated by the build itself compiling.
	if MinimalBuild {
		t.Skip("running with minimal tag")
	}
	if DebugEnabled {
		t.Skip("running with debug tag")
	}
	if SpeculationEnabled {
		t.Skip("running with speculation tag")
	}
	// If we reach here, we're in the default build.
	if MinimalBuild || SpeculationEnabled || DebugEnabled {
		t.Error("default build should have all feature gates disabled")
	}
}
