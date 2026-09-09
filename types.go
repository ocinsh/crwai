package crwai

import "github.com/ocinsh/crwai/internal/core"

// Product identity, surfaced by the CLI and the MCP handshake. Defined once in
// the internal core and re-exported here as the single public source of truth.
const (
	// Name is the product / binary name.
	Name = core.Name
	// Version is the product version.
	Version = core.Version
)

// The public data types are aliases of their internal-core definitions, so a
// value produced inside the library and a value named through this facade are the
// exact same type. External consumers use these names; the internal package stays
// unimportable.
type (
	// SymbolKind enumerates the kinds of symbol the library can address.
	SymbolKind = core.SymbolKind

	// SymbolID is the symbolic identity of a symbol — the only key used to locate
	// one. Kind disambiguates same-named symbols; Container scopes a method to its
	// receiver/class and is empty for a top-level symbol.
	SymbolID = core.SymbolID

	// Signature is the light form of a symbol: name, parameters, return type, and
	// doc — enough to understand and call it without loading its body.
	Signature = core.Signature

	// Edit is a single requested modification, addressed by symbolic identity.
	Edit = core.Edit

	// RelativeRange addresses a span relative to a symbol's start. It is a
	// reserved contract: no language resolves one in v1 (see
	// ErrRelativeRangeNotImplemented) and the MCP schema does not offer it.
	RelativeRange = core.RelativeRange

	// WriteResult is the all-or-nothing outcome of a batch write.
	WriteResult = core.WriteResult

	// EditOutcome is the per-edit result within a batch.
	EditOutcome = core.EditOutcome
)

// The symbol kinds, re-exported for use in SymbolID and Edit.
const (
	// KindFunc is a free/top-level function.
	KindFunc = core.KindFunc
	// KindMethod is a function bound to a receiver / enclosing type.
	KindMethod = core.KindMethod
	// KindInterface is an interface / protocol / trait declaration.
	KindInterface = core.KindInterface
	// KindStruct is a struct / class / record declaration.
	KindStruct = core.KindStruct
	// KindSection is a section of a non-code document, addressed by heading path.
	// It is produced only by the common-file tools (see DocWriter).
	KindSection = core.KindSection
)

// The sentinel errors the library can return, re-exported so callers can branch
// on them with errors.Is without importing the internal package. The set is
// complete: every sentinel the internal core defines appears here.
var (
	// ErrUnsupportedLanguage means no registered language handles the file's
	// extension, or Lang was given an unknown language name.
	ErrUnsupportedLanguage = core.ErrUnsupportedLanguage
	// ErrSymbolNotFound means no symbol matched the requested identity.
	ErrSymbolNotFound = core.ErrSymbolNotFound
	// ErrReadOnlyLanguage means a write was requested against a language that has
	// no write support.
	ErrReadOnlyLanguage = core.ErrReadOnlyLanguage
	// ErrSyntaxBroken means an edit would have left the file unparseable; the
	// batch was rejected.
	ErrSyntaxBroken = core.ErrSyntaxBroken
	// ErrStaleFile means the file changed on disk between read and write; re-read
	// and retry.
	ErrStaleFile = core.ErrStaleFile
	// ErrOverlappingEdits means two edits in a batch targeted overlapping spans.
	ErrOverlappingEdits = core.ErrOverlappingEdits
	// ErrNoEdits means a write was requested with an empty batch; nothing was
	// written.
	ErrNoEdits = core.ErrNoEdits
	// ErrRelativeRangeNotImplemented means an edit carried a RelativeRange, which
	// v1 does not resolve. Replace the whole symbol instead.
	ErrRelativeRangeNotImplemented = core.ErrRelativeRangeNotImplemented
	// ErrPathOutsideRoot means the path addressed a file outside the root the
	// engine was confined to with Root.
	ErrPathOutsideRoot = core.ErrPathOutsideRoot
	// ErrNotImplemented marks a declared capability that is not yet wired. No
	// shipped path returns it; a language under construction does.
	ErrNotImplemented = core.ErrNotImplemented
)

// LanguageInfo describes a supported language for discovery: its canonical name
// and the file extensions it claims (each with a leading dot).
type LanguageInfo struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
}

// ParseKind maps a kind string ("func", "method", "interface", "struct") to a
// SymbolKind, defaulting to KindFunc for anything unrecognised. It is the
// convenience used by front-ends that accept the kind as free text. Prefer
// TargetFor when a container is also in play: it applies the same
// container-implies-method rule the read methods use.
func ParseKind(s string) SymbolKind {
	switch s {
	case "method":
		return KindMethod
	case "interface":
		return KindInterface
	case "struct":
		return KindStruct
	default:
		return KindFunc
	}
}

// TargetFor builds the SymbolID a front-end addresses a symbol with, from free
// text. It is the single place that reconciles kind and container, so the write
// path resolves a symbol exactly the way the read path does.
//
// The rule: a non-empty container on a callable means a method. FunctionBody,
// Function and the equivalent tools already infer this (they take a container and
// no kind), so a caller that reads a method with container "Circle" and then
// writes it back with the same container — and either no kind or the default
// "func" — must hit the same symbol rather than ErrSymbolNotFound. An explicit
// "interface" or "struct" kind is always honoured as given.
func TargetFor(kind, name, container string) SymbolID {
	k := ParseKind(kind)
	if container != "" && k == KindFunc {
		k = KindMethod
	}
	return SymbolID{Kind: k, Name: name, Container: container}
}
