# crwai — build, test, and install the tree-sitter MCP server / CLI.
# The product name and version are sourced from the Go constant in core/version.go
# so this Makefile never duplicates the version string.

BIN     := crwai
PKG     := ./cmd/crwai
DISTDIR := dist
VERSION := $(shell go run $(PKG) version 2>/dev/null | tr -dc '0-9.' )

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

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format all Go sources
	gofmt -w .

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	go mod tidy

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
