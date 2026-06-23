package dart

import "errors"

// Typed errors for the Dart subpackage. They describe failure modes specific to
// this language layer; the core sentinels (core.ErrSymbolNotFound, …) cover the
// shared cases. Callers branch on these with errors.Is.
var (
	// ErrRelativeRangeNotImplemented is returned by ResolveEdits when an Edit
	// carries a RelativeRange. The v1 contract reserves the path but does not yet
	// resolve symbol-relative spans; the whole-symbol replacement path is the only
	// one implemented.
	ErrRelativeRangeNotImplemented = errors.New("dart: relative-range edits not implemented")

	// ErrNotSupportedByLanguage marks a capability that Dart cannot express. It is
	// kept for symmetry with the language contract; Dart maps interfaces to abstract
	// classes and structs to concrete classes, so it is currently unused but
	// available for future kinds.
	ErrNotSupportedByLanguage = errors.New("dart: capability not supported by language")
)
