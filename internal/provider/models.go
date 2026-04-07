// Package provider implements model metadata, provider auto-detection, and a
// central registry for available models.
package provider

// ModelInfo describes a single model's capabilities and pricing.
type ModelInfo struct {
	Name              string  // e.g. "claude-sonnet-4-6"
	Provider          string  // e.g. "anthropic", "openai", "ollama"
	InputCostPerMTok  float64 // USD per million input tokens
	OutputCostPerMTok float64 // USD per million output tokens
	ContextWindow     int     // max context in tokens
	OutputLimit       int     // max output tokens (0 = use default)
	SupportsTools     bool
	SupportsStreaming bool
}

// bundledModels is the static model catalog, compiled into the binary so no
// API call is required at startup.
var bundledModels = []ModelInfo{
	// --- Anthropic ---
	{Name: "claude-opus-4-6", Provider: "anthropic", InputCostPerMTok: 5, OutputCostPerMTok: 25, ContextWindow: 200000, SupportsTools: true, SupportsStreaming: true},
	{Name: "claude-sonnet-4-6", Provider: "anthropic", InputCostPerMTok: 3, OutputCostPerMTok: 15, ContextWindow: 200000, SupportsTools: true, SupportsStreaming: true},
	{Name: "claude-haiku-4-5", Provider: "anthropic", InputCostPerMTok: 1, OutputCostPerMTok: 5, ContextWindow: 200000, SupportsTools: true, SupportsStreaming: true},

	// --- OpenAI ---
	{Name: "gpt-4o", Provider: "openai", InputCostPerMTok: 2.5, OutputCostPerMTok: 10, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},
	{Name: "gpt-4o-mini", Provider: "openai", InputCostPerMTok: 0.15, OutputCostPerMTok: 0.6, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},
	{Name: "o3", Provider: "openai", InputCostPerMTok: 10, OutputCostPerMTok: 40, ContextWindow: 200000, SupportsTools: true, SupportsStreaming: true},
	{Name: "o4-mini", Provider: "openai", InputCostPerMTok: 1.1, OutputCostPerMTok: 4.4, ContextWindow: 200000, SupportsTools: true, SupportsStreaming: true},

	// --- DeepSeek ---
	{Name: "deepseek-r1", Provider: "deepseek", InputCostPerMTok: 0.55, OutputCostPerMTok: 2.19, ContextWindow: 64000, SupportsTools: false, SupportsStreaming: true},
	{Name: "deepseek-v3", Provider: "deepseek", InputCostPerMTok: 0.27, OutputCostPerMTok: 1.1, ContextWindow: 64000, SupportsTools: true, SupportsStreaming: true},

	// --- Meta (via common providers) ---
	{Name: "llama-4-maverick", Provider: "meta", InputCostPerMTok: 0.5, OutputCostPerMTok: 0.7, ContextWindow: 1048576, SupportsTools: true, SupportsStreaming: true},
	{Name: "llama-4-scout", Provider: "meta", InputCostPerMTok: 0.15, OutputCostPerMTok: 0.3, ContextWindow: 512000, SupportsTools: true, SupportsStreaming: true},

	// --- Google ---
	{Name: "gemini-2.5-pro", Provider: "google", InputCostPerMTok: 1.25, OutputCostPerMTok: 10, ContextWindow: 1048576, SupportsTools: true, SupportsStreaming: true},
	{Name: "gemini-2.5-flash", Provider: "google", InputCostPerMTok: 0.15, OutputCostPerMTok: 0.6, ContextWindow: 1048576, SupportsTools: true, SupportsStreaming: true},

	// --- Mistral ---
	{Name: "mistral-large", Provider: "mistral", InputCostPerMTok: 2, OutputCostPerMTok: 6, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},
	{Name: "codestral", Provider: "mistral", InputCostPerMTok: 0.3, OutputCostPerMTok: 0.9, ContextWindow: 256000, SupportsTools: true, SupportsStreaming: true},

	// --- Groq (hosted models) ---
	{Name: "llama-3.3-70b-versatile", Provider: "groq", InputCostPerMTok: 0.59, OutputCostPerMTok: 0.79, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},
	{Name: "llama-3.1-8b-instant", Provider: "groq", InputCostPerMTok: 0.05, OutputCostPerMTok: 0.08, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},

	// --- Ollama (common local models) ---
	{Name: "llama3.2", Provider: "ollama", InputCostPerMTok: 0, OutputCostPerMTok: 0, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},
	{Name: "qwen2.5-coder", Provider: "ollama", InputCostPerMTok: 0, OutputCostPerMTok: 0, ContextWindow: 32768, SupportsTools: true, SupportsStreaming: true},
	{Name: "deepseek-coder-v2", Provider: "ollama", InputCostPerMTok: 0, OutputCostPerMTok: 0, ContextWindow: 128000, SupportsTools: true, SupportsStreaming: true},

	// --- xAI ---
	{Name: "grok-3", Provider: "xai", InputCostPerMTok: 3, OutputCostPerMTok: 15, ContextWindow: 131072, SupportsTools: true, SupportsStreaming: true},
}

// BundledModels returns the full static model catalog.
func BundledModels() []ModelInfo {
	out := make([]ModelInfo, len(bundledModels))
	copy(out, bundledModels)
	return out
}

// bundledIndex provides O(1) lookup by model name.
var bundledIndex map[string]ModelInfo

func init() {
	bundledIndex = make(map[string]ModelInfo, len(bundledModels))
	for _, m := range bundledModels {
		bundledIndex[m.Name] = m
	}
}

// LookupBundled returns the ModelInfo for a known model name, or false.
func LookupBundled(name string) (ModelInfo, bool) {
	m, ok := bundledIndex[name]
	return m, ok
}
