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
  rejected and the disk is untouched. An empty batch is rejected too, so a no-op
  never rewrites a file. Writing through a symlink updates its target and keeps
  the symlink intact.
- **Symbol identity:** symbols are addressed by `SymbolID` (kind + name +
  container), never by file offset. The read and write paths derive that identity
  the same way: a container with no kind means a method.
- **Workspace boundary:** the engine addresses any path the process can reach
  unless it is confined. `--root <dir>` (`Engine.Root` in the library) rejects
  anything that does not resolve inside that directory, symlinks and `../`
  traversal included. Pass it when launching the MCP server.
- **Concurrency:** one engine is safe to share, and calls on different files are
  safe in parallel. The write pipeline defends against a lost update with a content
  hash, but that is a defense and not a lock: two writers racing on the *same* file
  can still lose an edit.

## Target languages

Go, Rust, Dart, Python, TypeScript, JavaScript, Java, C, C++. The agent picks the
reference language per call (resolved from the file extension, or forced with the
`--lang` flag / the tools' `lang` field — useful for an ambiguous extension such as
a C++ header named `.h`). Each language lives in its own subpackage under
`internal/lang/` and implements the common interfaces. To add one, follow the
step-by-step guide in [`internal/lang/TEMPLATE.md`](internal/lang/TEMPLATE.md)
(Go is the worked reference implementation).

## The tools

Two families, deliberately unlike each other. The **code tools** are backed by
tree-sitter, address a symbol by identity (kind, name, container), and resolve
their language from the file extension. The **common-file tools** use no
tree-sitter, carry no CGO, are never routed by extension, and address a target by
the document's own identity: a heading path, a request URL. An agent picks a
common-file tool explicitly; nothing infers one.

### Code tools

| Tool | Returns |
| --- | --- |
| `list_signatures` | every top-level symbol: functions, methods, interfaces, structs, and the constants, variables and named types that carry no body. Each entry is labelled with its kind, carries the container it belongs to, and — for everything but interfaces and structs — its verbatim declaration line, receiver and type parameters included. The cheap map |
| `get_function_body` | a function's body only |
| `get_function` | a whole function: doc + signature + body |
| `read_interface` | a full interface definition |
| `read_struct` | a full struct definition |
| `get_declaration` | the full source of **any** symbol, whatever its kind |
| `write_function` | applies N edits atomically (all-or-nothing) |

**Documentation is opt-in.** `list_signatures` returns kinds, names and
declaration lines by default and adds the doc comments only when asked
(`"doc": true`, or `--doc` on the CLI). The doc is the heaviest part of a listing
and a map is usually what is wanted.

**Everything listed can be opened.** `get_declaration` is the only reader for a
constant, a variable or a named type, and it works for functions, interfaces and
structs too, so a caller holding a name from a listing never has to pick a reader.
Leaving its `kind` empty searches every kind by name. A listing that named a
symbol no reader accepted would be a promise the tool could not keep, and a test
walks the whole bundled corpus to make sure that never happens.

`write_function` addresses a symbol exactly the way the readers do: an edit naming
a `container` and no `kind` targets that method, not a top-level function of the
same name. An empty edit list is rejected rather than reported as a successful
no-op.

### What counts as a symbol

The four original kinds (`func`, `method`, `interface`, `struct`) are joined by
`const`, `var` and `type`. The mapping follows each language rather than forcing
one shape on all of them:

| Language | `const` | `var` | `type` |
| --- | --- | --- | --- |
| Go | `const` | package-level `var` | named type that is not a struct or interface |
| Rust | `const` | `static` | `type` alias |
| C | object-like `#define` | file-scope global | `typedef` over a non-aggregate |
| C++ | `const`/`constexpr`, static members included | global and static members | `using` alias, `typedef` |
| TypeScript | `const` | `let`, `var` | `type` alias that is not an object type |
| JavaScript | `const` | `let`, `var` | none |
| Dart | `const` | `final`, `var` | `typedef` |
| Python | none | every module-level binding | PEP 695 `type` alias |
| Java | `static final` field | `static` field | none |

Python has no constant, so inferring one from an upper-case name would be a guess
dressed as a fact; Dart's `final` binds once at run time, which makes it a
variable rather than a compile-time constant. Java has no file-level declaration
at all: a class body is its top level, so a static field is what binds once per
program, while an instance field describes the type and is left to `read_struct`.
An enum reads as a struct everywhere it exists, because `read_struct` already
returns it whole. Declarations inside a function body are never listed: they are
local detail, not part of the file's surface.

### Common-file tools

| Tool | Returns |
| --- | --- |
| `outline_markdown` | every heading of a Markdown document, in order, with the heading path that addresses each section (the cheap map) |
| `read_section` | one section: its heading plus everything beneath it, down to the next heading of equal or shallower level |
| `write_section` | replaces N Markdown sections atomically, through the same all-or-nothing write path |
| `list_requests` | the endpoints of a Postman collection export, one per line, filtered by URL |
| `read_request` | method, URL, body and documentation of the endpoints matching a query |

These read non-code documents at a granularity that is useful on its own, and they
are deliberately **unrelated to the `Language` contract**.

- **Markdown** (`internal/common/markdown`): an outline of the heading index, one
  section read by heading path, and a section rewrite resolved for the shared
  atomic write path. Parsed with a line-oriented block scan, no CGO and no
  tree-sitter: a heading is a `#` line outside a fenced code block, and a section
  path that repeats receives a numeric suffix such as `Guide/Notes [2]`. The
  scanner requires a closing fence to match the opening marker and width. A section
  spans down to the next heading of equal-or-shallower level. Read and write are
  symmetric — writing a section back exactly as it was read leaves the file
  byte-identical, and a replacement keeps the document's blank-line spacing instead
  of welding the next heading onto the new text.
- **Postman** (`internal/common/postman`): **read-only**, built to **read the
  documentation** of a collection export (Collection Format v2.1). It lists
  endpoints filtered by URL and extracts method, URL, body, and documentation for
  the requests matching a query, mirroring a pair of personal shell tools it
  replaces. Dedicated **Postman MCP servers** already exist for this role; this is
  a deliberately minimal, read-only alternative, not a general Postman client. It
  never edits a collection and does not send requests.

Both are exposed through the public facade (`DocReader` / `DocWriter` /
`DocService` in `common.go`), as the five MCP tools above, and as the `outline`,
`section`, `requests` and `request` CLI commands.

## Usage

For a project-scoped Codex MCP setup, follow
[`docs/install-codex.md`](docs/install-codex.md). The repository includes
`.codex/config.toml`; build `dist/crwai` before starting Codex.

With **no subcommand** the binary speaks MCP over stdio (so an MCP client can
launch it directly). Every capability is also a one-word subcommand with a short
alias, for driving the system from a clean CLI:

```sh
crwai                      # run the MCP server over stdio (== `crwai serve`)
crwai langs                # list supported languages and extensions (alias: lng)

# code
crwai signatures <file>                         # map a file (aliases: sig, ls)
crwai function <file> <name> [-c <container>]   # whole function/method (alias: fn)
crwai body <file> <name> [-c <container>]       # body only (alias: bd)
crwai interface <file> <name>                   # alias: iface
crwai struct <file> <name>                      # alias: st
crwai declaration <file> <name> [-k <kind>]     # any symbol, any kind (alias: decl)

# non-code documents
crwai outline <file.md>                         # heading outline (alias: ol)
crwai section <file.md> <heading-path>          # one section (alias: sec)
crwai requests <collection.json> [-f <url>]     # endpoints (alias: reqs)
crwai request <collection.json> <query>         # endpoint detail (alias: req)

# the single mutation verb
crwai write <file> -n <name> [-k <kind>] [-c <container>] -t <text>   # alias: wr
crwai write <file.md> --heading <path> --from <file>                  # a section

crwai version              # product version (alias: ver)
crwai install              # choose detected MCP clients and link this executable
crwai check-update         # compare this version with the latest GitHub release
crwai update               # install the latest published release over this binary
crwai update --to v1.2.3   # install a specific published release
```

`install` detects the Codex and Claude Code CLIs and asks whether to configure
each one. An existing crwai entry can be replaced after confirmation. The MCP
entry points to the resolved path of the executable running the wizard; no copy
is made. Keep that executable in place. From source, `make install` builds the
binary and runs this command. `check-update` and `update` use
GitHub Releases; updates verify the published SHA-256 checksum before replacing
the running binary. An update needs write access to the directory containing
that binary. The update commands become usable after the first release is
published. See [`docs/releases.md`](docs/releases.md) for packaging and release
instructions.

Three persistent flags apply to every command:

- `--lang` / `-l <name>` forces the language by name (see `crwai langs`) instead of
  detecting it from the file extension, for example
  `crwai sig -l cpp widget.h`. The MCP tools expose the same override through an
  optional `lang` field.
- `--root` / `-r <dir>` confines every path to a directory. Anything outside it is
  refused with `path outside the configured root`, `../` traversal and symlinks
  included. Pass it when launching the MCP server: `crwai serve --root .`.
- `--json` prints the machine-readable form instead of the tree, for scripting.
- `--doc` / `-d` adds the documentation to a listing, which is left out by default.

Results go to **stdout** and errors to **stderr**, so redirection and pipes behave:
`crwai sig engine.go > map.txt` and `crwai sig engine.go | grep method` both work.

The human output is a tree. A listing nests each method under the type it belongs
to, so same-named methods are told apart by where they sit rather than by a field
you have to look up:

```
$ crwai sig examples/typescript/shapes.ts --doc
examples/typescript/shapes.ts
typescript, 10 symbols
├─ interface  Shape
│             Shape is anything with a measurable area.
│  └─ method  area(): number
│             area returns the shape's area.
├─ struct     Circle
│             Circle is a round shape.
│  ├─ method  constructor(private r: number)
│  ├─ method  area(): number
│  │          area returns the circle's area.
│  └─ method  static unit(): Circle
│             unit builds the unit circle.
└─ func       function area(w: number, h: number): number
              area is a free function sharing its name with the methods above.
```

Without `--doc` the same listing is the map alone. A file that declares nothing
but constants is no longer reported as empty:

```
$ crwai sig internal/core/version.go
internal/core/version.go
go, 2 symbols
├─ const  const Name = "crwai"
└─ const  const Version = "v0.1.0"
```

There are **no emoji** anywhere in the CLI. The kind of a symbol is a word in the
tag column; the connectors are the standard box-drawing glyphs `tree` and `eza`
use, and they degrade to plain text under `NO_COLOR` or in a pipe.

## Library

The same capabilities are exposed as a Go library through the root package
(`github.com/ocinsh/crwai`). The implementation lives under `internal/` and is not
importable from other modules; consumers depend only on the public facade:

```go
svc := crwai.New()                              // *Engine, implements Service and DocService
sigs, err := svc.ListSignatures("core/types.go")
res,  err := svc.Write("core/write.go", crwai.Edit{
    Target:  crwai.TargetFor("func", "BatchWrite", ""),
    NewText: "func BatchWrite(...) (...) { ... }",
})

// Two modifiers return a narrowed view and leave the receiver alone.
confined, err := crwai.New().Root(".")          // reject anything outside this tree
cpp,      err := crwai.New().Lang("cpp")        // force a language by name

// The common-file surface, separate from Reader/Writer/Service.
heads, err := svc.Outline("README.md")
sec,   err := svc.Section("README.md", "Usage")

// Declaration opens any symbol; an empty kind searches every kind by name.
text, err := svc.Declaration("core/errors.go", "", "ErrStaleFile", "")

// A listing keeps its documentation; StripDocs is how a front end makes it opt-in.
sigs = crwai.StripDocs(sigs)
```

`crwai.TargetFor(kind, name, container)` is how a front-end turns free text into a
symbol identity; it applies the container-implies-method rule, so a symbol read by
container is writable by the same container.

Every sentinel error is re-exported from the root package, so callers can branch
on them with `errors.Is` without importing `internal/`:
`ErrUnsupportedLanguage`, `ErrSymbolNotFound`, `ErrReadOnlyLanguage`,
`ErrSyntaxBroken`, `ErrStaleFile`, `ErrOverlappingEdits`, `ErrNoEdits`,
`ErrRelativeRangeNotImplemented`, `ErrPathOutsideRoot`, `ErrNotImplemented`.

## Out of scope for v1

- Cross-file search and operations (needs a repository index).
- `refactor_function_name` (intrinsically cross-file: renaming must update
  call-sites in other files).
- Relative-range edits. `Edit.Rel` exists as a contract but no language resolves
  one, so it is deliberately absent from the MCP schema: a tool must not offer a
  parameter that always fails.

## Status

The shared scaffolding is **implemented**: the parse helper and concrete `Source`
(`core.Parse` in `internal/core/source.go`), the all-or-nothing write pipeline
(`core.BatchWrite` and its shared tail `core.ApplyResolved` in
`internal/core/write.go`, the 8-step contract), and tool registration
(`mcptool.Register`). On top of it, **Go** (`internal/lang/golang`, the reference
implementation), **Python**, **Java**, **JavaScript**, **C**, **C++**, **Rust**,
**Dart** and **TypeScript** are fully implemented — read (`list_signatures`,
`get_function`, `get_function_body`, `read_struct`, `read_interface`,
`get_declaration`) and atomic write (`write_function`), with their tree-sitter
grammars wired and CGO required to build. `ReadDeclaration` is part of the
`Language` contract rather than optional, so a language that lists a symbol it
cannot open does not compile.

Both common-file tools are implemented and **wired end to end**: public facade, MCP
tools, and CLI commands.

Language notes. C++ has no `interface` construct, so its `read_interface` reports a
typed error (`cpp.ErrInterfacesUnsupported`) — read an abstract class as a struct
instead. C also has no `interface` construct (and no method container):
`read_interface` reports `core.ErrSymbolNotFound`, and symbols sharing a name are
disambiguated by kind (e.g. `struct list` vs the function `list`). JavaScript has no
`interface` either; read a class as a struct. Dart has no distinct interface
declaration, so `read_struct` maps to a concrete class and `read_interface` to an
`abstract` class; its community grammar is pinned in `go.mod` and **`go mod tidy`
must be avoided** (it would pull a broken nested module; see the note in `go.mod`
and the `dart.go` header). TypeScript ships as **two** grammars from the same
`tree-sitter-typescript` module: it registers as two languages — `typescript` for
`.ts` (pure grammar) and `tsx` for `.tsx` (JSX-aware grammar) — sharing one
implementation; `read_interface` reads a real `interface`, `read_struct` reads a
`class` or an object-typed `type` alias, and namespace/module functions are
addressed by `Container`.

All nine target languages are implemented; adding a new one follows
[`internal/lang/TEMPLATE.md`](internal/lang/TEMPLATE.md), with Go as the reference.
Building requires a C toolchain — never `CGO_ENABLED=0`.

### Verification

`make check` runs the gofmt check, `go vet`, the Go test suite, and all nine
language harnesses. GitHub Actions runs only when a `vX.X.X` tag is pushed.
The [release workflow](.github/workflows/release.yml) runs these checks on Linux
and macOS, builds the packages, and verifies that `go.mod` and `go.sum` remain
unchanged.

The Go suite covers the write pipeline directly (`internal/core/write_test.go`:
empty batch, overlapping edits, broken syntax, stale file, permissions, temp-file
cleanup), the public facade (`crwai_test.go`: identity resolution, root
confinement including a symlink escape, the language registry, the Markdown
round-trip, the Postman reader), the nine languages, and both common-file
packages.

Two tests guard the listing's central promise. `TestEverySymbolListedCanBeOpened`
walks every bundled fixture in every language and calls `get_declaration` on every
symbol the listing named, so nothing can be named and left unopenable.
`TestBodylessDeclarationsAcrossLanguages` pins the kind each language reports for
its constants, variables and named types, against one
`examples/<language>/declarations.*` fixture per language; each language package
asserts the same thing on its own fixture.

Try it on the bundled corpus:

```sh
make build
./dist/crwai signatures examples/golang/big.go        # >= 20 symbols
./dist/crwai function   examples/golang/methods.go Greet -c Greeter  # disambiguated by container
./dist/crwai signatures examples/python/shapes.py
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

# every symbol, including the ones that carry no body
./dist/crwai signatures  internal/core/errors.go       # 10 sentinels, once invisible
./dist/crwai signatures  examples/golang/declarations.go --doc
./dist/crwai declaration internal/core/errors.go ErrStaleFile   # no kind needed
./dist/crwai declaration internal/core/types.go KindFunc -k const

# common-file tools
./dist/crwai outline  examples/markdown/sample.md
./dist/crwai section  examples/markdown/sample.md Guide/Usage
./dist/crwai requests examples/postman/sample.postman_collection.json
./dist/crwai request  examples/postman/sample.postman_collection.json /token

# end-to-end harnesses (unit + CLI + MCP), one per language
bash internal/lang/golang/script.sh          # prints Passed: 32/32
bash internal/lang/python/script.sh          # prints Passed: 17/17
bash internal/lang/java/script.sh            # prints Passed: 16/16
bash internal/lang/javascript/script.sh      # prints Passed: 22/22
bash internal/lang/c/script.sh               # prints Passed: 17/17
bash internal/lang/cpp/script.sh             # prints Passed: 19/19
bash internal/lang/rust/script.sh            # prints Passed: 21/21
bash internal/lang/dart/script.sh            # prints Passed: 27/27
bash internal/lang/typescript/script.sh      # prints Passed: 15/15
```

## Layout

```
crwai.go, engine.go, types.go   public library facade (Service/Reader/Writer, Engine, types)
common.go                       public common-file facade (DocService/DocReader/DocWriter)
crwai_test.go                   facade tests (identity, root confinement, registry, documents)
internal/core/                  shared types + small interfaces + the write pipeline
internal/mcptool/               tool decorator layer (descriptors, schemas, registration)
internal/lang/                  per-language subpackages + registry + TEMPLATE.md
internal/common/                common-file tools (markdown, postman); not Language-bound
internal/release/               GitHub release lookup, package verification, binary replacement
cmd/crwai/                      CLI + MCP server front-ends (cobra commands, stdio)
cmd/crwai/ui/                   terminal presentation layer (lipgloss, tree rendering)
examples/                       deterministic fixtures, one directory per language/format
.github/workflows/              tagged release checks and packages for Linux and macOS
```

The dependency direction is one-way: `cmd/crwai` (and its `ui`) depend on the
public root package; the root facade depends on `internal/`; nothing depends back
outward.
