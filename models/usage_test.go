package models

import (
	"math"
	"testing"
)

func TestAccumulateUsage_SumsTokenCounts(t *testing.T) {
	a := Usage{
		InputTokens:   1000,
		OutputTokens:  500,
		CacheRead:     200,
		CacheCreate:   100,
		WebSearchReqs: 2,
	}
	b := Usage{
		InputTokens:   2000,
		OutputTokens:  300,
		CacheRead:     150,
		CacheCreate:   50,
		WebSearchReqs: 1,
	}

	result := AccumulateUsage(a, b)

	if result.InputTokens != 3000 {
		t.Errorf("InputTokens = %d, want 3000", result.InputTokens)
	}
	if result.OutputTokens != 800 {
		t.Errorf("OutputTokens = %d, want 800", result.OutputTokens)
	}
	if result.CacheRead != 350 {
		t.Errorf("CacheRead = %d, want 350", result.CacheRead)
	}
	if result.CacheCreate != 150 {
		t.Errorf("CacheCreate = %d, want 150", result.CacheCreate)
	}
	if result.WebSearchReqs != 3 {
		t.Errorf("WebSearchReqs = %d, want 3", result.WebSearchReqs)
	}
}

func TestAccumulateUsage_LastWriteWinsForMetadata(t *testing.T) {
	a := Usage{
		InputTokens: 100,
		ServiceTier: "standard",
		Speed:       "standard",
	}
	b := Usage{
		InputTokens: 200,
		ServiceTier: "priority",
		Speed:       "fast",
	}

	result := AccumulateUsage(a, b)

	if result.ServiceTier != "priority" {
		t.Errorf("ServiceTier = %q, want 'priority' (last write wins)", result.ServiceTier)
	}
	if result.Speed != "fast" {
		t.Errorf("Speed = %q, want 'fast' (last write wins)", result.Speed)
	}
}

func TestAccumulateUsage_WithEmpty(t *testing.T) {
	empty := EmptyUsage()
	real := Usage{
		InputTokens:  5000,
		OutputTokens: 1000,
		CacheRead:    3000,
		Speed:        "standard",
	}

	result := AccumulateUsage(empty, real)

	if result.InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", result.InputTokens)
	}
	if result.OutputTokens != 1000 {
		t.Errorf("OutputTokens = %d, want 1000", result.OutputTokens)
	}
}

func TestEmptyUsage(t *testing.T) {
	u := EmptyUsage()
	if u.InputTokens != 0 || u.OutputTokens != 0 || u.CacheRead != 0 || u.CacheCreate != 0 {
		t.Error("EmptyUsage should have all zero token counts")
	}
	if u.Speed != "standard" {
		t.Errorf("Speed = %q, want 'standard'", u.Speed)
	}
	if u.ServiceTier != "standard" {
		t.Errorf("ServiceTier = %q, want 'standard'", u.ServiceTier)
	}
}

// --- Cost calculation tests ---

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

func TestCostForModel_Sonnet46(t *testing.T) {
	// Sonnet 4.6: $3 input, $15 output, $0.30 cache read, $3.75 cache write per Mtok
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		CacheRead:    1_000_000,
		CacheCreate:  1_000_000,
		Speed:        "standard",
	}

	cost := CostForModel("claude-sonnet-4-6-20250514", u)
	// $3 + $15 + $0.30 + $3.75 = $22.05
	if !almostEqual(cost, 22.05, 0.001) {
		t.Errorf("cost = %f, want 22.05", cost)
	}
}

func TestCostForModel_Opus46Standard(t *testing.T) {
	// Opus 4.6 standard: $5 input, $25 output per Mtok
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	cost := CostForModel("claude-opus-4-6-20250514", u)
	// $5 + $25 = $30
	if !almostEqual(cost, 30.0, 0.001) {
		t.Errorf("cost = %f, want 30.0", cost)
	}
}

func TestCostForModel_Opus46Fast(t *testing.T) {
	// Opus 4.6 fast: $30 input, $150 output per Mtok
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "fast",
	}

	cost := CostForModel("claude-opus-4-6-20250514", u)
	// $30 + $150 = $180
	if !almostEqual(cost, 180.0, 0.001) {
		t.Errorf("cost = %f, want 180.0", cost)
	}
}

func TestCostForModel_Haiku45(t *testing.T) {
	// Haiku 4.5: $1 input, $5 output per Mtok
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	cost := CostForModel("claude-haiku-4-5-20251001", u)
	// $1 + $5 = $6
	if !almostEqual(cost, 6.0, 0.001) {
		t.Errorf("cost = %f, want 6.0", cost)
	}
}

func TestCostForModel_WebSearchRequests(t *testing.T) {
	u := Usage{
		WebSearchReqs: 10,
		Speed:         "standard",
	}

	cost := CostForModel("claude-sonnet-4-6-20250514", u)
	// 10 * $0.01 = $0.10
	if !almostEqual(cost, 0.10, 0.001) {
		t.Errorf("cost = %f, want 0.10", cost)
	}
}

func TestCostForModel_UnknownModelFallsBackToSonnet(t *testing.T) {
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	// Unknown model should fall back to Sonnet tier ($3/$15)
	cost := CostForModel("claude-unknown-99", u)
	if !almostEqual(cost, 18.0, 0.001) {
		t.Errorf("cost = %f, want 18.0 (sonnet fallback)", cost)
	}
}

func TestCostForModel_SmallTokenCounts(t *testing.T) {
	// Realistic small usage: 5K input, 1K output
	u := Usage{
		InputTokens:  5000,
		OutputTokens: 1000,
		CacheRead:    4000,
		Speed:        "standard",
	}

	cost := CostForModel("claude-sonnet-4-6-20250514", u)
	// (5000/1M)*3 + (1000/1M)*15 + (4000/1M)*0.30
	// = 0.015 + 0.015 + 0.0012 = 0.0312
	if !almostEqual(cost, 0.0312, 0.0001) {
		t.Errorf("cost = %f, want ~0.0312", cost)
	}
}

func TestCostForModel_ZeroUsage(t *testing.T) {
	u := EmptyUsage()
	cost := CostForModel("claude-sonnet-4-6-20250514", u)
	if cost != 0 {
		t.Errorf("cost = %f, want 0 for empty usage", cost)
	}
}

// --- CostForModelWithConfig tests ---

func TestCostForModelWithConfig_CustomCosts(t *testing.T) {
	cc := &CostConfig{
		CustomCosts: map[string]ModelCosts{
			"deepseek/deepseek-r1": {
				InputTokens:  0.55,
				OutputTokens: 2.19,
			},
		},
	}

	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	cost := CostForModelWithConfig("deepseek/deepseek-r1", u, cc)
	// $0.55 + $2.19 = $2.74
	if !almostEqual(cost, 2.74, 0.001) {
		t.Errorf("cost = %f, want 2.74", cost)
	}
}

func TestCostForModelWithConfig_FallsBackToBuiltIn(t *testing.T) {
	cc := &CostConfig{
		CustomCosts: map[string]ModelCosts{
			"some-other-model": {InputTokens: 1, OutputTokens: 1},
		},
	}

	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	// claude-sonnet should fall back to built-in pricing.
	cost := CostForModelWithConfig("claude-sonnet-4-6-20250514", u, cc)
	expected := CostForModel("claude-sonnet-4-6-20250514", u)
	if !almostEqual(cost, expected, 0.001) {
		t.Errorf("cost = %f, want %f (built-in)", cost, expected)
	}
}

func TestCostForModelWithConfig_NilConfig(t *testing.T) {
	u := Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Speed:        "standard",
	}

	// nil CostConfig should fall back to built-in.
	cost := CostForModelWithConfig("claude-sonnet-4-6-20250514", u, nil)
	expected := CostForModel("claude-sonnet-4-6-20250514", u)
	if !almostEqual(cost, expected, 0.001) {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestCostForModelWithConfig_FreeModel(t *testing.T) {
	cc := &CostConfig{
		CustomCosts: map[string]ModelCosts{
			"llama3:70b": {InputTokens: 0, OutputTokens: 0},
		},
	}

	u := Usage{
		InputTokens:  10_000_000,
		OutputTokens: 5_000_000,
	}

	cost := CostForModelWithConfig("llama3:70b", u, cc)
	if cost != 0 {
		t.Errorf("cost = %f, want 0 for free model", cost)
	}
}
