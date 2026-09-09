// Package core holds the shared types and the small, orthogonal interfaces that
// every language subpackage implements. It is the contract layer of the
// tree-sitter MCP server: it defines WHAT a symbol is and WHICH capabilities a
// language can expose, but contains no language-specific logic and no I/O.
//
// Design follows the Go stdlib `io` style: many small single-capability
// interfaces, composed and discovered via type assertion rather than via one
// large interface. A read-only language implements only the read interfaces; a
// writable language additionally implements FunctionWriter.
//
// Every type that crosses the MCP wire carries explicit `json` tags in
// lower_snake_case with omitempty on the optional fields. The tags are part of
// the contract, not decoration: the wire form must stay stable and must not spend
// an agent's context on empty strings and null arrays.
package core

// SymbolKind enumerates the kinds of symbols the server can address. It is an
// open enum backed by a string so it is self-describing on the MCP wire (the
// inferred JSON schema is a plain "string" and the value marshals to "func",
// "method", … with no extra code); new kinds may be appended without breaking
// existing callers.
type SymbolKind string

const (
	// KindFunc is a free/top-level function (no receiver, no enclosing type).
	KindFunc SymbolKind = "func"
	// KindMethod is a function bound to a receiver / enclosing type
	// (Go method, Rust impl method, Java/Python class method).
	KindMethod SymbolKind = "method"
	// KindInterface is an interface / protocol / trait declaration.
	KindInterface SymbolKind = "interface"
	// KindStruct is a struct / class / record declaration.
	KindStruct SymbolKind = "struct"
	// KindSection is a section of a non-code document, addressed by heading path
	// rather than by name and container. It is the one kind the COMMON-FILE tools
	// produce (see internal/common/markdown): the write pipeline and its
	// WriteResult are shared between the two families, so the outcome of a section
	// rewrite has to say what it rewrote. No language ever emits it.
	KindSection SymbolKind = "section"
)

// String renders a SymbolKind for diagnostics and tool output. The value already
// is its label; the only special case is the empty zero value.
func (k SymbolKind) String() string {
	if k == "" {
		return "unknown"
	}
	return string(k)
}

// SymbolID is the unique symbolic identity of a symbol — the ONLY key used to
// locate a symbol. Callers never pass raw file offsets across the MCP boundary;
// identity is resolved to a concrete position fresh on every call (the server is
// stateless and the disk is the source of truth).
type SymbolID struct {
	// Kind disambiguates symbols that may share a name across kinds.
	Kind SymbolKind `json:"kind"`
	// Name is the declared identifier of the symbol.
	Name string `json:"name"`
	// Container scopes the symbol within an enclosing entity. It is the
	// receiver type for a Go method, the type of the enclosing `impl` block in
	// Rust, the enclosing class for Java/Python, and "" for a top-level symbol.
	Container string `json:"container,omitempty"`
}

// Position is a 0-based line/column location within a file.
type Position struct {
	Line uint `json:"line"`
	Col  uint `json:"col"`
}

// Symbol is the lightweight descriptor of a located symbol. It carries identity
// and location but deliberately NOT the body — bodies are fetched on demand via
// FunctionReader / FunctionBodyReader to keep agent context small.
type Symbol struct {
	// ID is the symbolic identity used to address this symbol.
	ID SymbolID `json:"id"`
	// ByteRange is [start, end) byte offsets of the whole symbol in the file.
	ByteRange [2]uint `json:"byte_range"`
	// StartPos / EndPos are the line/column span of the whole symbol.
	StartPos Position `json:"start_pos"`
	EndPos   Position `json:"end_pos"`
	// Signature is the textual signature (no body).
	Signature string `json:"signature,omitempty"`
}

// Signature is the "light" form of a symbol that the agent reads by default:
// enough to understand and call the symbol without loading its body.
type Signature struct {
	// Kind labels the symbol (func/method/interface/struct) so the cheap listing
	// is enough to discover not just a symbol's name but which reader to call for
	// it (read_struct vs read_interface vs get_function) and to disambiguate
	// same-named symbols that differ only by kind (e.g. C's `struct list` vs the
	// free function `list`).
	Kind SymbolKind `json:"kind"`
	// Name is the declared identifier.
	Name string `json:"name"`
	// Container scopes the symbol within an enclosing entity (the receiver type for
	// a method, the enclosing class/impl), "" for a top-level symbol. It is the
	// machine-readable companion to Text: it tells an agent which struct a method
	// is bound to without parsing the signature line.
	Container string `json:"container,omitempty"`
	// Text is the verbatim signature line as it appears in source — keyword,
	// receiver, type parameters, parameters and return type, with no body (e.g.
	// "func (s *Stack[T]) Len() int"). It is set for callable symbols
	// (func/method); for interfaces/structs it is empty and callers fall back to
	// Name. This is the faithful form the listing displays.
	Text string `json:"text,omitempty"`
	// Params are the textual parameter declarations, in order. They remain as a
	// structured, machine-readable companion to Text.
	Params []string `json:"params,omitempty"`
	// Returns is the textual return type / return list ("" if none).
	Returns string `json:"returns,omitempty"`
	// Doc is the associated documentation (see language notes: preceding-sibling
	// comments for Go/Java/JS/TS/Rust/C/C++; docstring inside the body for Python).
	Doc string `json:"doc,omitempty"`
}

// RelativeRange addresses a span by coordinates RELATIVE to the start of the
// symbol, not the file (e.g. "replace line 3 of the function"). This exists so
// that the future single-line edit granularity does not change any signatures.
//
// v1: the contract is present but resolution is NOT implemented — every language
// rejects a non-nil Edit.Rel with ErrRelativeRangeNotImplemented, and the field is
// deliberately absent from the MCP wire schema so an agent is never offered a
// parameter that cannot work.
type RelativeRange struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

// Edit is a single requested modification, addressed by symbolic identity. It
// supports two granularities:
//
//   - Rel == nil: replace the WHOLE target symbol (or its body) with NewText.
//   - Rel != nil: replace only the span described by Rel (relative to the symbol
//     start) with NewText. (v1 contract; not implemented, see RelativeRange.)
type Edit struct {
	// Target is the symbol to modify.
	Target SymbolID `json:"target"`
	// NewText is the replacement source text supplied by the agent. The server
	// never generates code; it only places caller-provided text.
	NewText string `json:"new_text"`
	// Rel optionally narrows the edit to a symbol-relative span.
	Rel *RelativeRange `json:"rel,omitempty"`
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
