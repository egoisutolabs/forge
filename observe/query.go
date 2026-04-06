package observe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionSummary is a compact listing entry for `forge log list`.
type SessionSummary struct {
	SessionID  string  `json:"session_id"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	EventCount int     `json:"event_count"`
	ToolCalls  int     `json:"tool_calls"`
	APICalls   int     `json:"api_calls"`
	TotalCost  float64 `json:"total_cost_usd"`
	FileSize   int64   `json:"file_size_bytes"`
}

// SessionStats is a detailed analysis for `forge log stats`.
type SessionStats struct {
	SessionSummary

	ToolBreakdown  []ToolStat `json:"tool_breakdown"`
	DurationSecs   float64    `json:"duration_secs"`
	TotalTokensIn  int        `json:"total_tokens_in"`
	TotalTokensOut int        `json:"total_tokens_out"`
	Turns          int        `json:"turns"`
	AgentSpawns    int        `json:"agent_spawns"`
	SkillInvokes   int        `json:"skill_invokes"`
	Errors         int        `json:"errors"`
}

// ToolStat summarizes one tool's usage within a session.
type ToolStat struct {
	Name       string `json:"name"`
	CallCount  int    `json:"call_count"`
	ErrorCount int    `json:"error_count"`
	TotalMs    int64  `json:"total_ms"`
	AvgMs      int64  `json:"avg_ms"`
	MaxMs      int64  `json:"max_ms"`
}

// ListSessions scans ~/.forge/logs/ and returns summaries sorted by time (newest first).
func ListSessions(limit int) ([]SessionSummary, error) {
	dir, err := logsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var summaries []SessionSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		summary, err := summarizeFile(path)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartTime > summaries[j].StartTime
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

// ReadEvents reads all events from a session log file.
func ReadEvents(sessionID string) ([]Event, error) {
	if err := sanitizeSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("ReadEvents: %w", err)
	}
	dir, err := logsDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "session-"+sessionID+".jsonl")
	return readEventsFromFile(path)
}

// ComputeStats analyzes a session's events and returns detailed statistics.
func ComputeStats(sessionID string) (*SessionStats, error) {
	// sanitizeSessionID is called inside ReadEvents, no need to duplicate.
	events, err := ReadEvents(sessionID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events found for session %s", sessionID)
	}

	stats := &SessionStats{}
	stats.SessionID = sessionID
	stats.StartTime = events[0].Timestamp.Format("2006-01-02T15:04:05Z")
	stats.EndTime = events[len(events)-1].Timestamp.Format("2006-01-02T15:04:05Z")
	stats.EventCount = len(events)
	stats.DurationSecs = events[len(events)-1].Timestamp.Sub(events[0].Timestamp).Seconds()

	toolStats := make(map[string]*ToolStat)
	maxTurn := 0

	for _, e := range events {
		if e.Turn > maxTurn {
			maxTurn = e.Turn
		}

		switch e.EventType {
		case EventToolCallEnd:
			stats.ToolCalls++
			var p ToolCallEndPayload
			json.Unmarshal(e.Payload, &p) //nolint:errcheck

			ts, ok := toolStats[p.ToolName]
			if !ok {
				ts = &ToolStat{Name: p.ToolName}
				toolStats[p.ToolName] = ts
			}
			ts.CallCount++
			ts.TotalMs += p.DurationMs
			if p.DurationMs > ts.MaxMs {
				ts.MaxMs = p.DurationMs
			}
			if p.IsError {
				ts.ErrorCount++
			}

		case EventAPICall:
			stats.APICalls++
			var p APICallPayload
			json.Unmarshal(e.Payload, &p) //nolint:errcheck
			stats.TotalCost += p.CostUSD
			stats.TotalTokensIn += p.InputTokens
			stats.TotalTokensOut += p.OutputTokens

		case EventAgentSpawn:
			stats.AgentSpawns++

		case EventSkillInvoke:
			stats.SkillInvokes++

		case EventError:
			stats.Errors++
		}
	}

	stats.Turns = maxTurn

	for _, ts := range toolStats {
		if ts.CallCount > 0 {
			ts.AvgMs = ts.TotalMs / int64(ts.CallCount)
		}
		stats.ToolBreakdown = append(stats.ToolBreakdown, *ts)
	}
	sort.Slice(stats.ToolBreakdown, func(i, j int) bool {
		return stats.ToolBreakdown[i].CallCount > stats.ToolBreakdown[j].CallCount
	})

	return stats, nil
}

func summarizeFile(path string) (SessionSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return SessionSummary{}, err
	}

	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially large lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var first, last Event
	count := 0
	toolCalls := 0
	apiCalls := 0
	var totalCost float64

	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if count == 0 {
			first = e
		}
		last = e
		count++

		switch e.EventType {
		case EventToolCallEnd:
			toolCalls++
		case EventAPICall:
			apiCalls++
			var p APICallPayload
			json.Unmarshal(e.Payload, &p) //nolint:errcheck
			totalCost += p.CostUSD
		}
	}

	if count == 0 {
		return SessionSummary{}, fmt.Errorf("empty log file: %s", path)
	}

	return SessionSummary{
		SessionID:  first.SessionID,
		StartTime:  first.Timestamp.Format("2006-01-02T15:04:05Z"),
		EndTime:    last.Timestamp.Format("2006-01-02T15:04:05Z"),
		EventCount: count,
		ToolCalls:  toolCalls,
		APICalls:   apiCalls,
		TotalCost:  totalCost,
		FileSize:   info.Size(),
	}, nil
}

func readEventsFromFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var events []Event
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, scanner.Err()
}
