package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmitToolStartEnd(t *testing.T) {
	dir := t.TempDir()
	sid := "test-emit-tool"

	if err := Init(sid, EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()

	SetTurn(1)

	traceID := EmitToolStart("Bash", "tool-use-123", json.RawMessage(`{"command":"ls"}`), true)
	if traceID == "" {
		t.Error("EmitToolStart returned empty traceID")
	}

	EmitToolEnd(traceID, "Bash", "tool-use-123", 200*time.Millisecond, "file1.go\nfile2.go", false)

	Shutdown()

	events, err := readEventsFromFile(filepath.Join(dir, "session-"+sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	if events[0].EventType != EventToolCallStart {
		t.Errorf("event[0] type = %q, want tool_call_start", events[0].EventType)
	}
	if events[0].TraceID != traceID {
		t.Errorf("event[0] traceID = %q, want %q", events[0].TraceID, traceID)
	}
	if events[0].Turn != 1 {
		t.Errorf("event[0] turn = %d, want 1", events[0].Turn)
	}

	var startPayload ToolCallStartPayload
	json.Unmarshal(events[0].Payload, &startPayload)
	if startPayload.ToolName != "Bash" {
		t.Errorf("start payload tool = %q, want Bash", startPayload.ToolName)
	}
	if !startPayload.IsConcSafe {
		t.Error("start payload IsConcSafe = false, want true")
	}

	if events[1].EventType != EventToolCallEnd {
		t.Errorf("event[1] type = %q, want tool_call_end", events[1].EventType)
	}
	var endPayload ToolCallEndPayload
	json.Unmarshal(events[1].Payload, &endPayload)
	if endPayload.DurationMs != 200 {
		t.Errorf("end payload duration = %d, want 200", endPayload.DurationMs)
	}
	if endPayload.IsError {
		t.Error("end payload IsError = true, want false")
	}
}

func TestEmitAgentSpawnComplete(t *testing.T) {
	dir := t.TempDir()
	sid := "test-emit-agent"

	if err := Init(sid, EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()

	EmitAgentSpawn("agent-abc", "search codebase", "Explore", "claude-sonnet-4-6", false, "find the main function")
	EmitAgentComplete("agent-abc", 5*time.Second, 4, false, "completed")

	Shutdown()

	events, err := readEventsFromFile(filepath.Join(dir, "session-"+sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	var spawn AgentSpawnPayload
	json.Unmarshal(events[0].Payload, &spawn)
	if spawn.AgentID != "agent-abc" {
		t.Errorf("spawn AgentID = %q, want agent-abc", spawn.AgentID)
	}
	if spawn.SubagentType != "Explore" {
		t.Errorf("spawn SubagentType = %q, want Explore", spawn.SubagentType)
	}

	var complete AgentCompletePayload
	json.Unmarshal(events[1].Payload, &complete)
	if complete.DurationMs != 5000 {
		t.Errorf("complete DurationMs = %d, want 5000", complete.DurationMs)
	}
	if complete.Turns != 4 {
		t.Errorf("complete Turns = %d, want 4", complete.Turns)
	}
}

func TestEmitSkillInvoke(t *testing.T) {
	dir := t.TempDir()
	sid := "test-emit-skill"

	if err := Init(sid, EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()

	EmitSkillInvoke("commit", "-m 'fix bug'", "bundled", []string{"Bash", "Read"}, 2847)

	Shutdown()

	events, err := readEventsFromFile(filepath.Join(dir, "session-"+sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	var payload SkillInvokePayload
	json.Unmarshal(events[0].Payload, &payload)
	if payload.SkillName != "commit" {
		t.Errorf("SkillName = %q, want commit", payload.SkillName)
	}
	if payload.PromptLen != 2847 {
		t.Errorf("PromptLen = %d, want 2847", payload.PromptLen)
	}
}

func TestEmitAPICall(t *testing.T) {
	dir := t.TempDir()
	sid := "test-emit-api"

	if err := Init(sid, EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()

	SetTurn(3)
	EmitAPICall(APICallPayload{
		Model:        "claude-sonnet-4-6",
		InputTokens:  4200,
		OutputTokens: 380,
		CacheRead:    3800,
		DurationMs:   1842,
		StopReason:   "tool_use",
		CostUSD:      0.0184,
		MaxTokens:    8192,
	})

	Shutdown()

	events, err := readEventsFromFile(filepath.Join(dir, "session-"+sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Turn != 3 {
		t.Errorf("turn = %d, want 3", events[0].Turn)
	}

	var payload APICallPayload
	json.Unmarshal(events[0].Payload, &payload)
	if payload.InputTokens != 4200 {
		t.Errorf("InputTokens = %d, want 4200", payload.InputTokens)
	}
}

func TestEmitNoopWhenUninitialized(t *testing.T) {
	// Ensure no global writer is active.
	Shutdown()

	// These should not panic or produce output.
	EmitToolStart("Bash", "id1", nil, true)
	EmitToolEnd("t1", "Bash", "id1", time.Second, "out", false)
	EmitAgentSpawn("a1", "desc", "Explore", "model", false, "prompt")
	EmitAgentComplete("a1", time.Second, 1, false, "done")
	EmitSkillInvoke("commit", "", "bundled", nil, 0)
	EmitAPICall(APICallPayload{})
	EmitError("api", "", "test error")
}

func TestRedactionEnabled(t *testing.T) {
	dir := t.TempDir()
	sid := "test-redact"

	if err := Init(sid, EmitterOpts{LogDir: dir, Redact: true}); err != nil {
		t.Fatal(err)
	}
	defer Shutdown()

	EmitToolStart("Bash", "tool-1", json.RawMessage(`{"command":"secret-cmd"}`), true)
	EmitToolEnd("t1", "Bash", "tool-1", 100*time.Millisecond, "secret output", false)
	EmitAgentSpawn("a1", "desc", "Explore", "model", false, "secret prompt")

	Shutdown()

	events, err := readEventsFromFile(filepath.Join(dir, "session-"+sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}

	// Check tool start input is redacted
	var startPayload ToolCallStartPayload
	json.Unmarshal(events[0].Payload, &startPayload)
	if string(startPayload.Input) != `"[REDACTED]"` {
		t.Errorf("start input = %s, want [REDACTED]", startPayload.Input)
	}

	// Check tool end output is redacted
	var endPayload ToolCallEndPayload
	json.Unmarshal(events[1].Payload, &endPayload)
	if endPayload.Output != "[REDACTED]" {
		t.Errorf("end output = %q, want [REDACTED]", endPayload.Output)
	}

	// Check agent prompt is redacted
	var spawnPayload AgentSpawnPayload
	json.Unmarshal(events[2].Payload, &spawnPayload)
	if spawnPayload.Prompt != "[REDACTED]" {
		t.Errorf("spawn prompt = %q, want [REDACTED]", spawnPayload.Prompt)
	}
}

func TestWriterNonBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	// Fill the buffer beyond capacity — should not block.
	for i := 0; i < channelBuffer+100; i++ {
		raw, _ := json.Marshal(map[string]string{"test": "event"})
		w.Write(Event{
			Timestamp: time.Now().UTC(),
			SessionID: "test",
			EventType: EventToolCallStart,
			Payload:   raw,
		})
	}

	// Close should drain remaining events.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify file was written.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("log file is empty after writing events")
	}
}
