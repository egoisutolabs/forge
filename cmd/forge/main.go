// Command forge is the CLI entrypoint for the Forge agent.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/auth"
	"github.com/egoisutolabs/forge/config"
	"github.com/egoisutolabs/forge/coordinator"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/features"
	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/internal/log"
	"github.com/egoisutolabs/forge/lsp"
	"github.com/egoisutolabs/forge/observe"
	"github.com/egoisutolabs/forge/orchestrator"
	"github.com/egoisutolabs/forge/plugins"
	"github.com/egoisutolabs/forge/prompt"
	"github.com/egoisutolabs/forge/provider"
	"github.com/egoisutolabs/forge/skills"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/agent"
	"github.com/egoisutolabs/forge/tools/askuser"
	"github.com/egoisutolabs/forge/tools/astgrep"
	"github.com/egoisutolabs/forge/tools/bash"
	"github.com/egoisutolabs/forge/tools/browser"
	"github.com/egoisutolabs/forge/tools/custom"
	"github.com/egoisutolabs/forge/tools/fileedit"
	"github.com/egoisutolabs/forge/tools/fileread"
	"github.com/egoisutolabs/forge/tools/filewrite"
	"github.com/egoisutolabs/forge/tools/glob"
	"github.com/egoisutolabs/forge/tools/grep"
	lsptool "github.com/egoisutolabs/forge/tools/lsp"
	"github.com/egoisutolabs/forge/tools/planmode"
	"github.com/egoisutolabs/forge/tools/sendmessage"
	"github.com/egoisutolabs/forge/tools/skill"
	"github.com/egoisutolabs/forge/tools/tasks"
	"github.com/egoisutolabs/forge/tools/toolsearch"
	"github.com/egoisutolabs/forge/tools/webfetch"
	"github.com/egoisutolabs/forge/tools/websearch"
	"github.com/egoisutolabs/forge/tui"
)

func main() {
	var (
		model        = flag.String("model", "", "Model ID (auto-detected if omitted)")
		maxTurns     = flag.Int("max-turns", 100, "Maximum agentic loop turns")
		maxBudget    = flag.Float64("max-budget", 0, "Maximum USD budget (0 = unlimited)")
		cwd          = flag.String("cwd", "", "Working directory (default: current directory)")
		systemPrompt = flag.String("system-prompt", "", "Additional system prompt text")
		apiKey       = flag.String("api-key", "", "Anthropic API key (overrides ANTHROPIC_API_KEY)")
		logRedact    = flag.Bool("log-redact", false, "Redact tool inputs/outputs in observability logs")
	)
	flag.Parse()

	// Handle "forge tool" subcommands before anything else (no API key needed).
	if flag.NArg() > 0 && flag.Arg(0) == "tool" {
		handleToolCommand(flag.Args())
		return
	}

	// Handle "forge log" subcommands before anything else.
	if flag.NArg() > 0 && flag.Arg(0) == "log" {
		handleLogCommand(flag.Args()[1:])
		return
	}

	// Resolve working directory (needed before config loading).
	dir := *cwd
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot get working directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Load config and resolve provider. Falls back to --api-key flag behavior
	// if no config file exists or model resolution fails.
	var caller api.Caller
	cfg, cfgErr := config.Load(dir)
	if cfgErr != nil {
		log.Debug("config load: %v", cfgErr)
	}

	// Use the model registry to auto-detect providers and resolve the model.
	registry := provider.NewRegistry(cfg)

	resolvedModel := *model
	noAuth := false
	if resolvedModel == "" {
		resolvedModel = registry.DefaultModel()
	}

	if resolvedModel == "" && !registry.HasModels() {
		noAuth = true
	}

	if cfg != nil && len(cfg.Providers) > 0 {
		// Use config-based provider resolution.
		prov, err := cfg.ResolveModel(resolvedModel)
		if err == nil {
			// CLI --api-key flag overrides config file key (highest precedence).
			if *apiKey != "" {
				prov.APIKey = *apiKey
			}
			caller = api.NewCaller(prov)
		} else {
			log.Debug("model resolution: %v (falling back to direct API key)", err)
		}
	}

	// Fallback: use direct API key + model (original behavior).
	// If no auth is found anywhere, start the TUI anyway with a notification
	// prompting the user to run /connect.
	if caller == nil {
		key := *apiKey
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
		if key == "" {
			// Check all auth sources before giving up.
			key = auth.GetAPIKey("anthropic", cfg)
		}
		if key != "" {
			caller = api.NewAnthropicCaller(key, resolvedModel)
		} else if !auth.HasAnyAuth(cfg) {
			noAuth = true
			// Create a placeholder caller so the TUI can start.
			// The first API call will fail, but /connect will fix it.
			caller = api.NewAnthropicCaller("", resolvedModel)
		} else {
			fmt.Fprintln(os.Stderr, "error: no API key found for the selected model. Pass --api-key or set the environment variable.")
			os.Exit(1)
		}
	}

	// Record the model as recently used.
	_ = provider.RecordUsage(resolvedModel)

	// Detect git repo.
	isGit := isGitRepo(dir)

	// Build skill registry and register the /forge orchestrator skill.
	skillRegistry := skills.BundledRegistry()
	orchestrator.RegisterForgeSkill(skillRegistry)
	_ = skills.LoadDefaultSkills(dir, skillRegistry)

	// Discover plugins from ~/.forge/plugins/ and register their skills.
	forgeHome := filepath.Join(homeDir(), ".forge")
	pluginsDir := filepath.Join(forgeHome, "plugins")
	discoveredPlugins, pluginErr := plugins.DiscoverPlugins(pluginsDir)
	if pluginErr != nil {
		log.Debug("plugin discovery: %v", pluginErr)
	}
	for _, p := range discoveredPlugins {
		for _, sp := range p.SkillPaths {
			_ = skills.LoadDefaultSkills(sp, skillRegistry)
		}
	}

	// Load hook settings from ~/.forge/settings.json (if it exists).
	var hookSettings hooks.HooksSettings
	settingsPath := filepath.Join(forgeHome, "settings.json")
	if fileSettings, err := hooks.LoadHooksFromFile(settingsPath); err == nil {
		hookSettings = fileSettings
	}
	// Merge in hook configs from discovered plugins.
	for _, p := range discoveredPlugins {
		if len(p.HookConfigs) > 0 {
			hookSettings = hooks.MergeHooks(hookSettings, p.HookConfigs)
		}
	}

	// Register all tools.
	allTools := buildTools(caller, skillRegistry)

	// Set up background agent notification channel. The agent registry
	// writes formatted <task-notification> strings here when workers
	// complete or fail; the engine loop drains them between turns.
	notifCh := make(chan string, 64)
	agent.DefaultRegistry.NotifyCh = notifCh

	// First-run experience: if no auth is configured, notify the user.
	if noAuth {
		notifCh <- "No API providers configured. Type /connect to add one, or set ANTHROPIC_API_KEY in your environment."
	}

	// Coordinator mode: log activation.
	if coordinator.IsCoordinatorMode() {
		fmt.Fprintln(os.Stderr, "forge: coordinator mode active (FORGE_COORDINATOR_MODE=1)")
	}

	// Build tool names for the system prompt.
	toolNames := make([]string, 0, len(allTools))
	for _, t := range allTools {
		toolNames = append(toolNames, t.Name())
	}

	// Build system prompt.
	pb := prompt.NewBuilder(prompt.Config{
		Cwd:                dir,
		Model:              resolvedModel,
		IsGitRepo:          isGit,
		ToolNames:          toolNames,
		AppendSystemPrompt: *systemPrompt,
	})
	sysPrompt := pb.Build()

	// Detect and start LSP language servers for code intelligence.
	var lspMgr *lsp.Manager
	if lspConfigs := lsp.DetectConfigs(dir); len(lspConfigs) > 0 {
		lspMgr = lsp.NewManager(dir, lspConfigs)
		log.Debug("LSP: detected %d language server(s)", len(lspConfigs))
	}

	// Create engine and TUI bridge. NewBridge wires OnEvent and PermissionPrompt
	// onto the engine so streaming events and permission requests flow to the TUI.
	eng := engine.New(engine.Config{
		Model:          resolvedModel,
		SystemPrompt:   sysPrompt,
		Tools:          allTools,
		MaxTurns:       *maxTurns,
		MaxBudgetUSD:   *maxBudget,
		Cwd:            dir,
		Skills:         skillRegistry,
		Hooks:          hookSettings,
		GlobMaxResults: 100,
		Notifications:  notifCh,
		LSPManager:     lspMgr,
	})

	// Initialize observability logging.
	redact := *logRedact || os.Getenv("FORGE_LOG_REDACT") == "1"
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := observe.Init(sessionID, observe.EmitterOpts{Redact: redact}); err != nil {
		log.Debug("observability init failed: %v", err)
	}
	defer observe.Shutdown()
	observe.RotateLogs("", 7*24*time.Hour, 500*1024*1024)

	bridge := tui.NewBridge(eng, caller)

	// Build and run the TUI.
	model_ := tui.New(bridge)
	model_.SetRegistry(registry)
	model_.SetConfig(cfg)
	if err := tui.Run(model_); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// buildTools constructs and returns all registered tools, respecting
// compile-time feature gates from the features package.
//
// Build tags:
//   - default: all tools included
//   - minimal: excludes web tools (WebFetch, WebSearch), Browser, AstGrep
//   - speculation: includes speculation-related tools (future)
//   - debug: enables verbose logging at startup
func buildTools(caller api.Caller, skillRegistry *skills.SkillRegistry) []tools.Tool {
	if features.DebugEnabled {
		log.Debug("Feature gates: debug=true speculation=%v minimal=%v",
			features.SpeculationEnabled, features.MinimalBuild)
	}

	// Core tools — always included.
	tt := []tools.Tool{
		&bash.Tool{},
		&fileread.Tool{},
		&fileedit.Tool{},
		&filewrite.Tool{},
		&glob.Tool{},
		&grep.Tool{},
		&lsptool.Tool{},
		&askuser.Tool{},
		&planmode.EnterTool{},
		&planmode.ExitTool{},
		&tasks.CreateTool{},
		&tasks.GetTool{},
		&tasks.ListTool{},
		&tasks.UpdateTool{},
		&tasks.StopTool{},
		&tasks.OutputTool{},
		&skill.Tool{},
		&toolsearch.Tool{},
		agent.New(caller, nil),
		sendmessage.New(nil),
	}

	// Non-minimal tools — excluded with `go build -tags minimal`.
	if !features.MinimalBuild {
		tt = append(tt,
			&browser.Tool{},
			&astgrep.Tool{},
			&webfetch.Tool{},
			&websearch.Tool{},
		)
	}

	// Deferred tool loading is available via the tools.Deferrable interface and
	// tools.SplitTools(), but no tools currently opt into deferral. When a tool
	// becomes expensive to load or rarely used, implement ShouldDefer() = true
	// on it and use SplitTools() here to separate loaded vs deferred sets.

	// Load custom tools from ~/.forge/tools/ and .forge/tools/.
	cwd, _ := os.Getwd()
	builtinNames := make(map[string]bool, len(tt))
	for _, t := range tt {
		builtinNames[t.Name()] = true
	}
	customTools, customErrs := custom.DiscoverTools(cwd, builtinNames)
	for _, e := range customErrs {
		log.Debug("custom tool: %v", e)
	}
	for _, ct := range customTools {
		tt = append(tt, ct)
	}

	return tt
}

// homeDir returns the user's home directory, falling back to "." if unknown.
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// handleToolCommand dispatches "forge tool <subcommand>" to the custom tools CLI.
func handleToolCommand(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Build a static list of built-in tool info (avoids instantiating all tools).
	builtins := []custom.BuiltinInfo{
		{Name: "Bash", Description: "Execute a bash command"},
		{Name: "FileRead", Description: "Reads a file from the local filesystem."},
		{Name: "FileEdit", Description: "A tool for editing files."},
		{Name: "FileWrite", Description: "Write a file to the local filesystem."},
		{Name: "Glob", Description: "Find files matching a glob pattern"},
		{Name: "Grep", Description: "Search file contents with ripgrep"},
		{Name: "AskUser", Description: "Ask the user multiple-choice questions"},
		{Name: "EnterPlanMode", Description: "Enter plan mode"},
		{Name: "ExitPlanMode", Description: "Exit plan mode and present implementation plan"},
		{Name: "TaskCreate", Description: "Create a task"},
		{Name: "TaskGet", Description: "Get task details"},
		{Name: "TaskList", Description: "List all tasks"},
		{Name: "TaskUpdate", Description: "Update a task"},
		{Name: "TaskStop", Description: "Stop a running task"},
		{Name: "TaskOutput", Description: "Get task output"},
		{Name: "Skill", Description: "Invoke a skill (slash command) by name."},
		{Name: "ToolSearch", Description: "Search for tools by keyword"},
		{Name: "Agent", Description: "Launch a sub-agent for complex tasks"},
		{Name: "SendMessage", Description: "Send a message to another agent"},
		{Name: "Browser", Description: "Browser automation"},
		{Name: "AstGrep", Description: "Structural code search (AST-based)"},
		{Name: "WebFetch", Description: "Fetch and analyze web pages"},
		{Name: "WebSearch", Description: "Search the web"},
		{Name: "LSP", Description: "Query language servers for code intelligence"},
	}

	if err := custom.DispatchCLI(os.Stdout, args, builtins, cwd); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// isGitRepo returns true if dir is inside a git repository.
func isGitRepo(dir string) bool {
	// Walk up looking for .git directory.
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
	}
	// Fall back: ask git itself.
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}
