package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

const (
	maxResponseBytes = 10 * 1024 * 1024 // 10MB
	maxMarkdownBytes = 100 * 1024       // 100KB
	fetchTimeout     = 60 * time.Second
)

// globalCache is the shared fetch result cache (15min TTL, 50MB max).
var globalCache = newFetchCache(cacheMaxBytes, cacheTTL)

type toolInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// Tool implements the WebFetch tool — fetch a URL and return its content as markdown.
//
// Transport, when non-nil, is used as the underlying RoundTripper. This lets
// tests inject a transport that trusts a test server's self-signed certificate
// without disabling the cross-host redirect policy. Production callers leave it nil.
//
// BlockedDomains is an optional list of host patterns to reject before fetching.
// Each entry matches as an exact hostname or as a suffix (e.g. "evil.com" blocks
// "evil.com" and "sub.evil.com"). Empty by default — no domains are blocked.
//
// SSRFBypass, when true, skips the SSRF private-IP check. This is only used
// in tests where the test server necessarily listens on 127.0.0.1.
type Tool struct {
	Transport      http.RoundTripper
	BlockedDomains []string
	SSRFBypass     bool
}

func (t *Tool) Name() string { return "WebFetch" }

func (t *Tool) Description() string {
	return "Fetches a URL and returns its content as markdown. " +
		"Best for one-shot retrieval of a known page. Use Browser for multi-step interaction and WebSearch for discovery. " +
		"Preapproved documentation hosts bypass the permission prompt."
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch (http or https)"
			},
			"prompt": {
				"type": "string",
				"description": "What information to extract or summarize from the page"
			}
		},
		"required": ["url", "prompt"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.URL) == "" {
		return fmt.Errorf("url is required and cannot be empty")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}
	u, err := url.Parse(in.URL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must use http or https scheme, got %q", u.Scheme)
	}
	return nil
}

// CheckPermissions returns PermAllow for preapproved hosts, PermAsk otherwise.
func (t *Tool) CheckPermissions(input json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}

	// Normalise to https for the permission check
	rawURL := upgradeHTTP(in.URL)

	u, err := url.Parse(rawURL)
	if err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid url"}, nil
	}

	if isPreapprovedHost(u.Hostname(), u.Path) {
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}

	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  fmt.Sprintf("Allow WebFetch to access %s?", u.Host),
	}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	fetchURL := upgradeHTTP(in.URL)

	// Domain blocklist check
	if err := t.checkBlocklist(fetchURL); err != nil {
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Cache hit
	if cached, ok := globalCache.get(fetchURL); ok {
		return &models.ToolResult{Content: cached}, nil
	}

	// Fetch with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	// Wrap the transport with SSRF protection unless bypassed (tests only).
	transport := t.Transport
	if !t.SSRFBypass {
		transport = ssrfSafeTransport(transport)
	}

	// Always apply the cross-host redirect policy. The injected Transport (if any)
	// controls TLS/connection behaviour (e.g. trusting test server certs), while the
	// CheckRedirect function enforces our security policy regardless.
	client := &http.Client{
		Timeout:   fetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			// Block credentials embedded in the redirect target URL.
			if req.URL.User != nil {
				return fmt.Errorf("redirect to URL with credentials blocked: %s", req.URL.Host)
			}
			// Block protocol-downgrade redirects (https → http).
			if via[0].URL.Scheme != req.URL.Scheme {
				return fmt.Errorf("protocol-downgrade redirect blocked: %s → %s", via[0].URL.Scheme, req.URL.Scheme)
			}
			// Block redirects to a different host:port. URL.Host includes the port
			// when explicitly present (e.g. "127.0.0.1:8443"), so a direct string
			// comparison correctly rejects cross-port redirects on the same IP.
			if via[0].URL.Host != req.URL.Host {
				return fmt.Errorf("cross-host redirect blocked: %s → %s", via[0].URL.Host, req.URL.Host)
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	markdown, err := fetchAndConvert(fetchCtx, fetchURL, client)
	if err != nil {
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Truncate to max markdown size
	if len(markdown) > maxMarkdownBytes {
		markdown = markdown[:maxMarkdownBytes]
	}

	// Wrap in untrusted-content markers to prevent prompt injection
	wrapped := wrapContentUntrusted(markdown, fetchURL)

	globalCache.set(fetchURL, wrapped)
	return &models.ToolResult{Content: wrapped}, nil
}

// checkBlocklist returns an error if the URL's host matches any entry in BlockedDomains.
// Matching is case-insensitive: "evil.com" blocks "evil.com" and "sub.evil.com".
func (t *Tool) checkBlocklist(rawURL string) error {
	if len(t.BlockedDomains) == 0 {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil // already validated upstream; don't double-error
	}
	host := strings.ToLower(u.Hostname())
	for _, pattern := range t.BlockedDomains {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		if host == pat || strings.HasSuffix(host, "."+pat) {
			return fmt.Errorf("domain blocked: %s", host)
		}
	}
	return nil
}

// upgradeHTTP converts http:// to https://.
func upgradeHTTP(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") {
		return "https://" + rawURL[7:]
	}
	return rawURL
}

// fetchAndConvert fetches rawURL and converts HTML to markdown.
// Returns plain text for non-HTML content types.
func fetchAndConvert(ctx context.Context, rawURL string, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; forge/1.0; +https://github.com/egoisutolabs/forge)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if isHTMLContentType(contentType) {
		converter := md.NewConverter("", true, nil)
		converted, convErr := converter.ConvertString(string(body))
		if convErr != nil {
			// Fall back to raw text on conversion failure
			return string(body), nil
		}
		return converted, nil
	}

	return string(body), nil
}

// isHTMLContentType reports whether the Content-Type header indicates HTML.
func isHTMLContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "application/xhtml")
}
