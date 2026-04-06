package browser

import (
	"context"
	"net/http"
	"time"
)

// Cookie represents an HTTP cookie for get/set operations.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  int64  `json:"expires,omitempty"` // Unix timestamp
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	SameSite string `json:"sameSite,omitempty"` // "Strict", "Lax", "None"
}

// ConsoleMessage captures a single browser console API call.
type ConsoleMessage struct {
	Level string `json:"level"` // "log", "warn", "error", "info", "debug"
	Text  string `json:"text"`
}

// SnapshotOpts controls what a snapshot returns.
type SnapshotOpts struct {
	Interactive bool
	Compact     bool
	Depth       *int
	Scope       string
}

// Backend is the abstraction over browser automation engines.
// The chromedp backend uses native Chrome DevTools Protocol; the CLI backend
// shells out to the agent-browser binary.
type Backend interface {
	// Open navigates to the given URL.
	Open(ctx context.Context, url string) error

	// Snapshot returns an accessibility-tree representation of the page.
	Snapshot(ctx context.Context, opts SnapshotOpts) (string, error)

	// Click clicks an element identified by selector.
	Click(ctx context.Context, selector string) error

	// Type sends keystrokes to an element (appends to existing value).
	Type(ctx context.Context, selector, text string) error

	// Fill sets the value of an input element (replaces existing value).
	Fill(ctx context.Context, selector, text string) error

	// Press sends a single key press (e.g. "Enter", "Tab").
	Press(ctx context.Context, key string) error

	// Wait waits for a selector to appear or a duration to elapse.
	Wait(ctx context.Context, target string, timeout time.Duration) error

	// Get retrieves a value from the page: text, html, value, title, url, count, attr.
	Get(ctx context.Context, what, selector, attrName string) (string, error)

	// Screenshot saves a screenshot to the given path.
	Screenshot(ctx context.Context, path string) error

	// Scroll scrolls the page in a direction by optional pixels.
	Scroll(ctx context.Context, direction string, pixels int) error

	// Navigate performs back/forward/reload.
	Navigate(ctx context.Context, action string) error

	// Close shuts down the browser session.
	Close(ctx context.Context) error

	// Eval executes JavaScript in the page and returns the result as a string.
	Eval(ctx context.Context, js string) (string, error)

	// Upload sets a file input to the given file path.
	Upload(ctx context.Context, selector, filePath string) error

	// SetViewport changes the browser viewport dimensions.
	SetViewport(ctx context.Context, width, height int) error

	// PDF saves the current page as a PDF file.
	PDF(ctx context.Context, path string) error

	// Cookies returns all cookies for the current page.
	Cookies(ctx context.Context) ([]Cookie, error)

	// SetCookies sets cookies in the browser.
	SetCookies(ctx context.Context, cookies []Cookie) error

	// ConsoleMessages returns console messages captured since session start.
	ConsoleMessages() []ConsoleMessage
}

// cookieSameSiteToHTTP maps our SameSite string to net/http constants.
func cookieSameSiteToHTTP(s string) http.SameSite {
	switch s {
	case "Strict":
		return http.SameSiteStrictMode
	case "Lax":
		return http.SameSiteLaxMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}
