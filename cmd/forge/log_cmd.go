package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/egoisutolabs/forge/observe"
)

func handleLogCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: forge log <list|show|stats> [session-id]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		limit := 20
		sessions, err := observe.ListSessions(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SESSION\tSTARTED\tEVENTS\tTOOLS\tCOST")
		for _, s := range sessions {
			sid := s.SessionID
			if len(sid) > 8 {
				sid = sid[:8]
			}
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t$%.4f\n",
				sid,
				s.StartTime[:16],
				s.EventCount,
				s.ToolCalls,
				s.TotalCost,
			)
		}
		tw.Flush()

	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: forge log show <session-id> [--tools-only]")
			os.Exit(1)
		}
		sessionID := resolveSessionID(args[1])
		toolsOnly := len(args) > 2 && args[2] == "--tools-only"

		events, err := observe.ReadEvents(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		for _, e := range events {
			if toolsOnly && e.EventType != observe.EventToolCallStart && e.EventType != observe.EventToolCallEnd {
				continue
			}
			printFormattedEvent(e)
		}

	case "stats":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: forge log stats <session-id>")
			os.Exit(1)
		}
		sessionID := resolveSessionID(args[1])

		stats, err := observe.ComputeStats(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Session:    %s\n", truncateID(stats.SessionID))
		fmt.Printf("Duration:   %.1fs\n", stats.DurationSecs)
		fmt.Printf("Turns:      %d\n", stats.Turns)
		fmt.Printf("API calls:  %d\n", stats.APICalls)
		fmt.Printf("Tool calls: %d\n", stats.ToolCalls)
		fmt.Printf("Agents:     %d\n", stats.AgentSpawns)
		fmt.Printf("Skills:     %d\n", stats.SkillInvokes)
		fmt.Printf("Errors:     %d\n", stats.Errors)
		fmt.Printf("Tokens:     %d in / %d out\n", stats.TotalTokensIn, stats.TotalTokensOut)
		fmt.Printf("Cost:       $%.4f\n\n", stats.TotalCost)

		if len(stats.ToolBreakdown) > 0 {
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TOOL\tCALLS\tERRORS\tAVG(ms)\tMAX(ms)\tTOTAL(ms)")
			for _, ts := range stats.ToolBreakdown {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\n",
					ts.Name, ts.CallCount, ts.ErrorCount,
					ts.AvgMs, ts.MaxMs, ts.TotalMs,
				)
			}
			tw.Flush()
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

// resolveSessionID matches a prefix to a full session ID from available logs.
func resolveSessionID(prefix string) string {
	sessions, _ := observe.ListSessions(0)
	for _, s := range sessions {
		if strings.HasPrefix(s.SessionID, prefix) {
			return s.SessionID
		}
	}
	return prefix // pass through, will fail at ReadEvents
}

func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// printFormattedEvent prints a single event in human-readable format.
func printFormattedEvent(e observe.Event) {
	ts := e.Timestamp.Format("15:04:05.000")
	switch e.EventType {
	case observe.EventToolCallStart:
		var p observe.ToolCallStartPayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		id := truncateID(p.ToolUseID)
		fmt.Printf("[%s] >> %s (id=%s)\n", ts, p.ToolName, id)
	case observe.EventToolCallEnd:
		var p observe.ToolCallEndPayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		status := "OK"
		if p.IsError {
			status = "ERR"
		}
		fmt.Printf("[%s] << %s %s (%dms)\n", ts, p.ToolName, status, p.DurationMs)
	case observe.EventAgentSpawn:
		var p observe.AgentSpawnPayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		bg := ""
		if p.IsBackground {
			bg = " [bg]"
		}
		fmt.Printf("[%s] ++ Agent %s (%s)%s\n", ts, p.Description, p.SubagentType, bg)
	case observe.EventAgentComplete:
		var p observe.AgentCompletePayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		fmt.Printf("[%s] -- Agent %s done (%dms, %d turns)\n", ts, truncateID(p.AgentID), p.DurationMs, p.Turns)
	case observe.EventSkillInvoke:
		var p observe.SkillInvokePayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		fmt.Printf("[%s] ** Skill /%s (prompt: %d chars)\n", ts, p.SkillName, p.PromptLen)
	case observe.EventAPICall:
		var p observe.APICallPayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		fmt.Printf("[%s] ~~ API %s (in=%d out=%d $%.4f %dms)\n", ts, p.Model, p.InputTokens, p.OutputTokens, p.CostUSD, p.DurationMs)
	case observe.EventError:
		var p observe.ErrorPayload
		json.Unmarshal(e.Payload, &p) //nolint:errcheck
		fmt.Printf("[%s] !! Error [%s] %s\n", ts, p.Source, p.Message)
	}
}
