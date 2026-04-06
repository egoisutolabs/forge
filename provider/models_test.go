package provider

import "testing"

func TestBundledModels_ContainsExpected(t *testing.T) {
	expected := []struct {
		name     string
		provider string
	}{
		// Anthropic
		{"claude-opus-4-6", "anthropic"},
		{"claude-sonnet-4-6", "anthropic"},
		{"claude-haiku-4-5", "anthropic"},
		// OpenAI
		{"gpt-4o", "openai"},
		{"gpt-4o-mini", "openai"},
		{"o3", "openai"},
		{"o4-mini", "openai"},
		// DeepSeek
		{"deepseek-r1", "deepseek"},
		{"deepseek-v3", "deepseek"},
		// Meta
		{"llama-4-maverick", "meta"},
		{"llama-4-scout", "meta"},
		// Google
		{"gemini-2.5-pro", "google"},
		{"gemini-2.5-flash", "google"},
		// Mistral
		{"mistral-large", "mistral"},
		{"codestral", "mistral"},
		// Groq
		{"llama-3.3-70b-versatile", "groq"},
		// Ollama
		{"llama3.2", "ollama"},
		{"qwen2.5-coder", "ollama"},
		// xAI
		{"grok-3", "xai"},
	}

	models := BundledModels()
	index := make(map[string]ModelInfo, len(models))
	for _, m := range models {
		index[m.Name] = m
	}

	for _, tc := range expected {
		m, ok := index[tc.name]
		if !ok {
			t.Errorf("missing bundled model %q", tc.name)
			continue
		}
		if m.Provider != tc.provider {
			t.Errorf("model %q: provider = %q, want %q", tc.name, m.Provider, tc.provider)
		}
	}
}

func TestBundledModels_HasCosts(t *testing.T) {
	for _, m := range BundledModels() {
		// Ollama models are free, skip them.
		if m.Provider == "ollama" {
			continue
		}
		if m.InputCostPerMTok <= 0 {
			t.Errorf("model %q: InputCostPerMTok = %v, want > 0", m.Name, m.InputCostPerMTok)
		}
		if m.OutputCostPerMTok <= 0 {
			t.Errorf("model %q: OutputCostPerMTok = %v, want > 0", m.Name, m.OutputCostPerMTok)
		}
	}
}

func TestBundledModels_HasContextWindow(t *testing.T) {
	for _, m := range BundledModels() {
		if m.ContextWindow <= 0 {
			t.Errorf("model %q: ContextWindow = %d, want > 0", m.Name, m.ContextWindow)
		}
	}
}

func TestBundledModels_AllSupportStreaming(t *testing.T) {
	for _, m := range BundledModels() {
		if !m.SupportsStreaming {
			t.Errorf("model %q: SupportsStreaming = false, expected true", m.Name)
		}
	}
}

func TestLookupBundled(t *testing.T) {
	m, ok := LookupBundled("claude-sonnet-4-6")
	if !ok {
		t.Fatal("LookupBundled(claude-sonnet-4-6) returned false")
	}
	if m.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", m.Provider)
	}

	_, ok = LookupBundled("nonexistent-model")
	if ok {
		t.Error("LookupBundled(nonexistent-model) should return false")
	}
}

func TestBundledModels_ReturnsDefensiveCopy(t *testing.T) {
	a := BundledModels()
	b := BundledModels()
	a[0].Name = "mutated"
	if b[0].Name == "mutated" {
		t.Error("BundledModels should return a defensive copy")
	}
}
