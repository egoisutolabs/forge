package engine

import (
	"context"
	"math"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

func assistantTextWithUsage(text string, usage models.Usage) *models.Message {
	msg := assistantText(text)
	msg.Usage = &usage
	return msg
}

func TestLoop_AccumulatesUsage(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("first", models.Usage{
				InputTokens: 1000, OutputTokens: 200, Speed: "standard",
			}),
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Tools:    nil,
		Model:    "claude-sonnet-4-6-20250514",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalUsage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", result.TotalUsage.InputTokens)
	}
	if result.TotalUsage.OutputTokens != 200 {
		t.Errorf("OutputTokens = %d, want 200", result.TotalUsage.OutputTokens)
	}
}

func TestLoop_AccumulatesUsageAcrossTurns(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: tool call
			func() *models.Message {
				msg := assistantWithToolUse("checking", "Echo", `{}`)
				u := models.Usage{InputTokens: 500, OutputTokens: 100, Speed: "standard"}
				msg.Usage = &u
				return msg
			}(),
			// Turn 2: final text
			assistantTextWithUsage("done", models.Usage{
				InputTokens: 800, OutputTokens: 300, Speed: "standard",
			}),
		},
	}

	echoTool := &mockTool{name: "Echo", result: "ok", safe: true}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("test")},
		Tools:    []tools.Tool{echoTool},
		Model:    "claude-sonnet-4-6-20250514",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should sum across both turns
	if result.TotalUsage.InputTokens != 1300 {
		t.Errorf("InputTokens = %d, want 1300", result.TotalUsage.InputTokens)
	}
	if result.TotalUsage.OutputTokens != 400 {
		t.Errorf("OutputTokens = %d, want 400", result.TotalUsage.OutputTokens)
	}
}

func TestLoop_BudgetExceeded(t *testing.T) {
	// Each turn costs enough to exceed the budget
	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: costs will be computed
			func() *models.Message {
				msg := assistantWithToolUse("working", "Echo", `{}`)
				// 1M input + 1M output on Sonnet = $3 + $15 = $18
				u := models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "standard"}
				msg.Usage = &u
				return msg
			}(),
			// Turn 2: should not be reached if budget is $10
			assistantText("should not reach"),
		},
	}

	echoTool := &mockTool{name: "Echo", result: "ok", safe: true}
	budget := 10.0

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("expensive")},
		Tools:        []tools.Tool{echoTool},
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: &budget,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBudgetExceeded {
		t.Errorf("reason = %v, want %v", result.Reason, models.StopBudgetExceeded)
	}
	if result.Turns != 1 {
		t.Errorf("turns = %d, want 1 (should stop after first turn)", result.Turns)
	}
	if result.TotalCostUSD == 0 {
		t.Error("TotalCostUSD should be > 0")
	}
}

func TestLoop_NoBudgetMeansUnlimited(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("done", models.Usage{
				InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "standard",
			}),
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: nil, // no budget
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("reason = %v, want completed (no budget limit)", result.Reason)
	}
}

func TestLoop_CostOnResult(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("hi", models.Usage{
				InputTokens: 5000, OutputTokens: 1000, CacheRead: 4000, Speed: "standard",
			}),
		},
	}

	result, _, _ := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "claude-sonnet-4-6-20250514",
		MaxTurns: 10,
	})

	// (5000/1M)*3 + (1000/1M)*15 + (4000/1M)*0.30 = 0.015 + 0.015 + 0.0012 = 0.0312
	if !almostEqual(result.TotalCostUSD, 0.0312, 0.0001) {
		t.Errorf("TotalCostUSD = %f, want ~0.0312", result.TotalCostUSD)
	}
}

func TestSubmitMessage_AccumulatesUsageAcrossSubmissions(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("first", models.Usage{InputTokens: 100, OutputTokens: 50, Speed: "standard"}),
			assistantTextWithUsage("second", models.Usage{InputTokens: 200, OutputTokens: 100, Speed: "standard"}),
		},
	}

	qe := New(Config{Model: "claude-sonnet-4-6-20250514", MaxTurns: 10, Cwd: "/tmp"})

	_, err := qe.SubmitMessage(context.Background(), caller, "first")
	if err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}

	_, err = qe.SubmitMessage(context.Background(), caller, "second")
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}

	usage := qe.TotalUsage()
	if usage.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", usage.OutputTokens)
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}
