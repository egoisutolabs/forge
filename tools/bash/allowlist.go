package bash

import (
	"regexp"
	"strings"
)

// FlagArgType describes what argument a flag expects.
type FlagArgType string

const (
	FlagNone   FlagArgType = "none"   // flag takes no argument
	FlagString FlagArgType = "string" // flag takes a string argument
	FlagNumber FlagArgType = "number" // flag takes a numeric argument
	FlagChar   FlagArgType = "char"   // flag takes a single character
	FlagBraces FlagArgType = "{}"     // flag must be literally "{}"
	FlagEOF    FlagArgType = "EOF"    // flag must be literally "EOF"
)

// CommandConfig defines the safe flags and optional validators for a command.
type CommandConfig struct {
	SafeFlags          map[string]FlagArgType
	RespectsDoubleDash bool // default true
	// IsDangerous is an optional callback for additional validation.
	// Returns true if the command is dangerous given its raw tokens.
	IsDangerous func(tokens []string) bool
}

// commandAllowlist maps command names to their safe flag configurations.
// Derived from Claude Code's COMMAND_ALLOWLIST in readOnlyCommandValidation.ts.
//
// If a command is in this list, it can be auto-approved IF all its flags
// pass validation against the SafeFlags map.
var commandAllowlist = map[string]CommandConfig{
	// --- grep family ---
	"grep": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagString, "--regexp": FlagString,
			"-f": FlagString, "--file": FlagString,
			"-F": FlagNone, "-G": FlagNone, "-E": FlagNone, "-P": FlagNone,
			"-i": FlagNone, "-v": FlagNone, "-w": FlagNone, "-x": FlagNone,
			"-c": FlagNone, "--color": FlagString, "-L": FlagNone, "-l": FlagNone,
			"-m": FlagNumber, "-o": FlagNone, "-q": FlagNone, "-s": FlagNone,
			"-b": FlagNone, "-H": FlagNone, "-h": FlagNone, "--label": FlagString,
			"-n": FlagNone, "-T": FlagNone, "-u": FlagNone, "-Z": FlagNone, "-z": FlagNone,
			"-A": FlagNumber, "-B": FlagNumber, "-C": FlagNumber,
			"-a": FlagNone, "--binary-files": FlagString, "-D": FlagString,
			"-d": FlagString, "--exclude": FlagString, "--exclude-from": FlagString,
			"--exclude-dir": FlagString, "--include": FlagString,
			"-r": FlagNone, "-R": FlagNone,
			"--help": FlagNone, "-V": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"egrep": {SafeFlags: map[string]FlagArgType{ // same as grep
		"-e": FlagString, "-f": FlagString, "-F": FlagNone, "-E": FlagNone,
		"-i": FlagNone, "-v": FlagNone, "-w": FlagNone, "-x": FlagNone,
		"-c": FlagNone, "-l": FlagNone, "-n": FlagNone, "-o": FlagNone,
		"-q": FlagNone, "-s": FlagNone, "-h": FlagNone, "-H": FlagNone,
		"-A": FlagNumber, "-B": FlagNumber, "-C": FlagNumber,
		"-r": FlagNone, "-R": FlagNone, "--color": FlagString,
	}, RespectsDoubleDash: true},
	"fgrep": {SafeFlags: map[string]FlagArgType{
		"-e": FlagString, "-f": FlagString, "-i": FlagNone, "-v": FlagNone,
		"-c": FlagNone, "-l": FlagNone, "-n": FlagNone, "-o": FlagNone,
		"-q": FlagNone, "-h": FlagNone, "-H": FlagNone,
		"-A": FlagNumber, "-B": FlagNumber, "-C": FlagNumber,
		"-r": FlagNone, "-R": FlagNone, "--color": FlagString,
	}, RespectsDoubleDash: true},

	// --- ripgrep ---
	"rg": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagString, "--regexp": FlagString, "-f": FlagString,
			"-i": FlagNone, "-S": FlagNone, "-F": FlagNone, "-w": FlagNone, "-v": FlagNone,
			"-c": FlagNone, "-l": FlagNone, "--files-without-match": FlagNone,
			"-n": FlagNone, "-o": FlagNone,
			"-A": FlagNumber, "-B": FlagNumber, "-C": FlagNumber,
			"-H": FlagNone, "-h": FlagNone, "--heading": FlagNone, "-q": FlagNone,
			"--column": FlagNone,
			"-g":       FlagString, "-t": FlagString, "-T": FlagString, "--type": FlagString, "--type-not": FlagString, "--type-list": FlagNone,
			"--hidden": FlagNone, "--no-ignore": FlagNone, "-u": FlagNone,
			"-m": FlagNumber, "-d": FlagNumber, "-a": FlagNone, "-z": FlagNone,
			"-L": FlagNone, "--follow": FlagNone,
			"--color": FlagString, "--json": FlagNone, "--stats": FlagNone,
			"--help": FlagNone, "--version": FlagNone, "--debug": FlagNone,
			"--": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// --- sed ---
	"sed": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagString, "--expression": FlagString,
			"-n": FlagNone, "--quiet": FlagNone, "--silent": FlagNone,
			"-r": FlagNone, "-E": FlagNone, "--regexp-extended": FlagNone,
			"--posix": FlagNone,
			"-l":      FlagNumber, "--line-length": FlagNumber,
			"-z": FlagNone, "--zero-terminated": FlagNone,
			"-s": FlagNone, "--separate": FlagNone,
			"-u": FlagNone, "--unbuffered": FlagNone,
			"--debug": FlagNone, "--help": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        sedIsDangerous,
	},

	// --- xargs ---
	"xargs": {
		SafeFlags: map[string]FlagArgType{
			"-I": FlagBraces, "-n": FlagNumber, "-P": FlagNumber,
			"-L": FlagNumber, "-s": FlagNumber, "-E": FlagEOF,
			"-0": FlagNone, "-t": FlagNone, "-r": FlagNone,
			"-x": FlagNone, "-d": FlagChar,
		},
		RespectsDoubleDash: true,
	},

	// --- sort ---
	"sort": {
		SafeFlags: map[string]FlagArgType{
			"-b": FlagNone, "-d": FlagNone, "-f": FlagNone, "-g": FlagNone,
			"-h": FlagNone, "-i": FlagNone, "-M": FlagNone, "-n": FlagNone,
			"-R": FlagNone, "-r": FlagNone, "--sort": FlagString, "-s": FlagNone,
			"-u": FlagNone, "-V": FlagNone, "-z": FlagNone,
			"--key": FlagString, "-k": FlagString,
			"--field-separator": FlagString, "-t": FlagString,
			"--check": FlagNone, "-c": FlagNone, "-C": FlagNone,
			"--merge": FlagNone, "-m": FlagNone,
			"--buffer-size": FlagString, "-S": FlagString,
			"--parallel": FlagNumber, "--batch-size": FlagNumber,
			"--help": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// --- ps ---
	"ps": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagNone, "-A": FlagNone, "-a": FlagNone, "-d": FlagNone,
			"-N": FlagNone, "--deselect": FlagNone,
			"-f": FlagNone, "-F": FlagNone, "-l": FlagNone, "-j": FlagNone, "-y": FlagNone,
			"-w": FlagNone, "-ww": FlagNone, "--width": FlagNumber, "-c": FlagNone,
			"-H": FlagNone, "--forest": FlagNone, "--headers": FlagNone,
			"--no-headers": FlagNone, "-n": FlagString, "--sort": FlagString,
			"-L": FlagNone, "-T": FlagNone, "-m": FlagNone,
			"-C": FlagString, "-G": FlagString, "-g": FlagString, "-p": FlagString,
			"--pid": FlagString, "-q": FlagString, "--quick-pid": FlagString,
			"-s": FlagString, "--sid": FlagString, "-t": FlagString, "--tty": FlagString,
			"-U": FlagString, "-u": FlagString, "--user": FlagString,
			"--help": FlagNone, "--info": FlagNone, "-V": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// --- git subcommands ---
	"git status": {
		SafeFlags: map[string]FlagArgType{
			"--short": FlagNone, "-s": FlagNone, "--branch": FlagNone, "-b": FlagNone,
			"--porcelain": FlagNone, "--long": FlagNone, "--verbose": FlagNone, "-v": FlagNone,
			"--untracked-files": FlagString, "-u": FlagString,
			"--ignored": FlagNone, "--ignore-submodules": FlagString,
			"--column": FlagNone, "--no-column": FlagNone,
			"--ahead-behind": FlagNone, "--no-ahead-behind": FlagNone,
			"--renames": FlagNone, "--no-renames": FlagNone,
			"--find-renames": FlagString, "-M": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"git diff": {
		SafeFlags: map[string]FlagArgType{
			"--stat": FlagNone, "--numstat": FlagNone, "--shortstat": FlagNone,
			"--name-only": FlagNone, "--name-status": FlagNone,
			"--dirstat": FlagNone, "--summary": FlagNone,
			"--word-diff": FlagNone, "--word-diff-regex": FlagString,
			"--color-words": FlagNone, "--no-renames": FlagNone, "--check": FlagNone,
			"--full-index": FlagNone, "--binary": FlagNone,
			"--abbrev": FlagNumber, "--break-rewrites": FlagNone,
			"--find-renames": FlagNone, "--find-copies": FlagNone,
			"--diff-algorithm": FlagString, "--histogram": FlagNone, "--patience": FlagNone,
			"--minimal":             FlagNone,
			"--ignore-space-at-eol": FlagNone, "--ignore-space-change": FlagNone,
			"--ignore-all-space": FlagNone, "--ignore-blank-lines": FlagNone,
			"--inter-hunk-context": FlagNumber, "--function-context": FlagNone,
			"--exit-code": FlagNone, "--quiet": FlagNone,
			"--cached": FlagNone, "--staged": FlagNone,
			"--no-index": FlagNone, "--relative": FlagString,
			"--diff-filter": FlagString,
			"-p":            FlagNone, "-u": FlagNone, "-s": FlagNone,
			"-M": FlagNone, "-C": FlagNone, "-B": FlagNone, "-D": FlagNone, "-l": FlagNone,
			"-S": FlagString, "-G": FlagString, "-O": FlagString, "-R": FlagNone,
			"--color": FlagNone, "--no-color": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"git log": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagNone, "--graph": FlagNone, "--decorate": FlagNone,
			"--no-decorate": FlagNone, "--date": FlagString, "--relative-date": FlagNone,
			"--all": FlagNone, "--branches": FlagNone, "--tags": FlagNone, "--remotes": FlagNone,
			"--since": FlagString, "--after": FlagString, "--until": FlagString, "--before": FlagString,
			"--max-count": FlagNumber, "-n": FlagNumber,
			"--stat": FlagNone, "--numstat": FlagNone, "--shortstat": FlagNone,
			"--name-only": FlagNone, "--name-status": FlagNone,
			"--color": FlagNone, "--no-color": FlagNone,
			"--patch": FlagNone, "-p": FlagNone, "--no-patch": FlagNone, "-s": FlagNone,
			"--abbrev-commit": FlagNone, "--full-history": FlagNone,
			"--first-parent": FlagNone, "--merges": FlagNone, "--no-merges": FlagNone,
			"--reverse": FlagNone, "--walk-reflogs": FlagNone,
			"--skip": FlagNumber, "--follow": FlagNone,
			"--topo-order": FlagNone, "--date-order": FlagNone, "--author-date-order": FlagNone,
			"--pretty": FlagString, "--format": FlagString,
			"--diff-filter": FlagString, "--author": FlagString,
			"--committer": FlagString, "--grep": FlagString,
			"-S": FlagString, "-G": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"git show": {
		SafeFlags: map[string]FlagArgType{
			"--stat": FlagNone, "--numstat": FlagNone, "--shortstat": FlagNone,
			"--name-only": FlagNone, "--name-status": FlagNone,
			"--pretty": FlagString, "--format": FlagString,
			"--abbrev-commit": FlagNone, "--no-patch": FlagNone, "-s": FlagNone,
			"-p": FlagNone, "--patch": FlagNone,
			"--color": FlagNone, "--no-color": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"git blame": {
		SafeFlags: map[string]FlagArgType{
			"--color": FlagNone, "--no-color": FlagNone, "-L": FlagString,
			"--porcelain": FlagNone, "-p": FlagNone, "--line-porcelain": FlagNone,
			"--root": FlagNone, "--show-stats": FlagNone,
			"--show-name": FlagNone, "--show-number": FlagNone, "-n": FlagNone,
			"--show-email": FlagNone, "-e": FlagNone, "-f": FlagNone,
			"--date": FlagString, "-w": FlagNone,
			"--ignore-rev": FlagString, "--ignore-revs-file": FlagString,
			"-M": FlagNone, "-C": FlagNone, "--abbrev": FlagNumber,
			"-s": FlagNone, "-l": FlagNone, "-t": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"git branch": {
		SafeFlags: map[string]FlagArgType{
			"-l": FlagNone, "--list": FlagNone, "-a": FlagNone, "--all": FlagNone,
			"-r": FlagNone, "--remotes": FlagNone, "-v": FlagNone, "-vv": FlagNone,
			"--verbose": FlagNone, "--color": FlagNone, "--no-color": FlagNone,
			"--column": FlagNone, "--no-column": FlagNone, "--abbrev": FlagNumber,
			"--no-abbrev": FlagNone, "--contains": FlagString,
			"--no-contains": FlagString, "--merged": FlagNone, "--no-merged": FlagNone,
			"--points-at": FlagString, "--sort": FlagString, "--show-current": FlagNone,
			"-i": FlagNone, "--ignore-case": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous: func(tokens []string) bool {
			// Blocks branch creation: git branch <name> is positional → dangerous
			for _, t := range tokens {
				if t == "--" {
					break
				}
				if len(t) > 0 && t[0] != '-' {
					return true // positional arg = branch creation
				}
			}
			return false
		},
	},
	"git tag": {
		SafeFlags: map[string]FlagArgType{
			"-l": FlagNone, "--list": FlagNone, "-n": FlagNumber,
			"--contains": FlagString, "--no-contains": FlagString,
			"--merged": FlagString, "--no-merged": FlagString,
			"--sort": FlagString, "--format": FlagString, "--points-at": FlagString,
			"--column": FlagNone, "--no-column": FlagNone,
			"-i": FlagNone, "--ignore-case": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous: func(tokens []string) bool {
			// Blocks tag creation: positional args after flags = tag creation
			for _, t := range tokens {
				if t == "--" {
					break
				}
				if len(t) > 0 && t[0] != '-' {
					return true
				}
			}
			return false
		},
	},
	"git ls-files": {
		SafeFlags: map[string]FlagArgType{
			"--cached": FlagNone, "-c": FlagNone, "--deleted": FlagNone, "-d": FlagNone,
			"--modified": FlagNone, "-m": FlagNone, "--others": FlagNone, "-o": FlagNone,
			"--ignored": FlagNone, "-i": FlagNone, "--stage": FlagNone, "-s": FlagNone,
			"--killed": FlagNone, "-k": FlagNone, "--unmerged": FlagNone, "-u": FlagNone,
			"--directory": FlagNone, "--no-empty-directory": FlagNone, "--eol": FlagNone,
			"--full-name": FlagNone, "--abbrev": FlagNumber, "--debug": FlagNone,
			"-z": FlagNone, "-t": FlagNone, "-v": FlagNone, "-f": FlagNone,
			"--exclude": FlagString, "-x": FlagString, "--exclude-from": FlagString,
			"-X": FlagString, "--exclude-per-directory": FlagString,
			"--exclude-standard": FlagNone, "--error-unmatch": FlagNone,
			"--recurse-submodules": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"git remote": {
		SafeFlags: map[string]FlagArgType{
			"-v": FlagNone, "--verbose": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous: func(tokens []string) bool {
			// Bare "git remote -v" is safe, but positional args are not
			for _, t := range tokens {
				if len(t) > 0 && t[0] != '-' {
					return true
				}
			}
			return false
		},
	},
	"git rev-parse": {
		SafeFlags: map[string]FlagArgType{
			"--git-dir": FlagNone, "--git-common-dir": FlagNone,
			"--show-toplevel": FlagNone, "--show-cdup": FlagNone,
			"--show-prefix": FlagNone, "--is-inside-git-dir": FlagNone,
			"--is-inside-work-tree": FlagNone, "--is-bare-repository": FlagNone,
			"--short": FlagNone, "--verify": FlagNone, "--abbrev-ref": FlagNone,
			"--symbolic": FlagNone, "--symbolic-full-ref": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"git stash list": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagNone, "--graph": FlagNone, "--decorate": FlagNone,
			"--no-decorate": FlagNone, "--date": FlagString, "--relative-date": FlagNone,
			"--max-count": FlagNumber, "-n": FlagNumber,
		},
		RespectsDoubleDash: true,
	},
	"git config": {
		SafeFlags: map[string]FlagArgType{
			"--get": FlagNone, "--get-all": FlagNone, "--get-regexp": FlagNone,
			"--local": FlagNone, "--global": FlagNone, "--system": FlagNone,
			"--worktree": FlagNone, "--default": FlagString, "--type": FlagString,
			"--bool": FlagNone, "--int": FlagNone, "--bool-or-int": FlagNone,
			"--path": FlagNone, "--expiry-date": FlagNone, "-z": FlagNone,
			"--null": FlagNone, "--name-only": FlagNone,
			"--show-origin": FlagNone, "--show-scope": FlagNone,
			"-l": FlagNone, "--list": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        gitConfigIsDangerous,
	},

	// --- docker read-only ---
	"docker ps": {
		SafeFlags: map[string]FlagArgType{
			"-a": FlagNone, "--all": FlagNone, "-f": FlagString, "--filter": FlagString,
			"--format": FlagString, "-n": FlagNumber, "--last": FlagNumber,
			"-l": FlagNone, "--latest": FlagNone, "--no-trunc": FlagNone,
			"-q": FlagNone, "--quiet": FlagNone, "-s": FlagNone, "--size": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"docker logs": {
		SafeFlags: map[string]FlagArgType{
			"--follow": FlagNone, "-f": FlagNone, "--tail": FlagString, "-n": FlagString,
			"--timestamps": FlagNone, "-t": FlagNone, "--since": FlagString,
			"--until": FlagString, "--details": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"docker inspect": {
		SafeFlags: map[string]FlagArgType{
			"--format": FlagString, "-f": FlagString, "--type": FlagString,
			"--size": FlagNone, "-s": FlagNone,
		},
		RespectsDoubleDash: true,
	},
	"docker images": {
		SafeFlags: map[string]FlagArgType{
			"-a": FlagNone, "--all": FlagNone, "--digests": FlagNone,
			"-f": FlagString, "--filter": FlagString, "--format": FlagString,
			"--no-trunc": FlagNone, "-q": FlagNone, "--quiet": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// --- additional git subcommands ---

	// git reflog: safe for "show" / ref display, dangerous for expire/delete/exists.
	// SECURITY: "reflog expire" and "reflog delete" write to .git/logs/**. These
	// must be blocked via IsDangerous; they would otherwise pass flag validation
	// because --all and --expire are in the ref-selection/date-filter flag sets.
	"git reflog": {
		SafeFlags: map[string]FlagArgType{
			"--oneline": FlagNone, "--graph": FlagNone, "--decorate": FlagNone,
			"--no-decorate": FlagNone, "--date": FlagString, "--relative-date": FlagNone,
			"--max-count": FlagNumber, "-n": FlagNumber,
			"--all": FlagNone, "--branches": FlagNone, "--tags": FlagNone, "--remotes": FlagNone,
			"--since": FlagString, "--after": FlagString, "--until": FlagString, "--before": FlagString,
			"--author": FlagString, "--committer": FlagString, "--grep": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        gitReflogIsDangerous,
	},

	// git ls-remote: read-only ref listing from a remote.
	// SECURITY: URL/SSH specs in the remote argument can exfiltrate data via DNS/HTTP.
	// --server-option/-o are intentionally excluded (transmit data to remote).
	"git ls-remote": {
		SafeFlags: map[string]FlagArgType{
			"--branches": FlagNone, "-b": FlagNone,
			"--tags": FlagNone, "-t": FlagNone,
			"--heads": FlagNone, "-h": FlagNone,
			"--refs":  FlagNone,
			"--quiet": FlagNone, "-q": FlagNone,
			"--exit-code": FlagNone,
			"--get-url":   FlagNone,
			"--symref":    FlagNone,
			"--sort":      FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        gitLsRemoteIsDangerous,
	},

	// git merge-base: read-only common-ancestor queries.
	"git merge-base": {
		SafeFlags: map[string]FlagArgType{
			"--is-ancestor": FlagNone,
			"--fork-point":  FlagNone,
			"--octopus":     FlagNone,
			"--independent": FlagNone,
			"--all":         FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// git grep: read-only text search over tracked files.
	"git grep": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagString,
			"-E": FlagNone, "--extended-regexp": FlagNone,
			"-G": FlagNone, "--basic-regexp": FlagNone,
			"-F": FlagNone, "--fixed-strings": FlagNone,
			"-P": FlagNone, "--perl-regexp": FlagNone,
			"-i": FlagNone, "--ignore-case": FlagNone,
			"-v": FlagNone, "--invert-match": FlagNone,
			"-w": FlagNone, "--word-regexp": FlagNone,
			"-n": FlagNone, "--line-number": FlagNone,
			"-c": FlagNone, "--count": FlagNone,
			"-l": FlagNone, "--files-with-matches": FlagNone,
			"-L": FlagNone, "--files-without-match": FlagNone,
			"-h": FlagNone, "-H": FlagNone,
			"--heading": FlagNone, "--break": FlagNone,
			"--full-name": FlagNone,
			"--color":     FlagNone, "--no-color": FlagNone,
			"-o": FlagNone, "--only-matching": FlagNone,
			"-A": FlagNumber, "--after-context": FlagNumber,
			"-B": FlagNumber, "--before-context": FlagNumber,
			"-C": FlagNumber, "--context": FlagNumber,
			"--and": FlagNone, "--or": FlagNone, "--not": FlagNone,
			"--max-depth":          FlagNumber,
			"--untracked":          FlagNone,
			"--no-index":           FlagNone,
			"--recurse-submodules": FlagNone,
			"--cached":             FlagNone,
			"--threads":            FlagNumber,
			"-q":                   FlagNone, "--quiet": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// git stash show: read-only diff of a stash entry.
	"git stash show": {
		SafeFlags: map[string]FlagArgType{
			"--stat": FlagNone, "--numstat": FlagNone, "--shortstat": FlagNone,
			"--name-only": FlagNone, "--name-status": FlagNone,
			"--color": FlagNone, "--no-color": FlagNone,
			"--patch": FlagNone, "-p": FlagNone, "--no-patch": FlagNone,
			"--no-ext-diff": FlagNone, "-s": FlagNone,
			"--word-diff": FlagNone, "--word-diff-regex": FlagString,
			"--diff-filter": FlagString, "--abbrev": FlagNumber,
		},
		RespectsDoubleDash: true,
	},

	// git worktree list: read-only listing of linked working trees.
	// Dispatched via the 3-word git lookup in isGitCommandReadOnly.
	"git worktree list": {
		SafeFlags: map[string]FlagArgType{
			"--porcelain": FlagNone,
			"-v":          FlagNone, "--verbose": FlagNone,
			"--expire": FlagString,
		},
		RespectsDoubleDash: true,
	},

	// git remote show: display info about a named remote (read-only).
	// SECURITY: requires exactly one well-formed remote name; no positional URLs.
	// Dispatched via the 3-word git lookup in isGitCommandReadOnly.
	"git remote show": {
		SafeFlags: map[string]FlagArgType{
			"-n": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        gitRemoteShowIsDangerous,
	},

	// git shortlog: read-only commit summary by author.
	// Mirrors Claude Code's GIT_READ_ONLY_COMMANDS['git shortlog'].
	"git shortlog": {
		SafeFlags: map[string]FlagArgType{
			// Ref selection (GIT_REF_SELECTION_FLAGS)
			"--all": FlagNone, "--branches": FlagNone, "--tags": FlagNone, "--remotes": FlagNone,
			// Date filter (GIT_DATE_FILTER_FLAGS)
			"--since": FlagString, "--after": FlagString, "--until": FlagString, "--before": FlagString,
			// Summary options
			"-s": FlagNone, "--summary": FlagNone,
			"-n": FlagNone, "--numbered": FlagNone,
			"-e": FlagNone, "--email": FlagNone,
			"-c": FlagNone, "--committer": FlagNone,
			// Grouping
			"--group": FlagString,
			// Formatting
			"--format": FlagString,
			// Filtering
			"--no-merges": FlagNone, "--author": FlagString,
		},
		RespectsDoubleDash: true,
	},

	// git rev-list: read-only commit enumeration — lists commits reachable from refs.
	// Mirrors Claude Code's GIT_READ_ONLY_COMMANDS['git rev-list'].
	"git rev-list": {
		SafeFlags: map[string]FlagArgType{
			// Ref selection (GIT_REF_SELECTION_FLAGS)
			"--all": FlagNone, "--branches": FlagNone, "--tags": FlagNone, "--remotes": FlagNone,
			// Date filter (GIT_DATE_FILTER_FLAGS)
			"--since": FlagString, "--after": FlagString, "--until": FlagString, "--before": FlagString,
			// Count (GIT_COUNT_FLAGS)
			"--max-count": FlagNumber, "-n": FlagNumber,
			// Author filter (GIT_AUTHOR_FILTER_FLAGS)
			"--author": FlagString, "--committer": FlagString, "--grep": FlagString,
			// Counting
			"--count": FlagNone,
			// Traversal
			"--reverse": FlagNone, "--first-parent": FlagNone, "--ancestry-path": FlagNone,
			"--merges": FlagNone, "--no-merges": FlagNone,
			"--min-parents": FlagNumber, "--max-parents": FlagNumber,
			"--no-min-parents": FlagNone, "--no-max-parents": FlagNone,
			"--skip": FlagNumber, "--max-age": FlagNumber, "--min-age": FlagNumber,
			"--walk-reflogs": FlagNone,
			// Output formatting
			"--oneline": FlagNone, "--abbrev-commit": FlagNone,
			"--pretty": FlagString, "--format": FlagString, "--abbrev": FlagNumber,
			"--full-history": FlagNone, "--dense": FlagNone, "--sparse": FlagNone,
			"--source": FlagNone, "--graph": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// git describe: read-only — describes commits relative to the most recent tag.
	// Mirrors Claude Code's GIT_READ_ONLY_COMMANDS['git describe'].
	"git describe": {
		SafeFlags: map[string]FlagArgType{
			// Tag selection
			"--tags": FlagNone, "--match": FlagString, "--exclude": FlagString,
			// Output control
			"--long": FlagNone, "--abbrev": FlagNumber, "--always": FlagNone,
			"--contains": FlagNone, "--first-match": FlagNone, "--exact-match": FlagNone,
			"--candidates": FlagNumber,
			// Dirty markers
			"--dirty": FlagNone, "--broken": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// git cat-file: read-only object inspection — displays type, size, or content of objects.
	// SECURITY: --batch (without --check) is intentionally excluded — reading arbitrary
	// objects from stdin in a pipe could dump sensitive git objects (private keys, etc).
	// Mirrors Claude Code's GIT_READ_ONLY_COMMANDS['git cat-file'].
	"git cat-file": {
		SafeFlags: map[string]FlagArgType{
			"-t":                        FlagNone, // print object type
			"-s":                        FlagNone, // print object size
			"-p":                        FlagNone, // pretty-print object contents
			"-e":                        FlagNone, // exit with zero if object exists
			"--batch-check":             FlagNone, // type+size only, no content
			"--allow-undetermined-type": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// git for-each-ref: read-only ref iteration with optional formatting and filtering.
	// Mirrors Claude Code's GIT_READ_ONLY_COMMANDS['git for-each-ref'].
	"git for-each-ref": {
		SafeFlags: map[string]FlagArgType{
			"--format":      FlagString, // %(fieldname) placeholders
			"--sort":        FlagString, // sort key (e.g. refname, creatordate, version:refname)
			"--count":       FlagNumber,
			"--contains":    FlagString,
			"--no-contains": FlagString,
			"--merged":      FlagString,
			"--no-merged":   FlagString,
			"--points-at":   FlagString,
		},
		RespectsDoubleDash: true,
	},

	// --- gh CLI read-only commands ---
	// All use ghIsDangerous to prevent HOST/OWNER/REPO exfiltration.
	// --web/-w flags intentionally excluded (open browser).
	// --show-token/-t excluded (leaks secrets).

	"gh pr view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString, "--comments": FlagNone,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh pr list": {
		SafeFlags: map[string]FlagArgType{
			"--state": FlagString, "-s": FlagString,
			"--author": FlagString, "--assignee": FlagString,
			"--label": FlagString, "--limit": FlagNumber, "-L": FlagNumber,
			"--base": FlagString, "--head": FlagString,
			"--search": FlagString, "--json": FlagString,
			"--draft": FlagNone, "--app": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh pr diff": {
		SafeFlags: map[string]FlagArgType{
			"--color": FlagString, "--name-only": FlagNone,
			"--patch": FlagNone,
			"--repo":  FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh pr checks": {
		SafeFlags: map[string]FlagArgType{
			"--watch": FlagNone, "--required": FlagNone, "--fail-fast": FlagNone,
			"--json": FlagString, "--interval": FlagNumber,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh pr status": {
		SafeFlags: map[string]FlagArgType{
			"--conflict-status": FlagNone, "-c": FlagNone,
			"--json": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh issue view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString, "--comments": FlagNone,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh issue list": {
		SafeFlags: map[string]FlagArgType{
			"--state": FlagString, "-s": FlagString,
			"--assignee": FlagString, "--author": FlagString,
			"--label": FlagString, "--limit": FlagNumber, "-L": FlagNumber,
			"--milestone": FlagString, "--search": FlagString,
			"--json": FlagString, "--app": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh issue status": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh repo view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh run list": {
		SafeFlags: map[string]FlagArgType{
			"--branch": FlagString, "-b": FlagString,
			"--status": FlagString, "-s": FlagString,
			"--workflow": FlagString, "-w": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--json": FlagString,
			"--repo": FlagString, "-R": FlagString,
			"--event": FlagString, "-e": FlagString,
			"--user": FlagString, "-u": FlagString,
			"--created": FlagString, "--commit": FlagString, "-c": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh run view": {
		SafeFlags: map[string]FlagArgType{
			"--log": FlagNone, "--log-failed": FlagNone,
			"--exit-status": FlagNone, "--verbose": FlagNone, "-v": FlagNone,
			"--json": FlagString,
			"--repo": FlagString, "-R": FlagString,
			"--job": FlagString, "-j": FlagString,
			"--attempt": FlagNumber, "-a": FlagNumber,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh auth status": {
		SafeFlags: map[string]FlagArgType{
			"--active": FlagNone, "-a": FlagNone,
			"--hostname": FlagString, "-h": FlagString,
			"--json": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh release list": {
		SafeFlags: map[string]FlagArgType{
			"--exclude-drafts": FlagNone, "--exclude-pre-releases": FlagNone,
			"--json": FlagString, "--limit": FlagNumber, "-L": FlagNumber,
			"--order": FlagString, "-O": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh release view": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh workflow list": {
		SafeFlags: map[string]FlagArgType{
			"--all": FlagNone, "-a": FlagNone,
			"--json": FlagString, "--limit": FlagNumber, "-L": FlagNumber,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh workflow view": {
		SafeFlags: map[string]FlagArgType{
			"--ref": FlagString, "-r": FlagString,
			"--yaml": FlagNone, "-y": FlagNone,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh label list": {
		SafeFlags: map[string]FlagArgType{
			"--json": FlagString, "--limit": FlagNumber, "-L": FlagNumber,
			"--order": FlagString, "--search": FlagString, "-S": FlagString,
			"--sort": FlagString,
			"--repo": FlagString, "-R": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        ghIsDangerous,
	},
	"gh search repos": {
		SafeFlags: map[string]FlagArgType{
			"--archived": FlagNone, "--created": FlagString,
			"--followers": FlagString, "--forks": FlagString,
			"--good-first-issues": FlagString, "--help-wanted-issues": FlagString,
			"--include-forks": FlagString, "--json": FlagString,
			"--language": FlagString, "--license": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--match": FlagString, "--number-topics": FlagString,
			"--order": FlagString, "--owner": FlagString,
			"--size": FlagString, "--sort": FlagString,
			"--stars": FlagString, "--topic": FlagString,
			"--updated": FlagString, "--visibility": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"gh search issues": {
		SafeFlags: map[string]FlagArgType{
			"--app": FlagString, "--assignee": FlagString, "--author": FlagString,
			"--closed": FlagString, "--commenter": FlagString, "--comments": FlagString,
			"--created": FlagString, "--include-prs": FlagNone,
			"--interactions": FlagString, "--involves": FlagString,
			"--json": FlagString, "--label": FlagString, "--language": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--locked": FlagNone, "--match": FlagString, "--mentions": FlagString,
			"--milestone": FlagString, "--no-assignee": FlagNone,
			"--no-label": FlagNone, "--no-milestone": FlagNone, "--no-project": FlagNone,
			"--order": FlagString, "--owner": FlagString, "--project": FlagString,
			"--reactions": FlagString, "--repo": FlagString, "-R": FlagString,
			"--sort": FlagString, "--state": FlagString,
			"--team-mentions": FlagString, "--updated": FlagString,
			"--visibility": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"gh search prs": {
		SafeFlags: map[string]FlagArgType{
			"--app": FlagString, "--assignee": FlagString, "--author": FlagString,
			"--base": FlagString, "-B": FlagString,
			"--checks": FlagString, "--closed": FlagString,
			"--commenter": FlagString, "--comments": FlagString, "--created": FlagString,
			"--draft": FlagNone, "--head": FlagString, "-H": FlagString,
			"--interactions": FlagString, "--involves": FlagString,
			"--json": FlagString, "--label": FlagString, "--language": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--locked": FlagNone, "--match": FlagString, "--mentions": FlagString,
			"--merged": FlagNone, "--merged-at": FlagString, "--milestone": FlagString,
			"--no-assignee": FlagNone, "--no-label": FlagNone,
			"--no-milestone": FlagNone, "--no-project": FlagNone,
			"--order": FlagString, "--owner": FlagString, "--project": FlagString,
			"--reactions": FlagString, "--repo": FlagString, "-R": FlagString,
			"--review": FlagString, "--review-requested": FlagString,
			"--reviewed-by": FlagString,
			"--sort":        FlagString, "--state": FlagString,
			"--team-mentions": FlagString, "--updated": FlagString,
			"--visibility": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"gh search commits": {
		SafeFlags: map[string]FlagArgType{
			"--author": FlagString, "--author-date": FlagString,
			"--author-email": FlagString, "--author-name": FlagString,
			"--committer": FlagString, "--committer-date": FlagString,
			"--committer-email": FlagString, "--committer-name": FlagString,
			"--hash": FlagString, "--json": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--merge": FlagNone, "--order": FlagString, "--owner": FlagString,
			"--parent": FlagString, "--repo": FlagString, "-R": FlagString,
			"--sort": FlagString, "--tree": FlagString,
			"--visibility": FlagString,
		},
		RespectsDoubleDash: true,
	},
	"gh search code": {
		SafeFlags: map[string]FlagArgType{
			"--extension": FlagString, "--filename": FlagString,
			"--json": FlagString, "--language": FlagString,
			"--limit": FlagNumber, "-L": FlagNumber,
			"--match": FlagString, "--owner": FlagString,
			"--repo": FlagString, "-R": FlagString,
			"--size": FlagString,
		},
		RespectsDoubleDash: true,
	},

	// --- system tools (from TypeScript COMMAND_ALLOWLIST, missing from Go) ---

	// netstat: read-only network statistics viewer.
	"netstat": {
		SafeFlags: map[string]FlagArgType{
			"-a": FlagNone, "-L": FlagNone, "-l": FlagNone, "-n": FlagNone,
			"-f": FlagString,
			"-g": FlagNone, "-i": FlagNone, "-I": FlagString,
			"-s": FlagNone,
			"-r": FlagNone,
			"-m": FlagNone,
			"-v": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// ss: socket statistics (iproute2), read-only equivalent to netstat.
	// SECURITY: -K/--kill, -D/--diag, -F/--filter, -N/--net intentionally excluded.
	"ss": {
		SafeFlags: map[string]FlagArgType{
			"-h": FlagNone, "--help": FlagNone,
			"-V": FlagNone, "--version": FlagNone,
			"-n": FlagNone, "--numeric": FlagNone,
			"-r": FlagNone, "--resolve": FlagNone,
			"-a": FlagNone, "--all": FlagNone,
			"-l": FlagNone, "--listening": FlagNone,
			"-o": FlagNone, "--options": FlagNone,
			"-e": FlagNone, "--extended": FlagNone,
			"-m": FlagNone, "--memory": FlagNone,
			"-p": FlagNone, "--processes": FlagNone,
			"-i": FlagNone, "--info": FlagNone,
			"-s": FlagNone, "--summary": FlagNone,
			"-4": FlagNone, "--ipv4": FlagNone,
			"-6": FlagNone, "--ipv6": FlagNone,
			"-0": FlagNone, "--packet": FlagNone,
			"-t": FlagNone, "--tcp": FlagNone,
			"-M": FlagNone, "--mptcp": FlagNone,
			"-S": FlagNone, "--sctp": FlagNone,
			"-u": FlagNone, "--udp": FlagNone,
			"-d": FlagNone, "--dccp": FlagNone,
			"-w": FlagNone, "--raw": FlagNone,
			"-x": FlagNone, "--unix": FlagNone,
			"--tipc": FlagNone, "--vsock": FlagNone,
			"-f": FlagString, "--family": FlagString,
			"-A": FlagString, "--query": FlagString, "--socket": FlagString,
			"-Z": FlagNone, "--context": FlagNone,
			"-z": FlagNone, "--contexts": FlagNone,
			"-b": FlagNone, "--bpf": FlagNone,
			"-E": FlagNone, "--events": FlagNone,
			"-H": FlagNone, "--no-header": FlagNone,
			"-O": FlagNone, "--oneline": FlagNone,
			"--tipcinfo": FlagNone, "--tos": FlagNone,
			"--cgroup": FlagNone, "--inet-sockopt": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// fd / fdfind: fast file finder (fd-find).
	// SECURITY: -x/--exec and -X/--exec-batch intentionally excluded.
	// SECURITY: -l/--list-details excluded (internally executes ls subprocess).
	"fd": {
		SafeFlags: fdSafeFlags,
	},
	"fdfind": {
		SafeFlags: fdSafeFlags,
	},

	// pyright: static type checker (Microsoft).
	// SECURITY: --watch/-w launches a persistent file-watcher — excluded via IsDangerous.
	// RespectsDoubleDash: false because pyright treats -- as a file path.
	"pyright": {
		RespectsDoubleDash: false,
		SafeFlags: map[string]FlagArgType{
			"--outputjson": FlagNone,
			"--project":    FlagString, "-p": FlagString,
			"--pythonversion": FlagString, "--pythonplatform": FlagString,
			"--typeshedpath": FlagString, "--venvpath": FlagString,
			"--level": FlagString,
			"--stats": FlagNone, "--verbose": FlagNone,
			"--version": FlagNone, "--dependencies": FlagNone, "--warnings": FlagNone,
		},
		IsDangerous: func(tokens []string) bool {
			return sliceContains(tokens, "--watch") || sliceContains(tokens, "-w")
		},
	},

	// --- network / data-fetch tools ---

	// curl: HTTP client. Only stdout-output usage is approved.
	// SECURITY: -o/--output/-O/--remote-name write to local files.
	// -D/--dump-header and --trace also write to files.
	"curl": {
		SafeFlags: map[string]FlagArgType{
			"-s": FlagNone, "--silent": FlagNone,
			"-v": FlagNone, "--verbose": FlagNone,
			"-I": FlagNone, "--head": FlagNone,
			"-L": FlagNone, "--location": FlagNone,
			"--max-redirs": FlagNumber,
			"-H":           FlagString, "--header": FlagString,
			"-A": FlagString, "--user-agent": FlagString,
			"-k": FlagNone, "--insecure": FlagNone,
			"-b": FlagString, "--cookie": FlagString,
			"-m": FlagNumber, "--max-time": FlagNumber,
			"--connect-timeout": FlagNumber,
			"--compressed":      FlagNone,
			"-w":                FlagString, "--write-out": FlagString,
			"-e": FlagString, "--referer": FlagString,
			"--retry": FlagNumber, "--retry-delay": FlagNumber,
			"--retry-max-time": FlagNumber,
			"-x":               FlagString, "--proxy": FlagString,
			"-U": FlagString, "--proxy-user": FlagString,
			"--noproxy": FlagString,
			"-4":        FlagNone, "--ipv4": FlagNone,
			"-6": FlagNone, "--ipv6": FlagNone,
			"-n": FlagNone, "--netrc": FlagNone, "--netrc-optional": FlagNone,
			"-N": FlagNone, "--no-buffer": FlagNone,
			"--limit-rate": FlagString,
			"-u":           FlagString, "--user": FlagString,
			"-X": FlagString, "--request": FlagString,
			"-d": FlagString, "--data": FlagString,
			"--data-raw": FlagString, "--data-binary": FlagString,
			"--data-urlencode": FlagString,
			"-G":               FlagNone, "--get": FlagNone,
			"-F": FlagString, "--form": FlagString,
			"--json": FlagString,
			"--cert": FlagString, "--key": FlagString,
			"--cacert": FlagString, "--capath": FlagString,
			"--resolve": FlagString, "--dns-servers": FlagString,
			"--interface": FlagString, "--local-port": FlagString,
			"--http1.0": FlagNone, "--http1.1": FlagNone,
			"--http2": FlagNone, "--http2-prior-knowledge": FlagNone,
			"--http3": FlagNone,
			"--tlsv1": FlagNone, "--tlsv1.0": FlagNone, "--tlsv1.1": FlagNone,
			"--tlsv1.2": FlagNone, "--tlsv1.3": FlagNone,
			"-#": FlagNone, "--progress-bar": FlagNone,
			"--fail": FlagNone, "-f": FlagNone, "--fail-with-body": FlagNone,
			"--include": FlagNone, "-i": FlagNone,
			"--show-error": FlagNone, "-S": FlagNone,
			"--help": FlagNone, "-h": FlagNone,
			"--version": FlagNone, "-V": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        curlIsDangerous,
	},

	// wget: file downloader. Only --spider (HEAD-check mode) is safe.
	// Any download without --spider writes to the local filesystem by default.
	"wget": {
		SafeFlags: map[string]FlagArgType{
			"--spider": FlagNone,
			"-q":       FlagNone, "--quiet": FlagNone,
			"-v": FlagNone, "--verbose": FlagNone,
			"-S": FlagNone, "--server-response": FlagNone,
			"--no-check-certificate": FlagNone,
			"-T":                     FlagNumber, "--timeout": FlagNumber,
			"--dns-timeout": FlagNumber, "--connect-timeout": FlagNumber,
			"--read-timeout": FlagNumber,
			"-U":             FlagString, "--user-agent": FlagString,
			"--header":       FlagString,
			"--max-redirect": FlagNumber,
			"-t":             FlagNumber, "--tries": FlagNumber,
			"--help": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        wgetIsDangerous,
	},

	// --- JSON / YAML / text processing ---

	// jq: JSON processor. Dangerous flags that load executable code from files
	// (-f/--from-file, --rawfile, --slurpfile, --run-tests, -L/--library-path)
	// are blocked via IsDangerous.
	"jq": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagNone, "--exit-status": FlagNone,
			"-r": FlagNone, "--raw-output": FlagNone,
			"-R": FlagNone, "--raw-input": FlagNone,
			"-c": FlagNone, "--compact-output": FlagNone,
			"-s": FlagNone, "--slurp": FlagNone,
			"-n": FlagNone, "--null-input": FlagNone,
			"-a": FlagNone, "--ascii-output": FlagNone,
			"-S": FlagNone, "--sort-keys": FlagNone,
			"-C": FlagNone, "--color-output": FlagNone,
			"-M": FlagNone, "--monochrome-output": FlagNone,
			"--tab":    FlagNone,
			"--indent": FlagNumber,
			"--arg":    FlagString, "--argjson": FlagString,
			"--slurpfile": FlagString, // NOTE: listed for completeness but blocked by IsDangerous
			"--args":      FlagNone, "--jsonargs": FlagNone,
			"--join-output": FlagNone, "-j": FlagNone,
			"--stream": FlagNone,
			"--help":   FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        jqIsDangerous,
	},

	// yq: YAML/JSON processor (mikefarah/yq). Similar to jq for YAML.
	"yq": {
		SafeFlags: map[string]FlagArgType{
			"-e": FlagNone, "--exit-status": FlagNone,
			"-r": FlagNone, "--raw-output": FlagNone,
			"-P": FlagNone, "--prettyPrint": FlagNone,
			"-C": FlagNone, "--colors": FlagNone,
			"-M": FlagNone, "--no-colors": FlagNone,
			"-j": FlagNone, "--tojson": FlagNone,
			"-y": FlagNone, "--yaml-output": FlagNone,
			"-p": FlagString, "--input-format": FlagString,
			"-o": FlagString, "--output-format": FlagString,
			"-N": FlagNone, "--no-doc": FlagNone,
			"--expression": FlagString, "-e2": FlagString,
			"--header-preprocess": FlagNone,
			"--indent":            FlagNumber, "-I": FlagNumber,
			"--csv-separator":        FlagString,
			"--xml-attribute-prefix": FlagString,
			"--xml-content-name":     FlagString,
			"--xml-directive-name":   FlagString,
			"--xml-proc-inst-prefix": FlagString,
			"--help":                 FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
	},

	// awk: text processing. Dangerous when using system(), pipe writes, or -f.
	// SECURITY: system() calls, print-to-pipe, print-to-file, and -f (load program
	// from file) are blocked via IsDangerous.
	"awk": {
		SafeFlags: map[string]FlagArgType{
			"-F": FlagString, "--field-separator": FlagString,
			"-v": FlagString, "--assign": FlagString,
			// SECURITY: -f/--file intentionally excluded (executes awk code from file)
			"-b": FlagNone, "--characters-as-bytes": FlagNone,
			"--sandbox": FlagNone, // gawk sandbox mode — safer
			"--help":    FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        awkIsDangerous,
	},
	"gawk": {
		SafeFlags: map[string]FlagArgType{
			"-F": FlagString, "--field-separator": FlagString,
			"-v": FlagString, "--assign": FlagString,
			"-b": FlagNone, "--characters-as-bytes": FlagNone,
			"--sandbox": FlagNone,
			"--help":    FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        awkIsDangerous,
	},

	// --- package managers (read-only subcommands only) ---

	// npm: only list/view/search/outdated/audit/version are read-only.
	// SECURITY: install/uninstall/publish/run/ci/update all modify state.
	"npm": {
		SafeFlags: map[string]FlagArgType{
			"--global": FlagNone, "-g": FlagNone,
			"--depth": FlagNumber,
			"--json":  FlagNone,
			"--long":  FlagNone, "-l": FlagNone,
			"--parseable": FlagNone, "-p": FlagNone,
			"--prefix":    FlagString,
			"--workspace": FlagString, "-w": FlagString,
			"--include-workspace-root": FlagNone,
			"--all":                    FlagNone, "-a": FlagNone,
			"--audit-level": FlagString,
			"--omit":        FlagString,
			"--help":        FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        npmIsDangerous,
	},

	// yarn: read-only subcommands only (classic yarn and yarn berry).
	"yarn": {
		SafeFlags: map[string]FlagArgType{
			"--json":        FlagNone,
			"--no-progress": FlagNone,
			"--silent":      FlagNone,
			"--verbose":     FlagNone,
			"--cwd":         FlagString,
			"--help":        FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        yarnIsDangerous,
	},

	// pnpm: read-only subcommands only.
	"pnpm": {
		SafeFlags: map[string]FlagArgType{
			"--json":   FlagNone,
			"--depth":  FlagNumber,
			"--global": FlagNone, "-g": FlagNone,
			"--long":      FlagNone,
			"--parseable": FlagNone,
			"--help":      FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        pnpmIsDangerous,
	},

	// cargo: Rust package manager. Read-only subcommands: metadata, tree, search,
	// locate-project, pkgid, version.
	// SECURITY: build/test/run/install/publish/clean/update all modify state.
	"cargo": {
		SafeFlags: map[string]FlagArgType{
			"--version": FlagNone, "-V": FlagNone,
			"--help": FlagNone, "-h": FlagNone,
			"--list":    FlagNone,
			"--verbose": FlagNone, "-v": FlagNone,
			"--quiet": FlagNone, "-q": FlagNone,
			"--color":  FlagString,
			"--frozen": FlagNone, "--locked": FlagNone, "--offline": FlagNone,
			"--manifest-path": FlagString,
			"--workspace":     FlagNone, "--all": FlagNone,
			"--exclude": FlagString,
			"--package": FlagString, "-p": FlagString,
			"--format-version": FlagNumber,
			"--no-deps":        FlagNone,
			"--features":       FlagString, "--all-features": FlagNone,
			"--no-default-features": FlagNone,
			"--target":              FlagString,
			"--depth":               FlagNumber,
			"--prefix":              FlagNone, "--invert": FlagNone, "-i": FlagNone,
			"--prune": FlagString,
			"--limit": FlagNumber,
			"--index": FlagString,
		},
		RespectsDoubleDash: true,
		IsDangerous:        cargoIsDangerous,
	},

	// --- runtime version checks (version-only) ---

	// python/python3: ONLY --version/-V is safe. Executing scripts is dangerous.
	// SECURITY: python is a general-purpose interpreter. Even seemingly harmless
	// flags like -c (inline code) can execute arbitrary code. Positional args
	// (script names) also execute code. Only version display is approved.
	"python": {
		SafeFlags: map[string]FlagArgType{
			"--version": FlagNone, "-V": FlagNone,
			"--help": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        pythonIsDangerous,
	},
	"python3": {
		SafeFlags: map[string]FlagArgType{
			"--version": FlagNone, "-V": FlagNone,
			"--help": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        pythonIsDangerous,
	},

	// ruby: ONLY --version/-v is safe.
	"ruby": {
		SafeFlags: map[string]FlagArgType{
			"--version": FlagNone, "-v": FlagNone,
			"--help": FlagNone, "-h": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        rubyIsDangerous,
	},

	// node: ONLY --version/-v is safe (matches TypeScript regex behavior).
	// SECURITY: node -v --run <task> would execute package.json scripts.
	"node": {
		SafeFlags: map[string]FlagArgType{
			"--version": FlagNone, "-v": FlagNone,
			"--help": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        pythonIsDangerous, // same logic: block any positional args
	},

	// --- archive tools (list/inspect mode only) ---

	// tar: only -t/--list mode is approved. Extract/create/append are dangerous.
	"tar": {
		SafeFlags: map[string]FlagArgType{
			"-t": FlagNone, "--list": FlagNone,
			"-v": FlagNone, "--verbose": FlagNone,
			"-f": FlagString, "--file": FlagString,
			"-z": FlagNone, "--gzip": FlagNone, "--gunzip": FlagNone,
			"-j": FlagNone, "--bzip2": FlagNone,
			"-J": FlagNone, "--xz": FlagNone,
			"-Z": FlagNone, "--compress": FlagNone,
			"--lzma": FlagNone, "--lzop": FlagNone, "--lzip": FlagNone,
			"-I": FlagString, "--use-compress-program": FlagString,
			"--no-same-permissions": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        tarIsDangerous,
	},

	// unzip: only -l (list) and -v (verbose list) are approved.
	// Default unzip behavior (without -l/-v) extracts to the filesystem.
	"unzip": {
		SafeFlags: map[string]FlagArgType{
			"-l": FlagNone, // list archive contents
			"-v": FlagNone, // verbose list
			"-Z": FlagNone, // zipinfo-style listing
			"-p": FlagNone, // extract to pipe (stdout) — safe, no local files written
		},
		RespectsDoubleDash: true,
		IsDangerous:        unzipIsDangerous,
	},

	// --- commands moved from readOnlyCommands for targeted blocking ---

	// date: display mode is safe; positional time-setting args (POSIX MMDDhhmm form)
	// and -s/--set are dangerous (they set the system clock).
	// SECURITY: "date 1231235959" sets system time — previously auto-approved.
	"date": {
		SafeFlags: map[string]FlagArgType{
			"-u": FlagNone, "--utc": FlagNone, "--universal": FlagNone,
			"-R": FlagNone, "--rfc-email": FlagNone, "--rfc-2822": FlagNone,
			// --iso-8601 and --rfc-3339 accept an optional format via "=", not a
			// separate token (e.g. --iso-8601=seconds). FlagNone lets the inline
			// "=value" form be handled by the "--flag=value" path in validateFlags.
			"--iso-8601": FlagNone, "--rfc-3339": FlagNone,
			"-d": FlagString, "--date": FlagString,
			"-f": FlagString, "--file": FlagString,
			"-r": FlagString, "--reference": FlagString,
			"--debug": FlagNone,
			"--help":  FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        dateIsDangerous,
	},

	// hostname: display mode is safe; positional arg sets system hostname.
	// SECURITY: "hostname newname" sets hostname — previously auto-approved.
	"hostname": {
		SafeFlags: map[string]FlagArgType{
			"-f": FlagNone, "--fqdn": FlagNone, "--long": FlagNone,
			"-s": FlagNone, "--short": FlagNone,
			"-d": FlagNone, "--domain": FlagNone,
			"-i": FlagNone, "--ip-address": FlagNone,
			"-I": FlagNone, "--all-ip-addresses": FlagNone,
			"-A": FlagNone, "--all-fqdns": FlagNone,
			"-y": FlagNone, "--yp": FlagNone, "--nis": FlagNone,
			"--help": FlagNone, "--version": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        hostnameIsDangerous,
	},

	// tput: capability queries are safe; init/reset execute terminfo scripts.
	// SECURITY: "tput init" / "tput reset" run terminfo is1-is3/rs1-rs3 sequences
	// which can execute arbitrary programs from $TERMINFO — previously auto-approved.
	"tput": {
		SafeFlags: map[string]FlagArgType{
			"-T": FlagString, // terminal type override
			"-S": FlagNone,   // read multiple capabilities from stdin
			"-x": FlagNone,   // do not clear scrollback after clear
			"-V": FlagNone,   // print version
		},
		RespectsDoubleDash: true,
		IsDangerous:        tputIsDangerous,
	},

	// lsof: list open files. Safe for querying; +m/+M creates a mount supplement
	// file on disk. lsof uses + prefixed flags which bypass normal - flag checks.
	// SECURITY: "lsof +m /path" creates /path — previously auto-approved.
	// All flags treated as FlagNone because lsof's flag/arg boundary is ambiguous.
	"lsof": {
		SafeFlags: map[string]FlagArgType{
			"-a": FlagNone, "-b": FlagNone, "-c": FlagNone, "-C": FlagNone,
			"-d": FlagNone, "-D": FlagNone, "-e": FlagNone, "-E": FlagNone,
			"-f": FlagNone, "-F": FlagNone, "-g": FlagNone, "-G": FlagNone,
			"-h": FlagNone, "-H": FlagNone, "-i": FlagNone, "-I": FlagNone,
			"-k": FlagNone, "-K": FlagNone, "-l": FlagNone, "-L": FlagNone,
			"-m": FlagNone, "-n": FlagNone, "-N": FlagNone,
			"-o": FlagNone, "-O": FlagNone, "-p": FlagNone, "-P": FlagNone,
			"-Q": FlagNone, "-r": FlagNone, "-R": FlagNone, "-s": FlagNone,
			"-S": FlagNone, "-t": FlagNone, "-T": FlagNone, "-u": FlagNone,
			"-U": FlagNone, "-v": FlagNone, "-V": FlagNone, "-w": FlagNone,
			"-W": FlagNone, "-x": FlagNone, "-X": FlagNone, "-z": FlagNone,
			"-Z": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        lsofIsDangerous,
	},

	// tree: directory listing. -R generates HTML output files (00Tree.html).
	// SECURITY: "tree -R" and "tree --fromfile" write files — previously auto-approved.
	"tree": {
		SafeFlags: map[string]FlagArgType{
			"-a": FlagNone, "-d": FlagNone, "-l": FlagNone, "-f": FlagNone,
			"-x": FlagNone, "-L": FlagNumber, "-P": FlagString, "-I": FlagString,
			"-s": FlagNone, "-h": FlagNone, "--si": FlagNone,
			"-D": FlagNone, "-F": FlagNone,
			"-q": FlagNone, "-N": FlagNone, "-Q": FlagNone,
			"-C": FlagNone, "-n": FlagNone,
			"-v": FlagNone, "-r": FlagNone, "-t": FlagNone, "-U": FlagNone,
			"--dirsfirst": FlagNone, "--filesfirst": FlagNone,
			"--noreport": FlagNone, "--charset": FlagString,
			"--version": FlagNone, "--help": FlagNone,
		},
		RespectsDoubleDash: true,
		IsDangerous:        treeIsDangerous,
	},
}

// fdSafeFlags are the shared safe flags for fd and fdfind.
var fdSafeFlags = map[string]FlagArgType{
	"-h": FlagNone, "--help": FlagNone,
	"-V": FlagNone, "--version": FlagNone,
	"-H": FlagNone, "--hidden": FlagNone,
	"-I": FlagNone, "--no-ignore": FlagNone,
	"--no-ignore-vcs": FlagNone, "--no-ignore-parent": FlagNone,
	"-s": FlagNone, "--case-sensitive": FlagNone,
	"-i": FlagNone, "--ignore-case": FlagNone,
	"-g": FlagNone, "--glob": FlagNone,
	"--regex": FlagNone,
	"-F":      FlagNone, "--fixed-strings": FlagNone,
	"-a": FlagNone, "--absolute-path": FlagNone,
	"-L": FlagNone, "--follow": FlagNone,
	"-p": FlagNone, "--full-path": FlagNone,
	"-0": FlagNone, "--print0": FlagNone,
	"-d": FlagNumber, "--max-depth": FlagNumber,
	"--min-depth": FlagNumber, "--exact-depth": FlagNumber,
	"-t": FlagString, "--type": FlagString,
	"-e": FlagString, "--extension": FlagString,
	"-S": FlagString, "--size": FlagString,
	"--changed-within": FlagString, "--changed-before": FlagString,
	"-o": FlagString, "--owner": FlagString,
	"-E": FlagString, "--exclude": FlagString,
	"--ignore-file": FlagString,
	"-c":            FlagString, "--color": FlagString,
	"-j": FlagNumber, "--threads": FlagNumber,
	"--max-buffer-time": FlagString, "--max-results": FlagNumber,
	"-1": FlagNone, "-q": FlagNone, "--quiet": FlagNone,
	"--show-errors": FlagNone, "--strip-cwd-prefix": FlagNone,
	"--one-file-system": FlagNone, "--prune": FlagNone,
	"--search-path": FlagString, "--base-directory": FlagString,
	"--path-separator": FlagString, "--batch-size": FlagNumber,
	"--no-require-git": FlagNone, "--hyperlink": FlagString,
	"--and": FlagString, "--format": FlagString,
}

// safeTargetCommandsForXargs are commands safe to use as xargs targets.
var safeTargetCommandsForXargs = map[string]bool{
	"echo":   true,
	"printf": true,
	"wc":     true,
	"grep":   true,
	"head":   true,
	"tail":   true,
}

// gitRemoteNamePattern matches a well-formed git remote name (alphanumeric + _ -).
var gitRemoteNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ---------------------------------------------------------------------------
// IsDangerous helpers
// ---------------------------------------------------------------------------

// ghIsDangerous blocks HOST/OWNER/REPO repo specs, URLs, and SSH-style args
// that could be used to exfiltrate data via the gh CLI.
// Mirrors Claude Code's ghIsDangerousCallback in readOnlyCommandValidation.ts.
func ghIsDangerous(tokens []string) bool {
	for _, token := range tokens {
		var value string
		if strings.HasPrefix(token, "-") {
			// For flag tokens, inspect the value after '=' (cobra accepts both forms).
			eqIdx := strings.IndexByte(token, '=')
			if eqIdx == -1 {
				continue // flag without inline value — safe to skip
			}
			value = token[eqIdx+1:]
			if value == "" {
				continue
			}
		} else {
			value = token
		}
		// Skip values that clearly can't be a host-prefixed repo spec.
		if !strings.Contains(value, "/") && !strings.Contains(value, "://") && !strings.Contains(value, "@") {
			continue
		}
		// Reject URL schemes: https://, http://, git://, ssh://
		if strings.Contains(value, "://") {
			return true
		}
		// Reject SSH-style: git@github.com:user/repo
		if strings.Contains(value, "@") {
			return true
		}
		// Reject HOST/OWNER/REPO format (2+ slashes; normal is OWNER/REPO = 1 slash)
		if strings.Count(value, "/") >= 2 {
			return true
		}
	}
	return false
}

// gitConfigIsDangerous blocks write forms of git config. Read forms have 0 or 1
// positional argument (e.g. "git config --get user.name", "git config --list"),
// while write forms have key+value (2+ positionals, e.g. "git config user.email x").
func gitConfigIsDangerous(tokens []string) bool {
	var positionals []string
	for _, t := range tokens {
		if !strings.HasPrefix(t, "-") {
			positionals = append(positionals, t)
		}
	}
	return len(positionals) >= 2
}

// gitReflogIsDangerous blocks "git reflog expire", "git reflog delete", and
// "git reflog exists" — all of which write to .git/logs/**.
func gitReflogIsDangerous(tokens []string) bool {
	dangerous := map[string]bool{"expire": true, "delete": true, "exists": true}
	for _, t := range tokens {
		if t == "" || strings.HasPrefix(t, "-") {
			continue
		}
		// First non-flag positional is the subcommand.
		return dangerous[t] // true = dangerous, false = show/HEAD/ref = safe
	}
	return false
}

// gitLsRemoteIsDangerous blocks URL/SSH remote specs that could exfiltrate data.
// Mirrors the inline URL guard in Claude Code's isCommandSafeViaFlagParsing.
func gitLsRemoteIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			continue
		}
		// Reject URL schemes
		if strings.Contains(t, "://") {
			return true
		}
		// Reject SSH-style git@host:repo or host:path
		if strings.Contains(t, "@") || strings.Contains(t, ":") {
			return true
		}
		// Reject variable references
		if strings.Contains(t, "$") {
			return true
		}
	}
	return false
}

// gitRemoteShowIsDangerous requires exactly one well-formed remote name.
// Mirrors Claude Code's additionalCommandIsDangerousCallback for git remote show.
func gitRemoteShowIsDangerous(tokens []string) bool {
	// Collect positionals (everything except the -n flag)
	var positionals []string
	for _, a := range tokens {
		if a != "-n" {
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return true // 0 or 2+ positionals are dangerous
	}
	return !gitRemoteNamePattern.MatchString(positionals[0])
}

// curlIsDangerous blocks curl flags that write to local files.
func curlIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if t == "-o" || t == "-O" {
			return true
		}
		// Long-form flags and their equals-attached variants
		lower := strings.ToLower(t)
		if strings.HasPrefix(lower, "--output") ||
			strings.HasPrefix(lower, "--remote-name") ||
			strings.HasPrefix(lower, "--dump-header") ||
			t == "-D" ||
			strings.HasPrefix(lower, "--cookie-jar") ||
			strings.HasPrefix(lower, "--trace") {
			return true
		}
	}
	return false
}

// wgetIsDangerous requires --spider flag for safe operation.
// Without --spider, wget downloads files to the local filesystem.
func wgetIsDangerous(tokens []string) bool {
	// Block explicit file-write flags regardless of --spider
	for _, t := range tokens {
		if t == "-O" || t == "-P" || t == "-o" || t == "-a" ||
			t == "--output-document" || t == "--directory-prefix" ||
			t == "--output-file" || t == "--append-output" ||
			strings.HasPrefix(t, "--output-document=") ||
			strings.HasPrefix(t, "--directory-prefix=") {
			return true
		}
	}
	// Require --spider flag
	return !sliceContains(tokens, "--spider")
}

// jqIsDangerous blocks flags that load executable code or modules from files.
func jqIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if t == "-f" || t == "--from-file" ||
			t == "--rawfile" || t == "--slurpfile" ||
			t == "--run-tests" ||
			t == "-L" || t == "--library-path" ||
			strings.HasPrefix(t, "-f=") || strings.HasPrefix(t, "--from-file=") ||
			strings.HasPrefix(t, "-L=") || strings.HasPrefix(t, "--library-path=") {
			return true
		}
	}
	return false
}

// awkIsDangerous blocks awk programs that execute shell commands or write files.
// Checks for system(), pipe writes (| "cmd"), file writes (> file, >> file),
// and -f (load program from file).
func awkIsDangerous(tokens []string) bool {
	skipNext := false
	for _, t := range tokens {
		if skipNext {
			skipNext = false
			continue
		}
		// -f loads program from file (could contain system() calls)
		if t == "-f" || t == "--file" {
			return true
		}
		if strings.HasPrefix(t, "-f=") || strings.HasPrefix(t, "--file=") {
			return true
		}
		// -v and -F consume next token as value, skip it
		if t == "-v" || t == "--assign" || t == "-F" || t == "--field-separator" {
			skipNext = true
			continue
		}
		// Check awk program text for dangerous patterns
		if !strings.HasPrefix(t, "-") {
			if strings.Contains(t, "system(") ||
				strings.Contains(t, "| \"") ||
				strings.Contains(t, "|\"") ||
				strings.Contains(t, "| '") ||
				strings.Contains(t, " > ") ||
				strings.Contains(t, " >> ") ||
				strings.Contains(t, ">/") ||
				strings.Contains(t, ">>/") ||
				strings.Contains(t, "getline <") ||
				strings.Contains(t, "print >") ||
				strings.Contains(t, "print >>") {
				return true
			}
		}
	}
	return false
}

// npmIsDangerous blocks npm write-subcommands. Only list/view/search/outdated/audit/version
// are read-only.
func npmIsDangerous(tokens []string) bool {
	safeSubcmds := map[string]bool{
		"list": true, "ls": true, "ll": true, "la": true,
		"view": true, "info": true, "show": true,
		"search":   true,
		"outdated": true,
		"audit":    true,
		"version":  true,
		"whoami":   true,
		"doctor":   true,
		"ping":     true,
	}
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			continue
		}
		// First non-flag is the subcommand
		return !safeSubcmds[t]
	}
	return true // no subcommand — block bare "npm"
}

// yarnIsDangerous blocks yarn write-subcommands.
func yarnIsDangerous(tokens []string) bool {
	safeSubcmds := map[string]bool{
		"list": true, "info": true, "outdated": true,
		"audit": true, "version": true, "versions": true,
		"licenses": true, "why": true, "check": true,
		"workspaces": true,
	}
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			continue
		}
		return !safeSubcmds[t]
	}
	return true
}

// pnpmIsDangerous blocks pnpm write-subcommands.
func pnpmIsDangerous(tokens []string) bool {
	safeSubcmds := map[string]bool{
		"list": true, "ls": true, "outdated": true,
		"audit": true, "version": true, "versions": true,
		"why": true, "licenses": true,
	}
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			continue
		}
		return !safeSubcmds[t]
	}
	return true
}

// cargoIsDangerous blocks cargo write-subcommands. Only metadata/tree/search/
// locate-project/pkgid/version are read-only.
func cargoIsDangerous(tokens []string) bool {
	safeSubcmds := map[string]bool{
		"metadata":       true,
		"tree":           true,
		"search":         true,
		"locate-project": true,
		"pkgid":          true,
		"version":        true,
	}
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			// Allow --version as a top-level flag (e.g., "cargo --version")
			if t == "--version" || t == "-V" {
				return false
			}
			continue
		}
		return !safeSubcmds[t]
	}
	// Bare "cargo" or "cargo --version" — allow only if a version flag was seen
	// (cargoIsDangerous is called after validateFlags, so bare "cargo" gets here
	// only if validateFlags passed, meaning only safe flags were used).
	// Return safe for the case where only --version/-V flags appear.
	return false
}

// pythonIsDangerous blocks any positional arguments (script execution, -c, -m, etc.)
// Only --version/-V is safe. Mirrors the TypeScript regex: /^python --version$/.
func pythonIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") {
			if t == "--version" || t == "-V" || t == "--help" {
				continue
			}
			return true // any other flag is potentially dangerous
		}
		return true // positional arg = script execution
	}
	return false
}

// rubyIsDangerous blocks any flags other than version/help.
func rubyIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if t == "--version" || t == "-v" || t == "--help" || t == "-h" {
			continue
		}
		return true
	}
	return false
}

// tarIsDangerous ensures tar is used only in list mode (-t/--list).
// Extract (-x/--extract), create (-c/--create), append (-r/--append),
// and update (-u/--update) modes write to the filesystem.
func tarIsDangerous(tokens []string) bool {
	hasList := false
	for _, t := range tokens {
		switch t {
		case "-t", "--list":
			hasList = true
		case "-x", "--extract", "--get",
			"-c", "--create",
			"-r", "--append",
			"-u", "--update",
			"--delete":
			return true
		}
		// Handle bundled short flags: -tvf, -tzf, -tjf, -tf
		if strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "--") && len(t) > 1 {
			for _, c := range t[1:] {
				switch c {
				case 't':
					hasList = true
				case 'x', 'c', 'r', 'u':
					return true
				}
			}
		}
	}
	return !hasList
}

// unzipIsDangerous allows only list modes: -l, -v, -Z (zipinfo), -p (pipe to stdout).
// Default unzip without -l/-v/-Z extracts files to the filesystem.
func unzipIsDangerous(tokens []string) bool {
	hasListFlag := false
	for _, t := range tokens {
		switch t {
		case "-l", "-v", "-Z", "-p":
			hasListFlag = true
		}
		// Block explicit extract/overwrite/destination flags
		if t == "-o" || t == "-d" || t == "-a" || t == "-aa" || t == "-b" || t == "-n" {
			// -o (overwrite), -d (extract dir), -a/-aa (text conversions)
			// These imply extraction mode
			if t == "-d" || t == "-o" {
				return true
			}
		}
	}
	return !hasListFlag
}

// sliceContains is a small helper used by the IsDangerous callbacks.
func sliceContains(s []string, v string) bool {
	for _, el := range s {
		if el == v {
			return true
		}
	}
	return false
}

// dateIsDangerous blocks date forms that set the system clock.
// POSIX positional form (MMDDhhmm[CC[YY]][.ss]) is dangerous; format strings
// (starting with '+') are safe. The -s/--set flags explicitly set the date.
// Mirrors Claude Code's COMMAND_ALLOWLIST IsDangerous for date.
func dateIsDangerous(tokens []string) bool {
	// Flags that take a separate next-token argument (for date display, not setting)
	argFlags := map[string]bool{
		"-d": true, "--date": true,
		"-f": true, "--file": true,
		"-r": true, "--reference": true,
	}
	skipNext := false
	for _, t := range tokens {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(t, "-") {
			// -s and --set[=...] explicitly set the system date
			if t == "-s" || t == "--set" || strings.HasPrefix(t, "--set=") {
				return true
			}
			// Skip the argument of flags that take a display/parse date string
			if argFlags[t] {
				skipNext = true
			}
			continue
		}
		// Positional: format strings start with '+'; everything else is POSIX time-set
		if !strings.HasPrefix(t, "+") {
			return true
		}
	}
	return false
}

// hostnameIsDangerous blocks hostname forms that set the system hostname.
// A bare "hostname" or any flag-only form is safe; a positional arg sets the name.
// Mirrors Claude Code's COMMAND_ALLOWLIST IsDangerous for hostname.
func hostnameIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if len(t) > 0 && t[0] != '-' {
			return true // positional arg = setting hostname
		}
	}
	return false
}

// tputIsDangerous blocks terminfo capabilities that execute programs.
// "tput init" and "tput reset" run terminfo is1-is3/rs1-rs3 sequences which
// can execute arbitrary programs. "tput isgr0" also executes terminfo.
// Mirrors Claude Code's COMMAND_ALLOWLIST IsDangerous for tput.
func tputIsDangerous(tokens []string) bool {
	dangerousCaps := map[string]bool{
		"init": true, "reset": true, "isgr0": true,
	}
	for _, t := range tokens {
		if len(t) > 0 && t[0] != '-' {
			return dangerousCaps[t]
		}
	}
	return false
}

// lsofIsDangerous blocks +m/+M which create a mount supplement file on disk.
// lsof uses '+' prefixed flags that bypass normal '-' prefix flag validation.
// Mirrors Claude Code's COMMAND_ALLOWLIST IsDangerous for lsof.
func lsofIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if t == "+m" || t == "+M" {
			return true
		}
	}
	return false
}

// treeIsDangerous blocks -R/--fromfile which write HTML output files to disk.
// "tree -R" generates 00Tree.html files in traversed directories.
// Mirrors Claude Code's COMMAND_ALLOWLIST IsDangerous for tree.
func treeIsDangerous(tokens []string) bool {
	for _, t := range tokens {
		if t == "-R" || t == "--fromfile" {
			return true
		}
	}
	return false
}
