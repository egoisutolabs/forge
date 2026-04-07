# BashTool Parity Report

Systematic comparison of the Go BashTool port against Claude Code's TypeScript reference
implementation. Reference: `claudecode/tools/BashTool/` and `claudecode/utils/shell/`.

Generated: 2026-04-04

---

## 1. Matched Features

The following features are fully implemented and behave equivalently to the TypeScript source.

| Feature | Go file | TypeScript ref |
|---------|---------|----------------|
| `bash -c '<cmd>'` execution | `exec.go` | `BashTool.tsx` |
| stdout+stderr merged | `exec.go:63` | `Shell.ts` |
| Process group kill (Setpgid + SIGKILL) | `exec.go:71,153` | `Shell.ts:tree-kill` |
| Timeout: 120s default, 600s max | `exec.go:12-15` | `timeouts.ts` |
| Context cancellation → SIGKILL | `exec.go:112` | `Shell.ts` |
| Background tasks via `run_in_background` | `background.go` | `BashTool.tsx` |
| TaskRegistry with Add/Get/Stop/List | `background.go` | `backgroundTasks.ts` |
| Background output to `~/.forge/tasks/{id}.output` | `background.go:StartBackground` | `backgroundTasks.ts` |
| Size watchdog (5s interval, 5GB cap) | `background.go:sizeWatchdog` | `backgroundTasks.ts` |
| Output truncation at 30K chars | `truncate.go:DefaultMaxOutput` | `utils.ts:formatOutput` |
| Truncation message `... [N lines truncated] ...` | `truncate.go:TruncateOutput` | `utils.ts:formatOutput` |
| Large output persistence >50K to disk | `persist.go:MaxResultSizeChars` | `BashTool.tsx` |
| Persist path: `~/.forge/tool-results/{id}.txt` | `persist.go:PersistOutput` | `BashTool.tsx` |
| 2KB preview for persisted output | `persist.go:PreviewSizeBytes` | `BashTool.tsx` |
| Image output detection | `image.go:isImageOutput` | `utils.ts:isImageOutput` |
| Image regex: `(?i)^data:image/[a-z0-9.+_-]+;base64,` | `image.go:14` | `utils.ts` |
| MaxImageFileSize = 20MB | `image.go:MaxImageFileSize` | `utils.ts` |
| Progress callbacks (1s polling) | `progress.go:startProgressPoller` | `BashTool.tsx` |
| ProgressEvent: Output, TotalLines, TotalBytes, ElapsedMs | `progress.go:ProgressEvent` | `BashTool.tsx` |
| safeBuffer: mutex-protected concurrent read/write | `progress.go:safeBuffer` | `Shell.ts` |
| interpretExitCode: 0,1,2,126,127,130,137,143,128+N | `exitcode.go` | `BashTool.tsx` |
| ReturnCodeInterpretation in ToolResult | `tool.go:formatResult` | `BashTool.tsx` |
| containsUnquotedExpansion() exact port | `security.go:55` | `readOnlyValidation.ts` |
| hasDangerousPattern() (redirections, `$(`, backticks) | `readonly.go:462` | `readOnlyValidation.ts` |
| splitCompoundCommand() (&&, \|\|, ;, \| with quote awareness) | `readonly.go:514` | `readOnlyValidation.ts` |
| cd+git compound command blocking | `readonly.go:106` | `readOnlyValidation.ts:checkReadOnlyConstraints` |
| Git internal path write blocking (hooks/objects/refs/HEAD) | `security.go:157` | `readOnlyValidation.ts` |
| git `-c` / `--exec-path` / `--config-env` blocking | `readonly.go:206` | `readOnlyValidation.ts` |
| git stash: only `list` and `show` are read-only | `readonly.go:218` | `readOnlyValidation.ts` |
| git branch/tag: positional arg blocks (prevents creation) | `allowlist.go:252,275` | `readOnlyCommandValidation.ts` |
| git remote: bare -v safe, positional arg blocks | `allowlist.go:309` | `readOnlyCommandValidation.ts` |
| sed: flag-validated with IsDangerous callback | `allowlist.go:88,security.go:267` | `readOnlyValidation.ts` |
| safeEnvVars: 30+ env vars safe to prefix | `security.go:11` | `readOnlyValidation.ts:SAFE_ENV_VARS` |
| stripSafeEnvVars(): strip safe env prefix | `security.go:219` | `readOnlyValidation.ts` |
| stripSafeWrappers(): timeout/time/nice/stdbuf/nohup/env | `readonly.go:623` | `readOnlyValidation.ts` |
| findDangerousFlags: -exec/-delete/-execdir etc. | `readonly.go:79` | `readOnlyValidation.ts` |
| safeTargetCommandsForXargs validation | `allowlist.go:388` | `readOnlyValidation.ts` |
| validateFlags(): double dash, attached values, combined shorts | `readonly.go:308` | `readOnlyCommandValidation.ts:validateFlags` |
| git numeric shorthand: -N equiv to -n N | `readonly.go:354` | `readOnlyCommandValidation.ts` |
| docker: ps/logs/inspect/images in allowlist | `allowlist.go:352` | `readOnlyCommandValidation.ts:DOCKER_READ_ONLY_COMMANDS` |
| grep/egrep/fgrep/rg: extensive flag-validated allowlist | `allowlist.go:31` | `readOnlyCommandValidation.ts` |
| git diff/log/show/blame/status: flag-validated | `allowlist.go:166` | `readOnlyCommandValidation.ts` |
| git ls-files/rev-parse/stash list/config: flag-validated | `allowlist.go:288` | `readOnlyCommandValidation.ts` |
| readOnlyGitSubcommands: describe/shortlog/cat-file/rev-list/etc. | `readonly.go:60` | `readOnlyCommandValidation.ts` |

---

## 2. Missing Features

### 2a. Security-Critical Gaps (Regressions)

These are behaviors present in TypeScript that are **absent in Go**, causing Go to approve
commands that TypeScript would deny. Each represents a potential security regression.

#### Gap 1: `git reflog` — missing expire/delete/exists blocking

**TypeScript**: `git reflog` is in `GIT_READ_ONLY_COMMANDS` with `IsDangerous` callback
blocking `expire`, `delete`, `exists` subactions (which destructively modify reflog).

**Go**: `reflog` is in `readOnlyGitSubcommands` (any flags OK, no subaction check).

**Impact**: `git reflog expire --expire=now --all` is currently auto-approved.

**Fix needed**: Move `git reflog` from `readOnlyGitSubcommands` to `commandAllowlist["git reflog"]`
with an `IsDangerous` callback that blocks `expire`/`delete`/`exists`.

---

#### Gap 2: `git config` — missing write-operation blocking

**TypeScript**: Only `git config --get` variant is in `GIT_READ_ONLY_COMMANDS`, blocking
all positional write forms.

**Go**: `commandAllowlist["git config"]` includes `--get`/`--list` flags but the `validateFlags`
function allows positional args for most commands, so `git config user.email foo@bar.com`
passes validation.

**Impact**: `git config user.email attacker@evil.com` is currently auto-approved.

**Fix needed**: Add `IsDangerous` callback to `commandAllowlist["git config"]` that requires
at least one of `--get`, `--get-all`, `--get-regexp`, `--list`, `-l` to be present.

---

#### Gap 3: `date` positional arg — system time setting

**TypeScript**: `date` is in `COMMAND_ALLOWLIST` with `IsDangerous` callback blocking
positional date arguments like `date 1231235959` (sets system clock).

**Go**: `date` is in `readOnlyCommands` (any flags OK, only `$` blocked).

**Impact**: `date 1231235959` (POSIX form to set time) is currently auto-approved.

**Fix needed**: Move `date` from `readOnlyCommands` to `commandAllowlist["date"]` with
`IsDangerous` blocking positional args (non-flag tokens).

---

#### Gap 4: `hostname` positional arg — hostname change

**TypeScript**: `hostname` is in `COMMAND_ALLOWLIST` with `IsDangerous` callback blocking
positional args like `hostname newname` (sets system hostname).

**Go**: `hostname` is in `readOnlyCommands` (any flags OK).

**Impact**: `hostname evil.internal` is currently auto-approved.

**Fix needed**: Same pattern — move to `commandAllowlist["hostname"]` with positional-blocking
`IsDangerous`.

---

#### Gap 5: `tput` dangerous capabilities

**TypeScript**: `tput` is in `COMMAND_ALLOWLIST` with `IsDangerous` callback blocking
`tput init`, `tput reset`, `tput isgr0` (these execute terminfo programs from `$TERMINFO`).

**Go**: `tput` is in `readOnlyCommands` (any flags OK, no capability check).

**Impact**: `tput init` (executes terminfo `is1`/`is2`/`is3` sequences, can run arbitrary
programs) is currently auto-approved.

**Fix needed**: Move to `commandAllowlist["tput"]` with `IsDangerous` blocking `init`, `reset`,
and other executable capabilities.

---

#### Gap 6: `lsof +m` — mount supplement file creation

**TypeScript**: `lsof` is in `COMMAND_ALLOWLIST` with `IsDangerous` callback blocking
`+m` (creates a mount supplement file on disk).

**Go**: `lsof` is in `readOnlyCommands` (any flags OK). Note: `+m` starts with `+` not `-`,
so it passes the `hasUnquotedDollar` check.

**Impact**: `lsof +m /tmp/mounts` is currently auto-approved.

**Fix needed**: Move to `commandAllowlist["lsof"]` with `IsDangerous` blocking `+m`.

---

#### Gap 7: `tree -R` — writes 00Tree.html files

**TypeScript**: `tree` is in `COMMAND_ALLOWLIST` with `-R` explicitly blocked
(creates HTML output files to disk).

**Go**: `tree` is in `readOnlyCommands` (any flags OK).

**Impact**: `tree -R /path` writes `00Tree.html` files and is currently auto-approved.

**Fix needed**: Move to `commandAllowlist["tree"]` blocking `-R`/`--fromfile`.

---

#### Gap 8: Full sed validation missing (task #4 in progress)

**TypeScript**: `sedCommandIsAllowedByAllowlist()` has two layers:
1. Allowlist: only `s/pattern/replacement/flags` and `-n` line-print patterns
2. Denylist: 30+ patterns including non-ASCII chars, `{}` blocks, `!` negation,
   `~` tilde addresses, `y` command, newlines, `#` comments, `\w` backreferences,
   `/pattern\s+[wWeE]`, malformed `s///`, substitution w/e flags

**Go**: `sedExprIsDangerous()` only checks for w/W/e/E commands — missing most denylist
checks. Task #4 is expected to port the full validation.

**Impact**: `sed '{q}' file`, `sed '1~2p'`, `sed '/x/! d'` are all currently auto-approved.

---

### 2b. Missing Commands/Allowlist Entries

These TypeScript-approved commands are absent from the Go allowlist (many being added by
task #7 with TDD tests already written):

| Command | TypeScript source | Go status |
|---------|------------------|-----------|
| `gh pr/issue/repo/run/auth/release/workflow` | `GH_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git ls-remote` with URL blocking | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git merge-base` | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git grep` | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git stash show` | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git worktree list` | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git remote show <name>` | `GIT_READ_ONLY_COMMANDS` | Missing (task #7 adds) |
| `git reflog show` (allow) vs `expire` (deny) | `GIT_READ_ONLY_COMMANDS` | Partially: reflog fully allowed |
| `fd`/`fdfind` | `COMMAND_ALLOWLIST` | Missing (task #7 adds) |
| `jq` with -f/--from-file blocking | `COMMAND_ALLOWLIST` | Missing (task #7 adds) |
| `ss` (socket statistics) | `COMMAND_ALLOWLIST` | Missing (task #7 adds) |
| `netstat` | `COMMAND_ALLOWLIST` | Missing (task #7 adds) |
| `pyright` (blocks --watch) | `COMMAND_ALLOWLIST` | Missing (task #7 adds) |
| `sleep` | `READONLY_COMMANDS` | Missing from readOnlyCommands |
| `seq` | `READONLY_COMMANDS` | Missing |
| `uniq` | `READONLY_COMMAND_REGEXES` | Missing |
| `history` | `READONLY_COMMAND_REGEXES` | Missing |
| `alias` | `READONLY_COMMAND_REGEXES` | Missing |
| `ip addr` (specific form only) | `READONLY_COMMAND_REGEXES` | Missing |
| `ifconfig` | `READONLY_COMMAND_REGEXES` | Missing |
| `node -v` (version only) | `READONLY_COMMAND_REGEXES` | Missing |

### 2c. Missing Infrastructure Features

| Feature | TypeScript | Go |
|---------|------------|-----|
| Auto-backgrounding after 15s (`ASSISTANT_BLOCKING_BUDGET_MS`) | `BashTool.tsx:spawnShellTask` | Not implemented |
| Bare git repo detection (`isCurrentDirectoryBareGitRepo`) | `readOnlyValidation.ts` | Not implemented |
| UNC path detection (`containsVulnerableUncPath`) | `readOnlyCommandValidation.ts` | Not implemented (Linux-less critical) |
| `_simulatedSedEdit` internal field | `BashTool.tsx:applySedEdit` | Not needed (internal testing hook) |
| `dangerouslyDisableSandbox` input field | `BashTool.tsx` | Not implemented |
| Separate `persistedOutputPath`/`persistedOutputSize` in output schema | `BashTool.tsx` | Embedded in content string |
| `assistantAutoBackgrounded` / `backgroundedByUser` output fields | `BashTool.tsx` | Not implemented |

---

## 3. Divergences

Same feature exists in both but behavior differs:

### D1: `ps -e` BSD flag handling
**TypeScript**: IsDangerous callback on `ps` flags BSD's `-e` (shows environment variables on macOS).
**Go**: `allowlist.go` lists `-e` as safe flag `FlagNone`.  
**Risk**: On macOS, `ps -e` dumps process environment variables, potentially exposing secrets.

### D2: `git config` write-through
**TypeScript**: Only `git config --get` form is allowed (keys only, no values).  
**Go**: `commandAllowlist["git config"]` has read-oriented flags but `validateFlags` allows
positional args (value writes) since there's no `IsDangerous` blocking them.

### D3: `sed` validation depth  
**TypeScript**: 30+ denylist patterns via `containsDangerousOperations()`.  
**Go**: Only checks `-i`, w/W/e/E in expression. Missing: `{}` blocks, `!`, `~`, `y`, newlines,
non-ASCII, `#` comments, malformed s/// (task #4 is porting this).

### D4: `docker ps`/`docker images` classification
**TypeScript**: In `EXTERNAL_READONLY_COMMANDS` = simple list, any flags OK.  
**Go**: In `commandAllowlist` with flag validation (stricter, but correct behavior).

### D5: `base64` double-dash behavior
**TypeScript**: `base64` in COMMAND_ALLOWLIST with `RespectsDoubleDash: false` (macOS base64
doesn't support `--`).  
**Go**: `base64` in `readOnlyCommands`, no double-dash logic, no flag validation.

### D6: Truncation threshold
**TypeScript**: 30K chars (appears to be `DEFAULT_MAX_OUTPUT`).  
**Go**: `DefaultMaxOutput = 30_000` but also `MaxOutputUpper = 150_000`. The 30K is the
default cap passed to `TruncateOutput()` — matches TypeScript.

### D7: `echo` safety check
**TypeScript**: READONLY_COMMAND_REGEXES has complex echo regex blocking `$` in double-quoted
context.  
**Go**: `hasUnquotedDollar` blocks `$` outside single quotes — more conservative (blocks even
`echo "literal"` if `$` present inside double quotes). Behavior is stricter, not a regression.

### D8: Progress polling threshold
**TypeScript**: `PROGRESS_THRESHOLD_MS = 2000` — only shows progress UI after 2s.  
**Go**: Progress callback fires from the first tick (~1s). No 2s threshold guard. Minor
behavioral difference in UI latency, not a functional regression.

### D9: readOnlyGitSubcommands vs allowlist entry for `reflog`
**TypeScript**: `git reflog` has IsDangerous callback (selective approval).  
**Go**: `readOnlyGitSubcommands["reflog"] = true` (always approved). Security regression (see Gap 1).

### D10: Missing commands in Go vs additions in Go

**In TypeScript but NOT in Go's readOnlyCommands** (Go more restrictive, functionally correct):
`sleep`, `seq`, `tsort`, `pr`, `uniq`, `history`, `alias`, `ifconfig`, `ip addr`

**In Go's readOnlyCommands but NOT in TypeScript** (Go is more permissive, may or may not be correct):
`less`, `more`, `exa`, `eza`, `ack`, `ag`, `top`, `htop`, `ping`, `dig`, `nslookup`, `host`,
`b2sum`, `cksum`, `lsb_release`

---

## 4. Recommended Additional Tests

### 4a. Security Regression Tests (write `parity_test.go`)

Tests that verify the current regressions are caught once fixes land:

```
// git reflog
"git reflog expire --all"        → NOT read-only
"git reflog delete HEAD@{0}"     → NOT read-only
"git reflog exists HEAD"         → NOT read-only
"git reflog show HEAD"           → IS read-only
"git reflog --oneline"           → IS read-only

// git config write
"git config user.email x@y.com"          → NOT read-only
"git config core.hooksPath /evil"        → NOT read-only
"git config --global user.name Evil"    → NOT read-only
"git config --get user.email"           → IS read-only
"git config --list"                     → IS read-only

// date write
"date 1231235959"     → NOT read-only
"date 12312359"       → NOT read-only
"date --set=now"      → NOT read-only (TypeScript: blocks positional)
"date +%Y-%m-%d"      → IS read-only (format string, not setting)
"date -R"             → IS read-only (RFC 2822 output)

// hostname write
"hostname evil.internal"   → NOT read-only
"hostname newhost"         → NOT read-only
"hostname"                 → IS read-only (just display)
"hostname -f"              → IS read-only

// tput dangerous capabilities
"tput init"    → NOT read-only
"tput reset"   → NOT read-only
"tput clear"   → IS read-only (just outputs escape sequence)
"tput cols"    → IS read-only

// lsof +m
"lsof +m"                → NOT read-only
"lsof +m /tmp/mounts"    → NOT read-only
"lsof -i"               → IS read-only
"lsof -nP"              → IS read-only

// tree -R
"tree -R"             → NOT read-only
"tree -R -H ."        → NOT read-only
"tree -L 2"           → IS read-only
"tree --fromfile"     → NOT read-only (reads from stdin/file to build tree, writes HTML)
```

### 4b. Sed Denylist Tests (once task #4 lands)

```
// Curly brace blocks
"sed '{q}' file"           → NOT read-only (curly brace = block execution)
"sed '1{p}' file"          → NOT read-only
"sed '/x/{d}' file"        → NOT read-only

// Tilde step addresses
"sed '1~2p' file"          → NOT read-only (tilde is sed extension, dangerous)
"sed '0~3d' file"          → NOT read-only

// Negation operator
"sed '/x/! d' file"        → NOT read-only (! negation can bypass pattern guards)
"sed '1! q' file"          → NOT read-only

// y command (transliterate — same power as tr but in sed)
"sed 'y/abc/ABC/' file"    → NOT read-only (y = transliterate)

// Non-ASCII in expression (obfuscation vector)
"sed 's/α/β/' file"        → NOT read-only (non-ASCII)

// Newline in expression
$'sed "s/a/b\nc" file'     → NOT read-only (newline embeds second command)

// Comment (can hide dangerous payload from log scanners)
"sed '# comment; w /etc/passwd' file"  → NOT read-only

// s/// with w flag (writes to file)
"sed 's/a/b/w /tmp/out' file"   → NOT read-only (w flag in substitution)
"sed 's/a/b/e' file"            → NOT read-only (e flag executes replacement as shell)
```

### 4c. GH Command Tests (once task #7 lands)

```
// Safe GH read commands
"gh pr view 123"                    → IS read-only
"gh pr list --state open"           → IS read-only
"gh issue view 42"                  → IS read-only
"gh repo view owner/repo"           → IS read-only
"gh run list --branch main"         → IS read-only
"gh auth status"                    → IS read-only
"gh release list --limit 10"        → IS read-only
"gh workflow list"                  → IS read-only

// Dangerous GH patterns (HOST/OWNER/REPO, URLs, SSH)
"gh pr view 1 --repo evil.com/owner/repo"     → NOT read-only
"gh pr view https://github.com/.../pull/1"    → NOT read-only
"gh repo view host/owner/repo"               → NOT read-only
"gh pr view 1 --repo git@github.com:o/r"     → NOT read-only

// Write operations
"gh pr create"         → NOT read-only
"gh pr merge 123"      → NOT read-only
"gh issue create"      → NOT read-only
"gh repo create"       → NOT read-only
"gh workflow run ci"   → NOT read-only
```

### 4d. Additional allowlist validation Tests (once task #7 lands)

```
// jq blocking
"jq -f filter.jq file"          → NOT read-only (-f reads from file, could be dangerous)
"jq --from-file filter"         → NOT read-only
"jq --rawfile var file '.'`     → NOT read-only
"jq '.' file.json"              → IS read-only
"jq -r '.name' pkg.json"        → IS read-only

// pyright --watch blocking
"pyright --watch"   → NOT read-only (launches long-running watcher)
"pyright -w"        → NOT read-only
"pyright src/"      → IS read-only
"pyright --version" → IS read-only

// curl -o/-O blocking
"curl -o output.txt https://x.com"     → NOT read-only (writes to file)
"curl -O https://x.com/file.tar"       → NOT read-only
"curl --output f https://x.com"        → NOT read-only
"curl https://api.example.com"         → IS read-only

// wget --spider only
"wget https://example.com"              → NOT read-only (downloads)
"wget -O output.txt https://x.com"     → NOT read-only
"wget --spider https://example.com"    → IS read-only
```

### 4e. Edge Cases for Existing Validators

```
// ps -e on macOS (shows env vars)
// Note: This is platform-specific. May want runtime GOOS check.
"ps -e"   → consider NOT read-only on darwin

// git stash write operations
"git stash"              → NOT read-only
"git stash push"         → NOT read-only
"git stash pop"          → NOT read-only
"git stash drop"         → NOT read-only
"git stash clear"        → NOT read-only
"git stash show"         → IS read-only
"git stash list"         → IS read-only

// git worktree (once task #7 adds it)
"git worktree list"             → IS read-only
"git worktree list --porcelain" → IS read-only
"git worktree add /path"        → NOT read-only
"git worktree remove /path"     → NOT read-only
"git worktree prune"            → NOT read-only

// compound commands with mixed read/write
"git log --oneline && git config user.email x@y.com"  → NOT read-only
"date && hostname newname"                             → NOT read-only
```

---

## 5. Summary

| Category | Count |
|----------|-------|
| Matched features | ~45 |
| Security regressions (gaps) | 8 critical |
| Missing commands/allowlist entries | ~20 (task #7 covers most) |
| Missing infrastructure features | 7 |
| Behavioral divergences | 10 |

**Priority order for fixes:**
1. `git reflog expire` blocking (Gap 1) — reflog expire destroys git history
2. `git config` write blocking (Gap 2) — modifies user identity/hooks path
3. Full sed validation (Gap 8, task #4) — sed is a powerful command interpreter
4. `date`/`hostname` positional blocking (Gaps 3–4) — system configuration changes
5. `tput init/reset` blocking (Gap 5) — executes terminfo scripts
6. `lsof +m` blocking (Gap 6) — creates files on disk
7. `tree -R` blocking (Gap 7) — creates HTML files on disk

Tasks #7 and #1 cover the missing allowlist entries. Task #4 covers sed validation.
