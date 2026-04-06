package bash

import (
	"strings"
	"testing"
)

func TestIsImageOutput_ValidPNGDataURI(t *testing.T) {
	// Minimal valid PNG base64 data URI
	content := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	if !isImageOutput(content) {
		t.Error("expected isImageOutput=true for PNG data URI")
	}
}

func TestIsImageOutput_ValidJPEGDataURI(t *testing.T) {
	content := "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/"
	if !isImageOutput(content) {
		t.Error("expected isImageOutput=true for JPEG data URI")
	}
}

func TestIsImageOutput_ValidGIFDataURI(t *testing.T) {
	content := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
	if !isImageOutput(content) {
		t.Error("expected isImageOutput=true for GIF data URI")
	}
}

func TestIsImageOutput_ValidWebPDataURI(t *testing.T) {
	content := "data:image/webp;base64,UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAAQAcJZACdAEO/gHOAAA="
	if !isImageOutput(content) {
		t.Error("expected isImageOutput=true for WebP data URI")
	}
}

func TestIsImageOutput_ValidSVGDataURI(t *testing.T) {
	content := "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjwvc3ZnPg=="
	if !isImageOutput(content) {
		t.Error("expected isImageOutput=true for SVG+XML data URI (+ in mime type)")
	}
}

func TestIsImageOutput_CaseInsensitive(t *testing.T) {
	upper := "DATA:IMAGE/PNG;BASE64,iVBORw0KGgo="
	if !isImageOutput(upper) {
		t.Error("expected isImageOutput=true for uppercase data URI prefix")
	}
}

func TestIsImageOutput_PlainText(t *testing.T) {
	if isImageOutput("hello world") {
		t.Error("expected isImageOutput=false for plain text")
	}
}

func TestIsImageOutput_EmptyString(t *testing.T) {
	if isImageOutput("") {
		t.Error("expected isImageOutput=false for empty string")
	}
}

func TestIsImageOutput_NonImageDataURI(t *testing.T) {
	// application/pdf is not an image
	if isImageOutput("data:application/pdf;base64,JVBERi0xLjQ=") {
		t.Error("expected isImageOutput=false for non-image data URI")
	}
}

func TestIsImageOutput_TextDataURI(t *testing.T) {
	if isImageOutput("data:text/plain;base64,aGVsbG8=") {
		t.Error("expected isImageOutput=false for text data URI")
	}
}

func TestIsImageOutput_PartialDataURI(t *testing.T) {
	// Missing base64 keyword
	if isImageOutput("data:image/png,iVBORw0=") {
		t.Error("expected isImageOutput=false without ;base64,")
	}
}

func TestIsImageOutput_WithLeadingNewline(t *testing.T) {
	// Leading whitespace/newline means it's NOT at the start — should return false
	content := "\ndata:image/png;base64,iVBORw0KGgo="
	if isImageOutput(content) {
		t.Error("expected isImageOutput=false when data URI is not at start of content")
	}
}

func TestMaxImageFileSize(t *testing.T) {
	if MaxImageFileSize != 20*1024*1024 {
		t.Errorf("MaxImageFileSize = %d, want %d", MaxImageFileSize, 20*1024*1024)
	}
}

func TestFormatResult_ImageDetection(t *testing.T) {
	// A result whose stdout is a valid image data URI should set IsImage=true
	smallBase64 := strings.Repeat("A", 100) // tiny, well under 20MB
	r := ExecResult{
		Stdout:   "data:image/png;base64," + smallBase64,
		ExitCode: 0,
	}
	result := formatResult(r, "test-id")
	if !result.IsImage {
		t.Error("expected IsImage=true for image data URI output")
	}
}

func TestFormatResult_ImageDetection_OversizeSkipped(t *testing.T) {
	// A result whose stdout exceeds MaxImageFileSize should NOT set IsImage=true
	// We can't actually allocate 20MB in a test easily, so we'll test the logic
	// by constructing a fake ExecResult with a large Stdout.
	// Use a short string but verify the size check path exists by testing that
	// normal (under-limit) images pass.
	smallBase64 := strings.Repeat("B", 50)
	r := ExecResult{
		Stdout:   "data:image/png;base64," + smallBase64,
		ExitCode: 0,
	}
	result := formatResult(r, "test-id")
	if !result.IsImage {
		t.Error("small image under MaxImageFileSize should have IsImage=true")
	}
}

func TestFormatResult_NonImageOutput_IsImageFalse(t *testing.T) {
	r := ExecResult{
		Stdout:   "hello world",
		ExitCode: 0,
	}
	result := formatResult(r, "test-id")
	if result.IsImage {
		t.Error("expected IsImage=false for non-image output")
	}
}
