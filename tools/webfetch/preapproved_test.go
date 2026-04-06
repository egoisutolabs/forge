package webfetch

import "testing"

func TestIsPreapprovedHost(t *testing.T) {
	tests := []struct {
		hostname string
		pathname string
		want     bool
	}{
		// Hostname-only entries
		{"go.dev", "/doc", true},
		{"pkg.go.dev", "/github.com/foo", true},
		{"react.dev", "/learn", true},
		{"developer.mozilla.org", "/en-US/docs/Web", true},
		{"docs.python.org", "/3/library/os.html", true},
		{"kubernetes.io", "/docs/", true},
		{"docs.docker.com", "/get-started/", true},

		// Path-scoped entry: github.com/anthropics
		{"github.com", "/anthropics", true},
		{"github.com", "/anthropics/claude-code", true},
		{"github.com", "/anthropics/", true},
		// Must NOT match other github paths
		{"github.com", "/anthropics-evil/malware", false},
		{"github.com", "/torvalds/linux", false},

		// vercel.com is path-scoped to /docs
		{"vercel.com", "/docs", true},
		{"vercel.com", "/docs/concepts", true},
		{"vercel.com", "/pricing", false},

		// Non-approved host
		{"evil.com", "/", false},
		{"example.com", "/page", false},
	}

	for _, tc := range tests {
		got := isPreapprovedHost(tc.hostname, tc.pathname)
		if got != tc.want {
			t.Errorf("isPreapprovedHost(%q, %q) = %v, want %v", tc.hostname, tc.pathname, got, tc.want)
		}
	}
}
