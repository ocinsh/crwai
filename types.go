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

	// Symbol is the located descriptor of a symbol: identity plus position, but
	// not its body.
	Symbol = core.Symbol

	// Position is a 0-based line/column location within a file.
	Position = core.Position

	// Edit is a single requested modification, addressed by symbolic identity.
	Edit = core.Edit

	// RelativeRange addresses a span relative to a symbol's start (reserved for a
	// future single-line edit granularity).
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
)

// The sentinel errors the library can return, re-exported so callers can branch
// on them with errors.Is without importing the internal package.
var (
	// ErrUnsupportedLanguage means no registered language handles the file's
	// extension.
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
	// ErrNotImplemented marks a capability declared but not yet wired in this
	// skeleton.
	ErrNotImplemented = core.ErrNotImplemented
)

// LanguageInfo describes a supported language for discovery: its canonical name
// and the file extensions it claims (each with a leading dot).
type LanguageInfo struct {
	Name       string
	Extensions []string
}

// ParseKind maps a kind string ("func", "method", "interface", "struct") to a
// SymbolKind, defaulting to KindFunc for anything unrecognised. It is the
// convenience used by front-ends that accept the kind as free text.
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
