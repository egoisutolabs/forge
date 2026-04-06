package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestEvents(t *testing.T, dir string, sessionID string, events []Event) string {
	t.Helper()
	path := filepath.Join(dir, "session-"+sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func makeEvent(sessionID string, eventType EventType, turn int, ts time.Time, payload any) Event {
	raw, _ := json.Marshal(payload)
	return Event{
		Timestamp: ts,
		SessionID: sessionID,
		EventType: eventType,
		Turn:      turn,
		Payload:   raw,
	}
}

func TestSummarizeFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	sid := "test-session-1234"

	events := []Event{
		makeEvent(sid, EventAPICall, 1, now, APICallPayload{
			Model: "claude-sonnet-4-6", InputTokens: 100, OutputTokens: 50, CostUSD: 0.01,
		}),
		makeEvent(sid, EventToolCallEnd, 1, now.Add(100*time.Millisecond), ToolCallEndPayload{
			ToolName: "Grep", DurationMs: 50,
		}),
	}
	path := writeTestEvents(t, dir, sid, events)

	summary, err := summarizeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SessionID != sid {
		t.Errorf("got session ID %q, want %q", summary.SessionID, sid)
	}
	if summary.EventCount != 2 {
		t.Errorf("got %d events, want 2", summary.EventCount)
	}
	if summary.ToolCalls != 1 {
		t.Errorf("got %d tool calls, want 1", summary.ToolCalls)
	}
	if summary.APICalls != 1 {
		t.Errorf("got %d API calls, want 1", summary.APICalls)
	}
	if summary.TotalCost != 0.01 {
		t.Errorf("got cost %f, want 0.01", summary.TotalCost)
	}
}

func TestReadEventsFromFile(t *testing.T) {
	dir := t.TempDir()
	sid := "test-read-events"
	now := time.Now().UTC()

	events := []Event{
		makeEvent(sid, EventToolCallStart, 1, now, ToolCallStartPayload{ToolName: "Bash", ToolUseID: "abc123"}),
		makeEvent(sid, EventToolCallEnd, 1, now.Add(200*time.Millisecond), ToolCallEndPayload{ToolName: "Bash", ToolUseID: "abc123", DurationMs: 200}),
		makeEvent(sid, EventAgentSpawn, 2, now.Add(500*time.Millisecond), AgentSpawnPayload{AgentID: "a1", Description: "test"}),
	}
	path := writeTestEvents(t, dir, sid, events)

	got, err := readEventsFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].EventType != EventToolCallStart {
		t.Errorf("event[0] type = %q, want tool_call_start", got[0].EventType)
	}
	if got[2].EventType != EventAgentSpawn {
		t.Errorf("event[2] type = %q, want agent_spawn", got[2].EventType)
	}
}

func TestComputeStatsFromEvents(t *testing.T) {
	dir := t.TempDir()
	sid := "test-stats"
	now := time.Now().UTC()

	events := []Event{
		makeEvent(sid, EventAPICall, 1, now, APICallPayload{
			Model: "claude-sonnet-4-6", InputTokens: 1000, OutputTokens: 200, CostUSD: 0.05, DurationMs: 500,
		}),
		makeEvent(sid, EventToolCallEnd, 1, now.Add(100*time.Millisecond), ToolCallEndPayload{
			ToolName: "Grep", DurationMs: 50,
		}),
		makeEvent(sid, EventToolCallEnd, 1, now.Add(200*time.Millisecond), ToolCallEndPayload{
			ToolName: "Grep", DurationMs: 80, IsError: true,
		}),
		makeEvent(sid, EventToolCallEnd, 2, now.Add(300*time.Millisecond), ToolCallEndPayload{
			ToolName: "Bash", DurationMs: 300,
		}),
		makeEvent(sid, EventAgentSpawn, 2, now.Add(400*time.Millisecond), AgentSpawnPayload{AgentID: "a1"}),
		makeEvent(sid, EventAgentComplete, 2, now.Add(2*time.Second), AgentCompletePayload{AgentID: "a1", DurationMs: 1600}),
		makeEvent(sid, EventSkillInvoke, 3, now.Add(3*time.Second), SkillInvokePayload{SkillName: "commit"}),
		makeEvent(sid, EventError, 3, now.Add(3500*time.Millisecond), ErrorPayload{Source: "tool", Message: "permission denied"}),
	}

	// Write and read back events to test the full pipeline.
	path := writeTestEvents(t, dir, sid, events)
	readBack, err := readEventsFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(readBack) != len(events) {
		t.Fatalf("readback: got %d events, want %d", len(readBack), len(events))
	}

	// Compute stats from the read events using summarizeFile.
	summary, err := summarizeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ToolCalls != 3 {
		t.Errorf("ToolCalls = %d, want 3", summary.ToolCalls)
	}
	if summary.APICalls != 1 {
		t.Errorf("APICalls = %d, want 1", summary.APICalls)
	}

	// Verify event-level aggregation by iterating manually (same as ComputeStats).
	var toolCalls, apiCalls, agentSpawns, skillInvokes, errors int
	var totalTokensIn, totalTokensOut int
	maxTurn := 0
	toolMap := make(map[string]*ToolStat)

	for _, e := range readBack {
		if e.Turn > maxTurn {
			maxTurn = e.Turn
		}
		switch e.EventType {
		case EventToolCallEnd:
			toolCalls++
			var p ToolCallEndPayload
			json.Unmarshal(e.Payload, &p)
			ts, ok := toolMap[p.ToolName]
			if !ok {
				ts = &ToolStat{Name: p.ToolName}
				toolMap[p.ToolName] = ts
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
			apiCalls++
			var p APICallPayload
			json.Unmarshal(e.Payload, &p)
			totalTokensIn += p.InputTokens
			totalTokensOut += p.OutputTokens
		case EventAgentSpawn:
			agentSpawns++
		case EventSkillInvoke:
			skillInvokes++
		case EventError:
			errors++
		}
	}

	if toolCalls != 3 {
		t.Errorf("toolCalls = %d, want 3", toolCalls)
	}
	if apiCalls != 1 {
		t.Errorf("apiCalls = %d, want 1", apiCalls)
	}
	if agentSpawns != 1 {
		t.Errorf("agentSpawns = %d, want 1", agentSpawns)
	}
	if skillInvokes != 1 {
		t.Errorf("skillInvokes = %d, want 1", skillInvokes)
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1", errors)
	}
	if totalTokensIn != 1000 {
		t.Errorf("totalTokensIn = %d, want 1000", totalTokensIn)
	}
	if totalTokensOut != 200 {
		t.Errorf("totalTokensOut = %d, want 200", totalTokensOut)
	}
	if maxTurn != 3 {
		t.Errorf("maxTurn = %d, want 3", maxTurn)
	}

	// Check Grep tool stats
	grepStats := toolMap["Grep"]
	if grepStats == nil {
		t.Fatal("no stats for Grep")
	}
	if grepStats.CallCount != 2 {
		t.Errorf("Grep calls = %d, want 2", grepStats.CallCount)
	}
	if grepStats.ErrorCount != 1 {
		t.Errorf("Grep errors = %d, want 1", grepStats.ErrorCount)
	}
	if grepStats.MaxMs != 80 {
		t.Errorf("Grep maxMs = %d, want 80", grepStats.MaxMs)
	}
}

func TestEmptyLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-empty.jsonl")
	os.WriteFile(path, []byte{}, 0600)

	_, err := summarizeFile(path)
	if err == nil {
		t.Error("expected error for empty file")
	}
}
