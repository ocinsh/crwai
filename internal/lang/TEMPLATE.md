# Adding a language

For the project's *why* and the public surface, see [`README.md`](../../README.md);
this file is the step-by-step **how**. Use `golang/golang.go` as the worked
reference — every other subpackage copies its exact shape.

A language is considered **done** only when all four pieces exist (look at any
implemented language, e.g. `golang/`, for the concrete shape):

- `internal/lang/<name>/<name>.go` — the implementation (steps 1–8 below).
- `internal/lang/<name>/<name>_test.go` — Go unit tests over the example corpus.
- `internal/lang/<name>/script.sh` — the end-to-end harness (step 9).
- `examples/<name>/` — sample sources the tests and harness run against (step 9).

## 1. Create the subpackage

`internal/lang/<name>/<name>.go`, `package <name>`. Define a stateless value type:

```go
type Lang struct{} // zero value ready; no per-call state (server is stateless)

var (
    _ core.Language       = (*Lang)(nil) // read capabilities — required
    _ core.FunctionWriter = (*Lang)(nil) // write-back — OMIT for a read-only language
)
```

The methods can use a value receiver (`func (Lang) ...`); the `(*Lang)(nil)`
assertions still hold because value methods are in the pointer method set.

## 2. Implement the metadata + parse

- `Name() string` — canonical lowercase name; the registry key (also the value the
  `--lang` flag / MCP `lang` field matches on).
- `Extensions() []string` — each with a leading dot (e.g. `".rs"`).
- `Parse(src []byte) (core.Source, error)` — import the grammar package and
  delegate to the shared core parse helper:
  ```go
  return core.Parse(ts.NewLanguage(ts<name>.Language()), src)
  ```
  Imports: `ts "github.com/tree-sitter/go-tree-sitter"` and the grammar. The usual
  path is `ts<name> "github.com/tree-sitter/tree-sitter-<name>/bindings/go"`, but a
  language without an official grammar uses a community one with its own import path
  and caveats — see Dart, whose grammar (`github.com/UserNobody14/tree-sitter-dart`)
  is pinned in `go.mod` and must never be reconciled with `go mod tidy` (it would
  pull a broken nested module; the pin is what keeps `go build`/`test`/`run` working).

## 3. Implement the read capabilities

`ListSignatures`, `FunctionBody`, `Function`, `ReadInterface`, `ReadStruct`. Each
takes an already-parsed `core.Source` (never raw bytes — one parse per call) and
addresses symbols by `core.SymbolID`, never by file offset.

If the language lacks a construct, the capability still has to exist (it is part of
`core.Language`), so pick one of two patterns the codebase already uses:

- return `core.ErrSymbolNotFound` — the capability simply never matches (C and
  JavaScript `ReadInterface` do this);
- return a package-local typed error when you want callers to distinguish "not
  supported" from "not found" (C++ `ReadInterface` returns
  `cpp.ErrInterfacesUnsupported`).

Some languages instead **map** a near-equivalent construct (Dart reads an
`abstract`/`interface` class via `ReadInterface` and a concrete class via
`ReadStruct`).

## 4. Implement write-back (optional)

`ResolveEdits(src, edits) ([]core.ResolvedEdit, error)` maps each `Edit` to a
concrete `[start, end)` byte span on `src` — the whole symbol when `Edit.Rel ==
nil`, or the symbol-relative span when set. Do NOT mutate `src` or touch disk:
`core.BatchWrite` orders, overlap-checks, applies bottom-up, re-parses, and
persists atomically. Omit this method for a read-only language; the server detects
its absence via `lang.(core.FunctionWriter)` and reports `ErrReadOnlyLanguage`.

The v1 path only replaces whole symbols: when `Edit.Rel != nil`, return a
package-local `ErrRelativeRangeNotImplemented` (see `golang.go`).

## 5. S-expression queries

Declare the tree-sitter queries as package constants (single source of truth for
the node kinds each capability targets). Typical captures: functions/methods,
interfaces/traits/protocols, structs/classes. Inspect the grammar's `node-types`
for exact kind names.

## 6. Documentation strategy (per language family)

- **Preceding-sibling comments** (Go, Rust, Dart, TypeScript, JavaScript, Java, C,
  C++): the doc is the comment node(s) immediately above the declaration.
- **Docstring inside the body** (Python): the doc is the first string-literal
  statement inside the function/class body — not a sibling comment.

## 7. Building `SymbolID.Container`

- Go: receiver type of the method (`""` for a free function).
- Rust: type of the enclosing `impl` block (or the trait).
- Java / Python / Dart / C++: the enclosing class.
- TypeScript / JavaScript: the enclosing class, and for TS also the enclosing
  `namespace`/`module`.
- C: always `""` (no methods; structs are top-level — same-named symbols are
  disambiguated by `SymbolKind`, not by container).

## 8. Register it

In `engine.go` (the `New` constructor at the repository root) add **both**:

```go
import "github.com/ocinsh/crwai/internal/lang/<name>" // in the import block
...
reg.Register(<name>.Lang{})                            // in New()
```

The registry indexes the language by name and by every extension, so it is selected
automatically from a file's extension and, when needed, forced by name (the CLI
`--lang`/`-l` flag and the MCP tools' optional `lang` field).

A multi-grammar language registers **more than one** type from a single module —
e.g. TypeScript registers `typescript.TypeScript{}` (`.ts`) and `typescript.TSX{}`
(`.tsx`), which share all read/write logic and differ only in
`Name`/`Extensions`/`Parse`.

## 9. Tests, examples, and the end-to-end harness

These are not optional — a language without them is not done.

- `examples/<name>/` — representative sources (one "big" file with ≥ 20 symbols,
  plus structs/classes, methods, and interfaces/traits where the language has
  them). Include the edge files other languages ship: UTF-8 BOM, CRLF line endings,
  and a file with no trailing newline.
- `internal/lang/<name>/<name>_test.go` — Go unit tests that parse the corpus and
  assert every read capability and `ResolveEdits`. Edge cases that would break
  `gofmt`/build if committed as real files (BOM/CRLF/no-newline) can be synthesized
  in the test from a clean example.
- `internal/lang/<name>/script.sh` — copy an existing one (e.g.
  `golang/script.sh`). It exercises three layers in order — the Go unit tests, the
  MCP server over stdio (every read tool plus the write reversibility / all-or-
  nothing / parse-rejection rules), and the human CLI — works only on a `mktemp`
  copy (never mutates `examples/`), needs no network, is idempotent, prints
  `[OK]`/`[FAIL]` per test with a final `Passed: X/Y`, and exits 0 iff all passed.

## 10. CGO hygiene

The grammar bindings allocate C memory. Any `Parser`/`Tree`/`Query`/`QueryCursor`/
`TreeCursor` you create must be `Close()`d (via `defer`). The shared parse helper
owns the `Source`'s parser+tree and releases them on `Source.Close()`. Build needs
a C toolchain — never `CGO_ENABLED=0`.

## 11. Keep the docs aligned

Per the mandatory rule in `CLAUDE.md`, a new language is a real change: update both
`CLAUDE.md` and `README.md` (the language list, the status section, and the example
commands) in the same change.
