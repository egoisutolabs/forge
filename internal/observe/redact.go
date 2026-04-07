package observe

import "encoding/json"

// Redact modifies event payload in-place, replacing tool inputs/outputs
// and agent prompts with "[REDACTED]" placeholders. Only applies to
// event types that carry sensitive data.
func Redact(e *Event) {
	switch e.EventType {
	case EventToolCallStart:
		redactToolStart(e)
	case EventToolCallEnd:
		redactToolEnd(e)
	case EventAgentSpawn:
		redactAgentSpawn(e)
	}
}

func redactToolStart(e *Event) {
	var p ToolCallStartPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return
	}
	p.Input = json.RawMessage(`"[REDACTED]"`)
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	e.Payload = raw
}

func redactToolEnd(e *Event) {
	var p ToolCallEndPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return
	}
	p.Output = "[REDACTED]"
	if p.ErrorMsg != "" {
		p.ErrorMsg = "[REDACTED]"
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	e.Payload = raw
}

func redactAgentSpawn(e *Event) {
	var p AgentSpawnPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return
	}
	p.Prompt = "[REDACTED]"
	raw, err := json.Marshal(p)
	if err != nil {
		return
	}
	e.Payload = raw
}
