package webfetch

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- isPrivateIP ----

func TestIsPrivateIP_Loopback(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"::1", true},
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", tc.ip)
		}
		if got := isPrivateIP(ip); got != tc.want {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsPrivateIP_RFC1918(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		// 10.0.0.0/8
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		// 172.16.0.0/12
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.255.255", false}, // just outside range
		{"172.32.0.0", false},     // just outside range
		// 192.168.0.0/16
		{"192.168.1.1", true},
		{"192.168.0.0", true},
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", tc.ip)
		}
		if got := isPrivateIP(ip); got != tc.want {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsPrivateIP_IPv6ULA(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"fc00::1", true},
		{"fd00::1", true},
		{"fe80::1", true}, // link-local
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", tc.ip)
		}
		if got := isPrivateIP(ip); got != tc.want {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestIsPrivateIP_LinkLocal(t *testing.T) {
	ip := net.ParseIP("169.254.1.1")
	if !isPrivateIP(ip) {
		t.Error("169.254.1.1 should be link-local (private)")
	}
}

func TestIsPrivateIP_Unspecified(t *testing.T) {
	tests := []string{"0.0.0.0", "::"}
	for _, tc := range tests {
		ip := net.ParseIP(tc)
		if !isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%s) should be true (unspecified)", tc)
		}
	}
}

func TestIsPrivateIP_PublicAddresses(t *testing.T) {
	tests := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",
		"2606:4700::6810:85e5", // Cloudflare
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", tc)
		}
		if isPrivateIP(ip) {
			t.Errorf("isPrivateIP(%s) = true, want false (public)", tc)
		}
	}
}

// ---- wrapContentUntrusted ----

func TestWrapContentUntrusted(t *testing.T) {
	content := "# Hello World"
	url := "https://example.com/page"
	wrapped := wrapContentUntrusted(content, url)

	if !strings.Contains(wrapped, "untrusted") {
		t.Error("wrapped content should contain 'untrusted'")
	}
	if !strings.Contains(wrapped, "<fetched-content") {
		t.Error("wrapped content should contain <fetched-content> tag")
	}
	if !strings.Contains(wrapped, "url=\"https://example.com/page\"") {
		t.Error("wrapped content should contain the fetch URL")
	}
	if !strings.Contains(wrapped, "trust=\"untrusted\"") {
		t.Error("wrapped content should have trust=untrusted attribute")
	}
	if !strings.Contains(wrapped, "# Hello World") {
		t.Error("wrapped content should preserve the original content")
	}
	if !strings.Contains(wrapped, "should be treated as untrusted") {
		t.Error("wrapped content should contain untrusted header comment")
	}
	if !strings.Contains(wrapped, "</fetched-content>") {
		t.Error("wrapped content should have closing tag")
	}
}

// ---- SSRF protection in Execute ----

func TestExecute_PrivateIPBlocked(t *testing.T) {
	resetCache(t)
	tool := newTool()

	privateIPs := []string{
		"https://127.0.0.1/page",
		"https://10.0.0.1/page",
		"https://192.168.1.1/page",
		"https://[::1]/page",
		"https://[fc00::1]/page",
	}

	for _, u := range privateIPs {
		input, _ := json.Marshal(map[string]string{"url": u, "prompt": "x"})
		result, err := tool.Execute(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected Go error for %s: %v", u, err)
		}
		if !result.IsError {
			t.Errorf("expected IsError=true for private IP %s", u)
		}
		if !strings.Contains(strings.ToLower(result.Content), "ssrf") &&
			!strings.Contains(strings.ToLower(result.Content), "private") &&
			!strings.Contains(strings.ToLower(result.Content), "blocked") {
			t.Errorf("expected SSRF/private/blocked error for %s, got: %s", u, result.Content)
		}
	}
}

func TestExecute_PublicIPAllowed(t *testing.T) {
	// Use a real TLS test server (which listens on 127.0.0.1) but override
	// the SSRF check via the transport to simulate a public IP scenario.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("public content"))
	}))
	defer srv.Close()
	resetCache(t)

	// Use the test server's transport directly (bypasses SSRF for test server)
	// but with SSRFBypass flag to simulate that the IP was checked and passed.
	tool := &Tool{Transport: srv.Client().Transport, SSRFBypass: true}
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected public IP to be allowed, got error: %s", result.Content)
	}
}

func TestExecute_RedirectToPrivateIPBlocked(t *testing.T) {
	// Server that redirects to a private IP address.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to localhost — should be caught by SSRF guard.
		http.Redirect(w, r, "https://127.0.0.1:9999/evil", http.StatusFound)
	}))
	defer srv.Close()
	resetCache(t)

	// The test transport will trust the test server cert, but the redirect
	// target (127.0.0.1:9999) goes through the SSRF check transport wrapper.
	tool := tlsTool(srv)
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for redirect to private IP")
	}
	// The error could be from cross-host redirect policy OR ssrf check
	if !strings.Contains(strings.ToLower(result.Content), "blocked") &&
		!strings.Contains(strings.ToLower(result.Content), "ssrf") {
		t.Errorf("expected blocked/ssrf error, got: %s", result.Content)
	}
}

// ---- Output wrapping ----

func TestExecute_OutputWrappedInUntrustedMarkers(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("some content"))
	}))
	defer srv.Close()
	resetCache(t)

	// Use SSRFBypass since test server is on 127.0.0.1
	tool := &Tool{Transport: srv.Client().Transport, SSRFBypass: true}
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "<fetched-content") {
		t.Error("output should be wrapped in <fetched-content> tags")
	}
	if !strings.Contains(result.Content, "trust=\"untrusted\"") {
		t.Error("output should have trust=untrusted attribute")
	}
	if !strings.Contains(result.Content, "</fetched-content>") {
		t.Error("output should have closing </fetched-content> tag")
	}
	if !strings.Contains(result.Content, "should be treated as untrusted") {
		t.Error("output should contain untrusted header comment")
	}
	if !strings.Contains(result.Content, "some content") {
		t.Error("output should contain the original content")
	}
}

func TestExecute_CachedOutputAlsoWrapped(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("cached stuff"))
	}))
	defer srv.Close()
	resetCache(t)

	tool := &Tool{Transport: srv.Client().Transport, SSRFBypass: true}
	input, _ := json.Marshal(map[string]string{"url": srv.URL, "prompt": "x"})

	// First fetch populates cache
	result1, _ := tool.Execute(context.Background(), input, nil)
	if !strings.Contains(result1.Content, "<fetched-content") {
		t.Error("first fetch should be wrapped")
	}

	// Second fetch hits cache — should still be wrapped (wrapping is stored in cache)
	result2, _ := tool.Execute(context.Background(), input, nil)
	if !strings.Contains(result2.Content, "<fetched-content") {
		t.Error("cached fetch should also be wrapped in untrusted markers")
	}
}
