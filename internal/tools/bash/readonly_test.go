package bash

import (
	"strings"
	"testing"
)

func TestIsReadOnly_SafeCommands(t *testing.T) {
	safe := []string{
		"ls", "ls -la", "ls -la /tmp",
		"cat foo.go", "head -20 main.go", "tail -f log.txt",
		"wc -l *.go", "pwd", "whoami", "which go", "type bash",
		"uname -a", "df -h", "du -sh .", "diff a.txt b.txt",
		"true", "false", "uptime", "id", "stat foo.go",
		"basename /tmp/foo.go", "dirname /tmp/foo.go", "realpath .",
		"echo hello", "echo 'hello world'",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_GitReadOnlyCommands(t *testing.T) {
	safe := []string{
		"git status",
		"git status --short",
		"git log",
		"git log --oneline -20",
		"git diff",
		"git diff HEAD~1",
		"git diff --cached",
		"git diff --stat",
		"git show HEAD",
		"git branch",
		"git branch -a",
		"git branch --show-current",
		"git remote -v",
		"git blame main.go",
		"git blame -L 10,20 main.go",
		"git rev-parse HEAD",
		"git describe --tags",
		"git stash list",
		"git ls-files",
		"git ls-files --modified",
		"git config --list",
		"git config --get user.name",
		"git --no-pager log",
		"git --no-pager diff --stat",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_GitSecurity(t *testing.T) {
	// cd + git = sandbox escape risk
	if IsReadOnly("cd /tmp && git status") {
		t.Error("cd + git should not be read-only")
	}
	// git -c = inline config execution
	if IsReadOnly("git -c core.pager=evil log") {
		t.Error("git -c should not be read-only")
	}
	// git --exec-path = executable path override
	if IsReadOnly("git --exec-path=/tmp/evil log") {
		t.Error("git --exec-path should not be read-only")
	}
	// writing to hooks + git
	if IsReadOnly("mkdir hooks && git status") {
		t.Error("write to git-internal + git should not be read-only")
	}
}

func TestIsReadOnly_GrepWithFlags(t *testing.T) {
	safe := []string{
		"grep -r 'TODO' .",
		"grep -rn 'func main' .",
		"grep -i -l 'error' src/",
		"grep -A 5 -B 2 'pattern' file",
		"grep -E 'foo|bar' .",
		"grep --color=auto -n 'test' .",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_RipgrepWithFlags(t *testing.T) {
	safe := []string{
		"rg 'pattern'",
		"rg --type go 'func'",
		"rg -i 'error' src/",
		"rg -A5 -B2 'pattern'",
		"rg -C3 --hidden 'TODO'",
		"rg -l --json 'import'",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_SedSafe(t *testing.T) {
	safe := []string{
		"sed -n '5p' file.txt",
		"sed -e 's/foo/bar/g'",
		"sed -E 's/pattern/replace/' file",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_SedDangerous(t *testing.T) {
	dangerous := []string{
		"sed -i 's/foo/bar/' file.txt",     // in-place edit
		"sed -i.bak 's/foo/bar/' file.txt", // in-place with backup
		"sed --in-place 's/foo/bar/' file.txt",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_FindSafe(t *testing.T) {
	safe := []string{
		"find . -name '*.go'",
		"find /tmp -type f",
		"find . -name '*.go' -type f",
		"find . -maxdepth 2 -name '*.ts'",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_FindDangerous(t *testing.T) {
	dangerous := []string{
		"find . -name '*.tmp' -delete",
		"find . -exec rm {} \\;",
		"find . -execdir cat {} \\;",
		"find . -ok rm {} \\;",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_DangerousCommands(t *testing.T) {
	dangerous := []string{
		"rm foo.go", "rm -rf /", "rm -rf .",
		"mv a.go b.go", "cp a.go b.go",
		"mkdir newdir", "touch newfile",
		"chmod 755 script.sh", "chown root file",
		"kill -9 1234",
		"pip install malware", "npm install",
		"apt-get install", "brew install",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_GitWriteCommands(t *testing.T) {
	dangerous := []string{
		"git add .", "git commit -m 'test'",
		"git push", "git push origin main", "git push --force",
		"git reset --hard", "git checkout -- .",
		"git clean -fd", "git merge main", "git rebase main",
		"git stash", "git stash pop",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_CompoundCommands(t *testing.T) {
	tests := []struct {
		cmd      string
		readOnly bool
	}{
		{"ls && pwd", true},
		{"cat a.go && head b.go", true},
		{"ls && rm foo", false},
		{"echo hello | grep hello", true},
		{"cat foo | wc -l", true},
		{"echo hello; rm foo", false},
		{"git status && git log", true},
		{"git status && git push", false},
	}
	for _, tt := range tests {
		got := IsReadOnly(tt.cmd)
		if got != tt.readOnly {
			t.Errorf("IsReadOnly(%q) = %v, want %v", tt.cmd, got, tt.readOnly)
		}
	}
}

func TestIsReadOnly_VariableExpansion(t *testing.T) {
	dangerous := []string{
		"echo $HOME", "cat $FILE", "ls $DIR",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only (variable expansion): %q", cmd)
		}
	}
}

func TestIsReadOnly_Redirections(t *testing.T) {
	dangerous := []string{
		"echo hello > file.txt", "cat a >> b", "ls > /tmp/out",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only (redirection): %q", cmd)
		}
	}
}

func TestIsReadOnly_CommandSubstitution(t *testing.T) {
	dangerous := []string{
		"echo $(rm -rf /)", "cat `whoami`",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only (command substitution): %q", cmd)
		}
	}
}

func TestIsReadOnly_EnvVarPrefixes(t *testing.T) {
	safe := []string{
		"LANG=C ls",
		"NODE_ENV=test cat package.json",
		"GOOS=linux go version",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only with safe env prefix: %q", cmd)
		}
	}
}

func TestIsReadOnly_WrapperCommands(t *testing.T) {
	safe := []string{
		"time ls",
		"nice ls",
		"nohup ls",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only with safe wrapper: %q", cmd)
		}
	}
}

func TestIsReadOnly_DockerReadOnly(t *testing.T) {
	safe := []string{
		"docker ps",
		"docker ps -a",
		"docker logs --tail 100 container_id",
		"docker inspect container_id",
		"docker images",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}

	dangerous := []string{
		"docker run image",
		"docker exec -it container bash",
		"docker rm container",
		"docker stop container",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestIsReadOnly_MaxSubcommands(t *testing.T) {
	// Build a command with 60 subcommands — should fall back to 'not read-only'
	parts := make([]string, 60)
	for i := range parts {
		parts[i] = "ls"
	}
	cmd := strings.Join(parts, " && ")

	if IsReadOnly(cmd) {
		t.Error("commands with >50 subcommands should not be auto-approved")
	}
}

func TestIsReadOnly_Stderr2Stdout(t *testing.T) {
	// 2>&1 suffix is safe (just merges stderr into stdout)
	if !IsReadOnly("ls -la 2>&1") {
		t.Error("2>&1 suffix should be safe")
	}
}

func TestIsReadOnly_GitBranchCreation(t *testing.T) {
	// "git branch" (listing) is safe, "git branch newname" (creation) is not
	if !IsReadOnly("git branch") {
		t.Error("git branch (list) should be read-only")
	}
	if !IsReadOnly("git branch -a") {
		t.Error("git branch -a should be read-only")
	}
	if IsReadOnly("git branch newbranch") {
		t.Error("git branch <name> should not be read-only")
	}
}

func TestIsReadOnly_SortWithFlags(t *testing.T) {
	if !IsReadOnly("sort -n -r file.txt") {
		t.Error("sort with safe flags should be read-only")
	}
	if !IsReadOnly("sort -k2,2 -t: file.txt") {
		t.Error("sort with key/field flags should be read-only")
	}
}
