package tui

import (
	"strings"
	"testing"
)

func TestDetectURLs_Simple(t *testing.T) {
	text := "Check out https://example.com for more info."
	urls := DetectURLs(text)
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if urls[0] != "https://example.com" {
		t.Fatalf("expected 'https://example.com', got %q", urls[0])
	}
}

func TestDetectURLs_Multiple(t *testing.T) {
	text := "Visit https://example.com and http://test.org/path?q=1 for details."
	urls := DetectURLs(text)
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
	if urls[0] != "https://example.com" {
		t.Fatalf("expected 'https://example.com', got %q", urls[0])
	}
	if urls[1] != "http://test.org/path?q=1" {
		t.Fatalf("expected 'http://test.org/path?q=1', got %q", urls[1])
	}
}

func TestDetectURLs_None(t *testing.T) {
	text := "No links here, just text."
	urls := DetectURLs(text)
	if len(urls) != 0 {
		t.Fatalf("expected 0 URLs, got %d", len(urls))
	}
}

func TestDetectURLs_WithTrailingPunctuation(t *testing.T) {
	text := "See https://example.com/page."
	urls := DetectURLs(text)
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	// Trailing period should not be part of URL
	if strings.HasSuffix(urls[0], ".") {
		t.Fatalf("URL should not end with period: %q", urls[0])
	}
}

func TestDetectURLs_ComplexPath(t *testing.T) {
	text := "Link: https://github.com/user/repo/blob/main/file.go#L10-L20"
	urls := DetectURLs(text)
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if urls[0] != "https://github.com/user/repo/blob/main/file.go#L10-L20" {
		t.Fatalf("expected complex URL, got %q", urls[0])
	}
}

func TestDetectURLs_Empty(t *testing.T) {
	urls := DetectURLs("")
	if len(urls) != 0 {
		t.Fatalf("expected 0 URLs for empty string, got %d", len(urls))
	}
}

func TestRenderHyperlinks_NoURLs(t *testing.T) {
	text := "Just plain text here."
	got := RenderHyperlinks(text)
	if got != text {
		t.Fatalf("expected unchanged text, got %q", got)
	}
}

func TestRenderHyperlinks_Empty(t *testing.T) {
	got := RenderHyperlinks("")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestRenderHyperlinks_WithURL(t *testing.T) {
	// Set TERM_PROGRAM so supportsHyperlinks() returns true and OSC 8 codes are emitted
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	text := "Visit https://example.com for info."
	got := RenderHyperlinks(text)
	// Should contain the URL (either as OSC 8 or styled text)
	if !strings.Contains(got, "example.com") {
		t.Fatal("expected URL text preserved in output")
	}
	// Should differ from input (OSC 8 escape sequences added)
	if got == text {
		t.Fatal("expected hyperlink rendering to modify the output")
	}
}

func TestRenderHyperlinks_PreserveSurrounding(t *testing.T) {
	text := "Before https://example.com after"
	got := RenderHyperlinks(text)
	stripped := stripANSI(got)
	// Remove OSC 8 sequences for comparison
	stripped = stripOSC8(stripped)
	if !strings.Contains(stripped, "Before") {
		t.Fatal("expected 'Before' preserved")
	}
	if !strings.Contains(stripped, "after") {
		t.Fatal("expected 'after' preserved")
	}
}

func TestOSC8Link(t *testing.T) {
	link := osc8Link("https://example.com", "Example")
	if !strings.Contains(link, "\x1b]8;;https://example.com\x1b\\") {
		t.Fatal("expected OSC 8 opening sequence")
	}
	if !strings.Contains(link, "Example") {
		t.Fatal("expected display text")
	}
	if !strings.HasSuffix(link, "\x1b]8;;\x1b\\") {
		t.Fatal("expected OSC 8 closing sequence")
	}
}

func TestSupportsHyperlinks_ReturnsBoolean(t *testing.T) {
	// Just verify it doesn't panic and returns a bool
	_ = supportsHyperlinks()
}

// stripOSC8 removes OSC 8 hyperlink sequences from a string.
func stripOSC8(s string) string {
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if i+3 < len(s) && s[i] == '\x1b' && s[i+1] == ']' && s[i+2] == '8' {
			// Skip to ST (string terminator: ESC \)
			j := i + 3
			for j < len(s)-1 {
				if s[j] == '\x1b' && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
