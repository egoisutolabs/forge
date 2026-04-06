package tools

import (
	"fmt"
	"strings"
)

// Deferrable is an optional interface a Tool can implement to signal that it
// should be withheld from the initial prompt and announced only by name.
// The model fetches the full schema on demand via ToolSearch.
type Deferrable interface {
	ShouldDefer() bool
}

// DeferredToolSet is a slice of tools that have been marked as deferred.
type DeferredToolSet []Tool

// SplitTools partitions all into loaded (immediately available to the model)
// and deferred (announced by name; schemas fetched via ToolSearch on demand).
//
// A tool is deferred when it implements Deferrable and ShouldDefer() returns true.
// Tools that do not implement Deferrable, or return false, are loaded.
func SplitTools(all []Tool) (loaded []Tool, deferred DeferredToolSet) {
	for _, t := range all {
		if d, ok := t.(Deferrable); ok && d.ShouldDefer() {
			deferred = append(deferred, t)
		} else {
			loaded = append(loaded, t)
		}
	}
	return
}

// GenerateSystemReminder produces the <system-reminder> block that lists all
// deferred tool names.  It is injected into the conversation so the model can
// request full schemas via ToolSearch.
//
// Returns an empty string when there are no deferred tools.
func (s DeferredToolSet) GenerateSystemReminder() string {
	if len(s) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<system-reminder>\n")
	b.WriteString(fmt.Sprintf("The following %d deferred tools are now available via ToolSearch:\n", len(s)))
	for _, t := range s {
		b.WriteString(t.Name())
		b.WriteByte('\n')
	}
	b.WriteString("</system-reminder>")
	return b.String()
}
