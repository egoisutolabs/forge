package bash

import "testing"

// allowlist_expansion_test.go — TDD tests for the expanded COMMAND_ALLOWLIST.
// Written BEFORE the corresponding entries in allowlist.go (TDD).

// ---------------------------------------------------------------------------
// Git additional subcommands
// ---------------------------------------------------------------------------

func TestAllowlist_GitReflog(t *testing.T) {
	safe := []string{
		"git reflog",
		"git reflog show HEAD",
		"git reflog show main",
		"git reflog --oneline",
		"git reflog -n 10",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"git reflog expire",
		"git reflog delete",
		"git reflog expire --expire=now --all",
		"git reflog exists HEAD",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitLsRemote(t *testing.T) {
	safe := []string{
		"git ls-remote",
		"git ls-remote origin",
		"git ls-remote --branches origin",
		"git ls-remote --tags origin",
		"git ls-remote -q origin",
		"git ls-remote --refs origin",
		"git ls-remote --sort=version:refname origin",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"git ls-remote https://evil.com/repo",
		"git ls-remote git@github.com:user/repo",
		"git ls-remote ssh://host/repo",
		"git ls-remote origin:path", // contains colon
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitMergeBase(t *testing.T) {
	safe := []string{
		"git merge-base main feature",
		"git merge-base --is-ancestor main feature",
		"git merge-base --octopus main feat1 feat2",
		"git merge-base --fork-point origin/main",
		"git merge-base --all main feature",
		"git merge-base --independent main feat1 feat2",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitGrep(t *testing.T) {
	safe := []string{
		"git grep 'pattern'",
		"git grep -n 'func main'",
		"git grep -i -l 'todo'",
		"git grep --cached 'pattern'",
		"git grep -E 'foo|bar'",
		"git grep -c 'error'",
		"git grep --heading 'pattern'",
		"git grep -A 3 'pattern'",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitStashShow(t *testing.T) {
	safe := []string{
		"git stash show",
		"git stash show -p",
		"git stash show --stat",
		"git stash show --name-only",
		"git stash show stash@{0}",
		"git stash show --color",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitWorktreeList(t *testing.T) {
	safe := []string{
		"git worktree list",
		"git worktree list --porcelain",
		"git worktree list -v",
		"git worktree list --verbose",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	// Other git worktree subcommands are write operations
	dangerous := []string{
		"git worktree add /path",
		"git worktree remove /path",
		"git worktree prune",
		"git worktree move /old /new",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitRemoteShow(t *testing.T) {
	safe := []string{
		"git remote show origin",
		"git remote show upstream",
		"git remote show -n origin",
		"git remote show my-remote",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"git remote show origin extra",  // two positionals
		"git remote show https://x.com", // URL
		"git remote show",               // no remote name (0 positionals)
		"git remote show bad@name",      // non-alphanumeric
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// GH commands
// ---------------------------------------------------------------------------

func TestAllowlist_GhPrView(t *testing.T) {
	safe := []string{
		"gh pr view 123",
		"gh pr view --json title,body",
		"gh pr view 123 --repo owner/repo",
		"gh pr view 1 -R owner/repo",
		"gh pr view --comments",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"gh pr view 1 --repo evil.com/owner/repo",         // HOST/OWNER/REPO
		"gh pr view 1 --repo=evil.com/owner/repo",         // equals form
		"gh pr view https://github.com/owner/repo/pull/1", // URL
		"gh pr view 1 --repo git@github.com:o/r",          // SSH style
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhPrList(t *testing.T) {
	safe := []string{
		"gh pr list",
		"gh pr list --state open",
		"gh pr list --author someone",
		"gh pr list --repo owner/repo",
		"gh pr list --json title,number -L 10",
		"gh pr list --draft",
		"gh pr list --base main",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhPrDiff(t *testing.T) {
	safe := []string{
		"gh pr diff 123",
		"gh pr diff --name-only",
		"gh pr diff --patch",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhPrChecks(t *testing.T) {
	safe := []string{
		"gh pr checks 123",
		"gh pr checks --required",
		"gh pr checks --json status",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhPrStatus(t *testing.T) {
	safe := []string{
		"gh pr status",
		"gh pr status --json title",
		"gh pr status -c",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhIssueView(t *testing.T) {
	safe := []string{
		"gh issue view 42",
		"gh issue view 42 --json title",
		"gh issue view 42 --repo owner/repo",
		"gh issue view --comments",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhIssueList(t *testing.T) {
	safe := []string{
		"gh issue list",
		"gh issue list --state open",
		"gh issue list --label bug",
		"gh issue list --limit 20",
		"gh issue list --author someone",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhIssueStatus(t *testing.T) {
	safe := []string{
		"gh issue status",
		"gh issue status --json title",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhRepoView(t *testing.T) {
	safe := []string{
		"gh repo view",
		"gh repo view owner/repo",
		"gh repo view --json name",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"gh repo view host/owner/repo", // 3 segments = HOST/OWNER/REPO
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhRunList(t *testing.T) {
	safe := []string{
		"gh run list",
		"gh run list --branch main",
		"gh run list --status success",
		"gh run list --limit 10",
		"gh run list --workflow ci.yml",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhRunView(t *testing.T) {
	safe := []string{
		"gh run view 123",
		"gh run view --log",
		"gh run view --json status",
		"gh run view --log-failed",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhAuthStatus(t *testing.T) {
	safe := []string{
		"gh auth status",
		"gh auth status --active",
		"gh auth status --hostname github.com",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhReleaseList(t *testing.T) {
	safe := []string{
		"gh release list",
		"gh release list --limit 10",
		"gh release list --json tagName",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhReleaseView(t *testing.T) {
	safe := []string{
		"gh release view v1.0",
		"gh release view --json tagName",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhWorkflowList(t *testing.T) {
	safe := []string{
		"gh workflow list",
		"gh workflow list --all",
		"gh workflow list --limit 10",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhWorkflowView(t *testing.T) {
	safe := []string{
		"gh workflow view ci.yml",
		"gh workflow view --yaml",
		"gh workflow view --ref main ci.yml",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhLabelList(t *testing.T) {
	safe := []string{
		"gh label list",
		"gh label list --search bug",
		"gh label list --limit 20",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhSearchRepos(t *testing.T) {
	safe := []string{
		"gh search repos 'my query'",
		"gh search repos --language go",
		"gh search repos --limit 10",
		"gh search repos --sort stars",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhSearchIssues(t *testing.T) {
	safe := []string{
		"gh search issues 'query'",
		"gh search issues --state open",
		"gh search issues --label bug",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhSearchPrs(t *testing.T) {
	safe := []string{
		"gh search prs 'query'",
		"gh search prs --state open",
		"gh search prs --base main",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhSearchCommits(t *testing.T) {
	safe := []string{
		"gh search commits 'fix'",
		"gh search commits --author someone",
		"gh search commits --limit 10",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhSearchCode(t *testing.T) {
	safe := []string{
		"gh search code 'pattern'",
		"gh search code --language go",
		"gh search code --limit 10",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GhNotAllowed(t *testing.T) {
	// Non-read-only gh commands should be blocked
	dangerous := []string{
		"gh pr create",
		"gh pr merge",
		"gh pr close",
		"gh issue create",
		"gh repo create",
		"gh release create",
		"gh workflow run",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// System tools (from TypeScript COMMAND_ALLOWLIST, missing from Go)
// ---------------------------------------------------------------------------

func TestAllowlist_Netstat(t *testing.T) {
	safe := []string{
		"netstat",
		"netstat -an",
		"netstat -r",
		"netstat -s",
		"netstat -i",
		"netstat -l",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Ss(t *testing.T) {
	safe := []string{
		"ss",
		"ss -tlnp",
		"ss -an",
		"ss --summary",
		"ss -4 -t",
		"ss -6 -u",
		"ss --listening",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Fd(t *testing.T) {
	safe := []string{
		"fd pattern",
		"fd -t f pattern",
		"fd --type d pattern src/",
		"fd -e go pattern",
		"fd --hidden pattern",
		"fd --no-ignore pattern",
		"fd -H pattern",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Fdfind(t *testing.T) {
	safe := []string{
		"fdfind pattern",
		"fdfind -t f pattern",
		"fdfind --hidden pattern",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Pyright(t *testing.T) {
	safe := []string{
		"pyright",
		"pyright src/",
		"pyright --outputjson",
		"pyright --version",
		"pyright --stats",
		"pyright -p pyrightconfig.json",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	// --watch/-w launches file watcher (long-running, not read-only)
	dangerous := []string{
		"pyright --watch",
		"pyright -w",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only (watch mode): %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Network tools
// ---------------------------------------------------------------------------

func TestAllowlist_Curl(t *testing.T) {
	safe := []string{
		"curl https://api.example.com",
		"curl -s https://api.example.com",
		"curl -I https://example.com",
		"curl -L https://example.com",
		"curl -H 'Authorization: Bearer token' https://api.example.com",
		"curl --silent --max-time 5 https://example.com",
		"curl -v https://example.com",
		"curl -X POST -d '{}' https://api.example.com",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"curl -o output.txt https://example.com",
		"curl -O https://example.com/file.tar",
		"curl --output output.txt https://x.com",
		"curl --remote-name https://x.com/file",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Wget(t *testing.T) {
	safe := []string{
		"wget --spider https://example.com",
		"wget -q --spider https://example.com",
		"wget --server-response --spider https://example.com",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	// wget without --spider downloads files by default
	dangerous := []string{
		"wget https://example.com",
		"wget -O output.txt https://example.com",
		"wget -P /tmp https://example.com",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// JSON / YAML / text processing
// ---------------------------------------------------------------------------

func TestAllowlist_Jq(t *testing.T) {
	safe := []string{
		"jq '.' file.json",
		"jq '.name' package.json",
		"jq -r '.version' package.json",
		"jq -c '.' file.json",
		"jq --raw-output '.name' file.json",
		"jq -e '.key' file.json",
		"jq -s '.' a.json b.json",
		"jq --sort-keys '.' file.json",
		"jq --arg name val '.' file.json",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"jq -f filter.jq file.json",
		"jq --from-file filter.jq file",
		"jq --rawfile var file.json '.'",
		"jq --slurpfile var file.json '.'",
		"jq -L /path/to/libs '.'",
		"jq --library-path /libs '.'",
		"jq --run-tests",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Yq(t *testing.T) {
	safe := []string{
		"yq '.' file.yaml",
		"yq '.name' config.yaml",
		"yq -r '.version' config.yaml",
		"yq --output-format json '.' file.yaml",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Awk(t *testing.T) {
	safe := []string{
		"awk '{print $1}' file.txt",
		"awk -F: '{print $1}' /etc/passwd",
		"awk 'NR==1' file.txt",
		"awk '{sum+=$1} END{print sum}' file.txt",
		"awk -v OFS=, '{print $1,$2}' file.txt",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		`awk '{system("ls")}' file.txt`,
		`awk 'BEGIN{system("ls")}' file`,
		`awk '{print $1 | "cat"}' file.txt`,
		`awk '{print $1 > "/etc/passwd"}' file.txt`,
		`awk '{print $1 >> "output.txt"}' file.txt`,
		"awk -f script.awk file.txt",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Package managers (read-only subcommands only)
// ---------------------------------------------------------------------------

func TestAllowlist_NpmReadOnly(t *testing.T) {
	safe := []string{
		"npm list",
		"npm ls",
		"npm view react",
		"npm info react version",
		"npm search query",
		"npm outdated",
		"npm audit",
		"npm version",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"npm install",
		"npm install react",
		"npm uninstall react",
		"npm publish",
		"npm run build",
		"npm ci",
		"npm update",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_YarnReadOnly(t *testing.T) {
	safe := []string{
		"yarn list",
		"yarn info react",
		"yarn outdated",
		"yarn audit",
		"yarn version",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"yarn install",
		"yarn add react",
		"yarn remove react",
		"yarn publish",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_PnpmReadOnly(t *testing.T) {
	safe := []string{
		"pnpm list",
		"pnpm ls",
		"pnpm outdated",
		"pnpm audit",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"pnpm install",
		"pnpm add react",
		"pnpm remove react",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_CargoReadOnly(t *testing.T) {
	safe := []string{
		"cargo metadata",
		"cargo tree",
		"cargo search query",
		"cargo locate-project",
		"cargo pkgid",
		"cargo version",
		"cargo --version",
		"cargo metadata --format-version 1",
		"cargo tree --depth 3",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"cargo build",
		"cargo test",
		"cargo run",
		"cargo install",
		"cargo publish",
		"cargo clean",
		"cargo update",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Runtime version checks
// ---------------------------------------------------------------------------

func TestAllowlist_PythonVersionOnly(t *testing.T) {
	safe := []string{
		"python --version",
		"python3 --version",
		"python -V",
		"python3 -V",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"python script.py",
		"python -c 'print(1)'",
		"python -m pip install",
		"python3 -m http.server",
		"python foo.py arg1",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_RubyVersionOnly(t *testing.T) {
	safe := []string{
		"ruby --version",
		"ruby -v",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"ruby script.rb",
		"ruby -e 'puts 1'",
		"ruby -r open-uri -e 'open(\"http://evil.com\")'",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Archive tools (list-only mode)
// ---------------------------------------------------------------------------

func TestAllowlist_TarListOnly(t *testing.T) {
	safe := []string{
		"tar -tf archive.tar",
		"tar -tvf archive.tar",
		"tar --list --file archive.tar",
		"tar -tzf archive.tar.gz",
		"tar -tjf archive.tar.bz2",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"tar -xf archive.tar",
		"tar -xvf archive.tar -C /tmp",
		"tar -cf archive.tar dir/",
		"tar --extract -f archive.tar",
		"tar --create -f out.tar dir/",
		"tar -uf archive.tar newfile",
		"tar -rf archive.tar newfile",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestAllowlist_Unzip(t *testing.T) {
	safe := []string{
		"unzip -l archive.zip",
		"unzip -v archive.zip",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
	dangerous := []string{
		"unzip archive.zip",         // default extracts
		"unzip -o archive.zip",      // overwrite extract
		"unzip archive.zip -d /tmp", // extract to dir
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Missing git subcommands (ported from Claude Code's GIT_READ_ONLY_COMMANDS)
// ---------------------------------------------------------------------------

func TestAllowlist_GitShortlog(t *testing.T) {
	safe := []string{
		"git shortlog",
		"git shortlog -s",
		"git shortlog --summary",
		"git shortlog -n",
		"git shortlog --numbered",
		"git shortlog -e",
		"git shortlog --email",
		"git shortlog --author=alice",
		"git shortlog --format=%s",
		"git shortlog --no-merges",
		"git shortlog --since=2024-01-01",
		"git shortlog --group=author HEAD",
		"git shortlog -s -n main",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitRevList(t *testing.T) {
	safe := []string{
		"git rev-list HEAD",
		"git rev-list --count HEAD",
		"git rev-list --oneline HEAD",
		"git rev-list --max-count=10 HEAD",
		"git rev-list --since=2024-01-01 HEAD",
		"git rev-list --author=alice HEAD",
		"git rev-list --no-merges HEAD",
		"git rev-list --first-parent HEAD",
		"git rev-list --ancestry-path main..HEAD",
		"git rev-list --reverse HEAD",
		"git rev-list --all",
		"git rev-list --branches --remotes",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitDescribe(t *testing.T) {
	safe := []string{
		"git describe",
		"git describe HEAD",
		"git describe --tags",
		"git describe --long",
		"git describe --always",
		"git describe --abbrev=8",
		"git describe --contains",
		"git describe --exact-match",
		"git describe --match v*",
		"git describe --exclude rc*",
		"git describe --dirty",
		"git describe --first-match",
		"git describe --candidates=10",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitCatFile(t *testing.T) {
	safe := []string{
		"git cat-file -t HEAD",
		"git cat-file -s HEAD",
		"git cat-file -p HEAD",
		"git cat-file -e HEAD:file.go",
		"git cat-file --batch-check",
		"git cat-file --allow-undetermined-type -t HEAD",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

func TestAllowlist_GitForEachRef(t *testing.T) {
	safe := []string{
		"git for-each-ref",
		"git for-each-ref refs/heads/",
		"git for-each-ref --format=%(refname)",
		"git for-each-ref --sort=-creatordate",
		"git for-each-ref --count=10",
		"git for-each-ref --contains=HEAD",
		"git for-each-ref --no-contains=main",
		"git for-each-ref --merged=HEAD",
		"git for-each-ref --no-merged=main",
		"git for-each-ref --points-at=HEAD",
		"git for-each-ref --format=%(refname:short) --sort=-version:refname refs/tags/",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}
