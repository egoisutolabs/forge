# Forge

Open-source AI coding agent for the terminal.

Forge reads your codebase, plans multi-step work, edits files, runs
commands, searches docs, and coordinates sub-agents from a single
terminal session. It's written in Go, ships as a single binary, and is
built for people who want a real coding agent — not just chat in a
shell.

## Status

Early access. The public surface is intentionally minimal while the
core stabilizes.

## What it feels like

```text
$ cd my-project
$ forge
> Fix the nil pointer in the login handler and run the relevant tests
```

Forge will typically:

1. Search the codebase to find the handler and its call sites
2. Read the relevant files to understand the bug
3. Edit the implementation
4. Run tests or verification commands
5. Stream the result in the TUI with tool output and diffs

You give Forge a task, and it decides how to complete it using tools,
permissions, iteration, and verification.

## Build

Requires Go 1.26+.

```bash
git clone https://github.com/egoisutolabs/forge.git
cd forge
make build
```

Or build directly:

```bash
go build -o bin/forge ./cmd/forge/
```

Optional: move it onto your `PATH`.

```bash
mv bin/forge /usr/local/bin/forge
```

### Optional tool dependencies

These improve capability but are not required for a basic session:

- `rg` (`ripgrep`) for fast code search
- `sg` (`ast-grep`) for structural search

## Quick start

Set an API key:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

Start Forge in a project:

```bash
cd ~/my-project
/path/to/forge/bin/forge
```

Ask for work, not code snippets:

```text
> Add a --verbose flag to the CLI and update the docs
> Investigate why tests are flaky on CI and propose a minimal fix
> Review my uncommitted changes for correctness and regressions
> Refactor the auth package to remove duplicated token parsing logic
```

## Providers

Forge supports multiple model backends:

- Anthropic
- OpenAI-compatible providers
- OpenRouter
- Groq
- Google
- Mistral
- xAI
- DeepInfra
- Ollama

Select models at startup:

```bash
forge --model claude-sonnet-4-6
forge --model opus
forge --model haiku
```

Or switch inside the TUI with `Ctrl+M`.

## Configuration

Forge reads configuration from, in order:

- CLI flags
- Environment variables
- `~/.forge/config.yaml` or `~/.forge/config.json`
- Project-level `.forge/config.yaml` or `.forge/config.json`
- `~/.forge/auth.json` for provider credentials

Common flags:

```bash
forge --model sonnet --max-turns 50 --max-budget 5 --cwd /path/to/project
```

Common environment variables:

- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `GROQ_API_KEY`
- `GOOGLE_API_KEY`
- `MISTRAL_API_KEY`
- `XAI_API_KEY`
- `DEEPINFRA_API_KEY`
- `FORGE_MODEL`
- `FORGE_DEBUG`

## Permissions

Forge is not YOLO-by-default.

- Read-only operations are auto-approved
- Writes and risky commands require confirmation
- Destructive shell commands get elevated warning treatment

That lets Forge move quickly on safe operations while still asking
before state-changing work.

## Development

```bash
make build       # build the binary
make test        # run unit tests
make test-race   # unit tests with the race detector
make vet         # go vet
make fmt         # gofmt
make clean       # remove build artifacts
```

Debug mode:

```bash
FORGE_DEBUG=1 forge
```

Minimal build:

```bash
go build -tags minimal ./cmd/forge/
```

## License

[MIT](LICENSE)
