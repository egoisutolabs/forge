package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const (
	defaultTimeout  = 30 * time.Second
	staleRetryCount = 3
	staleRetryDelay = 500 * time.Millisecond
	defaultWait     = 10 * time.Second
)

// ChromedpBackend drives a real Chrome browser via the DevTools Protocol.
type ChromedpBackend struct {
	allocCancel context.CancelFunc
	ctx         context.Context
	ctxCancel   context.CancelFunc

	mu      sync.Mutex
	console []ConsoleMessage
	started bool
}

// NewChromedpBackend creates a new chromedp-based browser backend.
// It launches a headless Chrome instance.
func NewChromedpBackend() (*ChromedpBackend, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	b := &ChromedpBackend{
		allocCancel: allocCancel,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
	}

	// Listen for console messages.
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			var parts []string
			for _, arg := range e.Args {
				if arg.Value != nil {
					var s string
					if err := json.Unmarshal(arg.Value, &s); err == nil {
						parts = append(parts, s)
					} else {
						parts = append(parts, string(arg.Value))
					}
				} else if arg.Description != "" {
					parts = append(parts, arg.Description)
				}
			}
			b.mu.Lock()
			b.console = append(b.console, ConsoleMessage{
				Level: e.Type.String(),
				Text:  strings.Join(parts, " "),
			})
			b.mu.Unlock()
		}
	})

	b.started = true
	return b, nil
}

func (b *ChromedpBackend) Open(ctx context.Context, url string) error {
	return b.runWithRetry(ctx, chromedp.Navigate(url))
}

func (b *ChromedpBackend) Snapshot(ctx context.Context, opts SnapshotOpts) (string, error) {
	// Use JavaScript to build an accessibility-like snapshot.
	js := buildSnapshotJS(opts)
	var result string
	if err := b.run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return "", err
	}
	return result, nil
}

func (b *ChromedpBackend) Click(ctx context.Context, selector string) error {
	return b.runWithRetry(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

func (b *ChromedpBackend) Type(ctx context.Context, selector, text string) error {
	return b.runWithRetry(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

func (b *ChromedpBackend) Fill(ctx context.Context, selector, text string) error {
	return b.runWithRetry(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

func (b *ChromedpBackend) Press(ctx context.Context, key string) error {
	kb := chromedp.KeyEvent(key)
	return b.run(ctx, kb)
}

func (b *ChromedpBackend) Wait(ctx context.Context, target string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultWait
	}

	// If target looks like a duration (all digits or ends with ms/s), sleep.
	if isDurationTarget(target) {
		d, err := parseDuration(target)
		if err != nil {
			return err
		}
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Otherwise treat it as a CSS selector.
	wCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return b.run(wCtx, chromedp.WaitVisible(target, chromedp.ByQuery))
}

func (b *ChromedpBackend) Get(ctx context.Context, what, selector, attrName string) (string, error) {
	var result string
	switch what {
	case "title":
		if err := b.run(ctx, chromedp.Title(&result)); err != nil {
			return "", err
		}
	case "url":
		if err := b.run(ctx, chromedp.Location(&result)); err != nil {
			return "", err
		}
	case "text":
		if err := b.run(ctx, chromedp.Text(selector, &result, chromedp.ByQuery)); err != nil {
			return "", err
		}
	case "html":
		if err := b.run(ctx, chromedp.OuterHTML(selector, &result, chromedp.ByQuery)); err != nil {
			return "", err
		}
	case "value":
		if err := b.run(ctx, chromedp.Value(selector, &result, chromedp.ByQuery)); err != nil {
			return "", err
		}
	case "attr":
		var ok bool
		if err := b.run(ctx, chromedp.AttributeValue(selector, attrName, &result, &ok, chromedp.ByQuery)); err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("attribute %q not found on %s", attrName, selector)
		}
	case "count":
		js := fmt.Sprintf(`String(document.querySelectorAll(%q).length)`, selector)
		if err := b.run(ctx, chromedp.Evaluate(js, &result)); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported get target %q", what)
	}
	return result, nil
}

func (b *ChromedpBackend) Screenshot(ctx context.Context, path string) error {
	var buf []byte
	if err := b.run(ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return err
	}
	if path == "" {
		path = "screenshot.png"
	}
	return os.WriteFile(path, buf, 0600)
}

func (b *ChromedpBackend) Scroll(ctx context.Context, direction string, pixels int) error {
	if pixels == 0 {
		pixels = 300
	}
	var js string
	switch direction {
	case "down":
		js = fmt.Sprintf("window.scrollBy(0, %d)", pixels)
	case "up":
		js = fmt.Sprintf("window.scrollBy(0, -%d)", pixels)
	case "right":
		js = fmt.Sprintf("window.scrollBy(%d, 0)", pixels)
	case "left":
		js = fmt.Sprintf("window.scrollBy(-%d, 0)", pixels)
	default:
		return fmt.Errorf("unsupported scroll direction %q", direction)
	}
	return b.run(ctx, chromedp.Evaluate(js, nil))
}

func (b *ChromedpBackend) Navigate(ctx context.Context, action string) error {
	switch action {
	case "back":
		return b.run(ctx, chromedp.NavigateBack())
	case "forward":
		return b.run(ctx, chromedp.NavigateForward())
	case "reload":
		return b.run(ctx, page.Reload())
	default:
		return fmt.Errorf("unsupported navigate action %q", action)
	}
}

func (b *ChromedpBackend) Close(_ context.Context) error {
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	return nil
}

func (b *ChromedpBackend) Eval(ctx context.Context, js string) (string, error) {
	var result string
	if err := b.run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return "", err
	}
	return result, nil
}

func (b *ChromedpBackend) Upload(ctx context.Context, selector, filePath string) error {
	return b.run(ctx, chromedp.SetUploadFiles(selector, []string{filePath}, chromedp.ByQuery))
}

func (b *ChromedpBackend) SetViewport(ctx context.Context, width, height int) error {
	return b.run(ctx, emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1.0, false))
}

func (b *ChromedpBackend) PDF(ctx context.Context, path string) error {
	var buf []byte
	if err := b.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, _, err = page.PrintToPDF().Do(ctx)
		return err
	})); err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0600)
}

func (b *ChromedpBackend) Cookies(ctx context.Context) ([]Cookie, error) {
	var result []Cookie
	err := b.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookies {
			result = append(result, Cookie{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Expires:  int64(c.Expires),
				HTTPOnly: c.HTTPOnly,
				Secure:   c.Secure,
				SameSite: sameSiteToString(c.SameSite),
			})
		}
		return nil
	}))
	return result, err
}

func (b *ChromedpBackend) SetCookies(ctx context.Context, cookies []Cookie) error {
	return b.run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		for _, c := range cookies {
			cp := network.SetCookie(c.Name, c.Value).
				WithDomain(c.Domain).
				WithPath(c.Path).
				WithHTTPOnly(c.HTTPOnly).
				WithSecure(c.Secure)
			if c.Expires > 0 {
				ts := cdp.TimeSinceEpoch(time.Unix(c.Expires, 0))
				cp = cp.WithExpires(&ts)
			}
			if err := cp.Do(ctx); err != nil {
				return fmt.Errorf("set cookie %q: %w", c.Name, err)
			}
		}
		return nil
	}))
}

func (b *ChromedpBackend) ConsoleMessages() []ConsoleMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ConsoleMessage, len(b.console))
	copy(out, b.console)
	return out
}

// run executes chromedp actions using the backend's browser context.
func (b *ChromedpBackend) run(ctx context.Context, actions ...chromedp.Action) error {
	// Merge the caller's deadline/cancellation with the browser context.
	mergedCtx, cancel := mergeContexts(b.ctx, ctx)
	defer cancel()
	return chromedp.Run(mergedCtx, actions...)
}

// runWithRetry runs actions with stale-element retry logic.
func (b *ChromedpBackend) runWithRetry(ctx context.Context, actions ...chromedp.Action) error {
	var err error
	for i := 0; i < staleRetryCount; i++ {
		err = b.run(ctx, actions...)
		if err == nil {
			return nil
		}
		if !isStaleError(err) {
			return err
		}
		select {
		case <-time.After(staleRetryDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

// mergeContexts returns a context that is cancelled when either parent is done.
// The returned cancel must be called to avoid leaking the monitoring goroutine.
func mergeContexts(browserCtx, callerCtx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(browserCtx)
	go func() {
		select {
		case <-callerCtx.Done():
			cancel()
		case <-browserCtx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func isStaleError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "stale") ||
		strings.Contains(msg, "node not found") ||
		strings.Contains(msg, "Could not find node")
}

func isDurationTarget(target string) bool {
	for _, c := range target {
		if c >= '0' && c <= '9' {
			continue
		}
		if c == 'm' || c == 's' {
			continue
		}
		return false
	}
	return len(target) > 0
}

func parseDuration(target string) (time.Duration, error) {
	// Pure digits = milliseconds.
	allDigits := true
	for _, c := range target {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return time.ParseDuration(target + "ms")
	}
	return time.ParseDuration(target)
}

func sameSiteToString(ss network.CookieSameSite) string {
	switch ss {
	case network.CookieSameSiteStrict:
		return "Strict"
	case network.CookieSameSiteLax:
		return "Lax"
	case network.CookieSameSiteNone:
		return "None"
	default:
		return ""
	}
}

// buildSnapshotJS generates JavaScript that produces a text snapshot of the page.
func buildSnapshotJS(opts SnapshotOpts) string {
	maxDepth := -1
	if opts.Depth != nil {
		maxDepth = *opts.Depth
	}
	scope := opts.Scope
	if scope == "" {
		scope = "body"
	}

	return fmt.Sprintf(`(function() {
  var maxDepth = %d;
  var interactive = %t;
  var compact = %t;
  var scopeSel = %q;

  var interactiveTags = new Set(["A","BUTTON","INPUT","SELECT","TEXTAREA","DETAILS","SUMMARY"]);
  var lines = [];

  function walk(node, depth) {
    if (maxDepth >= 0 && depth > maxDepth) return;
    if (node.nodeType === 3) {
      var t = node.textContent.trim();
      if (t && !compact || t.length > 0) {
        lines.push("  ".repeat(depth) + t);
      }
      return;
    }
    if (node.nodeType !== 1) return;
    var el = node;
    var tag = el.tagName;
    if (interactive && !interactiveTags.has(tag) && !el.querySelector("a,button,input,select,textarea")) {
      if (compact) return;
    }
    var label = tag.toLowerCase();
    if (el.id) label += "#" + el.id;
    if (el.getAttribute("role")) label += "[role=" + el.getAttribute("role") + "]";
    if (tag === "A" && el.href) label += ' href="' + el.getAttribute("href") + '"';
    if (tag === "INPUT") label += ' type="' + (el.type||"text") + '"';
    if (el.getAttribute("aria-label")) label += ' aria-label="' + el.getAttribute("aria-label") + '"';
    lines.push("  ".repeat(depth) + "- " + label);
    for (var i = 0; i < el.childNodes.length; i++) {
      walk(el.childNodes[i], depth + 1);
    }
  }

  var root = document.querySelector(scopeSel) || document.body;
  walk(root, 0);
  return lines.join("\n");
})()`, maxDepth, opts.Interactive, opts.Compact, scope)
}
