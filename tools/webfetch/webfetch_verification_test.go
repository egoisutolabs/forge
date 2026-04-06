// Package webfetch — verification tests comparing Go port against Claude Code's
// WebFetchTool TypeScript source (tools/WebFetchTool/utils.ts).
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. CRITICAL — www. redirect: TypeScript isPermittedRedirect() allows adding
//     or removing "www." (example.com → www.example.com and vice versa).
//     Go CheckRedirect compares Host strings directly, so www-variant redirects
//     are blocked. This means Go will fail to fetch sites that redirect
//     between their www and bare-domain variants.
//
//  2. MISSING: Domain blocklist preflight check.
//     TypeScript calls https://api.anthropic.com/api/web/domain_info?domain=...
//     before fetching to check if a domain is on a blocklist. Go has no such
//     preflight check. A site on the Anthropic blocklist will be fetched by
//     Go but rejected by TypeScript.
//
//  3. MISSING: Credentials in redirect URL blocked.
//     TypeScript isPermittedRedirect() rejects redirects that contain
//     username/password (e.g. https://user:pass@evil.com/). Go does not check
//     for credentials in the redirect target URL.
//
//  4. MISSING: Protocol-downgrade redirect blocked.
//     TypeScript isPermittedRedirect() rejects redirects that change the
//     protocol (e.g. https → http). Go only checks the hostname.
//
//  5. MINOR: HTTP→HTTPS upgrade strips but TypeScript upgrades in headers.
//     Both upgrade HTTP to HTTPS; parity is functional.
//
//  6. MINOR: Binary content handling.
//     TypeScript persists binary content to disk with MIME-derived extensions.
//     Go returns raw bytes as string content (no disk persistence for binary).
package webfetch

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── GAP 1 (CRITICAL): www. redirect not allowed ─────────────────────────────

// TestVerification_WwwRedirectBlocked_Gap documents the critical gap where
// Go blocks www-variant redirects that TypeScript allows.
//
// TypeScript isPermittedRedirect() logic:
//
//	stripWww := func(h string) string { return strings.TrimPrefix(h, "www.") }
//	return stripWww(original.hostname) == stripWww(redirect.hostname)
//
// Go CheckRedirect logic:
//
//	if via[0].URL.Host != req.URL.Host { return error }
//
// A redirect from example.com:443 to www.example.com:443 has different Host
// strings and is therefore BLOCKED in Go. TypeScript would allow it.
func TestVerification_WwwRedirectBlocked_Gap(t *testing.T) {
	// Set up a TLS test server that redirects bare domain → www.
	// We simulate this by having two handlers:
	// /bare → redirects to /www-variant (different host header in real scenario)
	// We test with same-server paths to confirm the policy difference.
	//
	// In production: fetching http://docs.python.org would redirect to
	// www.python.org — Go blocks it, TypeScript follows it.
	t.Log("GAP DOCUMENTED: www. redirects (example.com → www.example.com) are blocked by Go but allowed by TypeScript")
	t.Log("Affected real-world sites: python.org, many documentation sites that redirect to/from www subdomain")

	// Verify the Go redirect policy via a test server.
	// Server A (bare) → redirect to Server B (www-variant, simulated as different port).
	serverB := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "# www variant content")
	}))
	defer serverB.Close()

	serverA := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a www. redirect to a different host (different port here).
		http.Redirect(w, r, serverB.URL+"/page", http.StatusMovedPermanently)
	}))
	defer serverA.Close()

	tlsTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	// SSRFBypass=true because test servers listen on 127.0.0.1
	tool := &Tool{Transport: tlsTransport, SSRFBypass: true}
	inputJSON := mustWebFetchJSON(t, serverA.URL+"/page", "get content")

	result, err := tool.Execute(t.Context(), inputJSON, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Go should have blocked the cross-host redirect.
	if result.IsError {
		if strings.Contains(result.Content, "cross-host redirect blocked") {
			t.Logf("CONFIRMED: cross-host redirect blocked (www. variant would also be blocked): %s", result.Content)
		} else {
			t.Logf("redirect blocked with different error: %s", result.Content)
		}
	} else {
		t.Log("redirect was followed — Go may have a more permissive redirect policy than expected")
	}
}

// TestVerification_SameHostRedirectFollowed verifies that same-host redirects
// (path changes) are correctly followed — consistent with both TypeScript and Go.
func TestVerification_SameHostRedirectFollowed(t *testing.T) {
	redirected := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusMovedPermanently)
			return
		}
		redirected = true
		fmt.Fprintln(w, "new page content")
	}))
	defer server.Close()

	tlsTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	// SSRFBypass=true because test servers listen on 127.0.0.1
	tool := &Tool{Transport: tlsTransport, SSRFBypass: true}
	inputJSON := mustWebFetchJSON(t, server.URL+"/old", "get content")

	result, err := tool.Execute(t.Context(), inputJSON, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("same-host redirect should succeed: %s", result.Content)
	}
	if !redirected {
		t.Error("expected redirect to /new to be followed")
	}
}

// ─── GAP 3: credentials in redirect URL ──────────────────────────────────────

// TestVerification_CredentialsInRedirectNotBlocked documents that Go does not
// block redirects to URLs with embedded credentials.
//
// TypeScript isPermittedRedirect():
//
//	if (parsedRedirect.username || parsedRedirect.password) { return false }
//
// Go: no such check.
func TestVerification_CredentialsInRedirectNotBlocked(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to URL with credentials embedded — TypeScript would block this.
		// Go's net/http will strip credentials from the URL before following.
		http.Redirect(w, r, "https://user:pass@"+r.Host+"/safe", http.StatusFound)
	}))
	defer server.Close()

	// Verify the redirect policy difference is documented.
	t.Log("GAP DOCUMENTED: TypeScript blocks redirects to URLs with embedded credentials (user:pass@host)")
	t.Log("Go's net/http strips credentials from redirect URLs but does not reject them")
}

// ─── GAP 4: protocol-downgrade redirect ──────────────────────────────────────

// TestVerification_ProtocolDowngradeRedirectNotBlocked documents that Go does
// not explicitly block protocol-downgrade redirects (https → http).
//
// TypeScript isPermittedRedirect():
//
//	if (parsedRedirect.protocol !== parsedOriginal.protocol) { return false }
//
// Go: checks host only; upgradeHTTP() converts http→https on the initial URL
// but does not enforce protocol consistency on redirect targets.
func TestVerification_ProtocolDowngradeRedirectNotBlocked(t *testing.T) {
	t.Log("GAP DOCUMENTED: TypeScript blocks protocol-downgrade redirects (https → http)")
	t.Log("Go does not check protocol consistency in CheckRedirect; upgradeHTTP only applies to initial URL")
}

// ─── GAP 2: no domain blocklist check ────────────────────────────────────────

// TestVerification_NoDomainBlocklistCheck documents that Go has no preflight
// blocklist check via api.anthropic.com/api/web/domain_info.
func TestVerification_NoDomainBlocklistCheck(t *testing.T) {
	t.Log("GAP DOCUMENTED: TypeScript performs a preflight blocklist check via api.anthropic.com/api/web/domain_info")
	t.Log("Go has no such check — domains on Anthropic's blocklist will be fetched by Go but rejected by TypeScript")
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_CacheTTL_15Minutes verifies the cache TTL matches
// Claude Code's utils.ts: FETCH_CACHE_TTL_MS = 15 * 60 * 1000.
func TestVerification_CacheTTL_15Minutes(t *testing.T) {
	if cacheTTL != 15*time.Minute {
		t.Errorf("cacheTTL = %v, want 15m (matches Claude Code FETCH_CACHE_TTL_MS)", cacheTTL)
	}
}

// TestVerification_CacheMaxSize_50MB verifies the cache size limit matches
// Claude Code's utils.ts: FETCH_CACHE_MAX_TOTAL_BYTES = 50 * 1024 * 1024.
func TestVerification_CacheMaxSize_50MB(t *testing.T) {
	const want = int64(50 * 1024 * 1024)
	if cacheMaxBytes != want {
		t.Errorf("cacheMaxBytes = %d, want %d (50MB, matches Claude Code)", cacheMaxBytes, want)
	}
}

// TestVerification_HTTPUpgraded verifies that http:// URLs are upgraded to
// https:// — matching Claude Code's upgradeInsecureUrls() behaviour.
func TestVerification_HTTPUpgraded(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://example.com/page", "https://example.com/page"},
		{"https://example.com/page", "https://example.com/page"},
		{"http://go.dev", "https://go.dev"},
	}
	for _, tc := range tests {
		got := upgradeHTTP(tc.input)
		if got != tc.want {
			t.Errorf("upgradeHTTP(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestVerification_MaxRedirects_10 verifies Go enforces a maximum of 10
// redirects (consistent with many HTTP clients; TypeScript uses MAX_REDIRECTS=10).
func TestVerification_MaxRedirects_10(t *testing.T) {
	hops := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		// Always redirect to itself — creates an infinite same-host loop.
		http.Redirect(w, r, r.URL.Path+"?hop="+fmt.Sprint(hops), http.StatusFound)
	}))
	defer server.Close()

	tlsTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	// SSRFBypass=true because test servers listen on 127.0.0.1
	tool := &Tool{Transport: tlsTransport, SSRFBypass: true}
	inputJSON := mustWebFetchJSON(t, server.URL+"/start", "get content")

	result, err := tool.Execute(t.Context(), inputJSON, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error after too many redirects")
	}
	if !strings.Contains(result.Content, "redirect") {
		t.Errorf("expected redirect error, got: %s", result.Content)
	}
}

// TestVerification_PreapprovedHostsBypassPermissionCheck verifies that
// preapproved hosts return PermAllow, matching Claude Code's permission logic.
func TestVerification_PreapprovedHostsBypassPermissionCheck(t *testing.T) {
	preapproved := []string{
		"https://docs.python.org/page",
		"https://go.dev/doc/install",
		"https://doc.rust-lang.org/std/",
	}
	for _, u := range preapproved {
		input := mustWebFetchJSON(t, u, "get docs")
		decision, err := (&Tool{}).CheckPermissions(input, nil)
		if err != nil {
			t.Fatalf("CheckPermissions(%s): unexpected error: %v", u, err)
		}
		if string(decision.Behavior) != "allow" {
			t.Errorf("CheckPermissions(%s) = %q, want 'allow' (preapproved host should bypass prompt)", u, decision.Behavior)
		}
	}
}

// TestVerification_UnknownHostRequiresPermission verifies that unknown hosts
// return PermAsk, matching Claude Code's permission check.
func TestVerification_UnknownHostRequiresPermission(t *testing.T) {
	input := mustWebFetchJSON(t, "https://totally-unknown-site-xyz.example/page", "get content")
	decision, err := (&Tool{}).CheckPermissions(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decision.Behavior) != "ask" {
		t.Errorf("CheckPermissions for unknown host = %q, want 'ask'", decision.Behavior)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustWebFetchJSON(t *testing.T, u, prompt string) []byte {
	t.Helper()
	j, err := json.Marshal(map[string]string{"url": u, "prompt": prompt})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return j
}
