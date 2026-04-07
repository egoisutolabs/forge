package browser

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CLIBackend implements Backend by shelling out to the agent-browser CLI.
type CLIBackend struct {
	Command string
	Session string
	Runner  commandRunner
	console []ConsoleMessage // always empty for CLI backend
}

// NewCLIBackend creates a CLI-based browser backend.
func NewCLIBackend(command, session string, runner commandRunner) *CLIBackend {
	if command == "" {
		command = "agent-browser"
	}
	if runner == nil {
		runner = execRunner{}
	}
	return &CLIBackend{
		Command: command,
		Session: session,
		Runner:  runner,
	}
}

func (c *CLIBackend) exec(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{"--session", c.Session}, args...)
	out, err := c.Runner.CombinedOutput(ctx, c.Command, fullArgs...)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", c.Command, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *CLIBackend) Open(ctx context.Context, url string) error {
	_, err := c.exec(ctx, "open", url)
	return err
}

func (c *CLIBackend) Snapshot(ctx context.Context, opts SnapshotOpts) (string, error) {
	args := []string{"snapshot"}
	if opts.Interactive {
		args = append(args, "-i")
	}
	if opts.Compact {
		args = append(args, "-c")
	}
	if opts.Depth != nil {
		args = append(args, "-d", fmt.Sprintf("%d", *opts.Depth))
	}
	if opts.Scope != "" {
		args = append(args, "-s", opts.Scope)
	}
	return c.exec(ctx, args...)
}

func (c *CLIBackend) Click(ctx context.Context, selector string) error {
	_, err := c.exec(ctx, "click", selector)
	return err
}

func (c *CLIBackend) Type(ctx context.Context, selector, text string) error {
	_, err := c.exec(ctx, "type", selector, text)
	return err
}

func (c *CLIBackend) Fill(ctx context.Context, selector, text string) error {
	_, err := c.exec(ctx, "fill", selector, text)
	return err
}

func (c *CLIBackend) Press(ctx context.Context, key string) error {
	_, err := c.exec(ctx, "press", key)
	return err
}

func (c *CLIBackend) Wait(ctx context.Context, target string, _ time.Duration) error {
	_, err := c.exec(ctx, "wait", target)
	return err
}

func (c *CLIBackend) Get(ctx context.Context, what, selector, attrName string) (string, error) {
	switch what {
	case "title", "url":
		return c.exec(ctx, "get", what)
	case "attr":
		return c.exec(ctx, "get", "attr", attrName, selector)
	default:
		return c.exec(ctx, "get", what, selector)
	}
}

func (c *CLIBackend) Screenshot(ctx context.Context, path string) error {
	args := []string{"screenshot"}
	if path != "" {
		args = append(args, path)
	}
	_, err := c.exec(ctx, args...)
	return err
}

func (c *CLIBackend) Scroll(ctx context.Context, direction string, pixels int) error {
	args := []string{"scroll", direction}
	if pixels > 0 {
		args = append(args, fmt.Sprintf("%d", pixels))
	}
	_, err := c.exec(ctx, args...)
	return err
}

func (c *CLIBackend) Navigate(ctx context.Context, action string) error {
	_, err := c.exec(ctx, action)
	return err
}

func (c *CLIBackend) Close(ctx context.Context) error {
	_, err := c.exec(ctx, "close")
	return err
}

func (c *CLIBackend) Eval(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("eval is not supported by the CLI backend; install Chrome for JavaScript execution")
}

func (c *CLIBackend) Upload(_ context.Context, _, _ string) error {
	return fmt.Errorf("upload is not supported by the CLI backend; install Chrome for file uploads")
}

func (c *CLIBackend) SetViewport(_ context.Context, _, _ int) error {
	return fmt.Errorf("viewport is not supported by the CLI backend; install Chrome for viewport control")
}

func (c *CLIBackend) PDF(_ context.Context, _ string) error {
	return fmt.Errorf("pdf is not supported by the CLI backend; install Chrome for PDF export")
}

func (c *CLIBackend) Cookies(_ context.Context) ([]Cookie, error) {
	return nil, fmt.Errorf("cookies is not supported by the CLI backend; install Chrome for cookie management")
}

func (c *CLIBackend) SetCookies(_ context.Context, _ []Cookie) error {
	return fmt.Errorf("set cookies is not supported by the CLI backend; install Chrome for cookie management")
}

func (c *CLIBackend) ConsoleMessages() []ConsoleMessage {
	return nil
}
