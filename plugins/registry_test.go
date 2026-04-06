package plugins

import (
	"testing"

	"github.com/egoisutolabs/forge/hooks"
)

func makePlugin(name string, enabled bool, skillPaths ...string) *Plugin {
	return &Plugin{
		Name:       name,
		Version:    "1.0.0",
		SkillPaths: skillPaths,
		Enabled:    enabled,
	}
}

func makeHookPlugin(name string, event hooks.HookEvent, matcher string, cmd string) *Plugin {
	return &Plugin{
		Name:    name,
		Version: "1.0.0",
		Enabled: true,
		HookConfigs: hooks.HooksSettings{
			event: []hooks.HookMatcher{
				{Matcher: matcher, Hooks: []hooks.HookConfig{{Command: cmd}}},
			},
		},
	}
}

// ---- Register / Get / All ----

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewPluginRegistry()
	p := makePlugin("foo", true)
	r.Register(p)

	got, ok := r.Get("foo")
	if !ok {
		t.Fatal("expected to find plugin 'foo'")
	}
	if got.Name != "foo" {
		t.Errorf("Name = %q, want foo", got.Name)
	}
}

func TestRegistry_GetMiss(t *testing.T) {
	r := NewPluginRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected miss for nonexistent plugin")
	}
}

func TestRegistry_ReplaceDuplicate(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(&Plugin{Name: "foo", Version: "1.0.0", Enabled: true})
	r.Register(&Plugin{Name: "foo", Version: "2.0.0", Enabled: false})

	got, _ := r.Get("foo")
	if got.Version != "2.0.0" {
		t.Errorf("expected Version 2.0.0 after replace, got %q", got.Version)
	}
	// Should not duplicate in order
	all := r.All()
	if len(all) != 1 {
		t.Errorf("expected 1 entry after duplicate register, got %d", len(all))
	}
}

func TestRegistry_All_InsertionOrder(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("c", true))
	r.Register(makePlugin("a", true))
	r.Register(makePlugin("b", true))

	all := r.All()
	names := make([]string, len(all))
	for i, p := range all {
		names[i] = p.Name
	}
	want := []string{"c", "a", "b"}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("All()[%d] = %q, want %q", i, names[i], n)
		}
	}
}

// ---- RegisterBuiltin ----

func TestRegistry_RegisterBuiltin(t *testing.T) {
	r := NewPluginRegistry()
	p := makePlugin("builtin-plugin", true)
	r.RegisterBuiltin(p)

	got, ok := r.Get("builtin-plugin")
	if !ok {
		t.Fatal("expected to find builtin plugin")
	}
	if !got.IsBuiltin {
		t.Error("RegisterBuiltin should set IsBuiltin=true")
	}
}

// ---- Enable / Disable ----

func TestRegistry_EnableDisable(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("foo", true))

	if err := r.Disable("foo"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	got, _ := r.Get("foo")
	if got.Enabled {
		t.Error("expected plugin to be disabled")
	}

	if err := r.Enable("foo"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	got, _ = r.Get("foo")
	if !got.Enabled {
		t.Error("expected plugin to be enabled")
	}
}

func TestRegistry_EnableNotFound(t *testing.T) {
	r := NewPluginRegistry()
	if err := r.Enable("ghost"); err == nil {
		t.Error("expected error enabling nonexistent plugin")
	}
}

func TestRegistry_DisableNotFound(t *testing.T) {
	r := NewPluginRegistry()
	if err := r.Disable("ghost"); err == nil {
		t.Error("expected error disabling nonexistent plugin")
	}
}

// ---- GetAllSkillPaths ----

func TestRegistry_GetAllSkillPaths_Empty(t *testing.T) {
	r := NewPluginRegistry()
	if paths := r.GetAllSkillPaths(); len(paths) != 0 {
		t.Errorf("expected empty paths, got %v", paths)
	}
}

func TestRegistry_GetAllSkillPaths_EnabledOnly(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("enabled", true, "/skills/a", "/skills/b"))
	r.Register(makePlugin("disabled", false, "/skills/c"))

	paths := r.GetAllSkillPaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/skills/a" || paths[1] != "/skills/b" {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestRegistry_GetAllSkillPaths_InsertionOrder(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("first", true, "/first/s"))
	r.Register(makePlugin("second", true, "/second/s"))

	paths := r.GetAllSkillPaths()
	if len(paths) != 2 || paths[0] != "/first/s" || paths[1] != "/second/s" {
		t.Errorf("unexpected order: %v", paths)
	}
}

func TestRegistry_GetAllSkillPaths_AfterDisable(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("foo", true, "/foo/skills"))
	r.Disable("foo")

	paths := r.GetAllSkillPaths()
	if len(paths) != 0 {
		t.Errorf("expected 0 paths after disable, got %v", paths)
	}
}

// ---- GetAllHooks ----

func TestRegistry_GetAllHooks_Empty(t *testing.T) {
	r := NewPluginRegistry()
	h := r.GetAllHooks()
	if len(h) != 0 {
		t.Errorf("expected empty hooks, got %v", h)
	}
}

func TestRegistry_GetAllHooks_SinglePlugin(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makeHookPlugin("foo", hooks.HookEventPreToolUse, "Bash", "echo pre"))

	h := r.GetAllHooks()
	matchers := h[hooks.HookEventPreToolUse]
	if len(matchers) != 1 {
		t.Fatalf("expected 1 matcher, got %d", len(matchers))
	}
	if matchers[0].Matcher != "Bash" {
		t.Errorf("matcher = %q, want Bash", matchers[0].Matcher)
	}
}

func TestRegistry_GetAllHooks_MergedAcrossPlugins(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makeHookPlugin("alpha", hooks.HookEventPreToolUse, "Bash", "echo alpha"))
	r.Register(makeHookPlugin("beta", hooks.HookEventPreToolUse, "Read", "echo beta"))

	h := r.GetAllHooks()
	matchers := h[hooks.HookEventPreToolUse]
	if len(matchers) != 2 {
		t.Fatalf("expected 2 merged matchers, got %d", len(matchers))
	}
	if matchers[0].Matcher != "Bash" || matchers[1].Matcher != "Read" {
		t.Errorf("unexpected matchers: %v", matchers)
	}
}

func TestRegistry_GetAllHooks_DisabledPluginExcluded(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makeHookPlugin("active", hooks.HookEventPostToolUse, ".*", "echo post"))
	r.Register(makeHookPlugin("inactive", hooks.HookEventPostToolUse, ".*", "echo skip"))
	r.Disable("inactive")

	h := r.GetAllHooks()
	matchers := h[hooks.HookEventPostToolUse]
	if len(matchers) != 1 {
		t.Fatalf("expected 1 matcher (disabled excluded), got %d", len(matchers))
	}
	if matchers[0].Hooks[0].Command != "echo post" {
		t.Errorf("unexpected command: %q", matchers[0].Hooks[0].Command)
	}
}

func TestRegistry_GetAllHooks_MultipleEvents(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(&Plugin{
		Name:    "multi",
		Version: "1.0.0",
		Enabled: true,
		HookConfigs: hooks.HooksSettings{
			hooks.HookEventPreToolUse:  []hooks.HookMatcher{{Matcher: "Bash", Hooks: []hooks.HookConfig{{Command: "pre"}}}},
			hooks.HookEventPostToolUse: []hooks.HookMatcher{{Matcher: ".*", Hooks: []hooks.HookConfig{{Command: "post"}}}},
		},
	})

	h := r.GetAllHooks()
	if len(h[hooks.HookEventPreToolUse]) != 1 {
		t.Error("expected 1 PreToolUse matcher")
	}
	if len(h[hooks.HookEventPostToolUse]) != 1 {
		t.Error("expected 1 PostToolUse matcher")
	}
}

// ---- Concurrency safety ----

// ---- GetAllHooks tags source as "plugin" ----

func TestRegistry_GetAllHooks_TagsSourceAsPlugin(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makeHookPlugin("myplugin", hooks.HookEventPreToolUse, "Bash", "echo hello"))

	h := r.GetAllHooks()
	matchers := h[hooks.HookEventPreToolUse]
	if len(matchers) != 1 {
		t.Fatalf("expected 1 matcher, got %d", len(matchers))
	}
	for _, hook := range matchers[0].Hooks {
		if hook.Source != "plugin" {
			t.Errorf("hook Source = %q, want 'plugin'", hook.Source)
		}
	}
}

func TestRegistry_GetAllHooks_DoesNotMutateOriginalPlugin(t *testing.T) {
	r := NewPluginRegistry()
	p := makeHookPlugin("myplugin", hooks.HookEventPreToolUse, "Bash", "echo hello")
	r.Register(p)

	// Call GetAllHooks which tags source as "plugin"
	r.GetAllHooks()

	// Original plugin's hook configs should not be mutated
	for _, matchers := range p.HookConfigs {
		for _, m := range matchers {
			for _, hook := range m.Hooks {
				if hook.Source == "plugin" {
					t.Error("GetAllHooks should not mutate original plugin HookConfigs")
				}
			}
		}
	}
}

// ---- Concurrency safety ----

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(makePlugin("base", true, "/base/skills"))

	done := make(chan struct{})
	go func() {
		for range 100 {
			r.Register(makePlugin("concurrent", true, "/c/s"))
			r.Disable("concurrent")
			r.Enable("concurrent")
			r.GetAllSkillPaths()
			r.GetAllHooks()
		}
		close(done)
	}()

	for range 100 {
		r.All()
		r.Get("base")
	}
	<-done
}
