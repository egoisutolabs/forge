package bash

import "fmt"

// interpretExitCode returns a human-readable description of a shell exit code.
//
// POSIX specifies that exit codes 128+N mean "killed by signal N".
// The named signal cases (SIGINT=130, SIGKILL=137, SIGTERM=143) are the most
// common in practice and match what Claude Code surfaces in BashTool.tsx.
func interpretExitCode(code int) string {
	switch code {
	case 0:
		return "success"
	case 1:
		return "general error"
	case 2:
		return "misuse of shell built-in"
	case 126:
		return "command cannot execute (permission denied or not executable)"
	case 127:
		return "command not found"
	case 130:
		return "interrupted by SIGINT (Ctrl+C)"
	case 137:
		return "killed by SIGKILL (forced termination)"
	case 143:
		return "terminated by SIGTERM"
	default:
		if code > 128 {
			signal := code - 128
			return fmt.Sprintf("killed by signal %d", signal)
		}
		return fmt.Sprintf("exited with code %d", code)
	}
}
