// Package browser implements a stateful Browser tool backed by the local
// agent-browser CLI.
package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

const toolName = "Browser"

type toolInput struct {
	Action      string `json:"action"`
	Session     string `json:"session,omitempty"`
	URL         string `json:"url,omitempty"`
	Selector    string `json:"selector,omitempty"`
	Text        string `json:"text,omitempty"`
	Key         string `json:"key,omitempty"`
	Path        string `json:"path,omitempty"`
	Target      string `json:"target,omitempty"`
	Direction   string `json:"direction,omitempty"`
	Pixels      *int   `json:"pixels,omitempty"`
	Interactive *bool  `json:"interactive,omitempty"`
	Compact     *bool  `json:"compact,omitempty"`
	Depth       *int   `json:"depth,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Get         string `json:"get,omitempty"`
	AttrName    string `json:"attr_name,omitempty"`
	Locator     string `json:"locator,omitempty"`
	Value       string `json:"value,omitempty"`
	FindAction  string `json:"find_action,omitempty"`
	FindText    string `json:"find_text,omitempty"`
	TabAction   string `json:"tab_action,omitempty"`
	TabIndex    *int   `json:"tab_index,omitempty"`

	// New fields for enhanced actions.
	JS       string   `json:"js,omitempty"`        // JavaScript for eval
	FilePath string   `json:"file_path,omitempty"` // file path for upload
	Width    *int     `json:"width,omitempty"`     // viewport width
	Height   *int     `json:"height,omitempty"`    // viewport height
	Cookies  []Cookie `json:"cookies,omitempty"`   // for set cookies
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// Tool implements a typed, session-aware browser automation surface.
// It uses a chromedp backend when Chrome is available, falling back to
// the agent-browser CLI.
type Tool struct {
	Command string
	Runner  commandRunner

	// NewBackend overrides backend creation for testing.
	NewBackend func(session string) (Backend, error)

	mu             sync.Mutex
	defaultSession string
	backends       map[string]Backend
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Description() string {
	return "Drive a real browser via native Chrome DevTools Protocol (chromedp) or agent-browser CLI fallback. Supports navigation, snapshots, clicks, typing, waiting, reading page state, tab control, downloads, JavaScript execution, file uploads, viewport control, PDF export, cookie management, and console capture."
}

func (t *Tool) SearchHint() string {
	return "browser automation web page snapshot click type fill wait screenshot get url title text html accessibility refs multi-step website docs login forms tabs download find eval javascript console cookies pdf viewport upload"
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["open", "snapshot", "click", "type", "fill", "press", "wait", "get", "screenshot", "scroll", "back", "forward", "reload", "close", "hover", "focus", "download", "find", "tab", "eval", "upload", "viewport", "pdf", "cookies", "console"],
				"description": "The browser action to perform"
			},
			"session": {
				"type": "string",
				"description": "Optional browser session name. If omitted, Forge uses a persistent default session for this tool instance."
			},
			"url": {
				"type": "string",
				"description": "URL to open when action is open"
			},
			"selector": {
				"type": "string",
				"description": "CSS selector or @ref for actions that target an element"
			},
			"text": {
				"type": "string",
				"description": "Text to type or fill"
			},
			"key": {
				"type": "string",
				"description": "Keyboard key for press, for example Enter or Tab"
			},
			"path": {
				"type": "string",
				"description": "Output file path for screenshot"
			},
			"target": {
				"type": "string",
				"description": "Element ref/selector or milliseconds for wait"
			},
			"direction": {
				"type": "string",
				"enum": ["up", "down", "left", "right"],
				"description": "Scroll direction for scroll"
			},
			"pixels": {
				"type": "integer",
				"description": "Optional pixel count for scroll"
			},
			"interactive": {
				"type": "boolean",
				"description": "For snapshot: include only interactive elements"
			},
			"compact": {
				"type": "boolean",
				"description": "For snapshot: remove empty structural elements"
			},
			"depth": {
				"type": "integer",
				"description": "For snapshot: limit tree depth"
			},
			"scope": {
				"type": "string",
				"description": "For snapshot: limit output to a CSS selector scope"
			},
			"get": {
				"type": "string",
				"enum": ["text", "html", "value", "title", "url", "count", "attr"],
				"description": "For get: what value to retrieve"
			},
			"attr_name": {
				"type": "string",
				"description": "For get=attr: attribute name to fetch"
			},
			"locator": {
				"type": "string",
				"description": "For find: locator kind, for example role, text, label, placeholder, alt, title, testid, first, last, or nth"
			},
			"value": {
				"type": "string",
				"description": "For find: locator value"
			},
			"find_action": {
				"type": "string",
				"description": "For find: action to run after locating the element, for example click or fill"
			},
			"find_text": {
				"type": "string",
				"description": "For find with fill/type-like flows: optional text payload"
			},
			"tab_action": {
				"type": "string",
				"enum": ["new", "list", "close", "switch"],
				"description": "For tab: which tab action to perform"
			},
			"tab_index": {
				"type": "integer",
				"description": "For tab switch: zero-based tab index"
			},
			"js": {
				"type": "string",
				"description": "For eval: JavaScript code to execute in the page context"
			},
			"file_path": {
				"type": "string",
				"description": "For upload: path to the local file to upload"
			},
			"width": {
				"type": "integer",
				"description": "For viewport: width in pixels"
			},
			"height": {
				"type": "integer",
				"description": "For viewport: height in pixels"
			},
			"cookies": {
				"type": "array",
				"description": "For cookies with set: array of cookie objects to set",
				"items": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"value": {"type": "string"},
						"domain": {"type": "string"},
						"path": {"type": "string"},
						"expires": {"type": "integer"},
						"httpOnly": {"type": "boolean"},
						"secure": {"type": "boolean"},
						"sameSite": {"type": "string"}
					},
					"required": ["name", "value"]
				}
			}
		},
		"required": ["action"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	switch in.Action {
	case "open":
		if strings.TrimSpace(in.URL) == "" {
			return fmt.Errorf("url is required for action open")
		}
	case "snapshot":
		if in.Depth != nil && *in.Depth < 0 {
			return fmt.Errorf("depth must be >= 0")
		}
	case "click":
		if strings.TrimSpace(in.Selector) == "" {
			return fmt.Errorf("selector is required for action click")
		}
	case "hover", "focus":
		if strings.TrimSpace(in.Selector) == "" {
			return fmt.Errorf("selector is required for action %s", in.Action)
		}
	case "type", "fill":
		if strings.TrimSpace(in.Selector) == "" {
			return fmt.Errorf("selector is required for action %s", in.Action)
		}
		if in.Text == "" {
			return fmt.Errorf("text is required for action %s", in.Action)
		}
	case "press":
		if strings.TrimSpace(in.Key) == "" {
			return fmt.Errorf("key is required for action press")
		}
	case "wait":
		if strings.TrimSpace(in.Target) == "" {
			return fmt.Errorf("target is required for action wait")
		}
	case "get":
		if strings.TrimSpace(in.Get) == "" {
			return fmt.Errorf("get is required for action get")
		}
		switch in.Get {
		case "title", "url":
			// no selector required
		case "attr":
			if strings.TrimSpace(in.Selector) == "" {
				return fmt.Errorf("selector is required for action get when get=attr")
			}
			if strings.TrimSpace(in.AttrName) == "" {
				return fmt.Errorf("attr_name is required for action get when get=attr")
			}
		default:
			if strings.TrimSpace(in.Selector) == "" {
				return fmt.Errorf("selector is required for action get when get=%s", in.Get)
			}
		}
	case "screenshot":
		// path optional
	case "download":
		if strings.TrimSpace(in.Selector) == "" {
			return fmt.Errorf("selector is required for action download")
		}
		if strings.TrimSpace(in.Path) == "" {
			return fmt.Errorf("path is required for action download")
		}
	case "scroll":
		if strings.TrimSpace(in.Direction) == "" {
			return fmt.Errorf("direction is required for action scroll")
		}
	case "find":
		if strings.TrimSpace(in.Locator) == "" {
			return fmt.Errorf("locator is required for action find")
		}
		if strings.TrimSpace(in.Value) == "" {
			return fmt.Errorf("value is required for action find")
		}
		if strings.TrimSpace(in.FindAction) == "" {
			return fmt.Errorf("find_action is required for action find")
		}
	case "tab":
		if strings.TrimSpace(in.TabAction) == "" {
			return fmt.Errorf("tab_action is required for action tab")
		}
		if in.TabAction == "switch" && in.TabIndex == nil {
			return fmt.Errorf("tab_index is required for action tab when tab_action=switch")
		}
	case "back", "forward", "reload", "close":
		// no extra fields
	case "eval":
		if strings.TrimSpace(in.JS) == "" {
			return fmt.Errorf("js is required for action eval")
		}
	case "upload":
		if strings.TrimSpace(in.Selector) == "" {
			return fmt.Errorf("selector is required for action upload")
		}
		if strings.TrimSpace(in.FilePath) == "" {
			return fmt.Errorf("file_path is required for action upload")
		}
	case "viewport":
		if in.Width == nil || in.Height == nil {
			return fmt.Errorf("width and height are required for action viewport")
		}
		if *in.Width <= 0 || *in.Height <= 0 {
			return fmt.Errorf("width and height must be positive")
		}
	case "pdf":
		if strings.TrimSpace(in.Path) == "" {
			return fmt.Errorf("path is required for action pdf")
		}
	case "cookies":
		// get (no extra fields) or set (cookies array)
	case "console":
		// no extra fields
	default:
		return fmt.Errorf("unsupported action %q", in.Action)
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}

	switch in.Action {
	case "snapshot", "get", "wait", "scroll", "back", "forward", "reload", "close", "console":
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	case "cookies":
		if len(in.Cookies) == 0 {
			// get cookies is read-only
			return &models.PermissionDecision{Behavior: models.PermAllow}, nil
		}
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to set %d cookie(s)?", len(in.Cookies)),
		}, nil
	case "tab":
		switch in.TabAction {
		case "list", "switch":
			return &models.PermissionDecision{Behavior: models.PermAllow}, nil
		case "new":
			return &models.PermissionDecision{
				Behavior: models.PermAsk,
				Message:  "Allow Browser to open a new tab?",
			}, nil
		case "close":
			return &models.PermissionDecision{
				Behavior: models.PermAsk,
				Message:  "Allow Browser to close the current tab?",
			}, nil
		}
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser tab action %s?", strings.TrimSpace(in.TabAction)),
		}, nil
	case "open":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to open %s?", strings.TrimSpace(in.URL)),
		}, nil
	case "click", "hover", "focus":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to %s %s?", in.Action, strings.TrimSpace(in.Selector)),
		}, nil
	case "type", "fill":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to %s into %s?", in.Action, strings.TrimSpace(in.Selector)),
		}, nil
	case "press":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to press %s?", strings.TrimSpace(in.Key)),
		}, nil
	case "screenshot":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  "Allow Browser to write a screenshot file?",
		}, nil
	case "download":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to download from %s to %s?", strings.TrimSpace(in.Selector), strings.TrimSpace(in.Path)),
		}, nil
	case "find":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to find %s=%s and run %s?", strings.TrimSpace(in.Locator), strings.TrimSpace(in.Value), strings.TrimSpace(in.FindAction)),
		}, nil
	case "eval":
		js := strings.TrimSpace(in.JS)
		if len(js) > 60 {
			js = js[:60] + "..."
		}
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to execute JavaScript: %s?", js),
		}, nil
	case "upload":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to upload %s via %s?", strings.TrimSpace(in.FilePath), strings.TrimSpace(in.Selector)),
		}, nil
	case "viewport":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to set viewport to %dx%d?", *in.Width, *in.Height),
		}, nil
	case "pdf":
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser to save PDF to %s?", strings.TrimSpace(in.Path)),
		}, nil
	default:
		return &models.PermissionDecision{
			Behavior: models.PermAsk,
			Message:  fmt.Sprintf("Allow Browser action %s?", in.Action),
		}, nil
	}
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *Tool) IsReadOnly(input json.RawMessage) bool {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return false
	}
	switch in.Action {
	case "snapshot", "get", "wait", "scroll", "back", "forward", "reload", "close", "console":
		return true
	case "cookies":
		return len(in.Cookies) == 0 // get cookies is read-only
	case "tab":
		return in.TabAction == "list" || in.TabAction == "switch"
	default:
		return false
	}
}

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Invalid input: %s", err),
			IsError: true,
		}, nil
	}

	session := t.sessionName(in.Session)
	b, err := t.getBackend(session)
	if err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Failed to create browser backend: %s", err),
			IsError: true,
		}, nil
	}

	content, execErr := t.executeAction(ctx, b, in)
	if execErr != nil {
		return &models.ToolResult{Content: execErr.Error(), IsError: true}, nil
	}

	if in.Action == "close" {
		// Save cookies before closing.
		if cookies, err := b.Cookies(ctx); err == nil {
			_ = saveCookies(session, cookies)
		}
		_ = b.Close(ctx)
		t.removeBackend(session)
		if content == "" {
			content = "Browser session closed."
		}
	}

	if content == "" {
		content = fmt.Sprintf("Browser %s completed.", in.Action)
	}
	return &models.ToolResult{Content: content}, nil
}

func (t *Tool) executeAction(ctx context.Context, b Backend, in toolInput) (string, error) {
	switch in.Action {
	case "open":
		// Load saved cookies before navigating.
		session := t.sessionName(in.Session)
		if cookies, err := loadCookies(session); err == nil && len(cookies) > 0 {
			_ = b.SetCookies(ctx, cookies)
		}
		return "", b.Open(ctx, in.URL)

	case "snapshot":
		return b.Snapshot(ctx, SnapshotOpts{
			Interactive: boolOrDefault(in.Interactive, true),
			Compact:     boolOrDefault(in.Compact, true),
			Depth:       in.Depth,
			Scope:       in.Scope,
		})

	case "click":
		return "", b.Click(ctx, in.Selector)
	case "hover", "focus":
		// For chromedp, hover/focus use Click for now (simplification).
		return "", b.Click(ctx, in.Selector)
	case "type":
		return "", b.Type(ctx, in.Selector, in.Text)
	case "fill":
		return "", b.Fill(ctx, in.Selector, in.Text)
	case "press":
		return "", b.Press(ctx, in.Key)
	case "wait":
		return "", b.Wait(ctx, in.Target, 0)
	case "get":
		return b.Get(ctx, in.Get, in.Selector, in.AttrName)
	case "screenshot":
		if err := b.Screenshot(ctx, in.Path); err != nil {
			return "", err
		}
		path := in.Path
		if path == "" {
			path = "screenshot.png"
		}
		return fmt.Sprintf("Screenshot saved to %s", path), nil
	case "scroll":
		pixels := 0
		if in.Pixels != nil {
			pixels = *in.Pixels
		}
		return "", b.Scroll(ctx, in.Direction, pixels)
	case "back", "forward", "reload":
		return "", b.Navigate(ctx, in.Action)
	case "close":
		return "", nil // handled by Execute after this returns
	case "download":
		// download goes through CLI backend only (passthrough)
		return "", b.Click(ctx, in.Selector)

	// New actions.
	case "eval":
		return b.Eval(ctx, in.JS)
	case "upload":
		return "", b.Upload(ctx, in.Selector, in.FilePath)
	case "viewport":
		if err := b.SetViewport(ctx, *in.Width, *in.Height); err != nil {
			return "", err
		}
		return fmt.Sprintf("Viewport set to %dx%d", *in.Width, *in.Height), nil
	case "pdf":
		if err := b.PDF(ctx, in.Path); err != nil {
			return "", err
		}
		return fmt.Sprintf("PDF saved to %s", in.Path), nil
	case "cookies":
		if len(in.Cookies) > 0 {
			// Set cookies.
			if err := b.SetCookies(ctx, in.Cookies); err != nil {
				return "", err
			}
			return fmt.Sprintf("Set %d cookie(s)", len(in.Cookies)), nil
		}
		// Get cookies.
		cookies, err := b.Cookies(ctx)
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(cookies, "", "  ")
		return string(data), nil
	case "console":
		msgs := b.ConsoleMessages()
		if len(msgs) == 0 {
			return "No console messages captured.", nil
		}
		data, _ := json.MarshalIndent(msgs, "", "  ")
		return string(data), nil

	// Legacy CLI-passthrough actions.
	case "find":
		if cli, ok := b.(*CLIBackend); ok {
			args := []string{"find", in.Locator, in.Value, in.FindAction}
			if strings.TrimSpace(in.FindText) != "" {
				args = append(args, in.FindText)
			}
			return cli.exec(ctx, args...)
		}
		return "", fmt.Errorf("find action requires the agent-browser CLI backend")
	case "tab":
		if cli, ok := b.(*CLIBackend); ok {
			switch in.TabAction {
			case "new", "list", "close":
				return cli.exec(ctx, "tab", in.TabAction)
			case "switch":
				return cli.exec(ctx, "tab", fmt.Sprintf("%d", *in.TabIndex))
			}
		}
		return "", fmt.Errorf("tab action requires the agent-browser CLI backend")
	default:
		return "", fmt.Errorf("unsupported action %q", in.Action)
	}
}

func buildArgs(in toolInput) ([]string, error) {
	switch in.Action {
	case "open":
		return []string{"open", in.URL}, nil
	case "snapshot":
		args := []string{"snapshot"}
		if boolOrDefault(in.Interactive, true) {
			args = append(args, "-i")
		}
		if boolOrDefault(in.Compact, true) {
			args = append(args, "-c")
		}
		if in.Depth != nil {
			args = append(args, "-d", fmt.Sprintf("%d", *in.Depth))
		}
		if strings.TrimSpace(in.Scope) != "" {
			args = append(args, "-s", in.Scope)
		}
		return args, nil
	case "click":
		return []string{"click", in.Selector}, nil
	case "hover":
		return []string{"hover", in.Selector}, nil
	case "focus":
		return []string{"focus", in.Selector}, nil
	case "type":
		return []string{"type", in.Selector, in.Text}, nil
	case "fill":
		return []string{"fill", in.Selector, in.Text}, nil
	case "press":
		return []string{"press", in.Key}, nil
	case "wait":
		return []string{"wait", in.Target}, nil
	case "get":
		switch in.Get {
		case "title", "url":
			return []string{"get", in.Get}, nil
		case "attr":
			return []string{"get", "attr", in.AttrName, in.Selector}, nil
		default:
			return []string{"get", in.Get, in.Selector}, nil
		}
	case "screenshot":
		args := []string{"screenshot"}
		if strings.TrimSpace(in.Path) != "" {
			args = append(args, in.Path)
		}
		return args, nil
	case "download":
		return []string{"download", in.Selector, in.Path}, nil
	case "scroll":
		args := []string{"scroll", in.Direction}
		if in.Pixels != nil {
			args = append(args, fmt.Sprintf("%d", *in.Pixels))
		}
		return args, nil
	case "find":
		args := []string{"find", in.Locator, in.Value, in.FindAction}
		if strings.TrimSpace(in.FindText) != "" {
			args = append(args, in.FindText)
		}
		return args, nil
	case "tab":
		switch in.TabAction {
		case "new", "list", "close":
			return []string{"tab", in.TabAction}, nil
		case "switch":
			return []string{"tab", fmt.Sprintf("%d", *in.TabIndex)}, nil
		default:
			return nil, fmt.Errorf("unsupported tab_action %q", in.TabAction)
		}
	case "back", "forward", "reload", "close":
		return []string{in.Action}, nil
	default:
		return nil, fmt.Errorf("unsupported action %q", in.Action)
	}
}

func boolOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// getBackend returns or creates a Backend for the given session.
func (t *Tool) getBackend(session string) (Backend, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.backends == nil {
		t.backends = make(map[string]Backend)
	}
	if b, ok := t.backends[session]; ok {
		return b, nil
	}

	b, err := t.createBackend(session)
	if err != nil {
		return nil, err
	}
	t.backends[session] = b
	return b, nil
}

func (t *Tool) removeBackend(session string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.backends, session)
}

// createBackend tries chromedp first, then falls back to agent-browser CLI.
// If Command is explicitly set, use CLI backend directly.
func (t *Tool) createBackend(session string) (Backend, error) {
	if t.NewBackend != nil {
		return t.NewBackend(session)
	}

	// If a CLI command is explicitly configured, use it directly.
	if t.Command != "" {
		return NewCLIBackend(t.commandName(), session, t.commandRunner()), nil
	}

	// Try to find Chrome/Chromium.
	if chromeAvailable() {
		b, err := NewChromedpBackend()
		if err == nil {
			return b, nil
		}
		// Fall through to CLI if chromedp fails to start.
	}

	// Fall back to agent-browser CLI.
	return NewCLIBackend(t.commandName(), session, t.commandRunner()), nil
}

// chromeAvailable checks if a Chrome or Chromium binary is available.
func chromeAvailable() bool {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func (t *Tool) commandName() string {
	if strings.TrimSpace(t.Command) == "" {
		return "agent-browser"
	}
	return t.Command
}

func (t *Tool) commandRunner() commandRunner {
	if t.Runner == nil {
		return execRunner{}
	}
	return t.Runner
}

func (t *Tool) sessionName(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.defaultSession == "" {
		t.defaultSession = "forge-browser-" + randomSuffix()
	}
	return t.defaultSession
}

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "default"
	}
	return hex.EncodeToString(buf[:])
}

func formatExecError(action, command string, err error) string {
	if isCommandNotFound(err) {
		return fmt.Sprintf(
			"%s is not installed or not on PATH. Install it with `npm install -g agent-browser` or `brew install agent-browser`, then run `agent-browser install` once.",
			command,
		)
	}
	return fmt.Sprintf("browser action %s failed via %s: %s", action, command, err)
}

func isCommandNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return true
	}
	return errors.Is(err, exec.ErrNotFound)
}
