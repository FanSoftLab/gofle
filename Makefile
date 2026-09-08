# GoFLE lint/format automation
#
# These targets shell out to golangci-lint (https://golangci-lint.run).
# Install it with:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0
#
# CGO note: Wails v2 pulls a C toolchain, so golangci-lint type-checks need
# gcc (or your platform's C compiler) and CGO_ENABLED=1.

GOLANGCI ?= golangci-lint
PKGS      ?= ./...

.PHONY: lint lint-fix fmt lint-ci

# Report all findings (gate).
lint:
	$(GOLANGCI) run $(PKGS)

# Auto-fix what the linters can, then report the rest.
lint-fix:
	$(GOLANGCI) run --fix $(PKGS)

# Format with the configured formatters (gofmt + gofumpt).
fmt:
	$(GOLANGCI) fmt $(PKGS)

# Mirror the CI invocation locally (no cache, verbose).
lint-ci:
	$(GOLANGCI) run --timeout 5m --concurrency 4 $(PKGS)
