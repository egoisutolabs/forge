package compact

import (
	"fmt"

	"github.com/egoisutolabs/forge/models"
)

// DefaultKeepRecent is the number of most recent tool results to keep intact.
const DefaultKeepRecent = 5

// MicroCompactResult holds the output of a MicroCompact operation.
type MicroCompactResult struct {
	Messages    []*models.Message
	TokensSaved int
}

// MicroCompact replaces old tool_result content with a placeholder to reduce
// token usage before the more expensive full auto-compact kicks in.
//
// It keeps the most recent keepRecent tool results intact and replaces older
// ones with "[tool result cleared - N bytes]". If keepRecent <= 0, it defaults
// to DefaultKeepRecent.
//
// Returns modified messages (a shallow copy of the slice with affected messages
// deep-copied) and an estimate of tokens saved.
func MicroCompact(messages []*models.Message, keepRecent int) MicroCompactResult {
	if keepRecent <= 0 {
		keepRecent = DefaultKeepRecent
	}
	if len(messages) == 0 {
		return MicroCompactResult{Messages: messages}
	}

	// Count total tool_result blocks across all messages.
	type toolResultLoc struct {
		msgIdx   int
		blockIdx int
	}
	var locs []toolResultLoc
	for i, msg := range messages {
		for j, b := range msg.Content {
			if b.Type == models.BlockToolResult {
				locs = append(locs, toolResultLoc{i, j})
			}
		}
	}

	// If there are fewer tool results than keepRecent, nothing to clear.
	if len(locs) <= keepRecent {
		return MicroCompactResult{Messages: messages}
	}

	// The ones to clear are all except the last keepRecent.
	toClear := locs[:len(locs)-keepRecent]

	// Build a set of message indices that need modification.
	modifiedMsgs := make(map[int]bool)
	for _, loc := range toClear {
		modifiedMsgs[loc.msgIdx] = true
	}

	// Create a shallow copy of the messages slice, deep-copying only modified messages.
	result := make([]*models.Message, len(messages))
	copy(result, messages)
	for idx := range modifiedMsgs {
		orig := messages[idx]
		cp := *orig
		cp.Content = make([]models.Block, len(orig.Content))
		copy(cp.Content, orig.Content)
		result[idx] = &cp
	}

	// Replace old tool_result content.
	tokensSaved := 0
	for _, loc := range toClear {
		block := &result[loc.msgIdx].Content[loc.blockIdx]
		origLen := len(block.Content)
		if origLen == 0 {
			continue
		}
		placeholder := fmt.Sprintf("[tool result cleared - %d bytes]", origLen)
		tokensSaved += (origLen - len(placeholder)) / 4 // rough token estimate
		block.Content = placeholder
	}

	return MicroCompactResult{
		Messages:    result,
		TokensSaved: tokensSaved,
	}
}
