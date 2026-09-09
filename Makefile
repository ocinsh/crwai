# crwai — build, test, and install the tree-sitter MCP server / CLI.
# The product name and version are sourced from the Go constant in
# internal/core/version.go, so this Makefile never duplicates the version string:
# `make version` asks the binary.
#
# There is deliberately no `tidy` target. `go mod tidy` resolves the Dart grammar's
# broken nested module and breaks the build; the pin in go.mod is what holds it
# together (see the note there).

BIN     := crwai
PKG     := ./cmd/crwai
DISTDIR := dist

# CGO is required: the tree-sitter grammars are C. Do NOT set CGO_ENABLED=0.
export CGO_ENABLED := 1

.DEFAULT_GOAL := build

.PHONY: build
build: ## Compile the binary into dist/
	go build -o $(DISTDIR)/$(BIN) $(PKG)

.PHONY: install
install: ## Install the binary into $GOBIN / $GOPATH/bin
	go install $(PKG)

.PHONY: run
run: ## Run the MCP server over stdio (the default, no-subcommand action)
	go run $(PKG)

.PHONY: test
test: ## Run the test suite
	go test ./...

.PHONY: check
check: ## Run everything CI runs: format check, vet, tests, and the language harnesses
	@test -z "$$(gofmt -l .)" || { echo "gofmt: these files need formatting:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...
	@for s in internal/lang/*/script.sh; do echo "--- $$s"; bash $$s || exit 1; done

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format all Go sources
	gofmt -w .

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DISTDIR)

.PHONY: version
version: ## Print the product version
	@go run $(PKG) version

.PHONY: help
help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
