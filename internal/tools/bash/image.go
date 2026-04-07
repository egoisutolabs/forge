package bash

import "regexp"

// MaxImageFileSize caps image data URIs at 20 MB — matching Claude Code's
// MAX_IMAGE_FILE_SIZE in utils.ts. Anything larger would exceed the API
// limit and risk OOMing if held in memory.
const MaxImageFileSize = 20 * 1024 * 1024

// imageDataURIRegex matches a data URI with an image/* media type.
// (?i) makes the match case-insensitive, matching Claude Code's /i flag.
// The mime-type segment allows dots, plus signs, underscores and hyphens
// to cover types like image/svg+xml and image/vnd.ms-photo.
var imageDataURIRegex = regexp.MustCompile(`(?i)^data:image/[a-z0-9.+_-]+;base64,`)

// isImageOutput reports whether content looks like a base64-encoded image
// data URI. Mirrors isImageOutput() in Claude Code's utils.ts.
func isImageOutput(content string) bool {
	return imageDataURIRegex.MatchString(content)
}
