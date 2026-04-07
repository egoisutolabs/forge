package bash

// sed_validation.go — full port of Claude Code's sedValidation.ts.
//
// Security model: two-layer defence.
//  1. Allowlist (Pattern 1 / Pattern 2): only specific, well-understood sed
//     forms are approved.
//  2. Denylist (containsDangerousOperations): defence-in-depth even when the
//     allowlist matches.
//
// Public entry-points:
//   sedExpressionIsReadOnly   – validate a single sed expression
//   sedCommandIsAllowedByAllowlist – validate a full "sed …" command string

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ── compiled regexes (package-level to avoid repeated compilation) ────────────

var (
	sedPrefixRe          = regexp.MustCompile(`^\s*sed\s+`)
	dangerousFlagComboRe = regexp.MustCompile(`-e[wWe]|-w[eE]`)
	printCmdRe           = regexp.MustCompile(`^(?:\d+|\d+,\d+)?p$`)
	negStartRe           = regexp.MustCompile(`^!`)
	negAfterRe           = regexp.MustCompile(`[/\d$]!`)
	tildeAddrRe          = regexp.MustCompile(`\d\s*~\s*\d|,\s*~\s*\d|\$\s*~\s*\d`)
	offsetAddrRe         = regexp.MustCompile(`,\s*[+-]`)
	// bsTricksRe: "s\" OR "\" followed by one of |#%@
	bsTricksRe            = regexp.MustCompile(`s\\|\\[|#%@]`)
	escapedSlashWriteRe   = regexp.MustCompile(`\\\/.*[wW]`)
	slashSpaceWriteRe     = regexp.MustCompile(`/[^/]*\s+[wWeE]`)
	malformedSlashSubstRe = regexp.MustCompile(`^s/[^/]*/[^/]*/[^/]*$`)
	// Write-command patterns
	writeAtStartRe          = regexp.MustCompile(`^[wW]\s*\S+`)
	writeAfterLineRe        = regexp.MustCompile(`^\d+\s*[wW]\s*\S+`)
	writeAfterDollarRe      = regexp.MustCompile(`^\$\s*[wW]\s*\S+`)
	writeAfterPatternRe     = regexp.MustCompile(`^/[^/]*/[IMim]*\s*[wW]\s*\S+`)
	writeAfterRangeRe       = regexp.MustCompile(`^\d+,\d+\s*[wW]\s*\S+`)
	writeAfterDollarRangeRe = regexp.MustCompile(`^\d+,\$\s*[wW]\s*\S+`)
	writeAfterPatRangeRe    = regexp.MustCompile(`^/[^/]*/[IMim]*,/[^/]*/[IMim]*\s*[wW]\s*\S+`)
	// Execute-command patterns
	execAtStartRe          = regexp.MustCompile(`^e`)
	execAfterLineRe        = regexp.MustCompile(`^\d+\s*e`)
	execAfterDollarRe      = regexp.MustCompile(`^\$\s*e`)
	execAfterPatternRe     = regexp.MustCompile(`^/[^/]*/[IMim]*\s*e`)
	execAfterRangeRe       = regexp.MustCompile(`^\d+,\d+\s*e`)
	execAfterDollarRangeRe = regexp.MustCompile(`^\d+,\$\s*e`)
	execAfterPatRangeRe    = regexp.MustCompile(`^/[^/]*/[IMim]*,/[^/]*/[IMim]*\s*e`)
	// y transliterate command
	yCommandRe = regexp.MustCompile(`y([^\\\n])`)
	// Valid substitution flags: g p i I m M and one optional digit 1-9
	substFlagRe = regexp.MustCompile(`^[gpimIM]*[1-9]?[gpimIM]*$`)
)

// ── Public API ────────────────────────────────────────────────────────────────

// sedExpressionIsReadOnly returns true if the sed expression is safe (read-only).
// It is the inverse of containsDangerousOperations.
func sedExpressionIsReadOnly(expr string) bool {
	return !containsDangerousOperations(expr)
}

// sedCommandIsAllowedByAllowlist validates a full "sed …" command string.
// Returns true only when the command matches one of the two safe patterns
// AND passes the defence-in-depth denylist.
//
// When allowFileWrites is true (acceptEdits mode), -i / --in-place are also
// accepted for substitution commands.
func sedCommandIsAllowedByAllowlist(command string, allowFileWrites bool) bool {
	expressions, err := extractSedExpressions(command)
	if err != nil {
		return false
	}

	hasFileArguments := hasFileArgs(command)

	var isPattern1, isPattern2 bool
	if allowFileWrites {
		// acceptEdits: only substitution (Pattern 2) with -i allowed
		isPattern2 = isSubstitutionCommand(command, expressions, hasFileArguments, true)
	} else {
		isPattern1 = isLinePrintingCommand(command, expressions)
		isPattern2 = isSubstitutionCommand(command, expressions, hasFileArguments, false)
	}

	if !isPattern1 && !isPattern2 {
		return false
	}

	// Pattern 2 forbids semicolons (separating multiple sed commands).
	// Pattern 1 allows them for chaining print commands.
	for _, expr := range expressions {
		if isPattern2 && strings.ContainsRune(expr, ';') {
			return false
		}
	}

	// Defence-in-depth: denylist check even when the allowlist matched.
	for _, expr := range expressions {
		if containsDangerousOperations(expr) {
			return false
		}
	}

	return true
}

// ── Shell tokeniser ───────────────────────────────────────────────────────────

// shellTokenize splits a shell fragment into tokens, respecting:
//   - single-quoted strings (contents are literal, no escapes)
//   - double-quoted strings (backslash escapes honoured)
//   - backslash escapes outside quotes
//
// Returns an error for unclosed quotes or a trailing backslash.
func shellTokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	inToken := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			cur.WriteByte(ch)
			escaped = false
			inToken = true
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			inToken = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			inToken = true
			continue
		}

		if ch == '"' && !inSingle {
			inDouble = !inDouble
			inToken = true
			continue
		}

		if inSingle || inDouble {
			cur.WriteByte(ch)
			continue
		}

		// Unquoted whitespace is a token separator.
		if ch == ' ' || ch == '\t' || ch == '\n' {
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			continue
		}

		cur.WriteByte(ch)
		inToken = true
	}

	if inSingle {
		return nil, errors.New("unterminated single quote")
	}
	if inDouble {
		return nil, errors.New("unterminated double quote")
	}
	if escaped {
		return nil, errors.New("trailing backslash")
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}

	return tokens, nil
}

// ── Expression extraction ─────────────────────────────────────────────────────

// extractSedExpressions extracts the sed expressions from a full "sed …" command.
// Returns an error for dangerous flag combinations or unparseable shell syntax.
func extractSedExpressions(command string) ([]string, error) {
	match := sedPrefixRe.FindString(command)
	if match == "" {
		return nil, nil // not a sed command
	}
	withoutSed := command[len(match):]

	// Reject dangerous flag combinations like -ew, -eW, -ee, -we.
	if dangerousFlagComboRe.MatchString(withoutSed) {
		return nil, errors.New("dangerous flag combination detected")
	}

	tokens, err := shellTokenize(withoutSed)
	if err != nil {
		return nil, fmt.Errorf("malformed shell syntax: %w", err)
	}

	var expressions []string
	foundEFlag := false
	foundExpression := false

	for i := 0; i < len(tokens); i++ {
		arg := tokens[i]

		// -e <expr> or --expression <expr>
		if (arg == "-e" || arg == "--expression") && i+1 < len(tokens) {
			foundEFlag = true
			expressions = append(expressions, tokens[i+1])
			i++ // skip the expression token
			continue
		}

		// --expression=value
		if strings.HasPrefix(arg, "--expression=") {
			foundEFlag = true
			expressions = append(expressions, arg[len("--expression="):])
			continue
		}

		// -e=value (non-standard, defence-in-depth)
		if strings.HasPrefix(arg, "-e=") {
			foundEFlag = true
			expressions = append(expressions, arg[len("-e="):])
			continue
		}

		// Skip other flags
		if strings.HasPrefix(arg, "-") {
			continue
		}

		// First non-flag argument is the expression (when no -e used).
		if !foundEFlag && !foundExpression {
			expressions = append(expressions, arg)
			foundExpression = true
			continue
		}

		// Subsequent non-flag arguments after -e or standalone expr are filenames.
		break
	}

	return expressions, nil
}

// hasFileArgs returns true if the sed command has file arguments
// (i.e., it reads from files rather than just stdin).
func hasFileArgs(command string) bool {
	match := sedPrefixRe.FindString(command)
	if match == "" {
		return false
	}
	withoutSed := command[len(match):]

	tokens, err := shellTokenize(withoutSed)
	if err != nil {
		return true // assume dangerous if we cannot parse
	}

	argCount := 0
	hasEFlag := false

	for i := 0; i < len(tokens); i++ {
		arg := tokens[i]

		if (arg == "-e" || arg == "--expression") && i+1 < len(tokens) {
			hasEFlag = true
			i++
			continue
		}

		if strings.HasPrefix(arg, "--expression=") || strings.HasPrefix(arg, "-e=") {
			hasEFlag = true
			continue
		}

		if strings.HasPrefix(arg, "-") {
			continue
		}

		argCount++

		// With -e, ALL remaining non-flag args are file arguments.
		if hasEFlag {
			return true
		}
		// Without -e, the first non-flag arg is the expression;
		// a second non-flag arg means a file was given.
		if argCount > 1 {
			return true
		}
	}

	return false
}

// ── Flag helpers ──────────────────────────────────────────────────────────────

// validateFlagsAgainstAllowlist returns true if every flag is in the allowed set.
// Handles combined flags like -nE by checking each character individually.
func validateFlagsAgainstAllowlist(flags []string, allowed map[string]bool) bool {
	for _, flag := range flags {
		if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && len(flag) > 2 {
			// Combined flag (e.g. -nE): check each character.
			for _, ch := range flag[1:] {
				if !allowed["-"+string(ch)] {
					return false
				}
			}
		} else {
			if !allowed[flag] {
				return false
			}
		}
	}
	return true
}

// extractFlags returns all flag tokens (starting with '-') from a token list.
func extractFlags(tokens []string) []string {
	var flags []string
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") && t != "--" {
			flags = append(flags, t)
		}
	}
	return flags
}

// ── Pattern 1: line printing ──────────────────────────────────────────────────

// isPrintCommand returns true for the strict set of allowed print commands:
//
//	p, Np, N,Mp
func isPrintCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	return printCmdRe.MatchString(cmd)
}

// isLinePrintingCommand checks Pattern 1: safe "sed -n …" line-printing usage.
//
// Requires -n flag; expressions must each be only p/Np/N,Mp (semicolons allowed).
// File arguments are PERMITTED (reading from a file is safe).
func isLinePrintingCommand(command string, expressions []string) bool {
	match := sedPrefixRe.FindString(command)
	if match == "" {
		return false
	}
	withoutSed := command[len(match):]

	tokens, err := shellTokenize(withoutSed)
	if err != nil {
		return false
	}

	flags := extractFlags(tokens)

	allowedFlags := map[string]bool{
		"-n": true, "--quiet": true, "--silent": true,
		"-E": true, "--regexp-extended": true,
		"-r": true,
		"-z": true, "--zero-terminated": true,
		"--posix": true,
	}
	if !validateFlagsAgainstAllowlist(flags, allowedFlags) {
		return false
	}

	// -n flag is required.
	hasNFlag := false
	for _, flag := range flags {
		if flag == "-n" || flag == "--quiet" || flag == "--silent" {
			hasNFlag = true
			break
		}
		// Combined flags like -nE
		if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") &&
			strings.ContainsRune(flag, 'n') {
			hasNFlag = true
			break
		}
	}
	if !hasNFlag {
		return false
	}

	if len(expressions) == 0 {
		return false
	}

	// All expressions must consist solely of print commands.
	// Semicolons are allowed to chain them: "1p;2p;3p".
	for _, expr := range expressions {
		cmds := strings.Split(expr, ";")
		for _, cmd := range cmds {
			if !isPrintCommand(strings.TrimSpace(cmd)) {
				return false
			}
		}
	}

	return true
}

// ── Pattern 2: substitution ───────────────────────────────────────────────────

// isSubstitutionCommand checks Pattern 2: safe "sed 's/…/…/flags'" usage.
//
// Only the '/' delimiter is accepted. Allowed expression flags: g p i I m M 1-9.
// File arguments are rejected unless allowFileWrites is true.
func isSubstitutionCommand(command string, expressions []string, hasFileArguments bool, allowFileWrites bool) bool {
	if !allowFileWrites && hasFileArguments {
		return false
	}

	match := sedPrefixRe.FindString(command)
	if match == "" {
		return false
	}
	withoutSed := command[len(match):]

	tokens, err := shellTokenize(withoutSed)
	if err != nil {
		return false
	}

	flags := extractFlags(tokens)

	allowedFlags := map[string]bool{
		"-E": true, "--regexp-extended": true,
		"-r": true, "--posix": true,
	}
	if allowFileWrites {
		allowedFlags["-i"] = true
		allowedFlags["--in-place"] = true
	}

	if !validateFlagsAgainstAllowlist(flags, allowedFlags) {
		return false
	}

	// Exactly one expression required.
	if len(expressions) != 1 {
		return false
	}

	expr := strings.TrimSpace(expressions[0])

	// Must be a substitution starting with 's'.
	if !strings.HasPrefix(expr, "s") {
		return false
	}

	// Strict allowlist: only '/' delimiter accepted.
	if !strings.HasPrefix(expr, "s/") {
		return false
	}

	rest := expr[2:] // everything after the opening "s/"

	// Scan for the 2nd and 3rd '/' delimiters, respecting backslash escapes.
	delimCount := 0
	lastDelimPos := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if rest[i] == '/' {
			delimCount++
			lastDelimPos = i
		}
	}

	if delimCount != 2 {
		return false
	}

	exprFlags := rest[lastDelimPos+1:]
	return substFlagRe.MatchString(exprFlags)
}

// ── Defence-in-depth denylist ─────────────────────────────────────────────────

// extractSubstitutionFlags extracts the flags portion from a sed substitution
// command (any delimiter). Uses a simple linear scan without escape handling
// (matching TypeScript behaviour — conservative false-positives are acceptable).
//
// Returns (flags, true) on success, ("", false) if not a valid substitution.
func extractSubstitutionFlags(cmd string) (string, bool) {
	if len(cmd) < 2 || cmd[0] != 's' {
		return "", false
	}
	delim := cmd[1]
	if delim == '\\' || delim == '\n' {
		return "", false
	}

	// Count occurrences of delim starting from position 2.
	// The 2nd occurrence marks the end of the replacement; flags follow.
	count := 0
	for i := 2; i < len(cmd); i++ {
		if cmd[i] == delim {
			count++
			if count == 2 {
				return cmd[i+1:], true
			}
		}
	}
	return "", false
}

// containsDangerousOperations returns true if a sed expression contains any
// operation that could write files or execute arbitrary shell commands.
//
// This is a port of the TypeScript containsDangerousOperations() function.
// When in doubt, the function rejects the expression (conservative).
func containsDangerousOperations(expression string) bool {
	cmd := strings.TrimSpace(expression)
	if cmd == "" {
		return false
	}

	// Strip a leading shell quote if present. When isCommandSafeViaFlagParsing
	// splits a command with strings.Fields, shell-quoted expressions retain their
	// opening quote character: sed 's/a/b/e' file → token "'s/a/b/e'". Without
	// stripping, the leading ' causes cmd[0] != 's', bypassing all s-command checks.
	if len(cmd) > 0 && (cmd[0] == '\'' || cmd[0] == '"') {
		cmd = cmd[1:]
	}
	if cmd == "" {
		return false
	}

	// 1. Reject non-ASCII and null bytes.
	//    Catches Unicode homoglyphs, combining characters, etc.
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == 0x00 || cmd[i] > 0x7F {
			return true
		}
	}

	// 2. Reject curly braces (sed blocks) — too complex to validate safely.
	if strings.ContainsAny(cmd, "{}") {
		return true
	}

	// 3. Reject embedded newlines.
	if strings.ContainsRune(cmd, '\n') {
		return true
	}

	// 4. Reject '#' except immediately after 's' (where it is a delimiter).
	//    Examples: "#comment", "5#p" → rejected.  "s#pat#rep#" → allowed.
	if idx := strings.IndexByte(cmd, '#'); idx != -1 {
		if !(idx > 0 && cmd[idx-1] == 's') {
			return true
		}
	}

	// 5. Reject negation operator '!'.
	if negStartRe.MatchString(cmd) || negAfterRe.MatchString(cmd) {
		return true
	}

	// 6. Reject GNU step-address tilde: N~M, ,~M, $~M.
	if tildeAddrRe.MatchString(cmd) {
		return true
	}

	// 7. Reject comma at start (bare address shorthand for 1,$).
	if len(cmd) > 0 && cmd[0] == ',' {
		return true
	}

	// 8. Reject GNU offset addresses: ,+N or ,-N.
	if offsetAddrRe.MatchString(cmd) {
		return true
	}

	// 9. Reject backslash tricks:
	//    - "s\" (substitution with backslash delimiter)
	//    - "\" followed by |, #, %, @ (obfuscated alternate delimiters)
	if bsTricksRe.MatchString(cmd) {
		return true
	}

	// 10. Reject escaped-slash followed by w/W (write via path traversal tricks).
	if escapedSlashWriteRe.MatchString(cmd) {
		return true
	}

	// 11. Reject slash-space-dangerous_cmd patterns: /pattern w file.
	if slashSpaceWriteRe.MatchString(cmd) {
		return true
	}

	// 12. Reject malformed "s/" substitutions that don't follow the 3-part form.
	if strings.HasPrefix(cmd, "s/") && !malformedSlashSubstRe.MatchString(cmd) {
		return true
	}

	// 13. Paranoid: reject 's…' commands that end with a dangerous character
	//     unless they are a properly-formed substitution with safe flags.
	if len(cmd) >= 2 && cmd[0] == 's' {
		last := cmd[len(cmd)-1]
		if last == 'w' || last == 'W' || last == 'e' || last == 'E' {
			flags, ok := extractSubstitutionFlags(cmd)
			properSubst := ok && !strings.ContainsAny(flags, "wWeE")
			if !properSubst {
				return true
			}
		}
	}

	// 14. Reject write commands (w / W) in various address forms.
	if writeAtStartRe.MatchString(cmd) ||
		writeAfterLineRe.MatchString(cmd) ||
		writeAfterDollarRe.MatchString(cmd) ||
		writeAfterPatternRe.MatchString(cmd) ||
		writeAfterRangeRe.MatchString(cmd) ||
		writeAfterDollarRangeRe.MatchString(cmd) ||
		writeAfterPatRangeRe.MatchString(cmd) {
		return true
	}

	// 15. Reject execute commands (e) in various address forms.
	if execAtStartRe.MatchString(cmd) ||
		execAfterLineRe.MatchString(cmd) ||
		execAfterDollarRe.MatchString(cmd) ||
		execAfterPatternRe.MatchString(cmd) ||
		execAfterRangeRe.MatchString(cmd) ||
		execAfterDollarRangeRe.MatchString(cmd) ||
		execAfterPatRangeRe.MatchString(cmd) {
		return true
	}

	// 16. Reject substitution flags containing w/W (write) or e/E (execute).
	if flags, ok := extractSubstitutionFlags(cmd); ok {
		if strings.ContainsAny(flags, "wWeE") {
			return true
		}
	}

	// 17. Reject any y (transliterate) command. The y command is rarely needed for
	//     read-only inspection and is complex enough to warrant blocking for parity
	//     with Claude Code's TypeScript implementation.
	if yCommandRe.MatchString(cmd) {
		return true
	}

	return false
}
