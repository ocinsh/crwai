package core

import "errors"

// Sentinel errors shared across the server. They describe the failure modes the
// contracts promise, so callers (and the MCP layer) can branch on them with
// errors.Is. Every sentinel here is re-exported from the root package, so a
// library consumer can match all of them without importing internal/.
var (
	// ErrNotImplemented marks a declared capability whose body is not yet written.
	// No shipped code path returns it today: it exists for a language subpackage
	// under construction, whose methods return it until the grammar is wired (see
	// internal/lang/TEMPLATE.md).
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

	// ErrNoEdits means a write was requested with an empty batch. An empty batch
	// is rejected rather than reported as a successful no-op, so the pipeline
	// never rewrites a file it has no change for.
	ErrNoEdits = errors.New("no edits requested")

	// ErrRelativeRangeNotImplemented means an Edit carried a non-nil RelativeRange
	// (a symbol-relative span). The contract exists (see Edit.Rel) but resolution
	// of relative ranges is not implemented in v1; whole-symbol replacement only.
	ErrRelativeRangeNotImplemented = errors.New("relative-range edits not implemented")

	// ErrPathOutsideRoot means a call addressed a file outside the root the engine
	// was confined to (see Engine.Root / the --root flag). It is the workspace
	// boundary: with no root configured every readable path is allowed, and with
	// one configured nothing above it can be read or written.
	ErrPathOutsideRoot = errors.New("path outside the configured root")
)
