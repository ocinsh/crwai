# Agent guide

Use this file to navigate and change this repository. Read `README.md` for the product behavior. Read `internal/lang/TEMPLATE.md` before adding a language. Write documentation, comments, commit messages, and user-facing text in clear English. Avoid long dashes and decorative characters.

## First steps

1. Run `git status --short` before and after work. Preserve existing changes. Several agents may intentionally share this checkout. Do not create a branch unless explicitly asked.
2. Locate the owner of the behavior before editing. The package map below gives the boundaries.
3. Keep `AGENTS.md` and `README.md` accurate in the same change whenever project structure, commands, public API, contracts, dependencies, or implementation status change.
4. Add a test for each new guarantee. Run focused tests, then `make check`, `make build`, `git diff --check`, and inspect `git diff -- go.mod go.sum`. Do not run `go mod tidy`.
5. Before any commit, inspect `git log --oneline -20` and `git log -5 --format='%B'`. Follow the repository's prevailing English style, use small working commits, and add no signatures, trailers, or automatic attribution. Do not commit unless the task calls for it.

## Product and boundaries

`crwai` is a Go library with a public facade at the repository root, a human CLI, and an MCP stdio server. It uses tree-sitter to find and replace individual code symbols. The caller supplies replacement text. The library never generates code. Markdown section tools and a read-only Postman collection reader are separate document capabilities.

The dependency direction is `cmd/crwai` and `cmd/crwai/ui` to the public root package to `internal/`. Front ends use the public facade. `cmd/crwai/serve.go` alone imports `internal/mcptool` for MCP schemas and descriptors. Keep parsing and symbol rules out of the front ends.

The engine is stateless across calls. Disk is the source of truth. Each read call opens and parses one source, then closes it. Symbol identity is kind, name, and container, never a caller-provided byte offset. A nonempty container on a callable means method; use `TargetFor` or the facade's read methods rather than constructing identities in front ends.

Writes are batches with all-or-nothing semantics. `core.BatchWrite` reads, parses, resolves edits, checks overlaps, applies them in memory, reparses, checks staleness, and atomically renames a temporary file. `core.ApplyResolved` contains the shared format-independent tail for code and Markdown. A write through a symlink updates its target and preserves the link. Do not write directly from a language or document package. An empty batch fails with `ErrNoEdits`. The staleness check is not a lock: serialize writers to the same file.

`Engine.Root(dir)` rejects paths that resolve outside the root, including `..` and symlink escapes. The library is unconfined by default; launch the MCP server with `--root` for a workspace boundary. `Engine.Lang(name)` and `Engine.Root(dir)` return copies and leave the receiver unchanged.

Tree-sitter requires CGO and a C toolchain. Close every C-backed parser, tree, query, query cursor, and tree cursor. Do not set `CGO_ENABLED=0`. Do not run `go mod tidy`: the pinned Dart grammar has a broken nested module at `bindings/go`, as explained in `go.mod`.

## Package map

| Location | Responsibility |
| --- | --- |
| `crwai.go` | Public code reader, writer, and service interfaces |
| `engine.go` | Engine, language registry, path boundary, parse per call, service orchestration |
| `types.go` | Public aliases, error sentinels, `ParseKind`, `TargetFor`, and `StripDocs` |
| `common.go` | Public document interfaces and Markdown/Postman facade |
| `internal/core/types.go` | Symbol identities, signatures, edits, and shared ranges |
| `internal/core/interfaces.go` | Small language capabilities; `DeclarationReader` is required and `FunctionWriter` is optional |
| `internal/core/source.go` | Parsed source handle and implemented `core.Parse` helper |
| `internal/core/write.go` | Shared atomic write pipeline and result types |
| `internal/core/errors.go` | Shared sentinel errors; re-export every new sentinel from root `types.go` |
| `internal/release/` | GitHub release lookup, version validation, checksum verification, binary replacement |
| `internal/lang/registry.go` | Language lookup by name and extension |
| `internal/lang/<language>/` | Grammar, symbol listing, readers, and edit resolution for one language |
| `internal/common/markdown/` | Explicit Markdown outline, section read, and section write |
| `internal/common/postman/` | Explicit read-only Postman Collection v2.1 reader |
| `internal/mcptool/schemas.go` | Typed MCP inputs and outputs; SDK infers JSON schema |
| `internal/mcptool/descriptors.go` | Agent-facing descriptions for eight code and five document tools |
| `internal/lang/preview.go` | Shared tree-sitter preview redaction for callable bodies |
| `cmd/crwai/serve.go` | MCP registration and thin adapters to the public engine |
| `cmd/crwai/root.go` | Cobra root, persistent flags, and shared output path |
| `cmd/crwai/ui/` | Terminal tree, color, and plain-text rendering only |
| `.codex/config.toml` | Project-level Codex MCP server configuration with repository root confinement |
| `docs/install-codex.md` | Build, activation, and verification guide for Codex |
| `docs/releases.md` | Tag, package, install, and update procedure |

The root is the only public API. The library addresses path plus symbol identity. `internal/` is not importable from other Go modules. Common document tools are selected explicitly; they do not enter the language registry or use tree-sitter.

Markdown heading paths use the heading text and ancestor paths. When a path repeats, the next occurrence gets a numeric suffix such as `Guide/Notes [2]`; its children inherit that distinct path. The scanner tracks fence marker and width, so only a matching closing fence of sufficient width ends a fenced code block.

## Languages and symbols

Go, Python, Java, JavaScript, C, C++, Rust, Dart, and TypeScript are implemented end to end. TypeScript and TSX are registered separately but share one implementation. The Go package is the reference implementation. Each language has unit tests, an end-to-end `script.sh`, and fixtures under `examples/<language>/`, including a declarations fixture. Follow `internal/lang/TEMPLATE.md` for a new language and register it in `engine.go`.

`ListSignatures` reports seven kinds: `func`, `method`, `interface`, `struct`, `const`, `var`, and `type`. Every listed symbol must be openable through `ReadDeclaration`. `DeclarationReader` is therefore part of the required `core.Language` interface. `TestEverySymbolListedCanBeOpened` checks the bundled corpus. Never list declarations inside a function body. Consult the `What counts as a symbol` table in `README.md` for language-specific mapping. Enums are read as structs. Interface-like constructs vary by language; follow existing package behavior rather than inventing a construct.

`Engine.SeeFile` parses once, lists symbols, and uses `internal/lang/preview.go` to replace callable bodies in the original source while preserving comments and documentation. Its JSON has top-level `signatures` and `content`; each signature has `name`, `type`, and optional `container`. Python docstrings stay visible. Keep this read-only view under tests for every supported language; it is a preview, not a compilable source transformation.

Signature documentation is opt-in via CLI `--doc` or the MCP `doc` field. `StripDocs` is the common removal path. `ParseKind` must recognize every listed kind. `ReadDeclaration` accepts an empty kind to search by name across kinds. `TargetFor` handles the read/write identity convention.

A language's `ResolveEdits` maps symbols to spans and never touches disk. The current MCP schema deliberately omits `Edit.Rel`: relative ranges are not implemented in any language. If extending the wire format, update schema, descriptor, facade, scripts, and docs together.

## Front ends and tests

The CLI has one mutation command, `write`, with mutually exclusive `--name` for code and `--heading` for Markdown. Commands call the public facade and emit through `emit(cmd, data, human)` in `cmd/crwai/root.go`; results go to stdout and errors to stderr. Do not use Cobra `Print*` helpers. The UI owns colors and tree layout. Do not add emoji to CLI output. Box-drawing tree characters are allowed.

MCP input/output structs live in `internal/mcptool/schemas.go`. The SDK infers JSON schema from tags. Keep each descriptor's parameter text aligned with its input struct, including optional fields. Code tools accept a per-call language override; document tools do not. The server has eight code tools and five document tools. Its initialization instructions recommend crwai when symbol-level work reduces context, without overriding user or repository instructions.

`make check` runs gofmt verification, `go vet ./...`, `go test ./...`, and all nine language harnesses. GitHub Actions runs only on a pushed `vX.X.X` tag: `.github/workflows/release.yml` checks Linux and macOS, builds the packages, and verifies that `go.mod` and `go.sum` were not changed by a command. `make build` writes `dist/crwai`. Avoid `make fmt` on a narrow change because it formats the entire repository.

The Codex MCP setup uses the project-level `.codex/config.toml`. It resolves the Git root at startup, runs the built binary from `dist/`, and passes that root to `serve --root`. Keep the installation steps in `docs/install-codex.md` aligned with the configuration.

Release tags, package names, and `internal/core/version.go` use `vX.X.X`; the current source version is `v0.2.1`. The release workflow in `.github/workflows/release.yml` runs on tag push or manual dispatch with an existing tag, validates the source version, runs checks, builds four native CGO packages, and publishes checksums. Every job checks out the selected tag; the publish job needs Git metadata for `gh release create`. The CLI `install`, `check-update`, and `update` commands live in `cmd/crwai/distribution.go`; their download and verification logic belongs in `internal/release/`. The install wizard detects Codex in `PATH` or bundled with the macOS desktop app, prompts for each detected client, and registers the resolved path of its current executable without copying it. Codex registration is user-level and does not remove a project-level entry. Update replaces that same executable. Keep `docs/releases.md` aligned with these contracts.

Key tests: `internal/core/write_test.go` checks batch rejection, overlap, syntax, staleness, permissions, and temporary-file cleanup. `crwai_test.go` checks facade identity, root confinement, registry, Markdown, Postman, and listed-symbol readability. `internal/mcptool/descriptors_test.go` checks the MCP descriptor contract. Language and document packages contain their own tests.
