// Package core holds the shared types and the small, orthogonal interfaces that
// every language subpackage implements. It is the contract layer of the
// tree-sitter MCP server: it defines WHAT a symbol is and WHICH capabilities a
// language can expose, but contains no language-specific logic and no I/O.
//
// Design follows the Go stdlib `io` style: many small single-capability
// interfaces, composed and discovered via type assertion rather than via one
// large interface. A read-only language implements only the read interfaces; a
// writable language additionally implements FunctionWriter.
package core

// SymbolKind enumerates the kinds of symbols the server can address. It is an
// open enum: new kinds (e.g. enum, trait, type-alias) may be appended without
// breaking existing callers.
type SymbolKind int

const (
	// KindFunc is a free/top-level function (no receiver, no enclosing type).
	KindFunc SymbolKind = iota
	// KindMethod is a function bound to a receiver / enclosing type
	// (Go method, Rust impl method, Java/Python class method).
	KindMethod
	// KindInterface is an interface / protocol / trait declaration.
	KindInterface
	// KindStruct is a struct / class / record declaration.
	KindStruct
)

// String renders a SymbolKind for diagnostics and tool output.
func (k SymbolKind) String() string {
	switch k {
	case KindFunc:
		return "func"
	case KindMethod:
		return "method"
	case KindInterface:
		return "interface"
	case KindStruct:
		return "struct"
	default:
		return "unknown"
	}
}

// SymbolID is the unique symbolic identity of a symbol — the ONLY key used to
// locate a symbol. Callers never pass raw file offsets across the MCP boundary;
// identity is resolved to a concrete position fresh on every call (the server is
// stateless and the disk is the source of truth).
type SymbolID struct {
	// Kind disambiguates symbols that may share a name across kinds.
	Kind SymbolKind
	// Name is the declared identifier of the symbol.
	Name string
	// Container scopes the symbol within an enclosing entity. It is the
	// receiver type for a Go method, the type of the enclosing `impl` block in
	// Rust, the enclosing class for Java/Python, and "" for a top-level symbol.
	Container string
}

// Position is a 0-based line/column location within a file.
type Position struct {
	Line uint
	Col  uint
}

// Symbol is the lightweight descriptor of a located symbol. It carries identity
// and location but deliberately NOT the body — bodies are fetched on demand via
// FunctionReader / FunctionBodyReader to keep agent context small.
type Symbol struct {
	// ID is the symbolic identity used to address this symbol.
	ID SymbolID
	// ByteRange is [start, end) byte offsets of the whole symbol in the file.
	ByteRange [2]uint
	// StartPos / EndPos are the line/column span of the whole symbol.
	StartPos Position
	EndPos   Position
	// Signature is the textual signature (no body).
	Signature string
}

// Signature is the "light" form of a symbol that the agent reads by default:
// enough to understand and call the symbol without loading its body.
type Signature struct {
	// Name is the declared identifier.
	Name string
	// Params are the textual parameter declarations, in order.
	Params []string
	// Returns is the textual return type / return list ("" if none).
	Returns string
	// Doc is the associated documentation (see language notes: preceding-sibling
	// comments for Go/Java/JS/TS/Rust/C/C++; docstring inside the body for Python).
	Doc string
}

// RelativeRange addresses a span by coordinates RELATIVE to the start of the
// symbol, not the file (e.g. "replace line 3 of the function"). This exists so
// that the future single-line edit granularity does not change any signatures.
//
// v1: the contract is present; resolution may remain unimplemented.
type RelativeRange struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Edit is a single requested modification, addressed by symbolic identity. It
// supports two granularities:
//
//   - Rel == nil: replace the WHOLE target symbol (or its body) with NewText.
//   - Rel != nil: replace only the span described by Rel (relative to the symbol
//     start) with NewText. (v1 contract; may remain unimplemented.)
type Edit struct {
	// Target is the symbol to modify.
	Target SymbolID
	// NewText is the replacement source text supplied by the agent. The server
	// never generates code; it only places caller-provided text.
	NewText string
	// Rel optionally narrows the edit to a symbol-relative span.
	Rel *RelativeRange
}

// ResolvedEdit is an Edit mapped to a concrete byte span on a parsed Source. The
// language layer (FunctionWriter) produces these; the write pipeline in write.go
// orders, overlap-checks, and applies them. ResolvedEdits never cross the MCP
// boundary — they are an internal hand-off between the language and the pipeline.
type ResolvedEdit struct {
	// StartByte / EndByte is the [start, end) span to replace in the buffer.
	StartByte uint
	EndByte   uint
	// NewText is the replacement text.
	NewText string
	// From records which symbolic Edit produced this span (for error reporting
	// and overlap diagnostics).
	From SymbolID
}
