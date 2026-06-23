// Package crwai is the public API of the crwai library: a tree-sitter-backed
// reader and surgical writer that operates on source files at the granularity of
// a single symbol — a function, method, interface, or struct — addressed by its
// identity (name, kind, and optional container) rather than by byte offsets.
//
// This root package is a thin, stable façade. All of the implementation — the
// parse pipeline, the per-language tree-sitter logic, the symbol resolution, and
// the all-or-nothing write engine — lives under internal/ and is intentionally
// not importable by other modules. Consumers depend only on the interfaces and
// types declared here; the two front-ends shipped in this repository (the CLI and
// the MCP server under cmd/crwai) are themselves just clients of this façade.
//
// The entry point is New, which returns an *Engine implementing Service:
//
//	svc := crwai.New()
//	sigs, err := svc.ListSignatures("core/types.go")
//	body, err := svc.FunctionBody("core/write.go", "BatchWrite", "")
//	res,  err := svc.Write("core/write.go", crwai.Edit{
//	    Target:  crwai.SymbolID{Kind: crwai.KindFunc, Name: "BatchWrite"},
//	    NewText: "func BatchWrite(...) (...) { ... }",
//	})
//
// Every method addresses symbols purely by identity and re-parses the file from
// disk on each call: the library keeps no cache or session, so the disk is always
// the source of truth and calls are safe to make concurrently.
package crwai

// Reader is the read surface of the library. Each method parses the file at path
// exactly once and returns only what was asked for, keeping the caller's context
// small: list the signatures first to map a file, then fetch the one symbol you
// need. A method or struct/interface that cannot be located returns
// ErrSymbolNotFound; an unknown file extension returns ErrUnsupportedLanguage.
type Reader interface {
	// ListSignatures returns the signature of every top-level symbol in the file,
	// with no bodies loaded — the cheapest way to map a file.
	ListSignatures(path string) ([]Signature, error)

	// FunctionBody returns only the body of the named function, or of the method
	// named within container (empty container means a top-level function).
	FunctionBody(path, name, container string) (string, error)

	// Function returns the whole named function or method — doc, signature, and
	// body — for full context before an edit.
	Function(path, name, container string) (string, error)

	// Interface returns the full definition of the named interface / protocol /
	// trait.
	Interface(path, name string) (string, error)

	// Struct returns the full definition of the named struct / class / record.
	Struct(path, name string) (string, error)

	// Languages reports the languages the engine can parse, sorted by name.
	Languages() []LanguageInfo
}

// Writer is the write surface of the library: a scalpel that places
// caller-provided text and never generates code of its own. A batch is applied
// all-or-nothing — if any edit fails to resolve, overlaps another, or would break
// the file's syntax, the whole batch is rejected and the file is left untouched.
type Writer interface {
	// Write replaces one or more symbols with the supplied text in a single
	// atomic batch. The outcome is reported per edit in the returned WriteResult.
	Write(path string, edits ...Edit) (WriteResult, error)
}

// Service is the full library surface, combining read and write. *Engine is the
// canonical implementation, returned by New.
type Service interface {
	Reader
	Writer
}
