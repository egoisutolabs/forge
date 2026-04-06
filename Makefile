.PHONY: all build test test-v test-race test-e2e cover lint fmt vet clean tidy install-hooks

# Default target
all: tidy fmt vet test build

# Build the binary
build:
	go build -o bin/forge ./cmd/forge/

# Run unit tests (skip e2e)
test:
	go test $$(go list ./... | grep -v /tests/)

# Verbose unit tests
test-v:
	go test -v $$(go list ./... | grep -v /tests/)

# Unit tests with race detector
test-race:
	go test -race $$(go list ./... | grep -v /tests/)

# Integration tests
test-integration:
	go test -v ./tests/integration/...

# E2E tests (requires ANTHROPIC_API_KEY)
test-e2e:
	go test -v -tags e2e ./tests/e2e/...

# All tests including integration
test-all:
	go test ./...

# Test coverage (unit tests only)
cover:
	go test -coverprofile=coverage.out $$(go list ./... | grep -v /tests/)
	go tool cover -func=coverage.out
	@rm -f coverage.out

# Coverage HTML report
cover-html:
	go test -coverprofile=coverage.out $$(go list ./... | grep -v /tests/)
	go tool cover -html=coverage.out -o coverage.html
	@rm -f coverage.out
	@echo "Open coverage.html in a browser"

# Run specific package tests (usage: make test-pkg PKG=./models)
test-pkg:
	go test -v $(PKG)

# Lint with go vet
vet:
	go vet ./...

# Format code
fmt:
	gofmt -w .

# Tidy modules
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Install git hooks from .githooks/
install-hooks:
	@chmod +x .githooks/pre-commit .githooks/pre-push
	git config core.hooksPath .githooks
	@echo ""
	@echo "Git hooks installed."
	@echo "  pre-commit: gofmt, go vet, go mod tidy drift check"
	@echo "  pre-push:   go test -race (unit suite)"
	@echo ""
	@echo "To uninstall: git config --unset core.hooksPath"
