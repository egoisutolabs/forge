package observe

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Writer tests ---

func TestWriterWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(ErrorPayload{Source: "test", Message: "hello"})
	w.Write(Event{
		Timestamp: time.Now().UTC(),
		SessionID: "sess1",
		EventType: EventError,
		Turn:      1,
		Payload:   payload,
	})
	w.Write(Event{
		Timestamp: time.Now().UTC(),
		SessionID: "sess1",
		EventType: EventError,
		Turn:      2,
		Payload:   payload,
	})

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		events = append(events, e)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Turn != 1 || events[1].Turn != 2 {
		t.Errorf("unexpected turns: %d, %d", events[0].Turn, events[1].Turn)
	}
}

func TestWriterNonBlockingWhenFull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(ErrorPayload{Source: "test", Message: "x"})

	// Fill the channel beyond capacity — should not block
	for i := 0; i < channelBuffer+100; i++ {
		w.Write(Event{
			Timestamp: time.Now().UTC(),
			SessionID: "sess1",
			EventType: EventError,
			Payload:   payload,
		})
	}

	// If we got here without blocking, the test passes
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWriterCloseDrainsBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(ErrorPayload{Source: "test", Message: "drain"})
	count := 50
	for i := 0; i < count; i++ {
		w.Write(Event{
			Timestamp: time.Now().UTC(),
			SessionID: "sess1",
			EventType: EventError,
			Turn:      i,
			Payload:   payload,
		})
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// All events should be flushed to disk after Close
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	actual := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		actual++
	}
	if actual != count {
		t.Errorf("expected %d lines after Close, got %d", count, actual)
	}
}

// --- Emitter tests ---

func TestEmitNoOpWhenUninitialized(t *testing.T) {
	// Reset global state
	globalMu.Lock()
	globalWriter = nil
	globalMu.Unlock()

	// Should not panic
	Emit(EventError, ErrorPayload{Source: "test", Message: "noop"})
	EmitToolStart("Bash", "id1", json.RawMessage(`{}`), false)
	EmitToolEnd("trace1", "Bash", "id1", time.Second, "out", false)
	EmitAgentSpawn("a1", "desc", "general", "opus", false, "prompt")
	EmitAgentComplete("a1", time.Second, 3, false, "done")
	EmitSkillInvoke("commit", "-m fix", "bundled", nil, 100)
	EmitAPICall(APICallPayload{Model: "opus"})
	EmitError("test", "", "msg")
}

func TestEmitCreatesValidJSON(t *testing.T) {
	dir := t.TempDir()
	resetGlobal(t)

	err := Init("test-session", EmitterOpts{LogDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	SetTurn(5)
	Emit(EventError, ErrorPayload{Source: "api", Message: "timeout"})
	Shutdown()

	events := readEvents(t, filepath.Join(dir, "session-test-session.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.SessionID != "test-session" {
		t.Errorf("session_id = %q, want test-session", e.SessionID)
	}
	if e.EventType != EventError {
		t.Errorf("event_type = %q, want error", e.EventType)
	}
	if e.Turn != 5 {
		t.Errorf("turn = %d, want 5", e.Turn)
	}

	var p ErrorPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Source != "api" || p.Message != "timeout" {
		t.Errorf("payload = %+v", p)
	}
}

func TestConvenienceMethodsProduceCorrectEventTypes(t *testing.T) {
	dir := t.TempDir()
	resetGlobal(t)

	err := Init("conv-session", EmitterOpts{LogDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	EmitToolStart("Read", "tu1", json.RawMessage(`{"path":"/tmp"}`), true)
	EmitToolEnd("t1", "Read", "tu1", 50*time.Millisecond, "contents", false)
	EmitAgentSpawn("ag1", "search", "Explore", "sonnet", false, "find files")
	EmitAgentComplete("ag1", 2*time.Second, 3, false, "completed")
	EmitSkillInvoke("commit", "-m fix", "bundled", []string{"Bash"}, 500)
	EmitAPICall(APICallPayload{Model: "opus", InputTokens: 100, OutputTokens: 50})
	EmitError("tool", "Bash", "permission denied")

	Shutdown()

	events := readEvents(t, filepath.Join(dir, "session-conv-session.jsonl"))

	expected := []EventType{
		EventToolCallStart,
		EventToolCallEnd,
		EventAgentSpawn,
		EventAgentComplete,
		EventSkillInvoke,
		EventAPICall,
		EventError,
	}

	if len(events) != len(expected) {
		t.Fatalf("expected %d events, got %d", len(expected), len(events))
	}

	for i, want := range expected {
		if events[i].EventType != want {
			t.Errorf("event[%d].type = %q, want %q", i, events[i].EventType, want)
		}
	}
}

// --- Redact tests ---

func TestRedactToolInputOutput(t *testing.T) {
	input, _ := json.Marshal(ToolCallStartPayload{
		ToolName:  "Bash",
		ToolUseID: "t1",
		Input:     json.RawMessage(`{"command":"secret"}`),
	})
	e := Event{EventType: EventToolCallStart, Payload: input}

	Redact(&e)

	var p ToolCallStartPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if string(p.Input) != `"[REDACTED]"` {
		t.Errorf("input not redacted: %s", p.Input)
	}

	// tool_call_end
	endPayload, _ := json.Marshal(ToolCallEndPayload{
		ToolName: "Bash",
		Output:   "secret output",
		ErrorMsg: "secret error",
	})
	e2 := Event{EventType: EventToolCallEnd, Payload: endPayload}

	Redact(&e2)

	var p2 ToolCallEndPayload
	if err := json.Unmarshal(e2.Payload, &p2); err != nil {
		t.Fatal(err)
	}
	if p2.Output != "[REDACTED]" {
		t.Errorf("output not redacted: %s", p2.Output)
	}
	if p2.ErrorMsg != "[REDACTED]" {
		t.Errorf("error_msg not redacted: %s", p2.ErrorMsg)
	}
}

func TestRedactNonToolEventUnchanged(t *testing.T) {
	payload, _ := json.Marshal(APICallPayload{Model: "opus", InputTokens: 100})
	e := Event{EventType: EventAPICall, Payload: payload}
	original := string(e.Payload)

	Redact(&e)

	if string(e.Payload) != original {
		t.Errorf("non-tool event was modified: %s != %s", e.Payload, original)
	}
}

func TestRedactAgentPrompt(t *testing.T) {
	payload, _ := json.Marshal(AgentSpawnPayload{
		AgentID: "a1",
		Prompt:  "secret instructions",
	})
	e := Event{EventType: EventAgentSpawn, Payload: payload}

	Redact(&e)

	var p AgentSpawnPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Prompt != "[REDACTED]" {
		t.Errorf("prompt not redacted: %s", p.Prompt)
	}
}

func TestEmitWithRedactionEnabled(t *testing.T) {
	dir := t.TempDir()
	resetGlobal(t)

	err := Init("redact-session", EmitterOpts{LogDir: dir, Redact: true})
	if err != nil {
		t.Fatal(err)
	}

	EmitToolStart("Bash", "tu1", json.RawMessage(`{"command":"rm -rf /"}`), false)
	Shutdown()

	events := readEvents(t, filepath.Join(dir, "session-redact-session.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	var p ToolCallStartPayload
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if string(p.Input) != `"[REDACTED]"` {
		t.Errorf("input not redacted in emitted event: %s", p.Input)
	}
}

// --- Rotation tests ---

func TestRotateLogsDeletesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create an "old" file
	oldPath := filepath.Join(dir, "session-old.jsonl")
	if err := os.WriteFile(oldPath, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Backdate modification time
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(oldPath, oldTime, oldTime)

	// Create a "new" file
	newPath := filepath.Join(dir, "session-new.jsonl")
	if err := os.WriteFile(newPath, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}

	RotateLogs(dir, 7*24*time.Hour, 500*1024*1024)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old file should have been deleted")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("new file should still exist")
	}
}

func TestRotateLogsSizeCapEnforced(t *testing.T) {
	dir := t.TempDir()

	// Create files that total > 100 bytes
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, filepath.Base(
			func() string {
				s := "session-" + string(rune('a'+i)) + ".jsonl"
				return s
			}(),
		))
		data := make([]byte, 50) // 50 bytes each = 250 total
		for j := range data {
			data[j] = 'x'
		}
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
		// Stagger mod times so ordering is deterministic
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		os.Chtimes(name, modTime, modTime)
	}

	// Cap at 100 bytes — should keep only 2 files (newest)
	RotateLogs(dir, 24*time.Hour, 100)

	remaining, _ := os.ReadDir(dir)
	if len(remaining) > 2 {
		t.Errorf("expected <= 2 files after size cap, got %d", len(remaining))
	}

	// Total size should be <= 100
	var total int64
	for _, e := range remaining {
		info, _ := e.Info()
		total += info.Size()
	}
	if total > 100 {
		t.Errorf("total size %d exceeds cap of 100", total)
	}
}

// --- Security: session ID path traversal ---

func TestSanitizeSessionID_Valid(t *testing.T) {
	for _, id := range []string{"abc123", "test-session", "sess_01", "a"} {
		if err := sanitizeSessionID(id); err != nil {
			t.Errorf("sanitizeSessionID(%q) = %v, want nil", id, err)
		}
	}
}

func TestSanitizeSessionID_Rejects(t *testing.T) {
	for _, id := range []string{
		"",
		"../etc/passwd",
		"foo/bar",
		"foo\\bar",
		"..evil",
		"session/../../../etc/passwd",
	} {
		if err := sanitizeSessionID(id); err == nil {
			t.Errorf("sanitizeSessionID(%q) = nil, want error", id)
		}
	}
}

func TestInitRejectsTraversalSessionID(t *testing.T) {
	dir := t.TempDir()
	resetGlobal(t)

	err := Init("../../../tmp/evil", EmitterOpts{LogDir: dir})
	if err == nil {
		t.Fatal("Init should reject session ID with path traversal")
	}
	// Verify no file was created outside dir
	entries, _ := os.ReadDir(dir)
	if len(entries) > 0 {
		t.Errorf("no files should be created for rejected session ID, got %d", len(entries))
	}
}

func TestReadEventsRejectsTraversalSessionID(t *testing.T) {
	_, err := ReadEvents("../../etc/passwd")
	if err == nil {
		t.Fatal("ReadEvents should reject session ID with path traversal")
	}
}

// --- Security: directory permissions ---

func TestWriterCreatesDirectoryWith0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secure")
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("log directory permissions = %o, want 0700", perm)
	}
}

// --- Helpers ---

func resetGlobal(t *testing.T) {
	t.Helper()
	globalMu.Lock()
	if globalWriter != nil {
		globalWriter.Close()
	}
	globalWriter = nil
	globalSessID = ""
	globalRedact = false
	globalTurn = 0
	globalMu.Unlock()
}

func readEvents(t *testing.T, path string) []Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v\nline: %s", err, scanner.Text())
		}
		events = append(events, e)
	}
	return events
}
