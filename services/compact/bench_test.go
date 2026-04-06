package compact

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
)

// BenchmarkEstimateTokens_Small benchmarks token estimation on a small conversation.
func BenchmarkEstimateTokens_Small(b *testing.B) {
	msgs := make([]*models.Message, 10)
	for i := range msgs {
		msgs[i] = userMsg("This is a typical user message with some content.")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(msgs)
	}
}

// BenchmarkEstimateTokens_Large benchmarks token estimation on a large conversation.
func BenchmarkEstimateTokens_Large(b *testing.B) {
	msgs := make([]*models.Message, 200)
	for i := range msgs {
		msgs[i] = userMsg(strings.Repeat("x", 4000)) // ~1000 tokens each
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(msgs)
	}
}

// BenchmarkIncrementalEstimate benchmarks the incremental path (existing count + new messages).
func BenchmarkIncrementalEstimate(b *testing.B) {
	newMsgs := []*models.Message{
		userMsg("A new message in the conversation."),
		userMsg("And another one from the assistant side."),
	}
	existing := 50000 // typical mid-session estimate
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IncrementalEstimate(existing, newMsgs)
	}
}
