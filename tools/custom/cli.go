package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BuiltinInfo carries the name and description of a built-in tool.
// This avoids importing the full tool packages just for listing.
type BuiltinInfo struct {
	Name        string
	Description string
}

// DispatchCLI handles "forge tool <subcommand>" arguments.
// args should be os.Args[1:] starting with "tool".
// Returns an error on failure; callers should print the error and exit non-zero.
func DispatchCLI(w io.Writer, args []string, builtins []BuiltinInfo, cwd string, customDirs ...string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: forge tool <list|show|validate|test> [args]")
	}

	sub := args[1]
	switch sub {
	case "list":
		return runList(w, builtins, cwd, customDirs...)
	case "show":
		if len(args) < 3 {
			return fmt.Errorf("usage: forge tool show <name>")
		}
		return runShow(w, args[2], builtins, cwd, customDirs...)
	case "validate":
		if len(args) < 3 {
			return fmt.Errorf("usage: forge tool validate <path>")
		}
		return runValidate(w, args[2])
	case "test":
		if len(args) < 3 {
			return fmt.Errorf("usage: forge tool test <name> --input '{...}'")
		}
		input := extractInput(args[3:])
		return runTest(w, args[2], input, cwd, customDirs...)
	default:
		return fmt.Errorf("unknown subcommand %q: use list, show, validate, or test", sub)
	}
}

// extractInput parses --input '...' from CLI args.
func extractInput(args []string) string {
	for i, a := range args {
		if a == "--input" && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--input="); ok {
			return v
		}
	}
	return "{}"
}

// runList displays all built-in and custom tools.
func runList(w io.Writer, builtins []BuiltinInfo, cwd string, customDirs ...string) error {
	fmt.Fprintln(w, "Built-in tools:")
	// Find max name width for alignment.
	maxLen := 0
	for _, b := range builtins {
		if len(b.Name) > maxLen {
			maxLen = len(b.Name)
		}
	}
	for _, b := range builtins {
		fmt.Fprintf(w, "  %-*s  %s\n", maxLen, b.Name, truncate(b.Description, 60))
	}

	// Load custom tools.
	customs, _ := discoverForList(cwd, builtins, customDirs...)
	if len(customs) == 0 {
		return nil
	}

	// Sort custom tools by name.
	sort.Slice(customs, func(i, j int) bool {
		return customs[i].def.Name < customs[j].def.Name
	})

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Custom tools:")
	maxLen = 0
	for _, c := range customs {
		if len(c.def.Name) > maxLen {
			maxLen = len(c.def.Name)
		}
	}
	for _, c := range customs {
		fmt.Fprintf(w, "  %-*s  %s\n", maxLen, c.def.Name, truncate(c.def.Description, 60))
	}
	return nil
}

// runShow displays detailed information about a single tool.
func runShow(w io.Writer, name string, builtins []BuiltinInfo, cwd string, customDirs ...string) error {
	// Check built-in tools first.
	for _, b := range builtins {
		if b.Name == name {
			fmt.Fprintf(w, "Name:        %s\n", b.Name)
			fmt.Fprintf(w, "Source:      built-in\n")
			fmt.Fprintf(w, "Description: %s\n", b.Description)
			return nil
		}
	}

	// Check custom tools.
	customs, _ := discoverForList(cwd, builtins, customDirs...)
	for _, c := range customs {
		if c.def.Name == name {
			return showCustomTool(w, c.def)
		}
	}

	return fmt.Errorf("tool %q not found", name)
}

// showCustomTool formats detailed output for a custom tool.
func showCustomTool(w io.Writer, def *Definition) error {
	fmt.Fprintf(w, "Name:        %s\n", def.Name)
	fmt.Fprintf(w, "Description: %s\n", def.Description)
	fmt.Fprintf(w, "Command:     %s\n", def.Command)
	fmt.Fprintf(w, "Timeout:     %ds\n", def.Timeout)
	fmt.Fprintf(w, "Read-only:   %s\n", yesNo(def.ReadOnly))
	fmt.Fprintf(w, "Concurrent:  %s\n", yesNo(def.ConcurrencySafe))
	if def.SearchHintText != "" {
		fmt.Fprintf(w, "Search hint: %s\n", def.SearchHintText)
	}

	// Show input schema properties.
	props, _ := def.InputSchema["properties"].(map[string]any)
	required := toStringSet(def.InputSchema["required"])
	if len(props) > 0 {
		fmt.Fprintln(w, "Input Schema:")
		// Sort property names for stable output.
		names := make([]string, 0, len(props))
		for k := range props {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			v, _ := props[k].(map[string]any)
			typ, _ := v["type"].(string)
			desc, _ := v["description"].(string)
			req := ""
			if required[k] {
				req = ", required"
			}
			fmt.Fprintf(w, "  %s (%s%s)", k, typ, req)
			if desc != "" {
				fmt.Fprintf(w, ": %s", desc)
			}
			fmt.Fprintln(w)
		}
	}
	return nil
}

// runValidate parses and validates a YAML tool definition file.
func runValidate(w io.Writer, path string) error {
	def, err := ParseDefinition(path)
	if err != nil {
		return err
	}

	props, _ := def.InputSchema["properties"].(map[string]any)
	required := toStringSet(def.InputSchema["required"])
	reqCount := len(required)

	fmt.Fprintf(w, "✓ %s is valid\n", filepath.Base(path))
	fmt.Fprintf(w, "  Name: %s\n", def.Name)
	fmt.Fprintf(w, "  Command: %s\n", truncate(def.Command, 50))
	fmt.Fprintf(w, "  Inputs: %d properties (%d required)\n", len(props), reqCount)
	return nil
}

// runTest executes a custom tool with test input and displays the result.
func runTest(w io.Writer, name, inputStr string, cwd string, customDirs ...string) error {
	// Find the custom tool.
	customs, _ := discoverForList(cwd, nil, customDirs...)
	var target *Tool
	for _, c := range customs {
		if c.def.Name == name {
			target = c
			break
		}
	}
	if target == nil {
		return fmt.Errorf("custom tool %q not found", name)
	}

	// Parse and validate input.
	input := json.RawMessage(inputStr)
	if err := target.ValidateInput(input); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	fmt.Fprintf(w, "Running %s with input: %s\n", name, inputStr)
	fmt.Fprintln(w, "---")

	// Execute.
	start := time.Now()
	result := RunCommand(context.Background(), target.def.Command, input, target.def.Name, cwd, target.def.Timeout)
	elapsed := time.Since(start)

	// Display output.
	output := result.Stdout
	if output == "" && result.Stderr != "" {
		output = result.Stderr
	}
	if output == "" {
		output = "(no output)"
	}
	fmt.Fprint(w, strings.TrimRight(output, "\n"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "Duration: %s\n", formatDuration(elapsed))
	fmt.Fprintf(w, "Exit code: %d\n", result.ExitCode)
	if result.TimedOut {
		fmt.Fprintf(w, "Timed out: yes (limit: %ds)\n", target.def.Timeout)
	}
	return nil
}

// discoverForList loads custom tools from the given dirs or defaults.
func discoverForList(cwd string, builtins []BuiltinInfo, customDirs ...string) ([]*Tool, []error) {
	builtinNames := make(map[string]bool, len(builtins))
	for _, b := range builtins {
		builtinNames[b.Name] = true
	}
	return DiscoverTools(cwd, builtinNames, customDirs...)
}

// --- helpers ---

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func toStringSet(v any) map[string]bool {
	m := map[string]bool{}
	arr, ok := v.([]any)
	if !ok {
		return m
	}
	for _, item := range arr {
		if s, ok := item.(string); ok {
			m[s] = true
		}
	}
	return m
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
