# Crwai

**Crwai** — **C**ode, **R**ead, **W**rite, **A**rtificial **I**ntelligence.

An **MCP server in Go** that uses **tree-sitter** to query and surgically edit a
codebase at the level of a single symbol (function, method, interface, struct) —
instead of reading or rewriting whole files. It is a tool system that lets agents
read and write code efficiently, without wasting tokens on whole-file reads and
rewrites.

## Mission

Cut the context a coding agent spends on code. The agent doesn't read thousands of
lines: it asks the server for exactly the symbol it needs, in the form it needs
(signature, doc, body, or whole symbol), and when it must fix something it rewrites
a single symbol without touching the rest of the file.

## What makes it different

Existing tree-sitter MCP servers are read-only and written in Python. This one:

1. is in **Go**, shipped as a **single executable**;
2. does **surgical per-symbol write-back**, not just reads;
3. is both an **MCP server** and a **clean human CLI** over one library, so every
   capability can be driven and tested by hand.

The server is a **scalpel**, not an author: it locates a symbol, replaces it with
text the agent supplies, validates the re-parse, and rejects the change if the
syntax breaks. It never generates code.

## Architecture (binding decisions)

- **Transport:** stdio. A local process launched by the MCP client.
- **MCP SDK:** `github.com/modelcontextprotocol/go-sdk` (official).
- **tree-sitter binding:** `github.com/tree-sitter/go-tree-sitter` (official);
  grammars are separate packages (e.g. `.../tree-sitter-go/bindings/go`).
- **CGO:** the bindings are CGO; every C-allocating object (parser, tree, query,
  cursor) is `Close()`d via `defer`. Build requires a C toolchain — never
  `CGO_ENABLED=0`.
- **Stateless:** no session, no cache between calls. The disk is the only source of
  truth; every call re-reads and re-parses.
- **In-memory batch per call:** one write call may carry N edits; the file is loaded
  once, all N applied in memory, written once.
- **All-or-nothing:** if any edit breaks the final parse, the whole batch is
  rejected and the disk is untouched.
- **Symbol identity:** symbols are addressed by `SymbolID` (kind + name +
  container), never by file offset.

## Target languages

Go, Rust, Dart, Python, TypeScript, JavaScript, Java, C, C++. The agent picks the
reference language per call (resolved from the file extension). Each language lives
in its own subpackage under `internal/lang/` and implements the common interfaces.

## v1 tools

| Tool | Returns |
| --- | --- |
| `list_signatures` | signatures of every top-level symbol (the cheap map) |
| `get_function_body` | a function's body only |
| `get_function` | a whole function: doc + signature + body |
| `read_interface` | a full interface definition |
| `read_struct` | a full struct definition |
| `write_function` | applies N edits atomically (all-or-nothing) |

## Usage

With **no subcommand** the binary speaks MCP over stdio (so an MCP client can
launch it directly). Every capability is also a one-word subcommand with a short
alias, for driving the system from a clean CLI:

```sh
crwai                      # run the MCP server over stdio (== `crwai serve`)
crwai langs                # list supported languages and extensions
crwai signatures <file>    # map a file (aliases: sig, ls)
crwai function <file> <name> [-c <container>]   # whole function/method (alias: fn)
crwai body <file> <name>   # body only (alias: bd)
crwai interface <file> <name>                   # alias: iface
crwai struct <file> <name>                      # alias: st
crwai write <file> -n <name> -k <kind> -t <text>   # surgical edit (alias: wr)
crwai version              # product version
```

## Library

The same capabilities are exposed as a Go library through the root package
(`github.com/ocinsh/crwai`). The implementation lives under `internal/` and is not
importable from other modules; consumers depend only on the public façade:

```go
svc := crwai.New()                              // *Engine, implements crwai.Service
sigs, err := svc.ListSignatures("core/types.go")
res,  err := svc.Write("core/write.go", crwai.Edit{
    Target:  crwai.SymbolID{Kind: crwai.KindFunc, Name: "BatchWrite"},
    NewText: "func BatchWrite(...) (...) { ... }",
})
```

## Out of scope for v1

- Cross-file search and operations (needs a repository index).
- `refactor_function_name` (intrinsically cross-file: renaming must update
  call-sites in other files).

## Status

The shared scaffolding is **implemented**: the parse helper and concrete `Source`
(`core.Parse` in `internal/core/source.go`) and the all-or-nothing write pipeline
(`core.BatchWrite` in `internal/core/write.go`, the 8-step contract) plus tool
registration (`mcptool.Register`). On top of it, **Go** (`internal/lang/golang`,
the reference implementation), **Python** (`internal/lang/python`),
**Java** (`internal/lang/java`), **JavaScript** (`internal/lang/javascript`),
**C** (`internal/lang/c`), **C++** (`internal/lang/cpp`), **Rust** (`internal/lang/rust`),
**Dart** (`internal/lang/dart`) and **TypeScript** (`internal/lang/typescript`) are
fully implemented — read (`list_signatures`,
`get_function`, `get_function_body`, `read_struct`, `read_interface`) and atomic
write (`write_function`), with their tree-sitter grammars wired and CGO required to
build. C++ has no `interface` construct, so its `read_interface` reports a typed error
(`cpp.ErrInterfacesUnsupported`) — read an abstract class as a struct instead. C also
has no `interface` construct (and no method container): `read_interface` reports
`core.ErrSymbolNotFound`, and symbols sharing a name are disambiguated by kind
(e.g. `struct list` vs the function `list`). Dart has no distinct interface
declaration, so `read_struct` maps to a concrete class and `read_interface` to an
`abstract` class; its community grammar is pinned and `go mod tidy` must be avoided
(see `internal/lang/dart/GRAMMAR.md`). TypeScript ships as **two** grammars from the
same `tree-sitter-typescript` module: it registers as two languages — `typescript`
for `.ts` (pure grammar) and `tsx` for `.tsx` (JSX-aware grammar) — sharing one
implementation; `read_interface` reads a real `interface`, `read_struct` reads a
`class` or an object-typed `type` alias, and namespace/module functions are
addressed by `Container`. The
remaining language subpackages still carry contract comments and return
`core.ErrNotImplemented` until their grammar is wired (see `internal/lang/TEMPLATE.md`).
Building requires a C toolchain — never `CGO_ENABLED=0`.

Try it on the bundled corpus:

```sh
make build
./dist/crwai signatures examples/golang/big.go        # >= 20 symbols
./dist/crwai function   examples/golang/methods.go Greet -c Greeter  # disambiguated by container
./dist/crwai signatures examples/python/shapes.py     # note: human output is on stderr
./dist/crwai signatures examples/java/Catalog.java    # >= 20 symbols
./dist/crwai function   examples/java/Greeter.java greet -c Greeter
./dist/crwai signatures examples/javascript/complex.js   # >= 20 symbols
./dist/crwai struct     examples/javascript/shapes.js Circle
./dist/crwai signatures examples/cpp/big.cpp          # >= 20 symbols
./dist/crwai function   examples/cpp/shapes.cpp area -c Circle  # disambiguated by container
./dist/crwai signatures examples/c/complex.c          # >= 20 symbols
./dist/crwai struct     examples/c/shapes.c Point      # C struct definition
./dist/crwai signatures examples/rust/complex.rs       # >= 20 symbols
./dist/crwai function   examples/rust/methods.rs size -c Stack  # disambiguated by container
./dist/crwai signatures examples/typescript/complex.ts   # >= 20 symbols
./dist/crwai interface  examples/typescript/shapes.ts Shape   # real interface
./dist/crwai function   examples/typescript/shapes.ts area -c Circle  # disambiguated by container
./dist/crwai signatures examples/typescript/widget.tsx   # .tsx uses the TSX grammar
./dist/crwai signatures examples/dart/math_rich.dart   # >= 20 symbols
./dist/crwai function   examples/dart/shapes.dart describe -c Rectangle  # disambiguated by container
./dist/crwai interface  examples/dart/shapes.dart Shape   # abstract class as interface
bash internal/lang/golang/script.sh                   # Go end-to-end (unit + MCP + CLI), prints Passed: 32/32
bash internal/lang/python/script.sh                   # CLI + MCP end-to-end, prints Passed: N/N
bash internal/lang/cpp/script.sh                      # C++ end-to-end (CLI + MCP), prints Passed: 19/19
bash internal/lang/javascript/script.sh               # JavaScript end-to-end, prints Passed: 22/22
bash internal/lang/java/script.sh                     # Java collaudo, prints Passed: 16/16
bash internal/lang/c/script.sh                        # C end-to-end (unit + CLI + MCP), prints Passed: 17/17
bash internal/lang/rust/script.sh                     # Rust end-to-end (unit + MCP + CLI), prints Passed: 21/21
bash internal/lang/dart/script.sh                     # Dart end-to-end (unit + CLI + MCP), prints Passed: 27/27
```

## Layout

```
crwai.go, engine.go, types.go   public library façade (Service/Reader/Writer, Engine, types)
internal/core/                  shared types + small interfaces + write-pipeline contract
internal/mcptool/               tool decorator layer (descriptors, schemas, registration)
internal/lang/                  per-language subpackages + registry + TEMPLATE.md
cmd/crwai/                      CLI + MCP server front-ends (cobra commands, stdio)
cmd/crwai/ui/                   terminal presentation layer (lipgloss styling, icons)
```

The dependency direction is one-way: `cmd/crwai` (and its `ui`) depend on the
public root package; the root façade depends on `internal/`; nothing depends back
outward.
