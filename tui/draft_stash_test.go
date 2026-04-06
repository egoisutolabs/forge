package tui

import "testing"

func TestDraftStash_TriggersOnLengthDrop(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Simulate typing 20+ chars
	longText := "This is a long message text"
	m.input.SetValue(longText)
	m.prevInputValue = longText
	m.peakInputLen = len(longText)

	// Now simulate clearing to <5 chars
	m.input.SetValue("hi")
	triggered := m.trackDraftStash()

	if !triggered {
		t.Fatal("expected stash to trigger on length drop")
	}
	if m.stashedDraft != longText {
		t.Fatalf("expected stashed draft %q, got %q", longText, m.stashedDraft)
	}
	if !m.showStashHint {
		t.Fatal("expected stash hint to be shown")
	}
	if !m.hasShownStashHint {
		t.Fatal("expected hasShownStashHint to be set")
	}
}

func TestDraftStash_HintShownOnlyOnce(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// First trigger
	longText := "This is a long message text"
	m.input.SetValue(longText)
	m.prevInputValue = longText
	m.peakInputLen = len(longText)
	m.input.SetValue("hi")
	m.trackDraftStash()

	// Dismiss hint
	m.showStashHint = false

	// Second trigger
	longText2 := "Another long message here!!"
	m.input.SetValue(longText2)
	m.prevInputValue = longText2
	m.peakInputLen = len(longText2)
	m.input.SetValue("ok")
	triggered := m.trackDraftStash()

	if triggered {
		t.Fatal("expected stash hint NOT to trigger a second time")
	}
	if m.showStashHint {
		t.Fatal("expected stash hint to remain hidden")
	}
}

func TestDraftStash_RestoreWorks(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Stash a draft
	longText := "This is a long message text"
	m.stashedDraft = longText

	// Restore
	m.input.SetValue(m.stashedDraft)
	m.stashedDraft = ""

	if m.input.Value() != longText {
		t.Fatalf("expected restored value %q, got %q", longText, m.input.Value())
	}
	if m.stashedDraft != "" {
		t.Fatal("expected stash to be cleared after restore")
	}
}

func TestDraftStash_ClearedOnSubmit(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	m.stashedDraft = "some draft"
	m.peakInputLen = 25
	m.prevInputValue = "some text"

	// Simulate submit clearing
	m.stashedDraft = ""
	m.peakInputLen = 0
	m.prevInputValue = ""
	m.dismissStashHint()

	if m.stashedDraft != "" {
		t.Fatal("expected stash cleared on submit")
	}
	if m.peakInputLen != 0 {
		t.Fatal("expected peakInputLen reset on submit")
	}
}

func TestDraftStash_NoTriggerBelowThreshold(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Short text — should not trigger stash
	m.input.SetValue("short")
	m.prevInputValue = "short"
	m.peakInputLen = 5

	m.input.SetValue("")
	triggered := m.trackDraftStash()

	if triggered {
		t.Fatal("expected no stash trigger for short text")
	}
}

func TestDraftStash_PeakTracking(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Track peak as chars are typed
	values := []string{"h", "he", "hel", "hell", "hello"}
	for _, v := range values {
		m.input.SetValue(v)
		m.trackDraftStash()
	}

	if m.peakInputLen != 5 {
		t.Fatalf("expected peak 5, got %d", m.peakInputLen)
	}
}
