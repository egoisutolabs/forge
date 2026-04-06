package bash

import "testing"

// ── isPrintCommand ────────────────────────────────────────────────────────────

func TestIsPrintCommand_Safe(t *testing.T) {
	safe := []string{"p", "5p", "123p", "1,5p", "10,200p", "0p"}
	for _, cmd := range safe {
		if !isPrintCommand(cmd) {
			t.Errorf("expected print command: %q", cmd)
		}
	}
}

func TestIsPrintCommand_NotSafe(t *testing.T) {
	notSafe := []string{
		"", "d", "w", "e", "E", "W",
		"5P",     // uppercase P
		"p5",     // p must be last
		"a,bp",   // non-numeric address
		"1,2,3p", // too many commas
		"1p2",    // trailing digit
		"pp",     // double p
	}
	for _, cmd := range notSafe {
		if isPrintCommand(cmd) {
			t.Errorf("expected NOT print command: %q", cmd)
		}
	}
}

// ── shellTokenize ─────────────────────────────────────────────────────────────

func TestShellTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
		err   bool
	}{
		{"foo bar baz", []string{"foo", "bar", "baz"}, false},
		{"'single quoted'", []string{"single quoted"}, false},
		{`"double quoted"`, []string{"double quoted"}, false},
		{`-n '5p'`, []string{"-n", "5p"}, false},
		{`-e 's/foo/bar/g'`, []string{"-e", "s/foo/bar/g"}, false},
		{`'unclosed`, nil, true},
		{`"unclosed`, nil, true},
		{`-E 's/a/b/'`, []string{"-E", "s/a/b/"}, false},
	}
	for _, tt := range tests {
		got, err := shellTokenize(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("shellTokenize(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("shellTokenize(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("shellTokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("shellTokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// ── extractSedExpressions ─────────────────────────────────────────────────────

func TestExtractSedExpressions(t *testing.T) {
	tests := []struct {
		command string
		want    []string
	}{
		{"sed 's/foo/bar/g'", []string{"s/foo/bar/g"}},
		{"sed -e 's/foo/bar/g'", []string{"s/foo/bar/g"}},
		{"sed -e 's/a/b/' -e 's/c/d/'", []string{"s/a/b/", "s/c/d/"}},
		{"sed -n '5p'", []string{"5p"}},
		{"sed -E 's/a/b/'", []string{"s/a/b/"}},
		{"sed --expression='s/x/y/'", []string{"s/x/y/"}},
	}
	for _, tt := range tests {
		got, err := extractSedExpressions(tt.command)
		if err != nil {
			t.Errorf("extractSedExpressions(%q) error: %v", tt.command, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("extractSedExpressions(%q) = %v, want %v", tt.command, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractSedExpressions(%q)[%d] = %q, want %q", tt.command, i, got[i], tt.want[i])
			}
		}
	}
}

func TestExtractSedExpressions_DangerousCombo(t *testing.T) {
	// Dangerous flag combinations should return an error
	dangerous := []string{
		"sed -ew 's/foo/bar/g'",
		"sed -eW something",
	}
	for _, cmd := range dangerous {
		_, err := extractSedExpressions(cmd)
		if err == nil {
			t.Errorf("extractSedExpressions(%q): expected error for dangerous combo", cmd)
		}
	}
}

// ── hasFileArgs ───────────────────────────────────────────────────────────────

func TestHasFileArgs(t *testing.T) {
	noFile := []string{
		"sed 's/foo/bar/g'",
		"sed -n '5p'",
		"sed -e 's/foo/bar/g'",
		"sed -E 's/a/b/'",
	}
	for _, cmd := range noFile {
		if hasFileArgs(cmd) {
			t.Errorf("expected no file args: %q", cmd)
		}
	}

	withFile := []string{
		"sed 's/foo/bar/g' file.txt",
		"sed -e 's/foo/bar/g' file.txt",
		"sed -n '5p' file.txt",
	}
	for _, cmd := range withFile {
		if !hasFileArgs(cmd) {
			t.Errorf("expected file args: %q", cmd)
		}
	}
}

// ── sedExpressionIsReadOnly ───────────────────────────────────────────────────

func TestSedExpressionIsReadOnly_Safe(t *testing.T) {
	safe := []string{
		"s/foo/bar/g",
		"s/foo/bar/",
		"s/foo/bar/i",
		"s/foo/bar/I",
		"s/foo/bar/m",
		"s/foo/bar/M",
		"s/foo/bar/2",
		"s/foo/bar/Im",
		"5p",
		"1,5p",
		"p",
		"100p",
	}
	for _, expr := range safe {
		if !sedExpressionIsReadOnly(expr) {
			t.Errorf("expected safe expr: %q", expr)
		}
	}
}

func TestSedExpressionIsReadOnly_Dangerous(t *testing.T) {
	dangerous := []string{
		"w file",      // write command
		"W file",      // Write command
		"e",           // execute command
		"1e cmd",      // execute after line number
		"$e cmd",      // execute after $
		"s/foo/bar/w", // write flag
		"s/foo/bar/W", // Write flag
		"s/foo/bar/e", // execute flag
		"s/foo/bar/E", // Execute flag
		"1~2p",        // tilde address
		"0~3p",        // tilde step address
		"!d",          // negation at start
		"/pat/!d",     // negation after pattern
		"{n}",         // curly braces
		",3p",         // comma at start
		"y/abc/ABw/",  // y command with w
		"1w file",     // write after line number
		"$w file",     // write after $
		"/pat/w file", // write after pattern
		"1,10w file",  // write after range
	}
	for _, expr := range dangerous {
		if sedExpressionIsReadOnly(expr) {
			t.Errorf("expected dangerous expr: %q", expr)
		}
	}
}

func TestSedExpressionIsReadOnly_NonASCII(t *testing.T) {
	// Non-ASCII characters should be blocked
	nonASCII := []string{
		"ｗ file",             // fullwidth w
		"s/foo/\x80bar/g",    // non-ASCII byte in replacement
		"s/fo\xc3\xa9/bar/g", // UTF-8 accented char
	}
	for _, expr := range nonASCII {
		if sedExpressionIsReadOnly(expr) {
			t.Errorf("expected dangerous (non-ASCII): %q", expr)
		}
	}
}

// ── sedCommandIsAllowedByAllowlist ────────────────────────────────────────────

func TestSedCommandIsAllowedByAllowlist_Safe(t *testing.T) {
	safe := []string{
		"sed -n '5p'",
		"sed 's/foo/bar/g'",
		"sed -E 's/a/b/'",
		"sed -n '1,5p'",
		"sed -n '1p;2p;3p'",
		"sed -n 'p'",
		"sed 's/a/b/i'",
		"sed 's/a/b/I'",
		"sed 's/a/b/m'",
		"sed 's/a/b/2'",
		"sed 's/a/b/gim'",
		"sed -r 's/foo/bar/'",
		"sed -n '10p'",
		"sed -n '1,100p'",
	}
	for _, cmd := range safe {
		if !sedCommandIsAllowedByAllowlist(cmd, false) {
			t.Errorf("expected allowed: %q", cmd)
		}
	}
}

func TestSedCommandIsAllowedByAllowlist_Dangerous(t *testing.T) {
	dangerous := []string{
		"sed -e 'w file'",           // write command via -e
		"sed -e 'e'",                // execute command
		"sed 's/foo/bar/w'",         // write flag
		"sed 's/foo/bar/e'",         // execute flag
		"sed 's/foo/bar/W'",         // Write flag
		"sed 's/foo/bar/E'",         // Execute flag
		"sed '1~2p'",                // tilde step address
		"sed '/pat/!d'",             // negation
		"sed '{n}'",                 // block/curly brace
		"sed -i 's/foo/bar/g'",      // in-place without allowFileWrites
		"sed 's/foo/bar/g;s/a/b/g'", // semicolons in substitution
		"sed -e 's/foo/bar/g'",      // -e flag not in substitution allowedFlags
		"sed 's|foo|bar|g'",         // alternate delimiter not in allowlist
		"sed 's#foo#bar#g'",         // alternate delimiter not in allowlist
	}
	for _, cmd := range dangerous {
		if sedCommandIsAllowedByAllowlist(cmd, false) {
			t.Errorf("expected blocked: %q", cmd)
		}
	}
}

func TestSedCommandIsAllowedByAllowlist_FileArgs(t *testing.T) {
	// Line-printing with file args is allowed (reading from file)
	if !sedCommandIsAllowedByAllowlist("sed -n '5p' file.txt", false) {
		t.Error("sed -n '5p' file.txt should be allowed (read-only from file)")
	}
	// Substitution with file args: blocked when allowFileWrites=false
	if sedCommandIsAllowedByAllowlist("sed 's/foo/bar/g' file.txt", false) {
		t.Error("sed 's/foo/bar/g' file.txt should be blocked without allowFileWrites")
	}
}

func TestSedCommandIsAllowedByAllowlist_AllowFileWrites(t *testing.T) {
	// In-place editing allowed with allowFileWrites=true
	if !sedCommandIsAllowedByAllowlist("sed -i 's/foo/bar/g'", true) {
		t.Error("sed -i 's/foo/bar/g' should be allowed with allowFileWrites=true")
	}
	// Dangerous flags still blocked even with allowFileWrites
	if sedCommandIsAllowedByAllowlist("sed -i 's/foo/bar/w'", true) {
		t.Error("sed -i 's/foo/bar/w' should still be blocked (write flag)")
	}
}

func TestSedCommandIsAllowedByAllowlist_EdgeCases(t *testing.T) {
	// Escaped delimiters: both allowlist and denylist behavior
	// s/foo\/bar/baz/g — this may or may not be allowed depending on delimiter parsing
	// Test that malformed commands are rejected
	malformed := []string{
		"sed 's/foo'",           // unclosed quote
		"not-sed 's/foo/bar/g'", // not a sed command
		"sed",                   // no expression
	}
	for _, cmd := range malformed {
		// These should be blocked (not necessarily due to danger, just not matching allowlist)
		if sedCommandIsAllowedByAllowlist(cmd, false) {
			t.Errorf("expected blocked (malformed): %q", cmd)
		}
	}
}

// ── extractSubstitutionFlags ──────────────────────────────────────────────────

func TestExtractSubstitutionFlags(t *testing.T) {
	tests := []struct {
		cmd       string
		wantFlags string
		wantOk    bool
	}{
		{"s/foo/bar/g", "g", true},
		{"s/foo/bar/", "", true},
		{"s/foo/bar/gw file", "gw file", true},
		{"s/foo/bar/e", "e", true},
		{"s|foo|bar|g", "g", true}, // alternate delimiter
		{"s#foo#bar#gim", "gim", true},
		{"foo/bar/baz", "", false}, // no 's' prefix
		{"s\\foo\\bar", "", false}, // backslash delimiter rejected
		{"s/foo", "", false},       // missing delimiters
	}
	for _, tt := range tests {
		flags, ok := extractSubstitutionFlags(tt.cmd)
		if ok != tt.wantOk {
			t.Errorf("extractSubstitutionFlags(%q) ok=%v, want %v", tt.cmd, ok, tt.wantOk)
			continue
		}
		if ok && flags != tt.wantFlags {
			t.Errorf("extractSubstitutionFlags(%q) flags=%q, want %q", tt.cmd, flags, tt.wantFlags)
		}
	}
}
