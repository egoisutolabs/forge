package bash

import "testing"

func TestContainsUnquotedExpansion_Blocked(t *testing.T) {
	blocked := []string{
		"echo $HOME",
		"cat $FILE",
		`echo "$HOME"`, // $ in double quotes still expands
		"echo $@",
		"echo $*",
		"echo $?",
		"echo ${VAR}",
		"ls *.go",      // glob
		"ls file?.txt", // glob
		"ls [abc].txt", // glob
		"echo $(whoami)",
		"echo `whoami`",
	}
	for _, cmd := range blocked {
		if !containsUnquotedExpansion(cmd) {
			t.Errorf("expected blocked: %q", cmd)
		}
	}
}

func TestContainsUnquotedExpansion_Allowed(t *testing.T) {
	allowed := []string{
		"echo 'hello $world'",    // $ inside single quotes is literal
		`echo 'no $(expansion)'`, // inside single quotes
		"echo hello",
		"ls -la",
		"git status",
	}
	for _, cmd := range allowed {
		if containsUnquotedExpansion(cmd) {
			t.Errorf("expected allowed: %q", cmd)
		}
	}
}

func TestContainsUnquotedExpansion_EscapeHandling(t *testing.T) {
	// \$ should not be flagged (escaped dollar)
	if containsUnquotedExpansion(`echo \$HOME`) {
		t.Error(`expected \$HOME to be allowed (escaped)`)
	}
	// But \ inside single quotes is literal
	if containsUnquotedExpansion(`echo '\$HOME'`) {
		t.Error(`expected '\$HOME' to be allowed (single quotes)`)
	}
}

func TestHasBraceExpansion(t *testing.T) {
	if !hasBraceExpansion("{a,b}") {
		t.Error("expected brace expansion detected: {a,b}")
	}
	if !hasBraceExpansion("{1..5}") {
		t.Error("expected brace expansion detected: {1..5}")
	}
	if hasBraceExpansion("{solo}") {
		t.Error("no expansion in {solo}")
	}
	if hasBraceExpansion("nobrace") {
		t.Error("no expansion in plain text")
	}
}

func TestIsGitInternalPath(t *testing.T) {
	internal := []string{"HEAD", "hooks", "hooks/pre-commit", "objects", "objects/pack", "refs", "refs/heads/main"}
	for _, p := range internal {
		if !isGitInternalPath(p) {
			t.Errorf("expected git-internal: %q", p)
		}
	}
	safe := []string{"src/hooks", "README", "main.go", "objects.txt"}
	for _, p := range safe {
		if isGitInternalPath(p) {
			t.Errorf("expected NOT git-internal: %q", p)
		}
	}
}

func TestCommandWritesToGitInternalPaths(t *testing.T) {
	dangerous := []string{
		"mkdir hooks && git status",
		"echo evil > hooks/pre-commit",
		"touch HEAD && git log",
		"cp script.sh hooks/post-commit",
	}
	for _, cmd := range dangerous {
		if !commandWritesToGitInternalPaths(cmd) {
			t.Errorf("expected detected: %q", cmd)
		}
	}
	safe := []string{
		"echo hello",
		"mkdir src && git status",
		"ls hooks",
	}
	for _, cmd := range safe {
		if commandWritesToGitInternalPaths(cmd) {
			t.Errorf("expected NOT detected: %q", cmd)
		}
	}
}

func TestStripSafeEnvVars(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"LANG=C ls", "ls"},
		{"NODE_ENV=test npm test", "npm test"},
		{"GOOS=linux GOARCH=amd64 go build", "go build"},
		{"EVIL=bad ls", "EVIL=bad ls"}, // not in safe list
		{"ls", "ls"},
		{"LANG=C", ""},
	}
	for _, tt := range tests {
		got := stripSafeEnvVars(tt.input)
		if got != tt.want {
			t.Errorf("stripSafeEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSedIsDangerous(t *testing.T) {
	dangerous := []struct {
		tokens []string
	}{
		{[]string{"-i", "s/foo/bar/", "file.txt"}},
		{[]string{"-i.bak", "s/foo/bar/", "file.txt"}},
		{[]string{"--in-place", "s/foo/bar/", "file.txt"}},
		{[]string{"-e", "w output.txt"}},
		{[]string{"-e", "e"}},
	}
	for _, tt := range dangerous {
		if !sedIsDangerous(tt.tokens) {
			t.Errorf("expected dangerous: %v", tt.tokens)
		}
	}

	safe := []struct {
		tokens []string
	}{
		{[]string{"-n", "5p"}},
		{[]string{"-e", "s/foo/bar/g"}},
		{[]string{"-E", "s/pattern/replace/"}},
	}
	for _, tt := range safe {
		if sedIsDangerous(tt.tokens) {
			t.Errorf("expected safe: %v", tt.tokens)
		}
	}
}

func TestCompoundCommandHasCd(t *testing.T) {
	if !compoundCommandHasCd("cd /tmp && git status") {
		t.Error("should detect cd")
	}
	if compoundCommandHasCd("git status && ls") {
		t.Error("should not detect cd")
	}
}
