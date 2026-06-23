package core

// The interfaces below are intentionally small — one capability each — mirroring
// the Go stdlib `io` package. A language assembles the capabilities it supports;
// the server discovers what a given language can do via type assertion (the same
// way `io.Copy` discovers `io.WriterTo`). All read/write interfaces operate on a
// shared, already-parsed Source — never on raw []byte — so a single MCP call
// parses the file exactly once.

// SignatureLister lists the signatures of the symbols in a Source. Powers the
// `list_signatures` tool — the agent's default, cheapest entry point.
type SignatureLister interface {
	ListSignatures(src Source) ([]Signature, error)
}

// FunctionBodyReader returns ONLY the body of a function, addressed by identity.
// Powers `get_function_body`.
type FunctionBodyReader interface {
	FunctionBody(src Source, id SymbolID) (string, error)
}

// FunctionReader returns the WHOLE function — doc + signature + body — addressed
// by identity. Powers `get_function`.
type FunctionReader interface {
	Function(src Source, id SymbolID) (string, error)
}

// InterfaceReader returns the full definition of an interface, addressed by
// identity. Powers `read_interface`.
type InterfaceReader interface {
	ReadInterface(src Source, id SymbolID) (string, error)
}

// StructReader returns the full definition of a struct, addressed by identity.
// Powers `read_struct`.
type StructReader interface {
	ReadStruct(src Source, id SymbolID) (string, error)
}

// FunctionWriter is the OPTIONAL write capability. It maps a batch of symbolic
// Edits onto concrete byte spans on the in-memory Source. It does NOT touch the
// disk and does NOT order or apply the edits — that is the job of the
// language-agnostic pipeline in write.go (see BatchWrite). Powers `write_function`.
//
// A read-only language simply omits this interface; the server detects its
// absence via `lang.(FunctionWriter)` and reports ErrReadOnlyLanguage.
type FunctionWriter interface {
	ResolveEdits(src Source, edits []Edit) ([]ResolvedEdit, error)
}

// Language is the aggregate interface a language subpackage must satisfy. It
// bundles parsing, metadata, and the READ capabilities.
//
// FunctionWriter is deliberately NOT embedded here: writing is an optional
// capability discovered separately via type assertion, exactly as the stdlib
// keeps io.WriterTo out of io.Reader. This is what makes a read-only language a
// valid Language.
type Language interface {
	// Name is the canonical language name (e.g. "go", "rust", "python"), used as
	// a registry key and for explicit language selection.
	Name() string

	// Extensions returns the file extensions this language handles, each
	// including the leading dot (e.g. ".go", ".rs"). Used by the registry to map
	// a file path to its language.
	Extensions() []string

	// Parse loads src into a parsed Source for one call. The caller is
	// responsible for calling Close on the returned Source (via defer).
	Parse(src []byte) (Source, error)

	SignatureLister
	FunctionBodyReader
	FunctionReader
	InterfaceReader
	StructReader
}
