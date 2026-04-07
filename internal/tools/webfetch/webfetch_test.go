package webfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
)

// newTool returns a fresh Tool without an injected transport (uses default).
func newTool() *Tool { return &Tool{} }

// tlsTool returns a Tool wired to trust the given TLS test server's certificate.
// httptest.NewTLSServer gives https:// URLs; srv.Client().Transport trusts the cert.
// SSRFBypass is set because test servers listen on 127.0.0.1.
func tlsTool(srv *httptest.Server) *Tool {
	return &Tool{Transport: srv.Client().Transport, SSRFBypass: true}
}

// resetCache replaces the global cache with a fresh one and restores it after the test.
func resetCache(t *testing.T) {
	t.Helper()
	old := globalCache
	globalCache = newFetchCache(cacheMaxBytes, cacheTTL)
	t.Cleanup(func() { globalCache = old })
}

// ---- ValidateInput ----

func TestValidateInput_Valid(t *testing.T) {
	input := json.RawMessage(`{"url":"https://example.com","prompt":"summarize"}`)
	if err := newTool().ValidateInput(input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInput_MissingURL(t *testing.T) {
	input := json.RawMessage(`{"prompt":"summarize"}`)
	if err := newTool().ValidateInput(input); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestValidateInput_MissingPrompt(t *testing.T) {
	input := json.RawMessage(`{"url":"https://example.com"}`)
	if err := newTool().ValidateInput(input); err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestValidateInput_NonHTTPScheme(t *testing.T) {
	input := json.RawMessage(`{"url":"ftp://example.com","prompt":"x"}`)
	if err := newTool().ValidateInput(input); err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestValidateInput_HTTPAllowed(t *testing.T) {
	input := json.RawMessage(`{"url":"http://example.com","prompt":"x"}`)
	if err := newTool().ValidateInput(input); err != nil {
		t.Fatalf("http scheme should be valid input: %v", err)
	}
}

// ---- CheckPermissions ----

func TestCheckPermissions_PreapprovedHost(t *testing.T) {
	input := json.RawMessage(`{"url":"https://go.dev/doc","prompt":"x"}`)
	dec, err := newTool().CheckPermissions(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Errorf("got %q, want allow", dec.Behavior)
	}
}

func TestCheckPermissions_HTTPUpgradedForPreapproval(t *testing.T) {
	// http:// should be upgraded to https:// before checking the preapproved list.
	input := json.RawMessage(`{"url":"http://go.dev/doc","prompt":"x"}`)
	dec, err := newTool().CheckPermissions(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAllow {
		t.Errorf("got %q, want allow (preapproved after http upgrade)", dec.Behavior)
	}
}

func TestCheckPermissions_UnknownHost(t *testing.T) {
	input := json.RawMessage(`{"url":"https://unknown-host.example.com","prompt":"x"}`)
	dec, err := newTool().CheckPermissions(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAsk {
		t.Errorf("got %q, want ask", dec.Behavior)
	}
}

// ---- Interface invariants ----

func TestInterfaceInvariants(t *testing.T) {
	tool := newTool()
	input := json.RawMessage(`{"url":"https://example.com","prompt":"x"}`)
	if !tool.IsConcurrencySafe(input) {
		t.Error("WebFetch must be concurrency-safe")
	}
	if !tool.IsReadOnly(input) {
		t.Error("WebFetch must be read-only")
	}
}

func TestName(t *testing.T) {
	if newTool().Name() != "WebFetch" {
		t.Error("Name() must return WebFetch")
	}
}

// ---- Execute: HTML fetch + conversion ----

func TestExecute_HTMLFetch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Hello</h1><p>World</p></body></html>`))
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "summarize"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Hello") {
		t.Errorf("markdown should contain 'Hello', got: %s", result.Content)
	}
}

func TestExecute_PlainText(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text content"))
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "read"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "plain text content") {
		t.Errorf("expected content to contain 'plain text content', got %q", result.Content)
	}
}

func TestExecute_HTTPStatusError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for 404")
	}
	if !strings.Contains(result.Content, "404") {
		t.Errorf("expected 404 in error, got: %s", result.Content)
	}
}

func TestExecute_Cache(t *testing.T) {
	callCount := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("cached content"))
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})

	tool.Execute(context.Background(), input, nil)
	tool.Execute(context.Background(), input, nil)

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (second should be cached), got %d", callCount)
	}
}

func TestExecute_CrossHostRedirectBlocked(t *testing.T) {
	// Target server that the redirect points to
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("target"))
	}))
	defer target.Close()

	// Origin server that redirects cross-host (to a different port = different host:port)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/page", http.StatusFound)
	}))
	defer origin.Close()
	resetCache(t)

	// Inject origin's transport so we can connect to it; Execute always applies
	// the cross-host redirect policy regardless of the injected transport.
	// SSRFBypass=true because test servers listen on 127.0.0.1.
	tool := &Tool{Transport: origin.Client().Transport, SSRFBypass: true}
	input, _ := json.Marshal(map[string]string{"url": origin.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for cross-host redirect")
	}
	if !strings.Contains(result.Content, "cross-host redirect blocked") {
		t.Errorf("expected cross-host redirect error, got: %s", result.Content)
	}
}

func TestExecute_MarkdownTruncation(t *testing.T) {
	large := strings.Repeat("x", maxMarkdownBytes+1000)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(large))
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	// Content is now wrapped in untrusted markers, so total length exceeds
	// maxMarkdownBytes. But the inner content (before wrapping) was truncated.
	// The wrapper adds fixed-size text around the payload, so verify the
	// overall length is bounded by maxMarkdownBytes + wrapper overhead.
	wrapperOverhead := len(wrapContentUntrusted("", srv.URL))
	if len(result.Content) > maxMarkdownBytes+wrapperOverhead {
		t.Errorf("content not properly truncated: len=%d, maxInner=%d, wrapperOverhead=%d",
			len(result.Content), maxMarkdownBytes, wrapperOverhead)
	}
}

func TestExecute_HTTPUpgradeStoresCacheAsHTTPS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	resetCache(t)

	httpsURL := srv.URL              // already https://
	httpURL := "http" + httpsURL[5:] // strip "https" → "http"

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": httpURL, "prompt": "x"})
	tool.Execute(context.Background(), input, nil)

	// The cache key should be the https:// form after upgradeHTTP.
	if _, ok := globalCache.get(httpsURL); !ok {
		t.Error("expected cache entry stored under https:// key after http:// fetch")
	}
}

// ---- upgradeHTTP ----

func TestUpgradeHTTP(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://example.com", "https://example.com"},
		{"https://example.com", "https://example.com"},
		{"http://go.dev/doc", "https://go.dev/doc"},
	}
	for _, tc := range tests {
		got := upgradeHTTP(tc.in)
		if got != tc.want {
			t.Errorf("upgradeHTTP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---- isHTMLContentType ----

func TestIsHTMLContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/html; charset=utf-8", true},
		{"text/html", true},
		{"application/xhtml+xml", true},
		{"text/plain", false},
		{"application/json", false},
		{"", false},
	}
	for _, tc := range tests {
		got := isHTMLContentType(tc.ct)
		if got != tc.want {
			t.Errorf("isHTMLContentType(%q) = %v, want %v", tc.ct, got, tc.want)
		}
	}
}

func TestInputSchema_ValidJSON(t *testing.T) {
	schema := newTool().InputSchema()
	var v map[string]any
	if err := json.Unmarshal(schema, &v); err != nil {
		t.Fatalf("InputSchema returned invalid JSON: %v", err)
	}
}

// ---- Domain blocklist ----

func TestCheckBlocklist_Empty(t *testing.T) {
	tool := newTool() // BlockedDomains is nil
	if err := tool.checkBlocklist("https://evil.com/page"); err != nil {
		t.Fatalf("empty blocklist should allow all domains, got: %v", err)
	}
}

func TestCheckBlocklist_ExactMatch(t *testing.T) {
	tool := &Tool{BlockedDomains: []string{"evil.com"}}
	if err := tool.checkBlocklist("https://evil.com/page"); err == nil {
		t.Fatal("expected error for exact blocked domain")
	}
}

func TestCheckBlocklist_SubdomainMatch(t *testing.T) {
	tool := &Tool{BlockedDomains: []string{"evil.com"}}
	if err := tool.checkBlocklist("https://sub.evil.com/page"); err == nil {
		t.Fatal("expected error for subdomain of blocked domain")
	}
}

func TestCheckBlocklist_NotBlocked(t *testing.T) {
	tool := &Tool{BlockedDomains: []string{"evil.com"}}
	if err := tool.checkBlocklist("https://notevil.com/page"); err != nil {
		t.Fatalf("unrelated domain should not be blocked, got: %v", err)
	}
}

func TestCheckBlocklist_CaseInsensitive(t *testing.T) {
	tool := &Tool{BlockedDomains: []string{"EVIL.COM"}}
	if err := tool.checkBlocklist("https://evil.com/page"); err == nil {
		t.Fatal("blocklist matching should be case-insensitive")
	}
}

func TestExecute_BlockedDomainReturnsError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("should not reach"))
	}))
	defer srv.Close()
	resetCache(t)

	// Extract hostname from test server URL for blocklist.
	u, _ := url.Parse(srv.URL)
	tool := &Tool{
		Transport:      srv.Client().Transport,
		BlockedDomains: []string{u.Hostname()},
		SSRFBypass:     true,
	}
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for blocked domain")
	}
	if !strings.Contains(result.Content, "domain blocked") {
		t.Errorf("expected 'domain blocked' in error, got: %s", result.Content)
	}
}

// ---- Redirect security ----

func TestExecute_CredentialRedirectBlocked(t *testing.T) {
	// Server redirects to a URL with user:pass@ credentials embedded.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to a URL that includes credentials in the userinfo component.
		http.Redirect(w, r, "https://user:pass@127.0.0.1:9999/secret", http.StatusFound)
	}))
	defer srv.Close()
	resetCache(t)

	// SSRFBypass needed since origin server is on 127.0.0.1.
	tool := tlsTool(srv) // tlsTool sets SSRFBypass=true
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for credential redirect")
	}
	if !strings.Contains(result.Content, "credentials") {
		t.Errorf("expected 'credentials' in error message, got: %s", result.Content)
	}
}

func TestExecute_ProtocolDowngradeBlocked(t *testing.T) {
	// HTTPS server that redirects to an HTTP URL (protocol downgrade).
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to plain http — protocol downgrade.
		http.Redirect(w, r, "http://127.0.0.1:9998/page", http.StatusFound)
	}))
	defer srv.Close()
	resetCache(t)

	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for protocol downgrade redirect")
	}
	if !strings.Contains(result.Content, "blocked") {
		t.Errorf("expected 'blocked' in error message, got: %s", result.Content)
	}
}
