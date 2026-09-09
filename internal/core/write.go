package core

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// WriteResult is the all-or-nothing outcome of a batch write call.
type WriteResult struct {
	// Applied is true only if every edit in the batch was applied and persisted.
	// On any failure it is false and the disk is left untouched.
	Applied bool `json:"applied"`
	// Path is the file the batch targeted.
	Path string `json:"path"`
	// Edits reports the per-edit outcome, in request order.
	Edits []EditOutcome `json:"edits"`
}

// EditOutcome is the result of a single edit within a batch.
type EditOutcome struct {
	// Target is the symbol the edit addressed.
	Target SymbolID `json:"target"`
	// OK is true if this edit resolved and applied without conflict.
	OK bool `json:"ok"`
	// Reason explains a failure (symbol not found, overlap, broke syntax, ...);
	// "" on success.
	Reason string `json:"reason,omitempty"`
}

// Validator re-checks a buffer after the edits have been applied in memory, and
// returns an error if the result must be rejected. It is the pluggable half of
// step 6 of the write contract: the language pipeline passes a tree-sitter
// re-parse, while a common-file tool whose format has no breakable syntax (see
// internal/common/markdown) passes nil and skips the check.
//
// It is handed the WriteResult so a rejection can name the edit at fault on the
// outcome, not only in the returned error: the caller of a failed batch wants to
// know WHICH edit broke it.
type Validator func(buf []byte, res *WriteResult) error

// BatchWrite is the language-agnostic write pipeline. It is the single place that
// performs disk I/O and enforces the all-or-nothing, surgical-edit contract: the
// server is a scalpel that places caller-provided text, validates the re-parse,
// and refuses anything that breaks the syntax. It NEVER generates code.
//
// CONTRACT (the implementation MUST perform these steps, in order):
//
//  1. Read the file at path from disk and compute a content hash (the staleness
//     baseline for step 7).
//  2. Parse the bytes EXACTLY ONCE via lang.Parse -> Source; `defer src.Close()`.
//  3. Require lang to implement FunctionWriter (else ErrReadOnlyLanguage), then
//     resolve ALL N edits to concrete byte spans via ResolveEdits. Resolution is
//     by symbolic identity, never by externally supplied offsets. A symbol that
//     does not resolve fails the whole batch (ErrSymbolNotFound). An EMPTY batch
//     is rejected up front (ErrNoEdits) so a no-op never rewrites the file.
//  4. Detect overlaps among the resolved spans — two edits on the same symbol or
//     on nested symbols. On any overlap, reject the WHOLE batch
//     (ErrOverlappingEdits) and name the conflicting edits.
//  5. Sort resolved edits by DESCENDING start byte and apply them bottom-up to an
//     in-memory copy of the buffer, so offsets above each edit stay valid without
//     recomputation. Apply all N in memory; write to disk only once.
//  6. Re-parse the resulting buffer. If the root reports errors (Root().HasError()),
//     reject the WHOLE batch (ErrSyntaxBroken), naming the edit that broke the
//     syntax. The disk has not been touched.
//  7. Re-read the on-disk hash. If it differs from step 1, an external writer
//     changed the file; reject (ErrStaleFile) and ask the caller to re-read.
//  8. Persist ATOMICALLY: write a temp file in the SAME directory, then
//     os.Rename over the target (atomic on one filesystem). There must be no
//     instant at which the file is half-written.
//
// Semantics: all-or-nothing — either all N edits land or none do.
//
// Steps 1 and 4 to 8 are format-agnostic and live in ApplyResolved, which the
// common-file write tools share; BatchWrite is the language-specific front half
// (steps 2, 3 and the re-parse validator handed to step 6).
//
// CONCURRENCY: the staleness check of step 7 is a lost-update DEFENSE, not a
// lock. It catches a writer that changed the file before this call reached step 7;
// it cannot catch one that lands between step 7 and the rename of step 8. Two
// processes writing the same file at the same instant can still lose an edit. The
// engine is safe for concurrent use across DIFFERENT files; serialise callers
// yourself if several of them target one file.
//
// FUTURE (contracts to keep, not implemented in v1):
//   - Incremental parsing: feed each applied span to tree.Edit(*sitter.InputEdit)
//     and re-parse only the changed region instead of the full re-parse in step 6.
//   - RelativeRange edits (Edit.Rel != nil): resolve a symbol-relative span as an
//     alternative to whole-symbol replacement. ResolveEdits already carries the
//     information needed; this pipeline does not need a new signature.
func BatchWrite(lang Language, path string, edits []Edit) (WriteResult, error) {
	res := WriteResult{Path: path, Edits: outcomes(edits)}

	// 3a. An empty batch has nothing to apply and must not touch the file.
	if len(edits) == 0 {
		return res, ErrNoEdits
	}

	// 1. Read the file and snapshot its content hash (staleness baseline).
	original, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}

	// 2. Parse exactly once.
	src, err := lang.Parse(original)
	if err != nil {
		return res, err
	}
	defer src.Close()

	// 3b. Require write support, then resolve every edit to a concrete byte span.
	writer, ok := lang.(FunctionWriter)
	if !ok {
		return res, ErrReadOnlyLanguage
	}
	resolved, err := writer.ResolveEdits(src, edits)
	if err != nil {
		return res, err
	}

	// 4 to 8, plus the tree-sitter re-parse as the step 6 validator.
	return ApplyResolved(path, original, resolved, &res, reparseValidator(lang, original, resolved))
}

// ApplyResolved is the format-agnostic tail of the write contract: steps 4 to 8
// (overlap check, bottom-up in-memory apply, validation, staleness check, atomic
// rename). Callers resolve the spans in whatever way suits their format and hand
// them here, so the disk I/O and the all-or-nothing guarantee exist exactly once
// in the codebase.
//
// original must be the bytes read from path at the start of the call: it is both
// the buffer the spans index into and the staleness baseline. res is the outcome
// being filled in, seeded with one entry per requested edit. validate may be nil
// when the format has no syntax that an edit could break.
func ApplyResolved(path string, original []byte, resolved []ResolvedEdit, res *WriteResult, validate Validator) (WriteResult, error) {
	baseHash := sha256.Sum256(original)

	// 4. Reject the batch if any two resolved spans overlap.
	if a, b, overlap := firstOverlap(resolved); overlap {
		markFailed(res, a.From, "overlaps edit on "+b.From.Name)
		markFailed(res, b.From, "overlaps edit on "+a.From.Name)
		return *res, fmt.Errorf("%w: %s and %s", ErrOverlappingEdits, a.From.Name, b.From.Name)
	}

	// 5. Apply bottom-up (descending start byte) on an in-memory copy.
	ordered := make([]ResolvedEdit, len(resolved))
	copy(ordered, resolved)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartByte > ordered[j].StartByte })

	buf := make([]byte, len(original))
	copy(buf, original)
	for _, e := range ordered {
		if e.EndByte > uint(len(buf)) || e.StartByte > e.EndByte {
			markFailed(res, e.From, "resolved span out of range")
			return *res, fmt.Errorf("%w: %s", ErrSymbolNotFound, e.From.Name)
		}
		buf = append(buf[:e.StartByte], append([]byte(e.NewText), buf[e.EndByte:]...)...)
	}

	// 6. Validate the result; reject if the edits broke it.
	if validate != nil {
		if err := validate(buf, res); err != nil {
			return *res, err
		}
	}

	// 7. Re-read the on-disk hash; reject if the file changed under us.
	current, err := os.ReadFile(path)
	if err != nil {
		return *res, err
	}
	if sha256.Sum256(current) != baseHash {
		return *res, ErrStaleFile
	}

	// 8. Persist atomically: temp file in the same dir, then rename over target.
	if err := atomicWrite(path, buf); err != nil {
		return *res, err
	}

	for i := range res.Edits {
		res.Edits[i].OK = true
	}
	res.Applied = true
	return *res, nil
}

// reparseValidator builds the step 6 check for a tree-sitter language: the edited
// buffer must re-parse without error nodes. On failure it names the single edit at
// fault, when one can be isolated.
func reparseValidator(lang Language, original []byte, resolved []ResolvedEdit) Validator {
	return func(buf []byte, res *WriteResult) error {
		check, err := lang.Parse(buf)
		if err != nil {
			return err
		}
		broken := check.Root().HasError()
		check.Close()
		if !broken {
			return nil
		}
		if culprit := blamingEdit(lang, original, resolved); culprit != nil {
			markFailed(res, culprit.From, "edit produced invalid syntax")
			return fmt.Errorf("%w: %s", ErrSyntaxBroken, culprit.From.Name)
		}
		return ErrSyntaxBroken
	}
}

// Outcomes seeds one (failed-by-default) EditOutcome per requested target, in
// order. Common-file write tools use it to build the WriteResult they hand to
// ApplyResolved.
func Outcomes(targets []SymbolID) []EditOutcome {
	out := make([]EditOutcome, len(targets))
	for i, t := range targets {
		out[i] = EditOutcome{Target: t}
	}
	return out
}

// outcomes seeds one (failed-by-default) EditOutcome per requested edit, in order.
func outcomes(edits []Edit) []EditOutcome {
	targets := make([]SymbolID, len(edits))
	for i, e := range edits {
		targets[i] = e.Target
	}
	return Outcomes(targets)
}

// markFailed records a reason on the outcome matching target (first match).
func markFailed(res *WriteResult, target SymbolID, reason string) {
	for i := range res.Edits {
		if res.Edits[i].Target == target && res.Edits[i].Reason == "" {
			res.Edits[i].Reason = reason
			return
		}
	}
}

// blamingEdit identifies which single edit broke the re-parse: it applies each
// resolved edit alone to a fresh copy of the original buffer and returns the first
// one whose result no longer parses cleanly. Returns nil if no single edit is at
// fault (the breakage only emerges from the combination). O(N) re-parses, fine for
// the small batches the tool handles.
func blamingEdit(lang Language, original []byte, edits []ResolvedEdit) *ResolvedEdit {
	for i := range edits {
		e := edits[i]
		if e.EndByte > uint(len(original)) || e.StartByte > e.EndByte {
			return &e
		}
		buf := append([]byte{}, original[:e.StartByte]...)
		buf = append(buf, []byte(e.NewText)...)
		buf = append(buf, original[e.EndByte:]...)
		check, err := lang.Parse(buf)
		if err != nil {
			return &e
		}
		broken := check.Root().HasError()
		check.Close()
		if broken {
			return &e
		}
	}
	return nil
}

// firstOverlap returns the first pair of resolved edits whose [start,end) spans
// intersect (covering same-symbol and nested-symbol edits alike).
func firstOverlap(edits []ResolvedEdit) (ResolvedEdit, ResolvedEdit, bool) {
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			a, b := edits[i], edits[j]
			if a.StartByte < b.EndByte && b.StartByte < a.EndByte {
				return a, b, true
			}
		}
	}
	return ResolvedEdit{}, ResolvedEdit{}, false
}

// atomicWrite writes buf to a temp file in path's directory and renames it over
// path. The temp file inherits the target's permissions (0644 fallback) and is
// removed if the rename fails, so the target is never left half-written.
func atomicWrite(path string, buf []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".crwai-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpName) }

	if _, err := tmp.Write(buf); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpName, info.Mode().Perm())
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
