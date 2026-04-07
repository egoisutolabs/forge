package models

// LoopStopReason indicates why the agent loop terminated.
type LoopStopReason string

const (
	StopCompleted       LoopStopReason = "completed"
	StopPromptTooLong   LoopStopReason = "prompt_too_long"
	StopModelError      LoopStopReason = "model_error"
	StopAborted         LoopStopReason = "aborted"
	StopBlockingLimit   LoopStopReason = "blocking_limit"
	StopBudgetExceeded  LoopStopReason = "budget_exceeded"
	StopOutputTruncated LoopStopReason = "output_truncated"
)

// LoopResult is the final output of the agent loop.
type LoopResult struct {
	Reason       LoopStopReason
	Messages     int     // total messages in conversation
	Turns        int     // number of loop iterations
	TotalUsage   Usage   // accumulated token usage across all turns
	TotalCostUSD float64 // accumulated cost in USD
}
