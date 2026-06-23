package core

import "errors"

// Sentinel errors shared across the server. Skeleton methods return
// ErrNotImplemented; the rest describe the failure modes the contracts promise,
// so callers (and the MCP layer) can branch on them with errors.Is.
var (
	// ErrNotImplemented marks a contract that is declared but whose body is not
	// yet written in this skeleton.
	ErrNotImplemented = errors.New("not implemented")

	// ErrSymbolNotFound means no symbol matched the requested SymbolID in the
	// parsed Source.
	ErrSymbolNotFound = errors.New("symbol not found")

	// ErrUnsupportedLanguage means no registered Language handles the file
	// (unknown extension / unknown language name).
	ErrUnsupportedLanguage = errors.New("unsupported language")

	// ErrReadOnlyLanguage means the resolved Language does not implement
	// FunctionWriter, so a write was requested against a read-only language.
	ErrReadOnlyLanguage = errors.New("language is read-only")

	// ErrSyntaxBroken means the buffer failed to re-parse cleanly after applying
	// edits (the resulting tree contained error nodes); the batch is rejected and
	// the disk is left untouched.
	ErrSyntaxBroken = errors.New("edit produced invalid syntax")

	// ErrStaleFile means the file on disk changed between the initial read and
	// the final write (lost-update defense); the caller must re-read and retry.
	ErrStaleFile = errors.New("file changed on disk; re-read required")

	// ErrOverlappingEdits means two edits in a batch target the same or nested
	// byte spans; the whole batch is rejected.
	ErrOverlappingEdits = errors.New("edits overlap")

	// ErrRelativeRangeNotImplemented means an Edit carried a non-nil RelativeRange
	// (a symbol-relative span). The contract exists (see Edit.Rel) but resolution
	// of relative ranges is not implemented in v1; whole-symbol replacement only.
	ErrRelativeRangeNotImplemented = errors.New("relative-range edits not implemented")
)
