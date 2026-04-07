package bash

import (
	"regexp"
	"strings"
)

// readOnlyCommands is the set of commands always safe (read-only) via regex matching.
// These don't need flag validation — any flags are safe.
// Derived from Claude Code's READONLY_COMMANDS + READONLY_COMMAND_REGEXES.
var readOnlyCommands = map[string]bool{
	// File viewing
	"cat": true, "head": true, "tail": true, "wc": true, "stat": true,
	"strings": true, "hexdump": true, "od": true, "nl": true, "tac": true,
	"rev": true, "fold": true, "file": true, "less": true, "more": true,

	// System info
	"id": true, "uname": true, "free": true, "df": true, "du": true,
	"locale": true, "groups": true, "nproc": true, "whoami": true,
	"pwd": true, "arch": true, "lsb_release": true,
	// NOTE: "hostname" is in commandAllowlist (blocks positional hostname-set arg)
	// NOTE: "date" is in commandAllowlist (blocks positional time-set arg)

	// Text processing (read-only, no output flags)
	"cut": true, "paste": true, "tr": true, "column": true, "expand": true,
	"unexpand": true, "fmt": true, "comm": true, "cmp": true, "numfmt": true,
	"md5sum": true, "sha256sum": true, "sha1sum": true, "cksum": true, "b2sum": true,
	"base64": true,

	// Comparison
	"diff": true,

	// Path utilities
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"which": true,

	// Time/misc
	"cal": true, "uptime": true, "true": true, "false": true,
	// NOTE: "date" moved to commandAllowlist to block positional time-setting arg

	// Shell builtins (read-only)
	"type": true, "test": true, "expr": true, "getconf": true, "printenv": true,
	"echo": true, "printf": true,

	// Directory listing
	"ls": true, "exa": true, "eza": true,
	// NOTE: "tree" moved to commandAllowlist to block -R (writes 00Tree.html)

	// Search (basic — grep/rg/sed go through flag-validated allowlist)
	"find": true, "ag": true, "ack": true,

	// Process viewing
	"top": true, "htop": true, "pgrep": true,
	// NOTE: "lsof" moved to commandAllowlist to block +m (creates mount supplement file)
	// NOTE: "tput" moved to commandAllowlist to block init/reset (executes terminfo programs)

	// Network (read-only)
	"ping": true, "dig": true, "nslookup": true, "host": true,

	// Help
	"man": true, "help": true, "info": true,
}

// readOnlyGitSubcommands are git subcommands that don't modify the repo
// and are safe WITHOUT flag validation (any flags are safe).
// SECURITY: "reflog" is intentionally EXCLUDED — "git reflog expire" and
// "git reflog delete" write to .git/logs/**. It is handled via commandAllowlist
// with an IsDangerous callback that blocks those subcommands.
var readOnlyGitSubcommands = map[string]bool{
	"describe":      true,
	"shortlog":      true,
	"cat-file":      true,
	"rev-list":      true,
	"name-rev":      true,
	"for-each-ref":  true,
	"count-objects": true,
	"fsck":          true,
	"verify-pack":   true,
}

// readOnlyGoSubcommands are go subcommands that don't modify files.
var readOnlyGoSubcommands = map[string]bool{
	"version": true, "env": true, "list": true, "doc": true, "vet": true, "help": true,
}

// findDangerousFlags are find flags that can execute or delete.
var findDangerousFlags = regexp.MustCompile(`(?:^|\s)(?:-delete|-exec|-execdir|-ok|-okdir|-fprint0?|-fls|-fprintf)(?:\s|$)`)

// tputDangerousCapabilities are tput capability names that execute terminfo
// initialization/reset programs (is1/is2/is3/rs1/rs2/rs3 sequences), which
// can invoke arbitrary programs from $TERMINFO.
var tputDangerousCapabilities = map[string]bool{
	"init": true, "reset": true, "isgr0": true,
}

// IsReadOnly returns true if the command is safe to auto-approve (no side effects).
//
// This is the Go port of Claude Code's checkReadOnlyConstraints() +
// isCommandReadOnly() + isCommandSafeViaFlagParsing().
func IsReadOnly(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	// Check for dangerous patterns first (redirections, command substitution)
	if hasDangerousPattern(command) {
		return false
	}

	// Split compound commands
	subcommands := splitCompoundCommand(command)

	// Security: limit analysis of very complex commands
	if len(subcommands) > maxSubcommandsForSecurityCheck {
		return false
	}

	// Git security checks on compound commands
	if commandHasGit(command) {
		// cd + git in same compound = sandbox escape risk
		if compoundCommandHasCd(command) {
			return false
		}
		// Writing to git-internal paths + git = hook injection
		if commandWritesToGitInternalPaths(command) {
			return false
		}
	}

	for _, sub := range subcommands {
		if !isSubcommandReadOnly(sub) {
			return false
		}
	}

	return true
}

func isSubcommandReadOnly(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return true
	}

	// Strip trailing 2>&1 (stderr redirect to stdout is safe)
	command = strings.TrimSuffix(strings.TrimSpace(command), "2>&1")
	command = strings.TrimSpace(command)

	// Strip safe env var prefixes
	command = stripSafeEnvVars(command)
	if command == "" {
		return true
	}

	// Strip safe wrapper commands
	command = stripSafeWrappers(command)
	if command == "" {
		return true
	}

	parts := splitCommand(command)
	if len(parts) == 0 {
		return true
	}

	base := parts[0]

	// 1. Check simple allowlist (no flag validation needed).
	//    Globs in positional args are safe for read-only commands (they just expand to filenames).
	//    But $ variable expansion is still blocked — a read-only command with $VAR
	//    could expand to anything.
	if readOnlyCommands[base] {
		// Block $ expansion even for safe commands
		if hasUnquotedDollar(command) {
			return false
		}
		// Special case: find needs dangerous flag check
		if base == "find" {
			return !findDangerousFlags.MatchString(command)
		}
		// date: block time-setting flags (-s/--set) and bare POSIX time specs
		// (numeric strings ≥6 digits like MMDDhhmm or MMDDhhmmYY).
		// Format strings starting with + are safe (date +%Y-%m-%d).
		if base == "date" {
			for _, t := range parts[1:] {
				if t == "-s" || t == "--set" || strings.HasPrefix(t, "--set=") {
					return false
				}
				if !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "+") &&
					len(t) >= 6 && isNumeric(t) {
					return false
				}
			}
		}
		// hostname: any non-flag positional argument sets the hostname.
		if base == "hostname" {
			for _, t := range parts[1:] {
				if !strings.HasPrefix(t, "-") {
					return false
				}
			}
		}
		// tput: block dangerous terminal capabilities that execute terminfo programs.
		if base == "tput" {
			for _, t := range parts[1:] {
				if tputDangerousCapabilities[t] {
					return false
				}
			}
		}
		// lsof: +m/+M writes a mount supplement file to disk.
		if base == "lsof" {
			for _, t := range parts[1:] {
				if strings.HasPrefix(t, "+m") || strings.HasPrefix(t, "+M") {
					return false
				}
			}
		}
		// tree: -R generates 00Tree.html writes to disk; --fromfile reads from a file.
		if base == "tree" {
			for _, t := range parts[1:] {
				if t == "-R" || t == "--fromfile" {
					return false
				}
			}
		}
		return true
	}

	// 2. Handle git subcommands
	if base == "git" {
		return isGitCommandReadOnly(parts)
	}

	// 3. Handle go subcommands
	if base == "go" {
		return isGoReadOnly(parts)
	}

	// 4. Handle docker subcommands
	if base == "docker" && len(parts) >= 2 {
		return isDockerReadOnly(parts)
	}

	// 5. Handle gh subcommands (e.g., "gh pr view", "gh issue list")
	if base == "gh" {
		return isGhReadOnly(parts)
	}

	// 5. Try flag-validated allowlist
	return isCommandSafeViaFlagParsing(command, parts)
}

// isGitCommandReadOnly handles all git subcommand validation.
func isGitCommandReadOnly(parts []string) bool {
	if len(parts) < 2 {
		return true
	}

	// Skip flags before subcommand: git --no-pager log
	subIdx := 1
	for subIdx < len(parts) && strings.HasPrefix(parts[subIdx], "-") {
		subIdx++
	}
	if subIdx >= len(parts) {
		return true
	}
	sub := parts[subIdx]

	// Security: block git GLOBAL flags that can be dangerous.
	// Only check tokens BEFORE the subcommand (parts[:subIdx]) — tokens after
	// the subcommand belong to the subcommand itself (e.g., "git grep -c" is safe).
	for _, p := range parts[:subIdx] {
		if strings.HasPrefix(p, "-c") && len(p) <= 3 {
			return false
		}
		if strings.HasPrefix(p, "--exec-path") {
			return false
		}
		if strings.HasPrefix(p, "--config-env") {
			return false
		}
	}

	// Try 3-word git commands FIRST: "git <sub> <subsub>" (e.g., "git worktree list",
	// "git remote show", "git stash list"). This must come before both the
	// readOnlyGitSubcommands check and the stash special-case, so that a 3-word
	// allowlist entry takes precedence over a 2-word one.
	// Only try when the second token is not a flag (flags can't be sub-subcommands).
	if subIdx+1 < len(parts) && !strings.HasPrefix(parts[subIdx+1], "-") {
		threeWordKey := "git " + sub + " " + parts[subIdx+1]
		if config, ok := commandAllowlist[threeWordKey]; ok {
			return validateFlags(parts[subIdx+2:], config, "git")
		}
	}

	// git stash: only "list" and "show" are read-only
	if sub == "stash" {
		if subIdx+1 < len(parts) {
			stashSub := parts[subIdx+1]
			if stashSub == "list" || stashSub == "show" {
				// Try flag-validated allowlist for git stash list
				key := "git stash " + stashSub
				if config, ok := commandAllowlist[key]; ok {
					return validateFlags(parts[subIdx+2:], config, "git")
				}
				return true
			}
		}
		return false // bare "git stash" is write
	}

	// Check simple read-only git subcommands (no flag validation needed)
	if readOnlyGitSubcommands[sub] {
		return true
	}

	// Try flag-validated allowlist: "git <sub>"
	key := "git " + sub
	if config, ok := commandAllowlist[key]; ok {
		return validateFlags(parts[subIdx+1:], config, "git")
	}

	return false
}

func isGoReadOnly(parts []string) bool {
	if len(parts) < 2 {
		return true
	}
	return readOnlyGoSubcommands[parts[1]]
}

func isDockerReadOnly(parts []string) bool {
	if len(parts) < 2 {
		return false
	}
	key := "docker " + parts[1]
	if config, ok := commandAllowlist[key]; ok {
		return validateFlags(parts[2:], config, "docker")
	}
	return false
}

// isGhReadOnly handles all gh subcommand validation.
// All approved gh commands are 3-word patterns: "gh <sub1> <sub2>"
// (e.g., "gh pr view", "gh issue list", "gh search repos").
func isGhReadOnly(parts []string) bool {
	if len(parts) < 3 {
		return false
	}
	key := "gh " + parts[1] + " " + parts[2]
	if config, ok := commandAllowlist[key]; ok {
		return validateFlags(parts[3:], config, "gh")
	}
	return false
}

// isCommandSafeViaFlagParsing checks if a non-git, non-docker command is safe
// using the flag-validated allowlist.
func isCommandSafeViaFlagParsing(command string, parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	base := parts[0]

	config, ok := commandAllowlist[base]
	if !ok {
		return false
	}

	// For awk/gawk: strings.Fields breaks shell-quoted programs ('{print $1}'
	// becomes ['{print', '$1}']), so per-token $ checks would incorrectly block
	// safe awk programs. Use hasUnquotedDollar on the raw command string instead,
	// which correctly respects quoting context. Also check for dangerous awk
	// constructs (system(), file/pipe writes) using the raw string.
	if base == "awk" || base == "gawk" {
		if hasUnquotedDollar(command) {
			return false
		}
		if rawAwkIsDangerous(command) {
			return false
		}
	} else {
		// Check for $ in any token (parser differential defense)
		for _, token := range parts[1:] {
			if strings.Contains(token, "$") {
				return false
			}
		}
	}

	// Check for brace expansion
	for _, token := range parts[1:] {
		if hasBraceExpansion(token) {
			return false
		}
	}

	// Check for backticks
	if strings.Contains(command, "`") {
		return false
	}

	// Check for newlines in grep/rg (could hide dangerous commands)
	if (base == "grep" || base == "rg") && (strings.Contains(command, "\n") || strings.Contains(command, "\r")) {
		return false
	}

	return validateFlags(parts[1:], config, base)
}

// validateFlags validates command tokens against a CommandConfig's safe flags.
// This is the Go port of Claude Code's validateFlags() function.
func validateFlags(tokens []string, config CommandConfig, commandName string) bool {
	respectsDoubleDash := config.RespectsDoubleDash

	// Check IsDangerous callback first
	if config.IsDangerous != nil && config.IsDangerous(tokens) {
		return false
	}

	i := 0
	for i < len(tokens) {
		token := tokens[i]

		// -- stops flag processing
		if token == "--" {
			if respectsDoubleDash {
				break // remaining args are positional, allowed
			}
			i++
			continue
		}

		if !strings.HasPrefix(token, "-") || len(token) < 2 {
			// xargs special: check if it's a safe target command
			if commandName == "xargs" {
				if safeTargetCommandsForXargs[token] {
					break // rest is the target command, safe
				}
				return false
			}
			// Positional argument — allowed for most commands
			i++
			continue
		}

		// Handle --flag=value
		hasEquals := false
		flag := token
		inlineValue := ""
		if eqIdx := strings.Index(token, "="); eqIdx > 0 && strings.HasPrefix(token, "--") {
			hasEquals = true
			flag = token[:eqIdx]
			inlineValue = token[eqIdx+1:]
		}

		argType, ok := config.SafeFlags[flag]
		if !ok {
			// Try git numeric shorthand: -<number> equiv to -n <number>
			if commandName == "git" && len(flag) > 1 && isNumeric(flag[1:]) {
				i++
				continue
			}

			// Try attached value: -A20 (grep/rg numeric), -t: (sort char)
			if len(flag) > 2 && !strings.HasPrefix(flag, "--") {
				shortFlag := flag[:2]
				value := flag[2:]
				if shortArgType, shortOk := config.SafeFlags[shortFlag]; shortOk {
					if validateFlagArgument(value, shortArgType) {
						i++
						continue
					}
				}
			}

			// Try combined short flags: -nrE (all None) or -tvf (all None except
			// the LAST which may take an argument, e.g., tar -tvf archive.tar).
			if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && len(flag) > 2 {
				allValid := true
				lastType := FlagNone
				for j := 1; j < len(flag); j++ {
					singleFlag := "-" + string(flag[j])
					singleType, singleOk := config.SafeFlags[singleFlag]
					if !singleOk {
						allValid = false
						break
					}
					isLast := j == len(flag)-1
					if !isLast && singleType != FlagNone {
						// Non-None flags must be at the END of the bundle.
						allValid = false
						break
					}
					lastType = singleType
				}
				if allValid {
					if lastType == FlagNone {
						i++
					} else {
						// Last flag in the bundle takes an argument.
						if i+1 >= len(tokens) {
							return false // missing argument
						}
						argVal := tokens[i+1]
						if lastType == FlagString && strings.HasPrefix(argVal, "-") {
							if flag == "--sort" && commandName == "git" {
								i += 2
								continue
							}
							return false
						}
						if !validateFlagArgument(argVal, lastType) {
							return false
						}
						i += 2
					}
					continue
				}
			}

			return false // unknown flag
		}

		// Validate flag argument
		if argType == FlagNone {
			if hasEquals {
				return false // --flag=value with none type
			}
			i++
		} else {
			var argValue string
			if hasEquals {
				argValue = inlineValue
				i++
			} else {
				if i+1 >= len(tokens) {
					return false // flag needs argument but none provided
				}
				argValue = tokens[i+1]
				i += 2
			}

			// Defense: string args starting with - are suspicious
			if argType == FlagString && strings.HasPrefix(argValue, "-") {
				// Exception: git --sort allows -key
				if flag == "--sort" && commandName == "git" {
					continue
				}
				return false
			}

			if !validateFlagArgument(argValue, argType) {
				return false
			}
		}
	}

	return true
}

func validateFlagArgument(value string, argType FlagArgType) bool {
	switch argType {
	case FlagNone:
		return false
	case FlagNumber:
		return isNumeric(value)
	case FlagString:
		return true
	case FlagChar:
		return len(value) == 1
	case FlagBraces:
		return value == "{}"
	case FlagEOF:
		return value == "EOF"
	default:
		return false
	}
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// hasDangerousPattern checks for output redirections and command substitution
// outside of quotes.
func hasDangerousPattern(command string) bool {
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if inSingle || inDouble {
			continue
		}

		// Output redirection (but not 2>&1 which is safe stderr merge)
		if ch == '>' {
			// Check if this is N>&M (fd redirect) — safe pattern
			if i > 0 && command[i-1] >= '0' && command[i-1] <= '9' &&
				i+1 < len(command) && command[i+1] == '&' {
				continue // fd redirect like 2>&1
			}
			return true
		}
		// Command substitution
		if ch == '$' && i+1 < len(command) && command[i+1] == '(' {
			return true
		}
		// Backtick substitution
		if ch == '`' {
			return true
		}
	}
	return false
}

// splitCompoundCommand splits on &&, ||, ;, and | operators outside quotes.
func splitCompoundCommand(command string) []string {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escaped {
			escaped = false
			current.WriteByte(ch)
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			current.WriteByte(ch)
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteByte(ch)
			continue
		}

		if inSingle || inDouble {
			current.WriteByte(ch)
			continue
		}

		// Check for &&, ||
		if i+1 < len(command) {
			pair := command[i : i+2]
			if pair == "&&" || pair == "||" {
				parts = append(parts, current.String())
				current.Reset()
				i++
				continue
			}
		}

		// Check for ; and |
		if ch == ';' || ch == '|' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// hasUnquotedDollar checks for unquoted $ signs (variable expansion).
// This is a lighter check than containsUnquotedExpansion — it only blocks $,
// not globs (* ? [ ]). Safe for read-only commands where globs are harmless.
func hasUnquotedDollar(command string) bool {
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle {
			continue
		}
		if ch == '$' {
			return true
		}
	}
	return false
}

// splitCommand splits a command string into tokens.
func splitCommand(command string) []string {
	return strings.Fields(command)
}

// stripSafeWrappers removes safe wrapper commands from the front of a command.
// e.g., "timeout 10 ls" → "ls", "time ls" → "ls", "nice -n 5 ls" → "ls"
func stripSafeWrappers(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	i := 0
	// Skip env var assignments: FOO=bar
	for i < len(parts) {
		eq := strings.IndexByte(parts[i], '=')
		if eq <= 0 || strings.HasPrefix(parts[i], "-") {
			break
		}
		i++
	}
	if i >= len(parts) {
		return ""
	}

	switch parts[i] {
	case "timeout":
		// Skip timeout and its args: timeout [-flags] DURATION
		i++
		for i < len(parts) && strings.HasPrefix(parts[i], "-") {
			i++
			// Some flags take an argument
			if i < len(parts) && !strings.HasPrefix(parts[i], "-") {
				i++
			}
		}
		if i < len(parts) {
			i++ // skip the duration argument
		}
	case "time":
		i++
		if i < len(parts) && parts[i] == "--" {
			i++
		}
	case "nice":
		i++
		if i < len(parts) && parts[i] == "-n" {
			i += 2 // skip -n and the priority
		} else if i < len(parts) && strings.HasPrefix(parts[i], "-") && isNumeric(parts[i][1:]) {
			i++ // skip -N shorthand
		}
	case "stdbuf":
		i++
		for i < len(parts) && strings.HasPrefix(parts[i], "-") {
			i++
		}
	case "nohup":
		i++
	case "env":
		i++
		// Skip env var assignments after env
		for i < len(parts) {
			if strings.IndexByte(parts[i], '=') > 0 {
				i++
			} else {
				break
			}
		}
	}

	if i >= len(parts) {
		return ""
	}
	return strings.Join(parts[i:], " ")
}
