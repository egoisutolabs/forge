package bash

import (
	"fmt"
	"strings"
)

// Output size constants matching Claude Code's outputLimits.ts.
const (
	DefaultMaxOutput = 30_000  // characters
	MaxOutputUpper   = 150_000 // absolute ceiling
)

// TruncateOutput truncates output to maxLen characters, keeping the start.
// If truncated, appends a message showing how many lines were cut.
//
// This matches Claude Code's formatOutput() behavior:
//   - Keep first maxLen characters
//   - Count remaining newlines from truncation point
//   - Append "... [N lines truncated] ..."
func TruncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}

	// Keep the start
	kept := output[:maxLen]

	// Count lines in the truncated portion
	truncatedPart := output[maxLen:]
	truncatedLines := strings.Count(truncatedPart, "\n") + 1

	return fmt.Sprintf("%s\n\n... [%d lines truncated] ...", kept, truncatedLines)
}
