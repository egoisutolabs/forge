package models

import "strings"

// Usage tracks token consumption for a single API call or accumulated across turns.
//
// This is the Go equivalent of Claude Code's NonNullableUsage type.
// Token counts are summed during accumulation; metadata fields (ServiceTier, Speed)
// use last-write-wins semantics.
type Usage struct {
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	CacheRead     int    `json:"cache_read_input_tokens,omitempty"`
	CacheCreate   int    `json:"cache_creation_input_tokens,omitempty"`
	WebSearchReqs int    `json:"web_search_requests,omitempty"`
	ServiceTier   string `json:"service_tier,omitempty"`
	Speed         string `json:"speed,omitempty"` // "standard" or "fast"
}

// TotalTokens returns the sum of input and output tokens.
func (u *Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// EmptyUsage returns a zero-value Usage with default metadata.
func EmptyUsage() Usage {
	return Usage{
		ServiceTier: "standard",
		Speed:       "standard",
	}
}

// AccumulateUsage merges two Usage values.
// Token counts are summed. Metadata (ServiceTier, Speed) uses the value from b.
func AccumulateUsage(a, b Usage) Usage {
	return Usage{
		InputTokens:   a.InputTokens + b.InputTokens,
		OutputTokens:  a.OutputTokens + b.OutputTokens,
		CacheRead:     a.CacheRead + b.CacheRead,
		CacheCreate:   a.CacheCreate + b.CacheCreate,
		WebSearchReqs: a.WebSearchReqs + b.WebSearchReqs,
		ServiceTier:   b.ServiceTier, // last write wins
		Speed:         b.Speed,       // last write wins
	}
}

// --- Cost calculation ---

// ModelCosts holds per-million-token pricing for a model.
type ModelCosts struct {
	InputTokens         float64 // USD per million input tokens
	OutputTokens        float64 // USD per million output tokens
	PromptCacheRead     float64 // USD per million cache-read tokens
	PromptCacheWrite    float64 // USD per million cache-write tokens
	WebSearchPerRequest float64 // USD per web search request
}

// Pricing tiers from Claude Code's modelCost.ts.
var (
	costHaiku35 = ModelCosts{
		InputTokens: 0.80, OutputTokens: 4,
		PromptCacheWrite: 1, PromptCacheRead: 0.08,
		WebSearchPerRequest: 0.01,
	}
	costHaiku45 = ModelCosts{
		InputTokens: 1, OutputTokens: 5,
		PromptCacheWrite: 1.25, PromptCacheRead: 0.1,
		WebSearchPerRequest: 0.01,
	}
	costTier3_15 = ModelCosts{ // Sonnet family
		InputTokens: 3, OutputTokens: 15,
		PromptCacheWrite: 3.75, PromptCacheRead: 0.3,
		WebSearchPerRequest: 0.01,
	}
	costTier5_25 = ModelCosts{ // Opus 4.5, Opus 4.6 standard
		InputTokens: 5, OutputTokens: 25,
		PromptCacheWrite: 6.25, PromptCacheRead: 0.5,
		WebSearchPerRequest: 0.01,
	}
	costTier15_75 = ModelCosts{ // Opus 4.0, Opus 4.1
		InputTokens: 15, OutputTokens: 75,
		PromptCacheWrite: 18.75, PromptCacheRead: 1.5,
		WebSearchPerRequest: 0.01,
	}
	costTier30_150 = ModelCosts{ // Opus 4.6 fast mode
		InputTokens: 30, OutputTokens: 150,
		PromptCacheWrite: 37.5, PromptCacheRead: 3,
		WebSearchPerRequest: 0.01,
	}
)

// CostConfig allows overriding per-model costs for non-Anthropic providers.
type CostConfig struct {
	CustomCosts map[string]ModelCosts // model name → costs
}

// CostForModel calculates the USD cost for a given model and usage.
// Fast mode (usage.Speed == "fast") only affects Opus 4.6 pricing.
// Unknown models fall back to Sonnet tier.
func CostForModel(model string, u Usage) float64 {
	costs := costsForModel(model, u.Speed)
	return tokensToUSD(costs, u)
}

// CostForModelWithConfig checks custom costs first, then falls back to built-in pricing.
func CostForModelWithConfig(model string, u Usage, cc *CostConfig) float64 {
	if cc != nil {
		if custom, ok := cc.CustomCosts[model]; ok {
			return tokensToUSD(custom, u)
		}
	}
	return CostForModel(model, u)
}

func tokensToUSD(c ModelCosts, u Usage) float64 {
	return float64(u.InputTokens)/1_000_000*c.InputTokens +
		float64(u.OutputTokens)/1_000_000*c.OutputTokens +
		float64(u.CacheRead)/1_000_000*c.PromptCacheRead +
		float64(u.CacheCreate)/1_000_000*c.PromptCacheWrite +
		float64(u.WebSearchReqs)*c.WebSearchPerRequest
}

func costsForModel(model string, speed string) ModelCosts {
	m := strings.ToLower(model)

	switch {
	// Opus 4.6 — fast mode gets 6x pricing
	case strings.Contains(m, "opus-4-6") || strings.Contains(m, "opus-4.6"):
		if speed == "fast" {
			return costTier30_150
		}
		return costTier5_25

	// Opus 4.5
	case strings.Contains(m, "opus-4-5") || strings.Contains(m, "opus-4.5"):
		return costTier5_25

	// Opus 4.0, 4.1
	case strings.Contains(m, "opus-4-1") || strings.Contains(m, "opus-4.1"):
		return costTier15_75
	case strings.Contains(m, "opus-4-0") || strings.Contains(m, "opus-4.0") ||
		(strings.Contains(m, "opus-4") && !strings.Contains(m, "opus-4-")):
		return costTier15_75

	// Sonnet family (3.5v2, 3.7, 4, 4.5, 4.6) — all $3/$15
	case strings.Contains(m, "sonnet"):
		return costTier3_15

	// Haiku 4.5
	case strings.Contains(m, "haiku-4-5") || strings.Contains(m, "haiku-4.5"):
		return costHaiku45

	// Haiku 3.5
	case strings.Contains(m, "haiku-3-5") || strings.Contains(m, "haiku-3.5"):
		return costHaiku35

	// Haiku (generic)
	case strings.Contains(m, "haiku"):
		return costHaiku45

	// Unknown model — fall back to Sonnet tier
	default:
		return costTier3_15
	}
}
