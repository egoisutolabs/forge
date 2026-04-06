package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// internalHooks maps "forge-internal:<name>" suffixes to compiled Go functions.
// Populated by RegisterInternalHook, called from orchestrator/safeguards.go.
var internalHooks sync.Map // map[string]func(HookInput) (*HookResult, error)

// RegisterInternalHook registers a Go function as a hook handler for commands
// with the "forge-internal:<name>" prefix. This avoids subprocess spawning for
// forge's built-in safeguard hooks.
func RegisterInternalHook(name string, fn func(HookInput) (*HookResult, error)) {
	internalHooks.Store(name, fn)
}

const defaultTimeoutSecs = 10

// ExecuteHooks runs all hooks registered for the given event whose matcher
// matches the tool name in input. It stops early if any hook returns
// Continue=false or Decision="deny".
//
// Hooks within the same matcher are run in parallel (matching TypeScript's
// Promise.all behaviour). Results are processed in declaration order so that
// the first deny in the list takes precedence.
//
// The last UpdatedInput from any allow hook is carried through in the final
// result so callers can replace the tool input.
//
// trustedSources controls which hook sources are allowed to execute.
// A hook whose Source is not in the trusted list is silently skipped with
// a log warning. If trustedSources is nil, all sources are trusted
// (backward-compatible default). Empty Source is treated as "user".
func ExecuteHooks(ctx context.Context, settings HooksSettings, event HookEvent, input HookInput, trustedSources []string) (*HookResult, error) {
	matchers, ok := settings[event]
	if !ok {
		return &HookResult{Continue: true}, nil
	}

	final := &HookResult{Continue: true}

	for _, matcher := range matchers {
		if !toolMatches(matcher.Matcher, input.ToolName) {
			continue
		}

		// Run all hooks in this matcher concurrently (mirrors TypeScript Promise.all).
		// Hooks whose If condition does not match or whose source is not trusted are skipped.
		type indexedResult struct {
			result  *HookResult
			err     error
			skipped bool // true when cfg.If condition did not match or source untrusted
		}
		results := make([]indexedResult, len(matcher.Hooks))
		var wg sync.WaitGroup
		for i, cfg := range matcher.Hooks {
			// Check source trust boundary before launching.
			if !isSourceTrusted(cfg.Source, trustedSources) {
				log.Printf("hooks: skipping %s-sourced hook %q (source not in trusted list)", cfg.Source, cfg.Command)
				results[i] = indexedResult{skipped: true, result: &HookResult{Continue: true}}
				continue
			}
			// Evaluate the per-hook If condition before launching.
			if cfg.If != "" && !toolMatches(cfg.If, input.ToolName) {
				results[i] = indexedResult{skipped: true, result: &HookResult{Continue: true}}
				continue
			}
			wg.Add(1)
			go func(i int, cfg HookConfig) {
				defer wg.Done()
				r, e := RunHook(ctx, cfg, input)
				results[i] = indexedResult{result: r, err: e}
			}(i, cfg)
		}
		wg.Wait()

		// Process results in original order; stop at the first deny.
		// Async hooks are treated as non-blocking (Continue=true, decision ignored).
		for _, r := range results {
			if r.skipped {
				continue
			}
			if r.err != nil {
				return nil, r.err
			}
			// Async hooks are advisory — their decision does not affect the chain.
			if r.result.Async {
				continue
			}
			if len(r.result.UpdatedInput) > 0 {
				final.UpdatedInput = r.result.UpdatedInput
			}
			if r.result.SystemMessage != "" {
				final.SystemMessage = r.result.SystemMessage
			}
			if !r.result.Continue || r.result.Decision == "deny" {
				r.result.UpdatedInput = final.UpdatedInput
				return r.result, nil
			}
		}
	}
	return final, nil
}

// RunHook dispatches the hook. Commands with the "forge-internal:" prefix are
// routed to Go functions registered via RegisterInternalHook; all others spawn
// a subprocess as before.
//
// A non-zero exit code is treated as a non-blocking failure (Continue=true).
// A timeout causes an error to be returned.
func RunHook(ctx context.Context, config HookConfig, input HookInput) (*HookResult, error) {
	// Dispatch forge-internal: hooks to compiled Go functions.
	if strings.HasPrefix(config.Command, "forge-internal:") {
		name := strings.TrimPrefix(config.Command, "forge-internal:")
		if fn, ok := internalHooks.Load(name); ok {
			return fn.(func(HookInput) (*HookResult, error))(input)
		}
		// Unknown internal hook — treat as no-op.
		return &HookResult{Continue: true}, nil
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSecs
	}

	hookCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("hook: marshal input: %w", err)
	}

	cmd := exec.CommandContext(hookCtx, "sh", "-c", config.Command)
	cmd.Stdin = bytes.NewReader(inputJSON)

	out, err := cmd.Output()
	if err != nil {
		if hookCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("hook timed out after %ds: %s", timeout, config.Command)
		}
		// Non-zero exit: treat as non-blocking (hook chose not to act).
		return &HookResult{Continue: true}, nil
	}

	if len(bytes.TrimSpace(out)) == 0 {
		return &HookResult{Continue: true}, nil
	}

	var result HookResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("hook: parse output: %w", err)
	}
	return &result, nil
}

// regexpCache caches compiled hook matcher patterns to avoid recompilation on
// every tool call. Keys are the raw pattern strings; values are *regexp.Regexp.
var regexpCache sync.Map

// toolMatches returns true if pattern matches toolName.
// An empty pattern matches everything.
// Compiled patterns are cached package-wide so each unique pattern is compiled
// at most once regardless of how many tool calls fire.
func toolMatches(pattern, toolName string) bool {
	if pattern == "" {
		return true
	}
	var re *regexp.Regexp
	if cached, ok := regexpCache.Load(pattern); ok {
		re = cached.(*regexp.Regexp)
	} else {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		// LoadOrStore avoids a race between two goroutines compiling the same pattern.
		actual, _ := regexpCache.LoadOrStore(pattern, compiled)
		re = actual.(*regexp.Regexp)
	}
	return re.MatchString(toolName)
}

// isSourceTrusted reports whether a hook's source is permitted to execute.
// If trustedSources is nil, all sources are trusted (backward-compatible).
// Empty source is treated as "user".
func isSourceTrusted(source string, trustedSources []string) bool {
	if trustedSources == nil {
		return true
	}
	if source == "" {
		source = "user"
	}
	for _, ts := range trustedSources {
		if ts == source {
			return true
		}
	}
	return false
}

// TagSource sets the Source field on all HookConfigs in the given HooksSettings.
// This is used to mark hooks by their origin (e.g. "user", "plugin", "builtin").
func TagSource(settings HooksSettings, source string) {
	for _, matchers := range settings {
		for i := range matchers {
			for j := range matchers[i].Hooks {
				matchers[i].Hooks[j].Source = source
			}
		}
	}
}
