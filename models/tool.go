package models

// ToolResult is the output of a tool execution.
type ToolResult struct {
	Content                  string // text content returned to Claude
	IsError                  bool   // whether this represents an error
	IsImage                  bool   // true if content is a base64 image data URI
	ReturnCodeInterpretation string // human-readable meaning of the exit code
}
