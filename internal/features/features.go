// Package features provides compile-time feature gates controlled by Go build tags.
//
// Build tags:
//
//	go build ./cmd/forge                        — default: all tools, no speculation
//	go build -tags speculation ./cmd/forge       — enables speculative pre-execution
//	go build -tags debug ./cmd/forge             — enables verbose debug logging
//	go build -tags minimal ./cmd/forge           — excludes web tools, browser, astgrep
//	go build -tags "speculation,debug" ./cmd/forge — combine tags
//
// Each tag file declares all three constants to avoid redeclaration conflicts.
// The build constraint on features_default.go ensures exactly one file is active.
package features
