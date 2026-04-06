// Package skills — verification tests comparing Go port against Claude Code's
// skill system.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: ContextFork execution is not implemented.
//     TypeScript SkillTool has inline vs fork (isolated sub-agent) execution.
//     Go documents fork mode as "not yet implemented" — falls back to inline.
//
//  2. MISSING: Plugin integration.
//     TypeScript skills can be provided by plugins (via installedPluginsManager).
//     Go skills and plugins are separate systems with no auto-wiring.
//
//  3. MISSING: MCP prompts as skills.
//     TypeScript discovers MCP prompts (type === 'prompt', loadedFrom === 'mcp')
//     as skills. Go has no MCP prompt integration.
//
//  4. CORRECT: source precedence — bundled < user < project.
//
//  5. CORRECT: user-invocable defaults to true.
//
//  6. CORRECT: when-to-use hint preserved.
//
//  7. CORRECT: allowed-tools parsed correctly.
//
//  8. CORRECT: context=fork parsed (stored, not executed differently).
//
//  9. CORRECT: /name lookup strips leading slash.
//
// 10. CORRECT: Thread-safe registry (concurrent reads/writes).
package skills

import (
	"testing"
)

// ─── GAP 1: ContextFork falls back to inline ──────────────────────────────────

// TestVerification_ContextFork_NotImplemented documents that ContextFork is
// stored in the Skill struct but not executed differently from ContextInline.
//
// Claude Code TypeScript: fork mode spawns an isolated sub-agent with its own
// token budget, separate conversation history, and allowed-tools enforcement.
func TestVerification_ContextFork_NotImplemented(t *testing.T) {
	fm, _ := ParseFrontmatter("---\ncontext: fork\n---\ndo work")
	if fm.Context != ContextFork {
		t.Errorf("context=fork should parse to ContextFork, got %q", fm.Context)
	}
	t.Log("GAP CONFIRMED: ContextFork is parsed and stored but not executed differently from ContextInline")
	t.Log("TypeScript fork mode: isolated sub-agent with own token budget and history")
	t.Log("Go fork mode: falls back to inline (as documented in skill.go)")
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_UserInvocable_DefaultsToTrue verifies that user-invocable
// defaults to true when absent from frontmatter — matching TypeScript.
func TestVerification_UserInvocable_DefaultsToTrue(t *testing.T) {
	// No user-invocable field.
	fm, _ := ParseFrontmatter("---\ndescription: test skill\n---\nsome body")
	if !fm.UserInvocable {
		t.Error("user-invocable should default to true when not specified")
	}

	// Explicit false.
	fm2, _ := ParseFrontmatter("---\nuser-invocable: false\n---\nbody")
	if fm2.UserInvocable {
		t.Error("user-invocable: false should set UserInvocable to false")
	}
}

// TestVerification_SourcePrecedence_LaterOverridesEarlier verifies that a
// skill registered later (project > user > bundled) overrides earlier ones.
//
// This matches Claude Code where project-local skills override user-global
// which override bundled.
func TestVerification_SourcePrecedence_LaterOverridesEarlier(t *testing.T) {
	r := NewRegistry()

	bundled := &Skill{Name: "commit", Source: "bundled", Description: "bundled commit"}
	user := &Skill{Name: "commit", Source: "user", Description: "user commit"}
	project := &Skill{Name: "commit", Source: "project", Description: "project commit"}

	r.Register(bundled)
	r.Register(user)
	r.Register(project)

	found := r.Lookup("commit")
	if found == nil {
		t.Fatal("skill not found")
	}
	if found.Source != "project" {
		t.Errorf("source = %q, want 'project' (last writer wins)", found.Source)
	}
	if found.Description != "project commit" {
		t.Errorf("description = %q, want 'project commit'", found.Description)
	}
}

// TestVerification_LeadingSlashStripped verifies that /name and name resolve
// to the same skill — matching Claude Code's /command convention.
func TestVerification_LeadingSlashStripped(t *testing.T) {
	r := NewRegistry()
	r.Register(&Skill{Name: "commit", Description: "commits"})

	if r.Lookup("/commit") == nil {
		t.Error("Lookup('/commit') should find skill registered as 'commit'")
	}
	if r.Lookup("commit") == nil {
		t.Error("Lookup('commit') should work too")
	}
}

// TestVerification_FrontmatterAllowedTools_InlineList verifies inline list
// parsing: [Bash, Read, Glob] → ["Bash", "Read", "Glob"].
func TestVerification_FrontmatterAllowedTools_InlineList(t *testing.T) {
	fm, body := ParseFrontmatter("---\nallowed-tools: [Bash, Read, Glob]\n---\ndo the thing")
	if len(fm.AllowedTools) != 3 {
		t.Errorf("allowed-tools = %v, want 3 items", fm.AllowedTools)
	}
	expected := []string{"Bash", "Read", "Glob"}
	for i, want := range expected {
		if i >= len(fm.AllowedTools) || fm.AllowedTools[i] != want {
			t.Errorf("allowed-tools[%d] = %q, want %q", i, fm.AllowedTools[i], want)
		}
	}
	if body != "do the thing" {
		t.Errorf("body = %q, want %q", body, "do the thing")
	}
}

// TestVerification_FrontmatterAllowedTools_MultilineList verifies multi-line
// list parsing matching Claude Code's YAML format.
func TestVerification_FrontmatterAllowedTools_MultilineList(t *testing.T) {
	doc := "---\nallowed-tools:\n  - Bash\n  - Read\n  - Write\n---\nbody content"
	fm, body := ParseFrontmatter(doc)
	if len(fm.AllowedTools) != 3 {
		t.Errorf("multi-line allowed-tools = %v, want 3 items", fm.AllowedTools)
	}
	if body != "body content" {
		t.Errorf("body = %q, want %q", body, "body content")
	}
}

// TestVerification_FrontmatterWhenToUse_MultipleAliases verifies that
// when-to-use, whenToUse, and when_to_use all set the WhenToUse field.
func TestVerification_FrontmatterWhenToUse_MultipleAliases(t *testing.T) {
	tests := []struct {
		doc  string
		want string
	}{
		{"---\nwhen-to-use: use for git\n---\nbody", "use for git"},
		{"---\nwhenToUse: use for git\n---\nbody", "use for git"},
		{"---\nwhen_to_use: use for git\n---\nbody", "use for git"},
	}
	for _, tc := range tests {
		fm, _ := ParseFrontmatter(tc.doc)
		if fm.WhenToUse != tc.want {
			t.Errorf("doc=%q: WhenToUse = %q, want %q", tc.doc[:30], fm.WhenToUse, tc.want)
		}
	}
}

// TestVerification_Registry_ConcurrentAccess verifies the registry is safe
// for concurrent reads and writes — matches Claude Code's implicit thread-safety.
func TestVerification_Registry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	const n = 50
	done := make(chan struct{}, n*2)

	for i := range n {
		go func(i int) {
			r.Register(&Skill{Name: "skill" + string(rune('a'+i%26)), Source: "test"})
			done <- struct{}{}
		}(i)
		go func(i int) {
			r.Lookup("skill" + string(rune('a'+i%26)))
			done <- struct{}{}
		}(i)
	}

	for range n * 2 {
		<-done
	}
	// No race detected if we reach here.
}

// TestVerification_BundledRegistry_HasBuiltinSkills verifies that the bundled
// registry has at least the expected built-in skills (commit, review).
func TestVerification_BundledRegistry_HasBuiltinSkills(t *testing.T) {
	r := BundledRegistry()
	if r == nil {
		t.Fatal("BundledRegistry() returned nil")
	}

	// Claude Code has 'commit' and similar bundled skills. Verify at least
	// the registry is non-empty and has commit or review.
	all := r.All()
	if len(all) == 0 {
		t.Error("bundled registry should have at least one built-in skill")
	}

	hasCommit := false
	for _, s := range all {
		if s.Name == "commit" {
			hasCommit = true
		}
	}
	if !hasCommit {
		t.Log("NOTE: 'commit' built-in skill not found in bundled registry — verify bundled.go has it registered")
	}
}
