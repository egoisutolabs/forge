# Contributing

Thanks for your interest in Forge. Forge is in early access and the
public surface is intentionally small while the core stabilizes, so
contributions are currently limited to bug reports and focused fixes.

## Development setup

Requires Go 1.26+.

```bash
git clone https://github.com/egoisutolabs/forge.git
cd forge
make build
```

The binary lands at `bin/forge`.

## Git hooks (optional but recommended)

Forge ships `pre-commit` and `pre-push` hooks in `.githooks/` that run the
same checks CI does. Enable them once after cloning:

    make install-hooks

This points `core.hooksPath` at `.githooks/`, so:

- `pre-commit` runs `gofmt`, `go vet`, and a `go mod tidy` drift check
  (fast — typically under 10 seconds).
- `pre-push` runs the full unit test suite with `-race` (about 60 seconds).

To uninstall: `git config --unset core.hooksPath`.

## Before opening a pull request

1. `make fmt`
2. `make vet`
3. `make test`
4. Keep changes focused. Don't bundle refactors with bug fixes.
5. Match the existing style of the package you're touching.

## Reporting bugs

Open an issue with:

- What you ran (command, model, relevant config)
- What you expected
- What happened (include logs or terminal output)
- Forge version or commit hash

## License

By contributing, you agree that your contributions will be licensed
under the MIT license that covers the project.
