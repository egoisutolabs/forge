package bash

// parity_test.go — Security regression tests for BashTool Go/TypeScript parity gaps.
//
// These tests document behaviors present in the TypeScript reference implementation
// that are missing in the Go port. Tests marked "regression" FAIL on the current
// code and PASS once the corresponding fix lands. See PARITY_REPORT.md for details.
//
// Reference: claudecode/tools/BashTool/readOnlyValidation.ts (checkReadOnlyConstraints)
//            claudecode/utils/shell/readOnlyCommandValidation.ts (COMMAND_ALLOWLIST)
//            claudecode/tools/BashTool/sedValidation.ts (sedCommandIsAllowedByAllowlist)

import "testing"

// ---------------------------------------------------------------------------
// Gap 1: git reflog — missing expire/delete/exists blocking
//
// TypeScript: GIT_READ_ONLY_COMMANDS has IsDangerous callback blocking
// "expire", "delete", "exists" subactions.
// Go: readOnlyGitSubcommands["reflog"] = true (any flags allowed — regression).
// ---------------------------------------------------------------------------

func TestParity_GitReflog_ShowIsReadOnly(t *testing.T) {
	safe := []string{
		"git reflog",
		"git reflog show HEAD",
		"git reflog show main",
		"git reflog --oneline",
		"git reflog -n 10",
		"git reflog show stash@{0}",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
func TestParity_GitReflog_DangerousSubactionsNotReadOnly(t *testing.T) {
	dangerous := []string{
		"git reflog expire",
		"git reflog expire --all",
		"git reflog expire --expire=now --all",
		"git reflog expire --expire=now HEAD",
		"git reflog delete HEAD@{0}",
		"git reflog delete refs/heads/main@{0}",
		"git reflog exists HEAD",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 2: git config — missing write-operation blocking
//
// TypeScript: only "git config --get" form is in GIT_READ_ONLY_COMMANDS.
// Go: commandAllowlist["git config"] has read flags but validateFlags allows
// positional args (write operations) since IsDangerous callback is missing.
// ---------------------------------------------------------------------------

func TestParity_GitConfig_ReadOnlyForms(t *testing.T) {
	safe := []string{
		"git config --get user.email",
		"git config --get-all user.email",
		"git config --list",
		"git config -l",
		"git config --get core.editor",
		"git config --get-regexp remote.*",
		"git config --local --get user.name",
		"git config --global --list",
		"git config --show-origin --get user.email",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
func TestParity_GitConfig_WriteFormsNotReadOnly(t *testing.T) {
	dangerous := []string{
		"git config user.email attacker@evil.com",
		"git config user.name Evil",
		"git config core.hooksPath /tmp/evil-hooks",
		"git config --global user.email attacker@evil.com",
		"git config --local core.editor rm",
		"git config alias.status '!evil.sh'",
		"git config receive.fsckObjects false",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 3: date — positional args set system time
//
// TypeScript: date is in COMMAND_ALLOWLIST with IsDangerous blocking positional args.
// Go: date is in readOnlyCommands (any flags OK, only $ blocked — regression).
// ---------------------------------------------------------------------------

func TestParity_Date_DisplayFormsReadOnly(t *testing.T) {
	safe := []string{
		"date",
		"date -R",
		"date -u",
		"date +%Y-%m-%d",
		"date +%s",
		"date -d 'yesterday' +%Y-%m-%d",
		"date --iso-8601",
		"date --rfc-2822",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
func TestParity_Date_SetTimeNotReadOnly(t *testing.T) {
	dangerous := []string{
		"date 1231235959",   // POSIX MMDDhhmm[YY] form — sets system time
		"date 12312359",     // shorter form
		"date 123123592025", // POSIX with year
		"date --set now",
		"date --set='2025-01-01 00:00:00'",
		"date -s '2025-01-01'",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 4: hostname — positional args change system hostname
//
// TypeScript: hostname is in COMMAND_ALLOWLIST with IsDangerous blocking positional args.
// Go: hostname is in readOnlyCommands (any flags OK — regression).
// ---------------------------------------------------------------------------

func TestParity_Hostname_DisplayFormsReadOnly(t *testing.T) {
	safe := []string{
		"hostname",
		"hostname -f",
		"hostname -s",
		"hostname -d",
		"hostname -i",
		"hostname -A",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
func TestParity_Hostname_SetHostnameNotReadOnly(t *testing.T) {
	dangerous := []string{
		"hostname evil.internal",
		"hostname newhost",
		"hostname attacker.domain.com",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 5: tput — dangerous capabilities execute terminfo programs
//
// TypeScript: tput is in COMMAND_ALLOWLIST with IsDangerous blocking init/reset/isgr0.
// Go: tput is in readOnlyCommands (any flags OK — regression).
// ---------------------------------------------------------------------------

func TestParity_Tput_SafeCapabilitiesReadOnly(t *testing.T) {
	safe := []string{
		"tput cols",
		"tput lines",
		"tput colors",
		"tput clear",
		"tput bold",
		"tput sgr0",
		"tput setaf 1",
		"tput cup 10 20",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
// "tput init" and "tput reset" execute terminfo is1/is2/is3/rs1/rs2/rs3 sequences
// which can execute arbitrary programs from $TERMINFO.
func TestParity_Tput_DangerousCapabilitiesNotReadOnly(t *testing.T) {
	dangerous := []string{
		"tput init",
		"tput reset",
		"tput isgr0",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 6: lsof +m — creates mount supplement file on disk
//
// TypeScript: lsof is in COMMAND_ALLOWLIST with IsDangerous blocking +m.
// Go: lsof is in readOnlyCommands (any flags OK; +m starts with + not -, bypasses checks).
// ---------------------------------------------------------------------------

func TestParity_Lsof_SafeFormsReadOnly(t *testing.T) {
	safe := []string{
		"lsof",
		"lsof -i",
		"lsof -i :8080",
		"lsof -nP",
		"lsof -p 1234",
		"lsof -u root",
		"lsof /var/log/syslog",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
// "+m" creates a mount supplement file; "+m /path" creates the file at /path.
func TestParity_Lsof_MountSupplementNotReadOnly(t *testing.T) {
	dangerous := []string{
		"lsof +m",
		"lsof +m /tmp/mounts",
		"lsof +M",
		"lsof +M /tmp/mounts",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 7: tree -R — writes 00Tree.html files to disk
//
// TypeScript: tree is in COMMAND_ALLOWLIST blocking -R (and --fromfile).
// Go: tree is in readOnlyCommands (any flags OK — regression).
// ---------------------------------------------------------------------------

func TestParity_Tree_SafeFormsReadOnly(t *testing.T) {
	safe := []string{
		"tree",
		"tree -L 2",
		"tree -a",
		"tree --noreport",
		"tree -d",
		"tree -C",
		"tree -I 'node_modules'",
		"tree /tmp",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these currently return true but should return false.
// "tree -R" generates an HTML directory tree and writes 00Tree.html to disk.
func TestParity_Tree_RecursiveHTMLNotReadOnly(t *testing.T) {
	dangerous := []string{
		"tree -R",
		"tree -R /path",
		"tree --fromfile",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: expected NOT read-only, but was approved: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Gap 8: sed validation — missing denylist checks
//
// TypeScript: sedCommandIsAllowedByAllowlist() has 30+ denylist patterns.
// Go: sedExprIsDangerous() only checks w/W/e/E commands.
// These tests FAIL on the current code and PASS once task #4 lands.
// ---------------------------------------------------------------------------

func TestParity_Sed_SafeSubstitutionsReadOnly(t *testing.T) {
	safe := []string{
		"sed 's/foo/bar/' file",
		"sed 's/foo/bar/g' file",
		"sed -n 's/foo/bar/p' file",
		"sed -n '1p' file",
		"sed -n '1,5p' file",
		"sed -n '/pattern/p' file",
		"sed 's/a/b/i' file", // case-insensitive flag
		"sed 's/a/b/2' file", // second occurrence
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}

// REGRESSION: these all currently return true but should return false.
func TestParity_Sed_CurlyBraceBlocksNotReadOnly(t *testing.T) {
	dangerous := []string{
		"sed '{q}' file",
		"sed '1{p}' file",
		"sed '/pattern/{d}' file",
		"sed '{d}' file",
		"sed -e '{q}' file",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: curly brace blocks should not be read-only: %q", cmd)
		}
	}
}

// REGRESSION: tilde step addresses are a GNU sed extension with dangerous semantics.
func TestParity_Sed_TildeStepAddressesNotReadOnly(t *testing.T) {
	dangerous := []string{
		"sed '1~2p' file",
		"sed '0~3d' file",
		"sed '2~1p' file",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: tilde step addresses should not be read-only: %q", cmd)
		}
	}
}

// REGRESSION: negation with ! can bypass pattern guards.
func TestParity_Sed_NegationNotReadOnly(t *testing.T) {
	dangerous := []string{
		"sed '/x/! d' file",
		"sed '1! q' file",
		"sed '/pattern/! p' file",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: negation (!) should not be read-only: %q", cmd)
		}
	}
}

// REGRESSION: y command transliterates characters (same power as tr but in sed).
func TestParity_Sed_YCommandNotReadOnly(t *testing.T) {
	dangerous := []string{
		"sed 'y/abc/ABC/' file",
		"sed 'y/aeiou/AEIOU/' file",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: y (transliterate) command should not be read-only: %q", cmd)
		}
	}
}

// REGRESSION: s/// with w or e flags — writes to file or executes replacement as shell.
func TestParity_Sed_DangerousSubstitutionFlagsNotReadOnly(t *testing.T) {
	dangerous := []string{
		"sed 's/a/b/w /tmp/out' file", // w flag writes matches to file
		"sed 's/a/b/e' file",          // e flag executes replacement as shell command
		"sed 's/a/b/W /tmp/out' file", // W flag (GNU sed)
		"sed 's/a/b/E' file",          // E flag (GNU sed)
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: s/// with w/e/W/E flag should not be read-only: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Compound command regression tests
// ---------------------------------------------------------------------------

// Verify that safe+dangerous compounds are not read-only.
func TestParity_CompoundCommands_MixedNotReadOnly(t *testing.T) {
	dangerous := []string{
		"git log --oneline && git config user.email x@y.com",
		"date && hostname newname",
		"git reflog && git reflog expire --all",
		"ls && tput reset",
		"cat file.txt && lsof +m",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("REGRESSION: mixed compound should not be read-only: %q", cmd)
		}
	}
}

// Verify that safe+safe compounds remain read-only.
func TestParity_CompoundCommands_SafeRemainReadOnly(t *testing.T) {
	safe := []string{
		"git log --oneline && git status",
		"date && hostname",
		"cat file.txt | wc -l",
		"git diff | grep '^+'",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only compound: %q", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Regression: git stash write operations should not be read-only
// (These test existing behavior but confirm the stash guard works correctly.)
// ---------------------------------------------------------------------------

func TestParity_GitStash_WriteOperationsNotReadOnly(t *testing.T) {
	dangerous := []string{
		"git stash",
		"git stash push",
		"git stash pop",
		"git stash drop",
		"git stash drop stash@{0}",
		"git stash clear",
		"git stash apply",
		"git stash branch newbranch",
	}
	for _, cmd := range dangerous {
		if IsReadOnly(cmd) {
			t.Errorf("expected NOT read-only: %q", cmd)
		}
	}
}

func TestParity_GitStash_ReadOperationsAreReadOnly(t *testing.T) {
	safe := []string{
		"git stash list",
		"git stash show",
		"git stash show -p",
		"git stash show stash@{0}",
		"git stash show --stat",
	}
	for _, cmd := range safe {
		if !IsReadOnly(cmd) {
			t.Errorf("expected read-only: %q", cmd)
		}
	}
}
