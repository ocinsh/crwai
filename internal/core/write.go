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
//     does not resolve fails the whole batch (ErrSymbolNotFound).
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
// Semantics: tutto-o-niente — either all N edits land or none do.
//
// FUTURE (contracts to keep, not implemented in v1):
//   - Incremental parsing: feed each applied span to tree.Edit(*sitter.InputEdit)
//     and re-parse only the changed region instead of the full re-parse in step 6.
//   - RelativeRange edits (Edit.Rel != nil): resolve a symbol-relative span as an
//     alternative to whole-symbol replacement. ResolveEdits already carries the
//     information needed; this pipeline does not need a new signature.
func BatchWrite(lang Language, path string, edits []Edit) (WriteResult, error) {
	res := WriteResult{Path: path, Edits: outcomes(edits)}

	// 1. Read the file and snapshot its content hash (staleness baseline).
	original, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}
	baseHash := sha256.Sum256(original)

	// 2. Parse exactly once.
	src, err := lang.Parse(original)
	if err != nil {
		return res, err
	}
	defer src.Close()

	// 3. Require write support, then resolve every edit to a concrete byte span.
	writer, ok := lang.(FunctionWriter)
	if !ok {
		return res, ErrReadOnlyLanguage
	}
	resolved, err := writer.ResolveEdits(src, edits)
	if err != nil {
		return res, err
	}

	// 4. Reject the batch if any two resolved spans overlap.
	if a, b, overlap := firstOverlap(resolved); overlap {
		markFailed(&res, a.From, "overlaps edit on "+b.From.Name)
		markFailed(&res, b.From, "overlaps edit on "+a.From.Name)
		return res, fmt.Errorf("%w: %s and %s", ErrOverlappingEdits, a.From.Name, b.From.Name)
	}

	// 5. Apply bottom-up (descending start byte) on an in-memory copy.
	ordered := make([]ResolvedEdit, len(resolved))
	copy(ordered, resolved)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartByte > ordered[j].StartByte })

	buf := make([]byte, len(original))
	copy(buf, original)
	for _, e := range ordered {
		if e.EndByte > uint(len(buf)) || e.StartByte > e.EndByte {
			markFailed(&res, e.From, "resolved span out of range")
			return res, fmt.Errorf("%w: %s", ErrSymbolNotFound, e.From.Name)
		}
		buf = append(buf[:e.StartByte], append([]byte(e.NewText), buf[e.EndByte:]...)...)
	}

	// 6. Re-parse the result; reject if it no longer parses cleanly.
	check, err := lang.Parse(buf)
	if err != nil {
		return res, err
	}
	broken := check.Root().HasError()
	check.Close()
	if broken {
		culprit := blamingEdit(lang, original, resolved)
		if culprit != nil {
			markFailed(&res, culprit.From, "edit produced invalid syntax")
			return res, fmt.Errorf("%w: %s", ErrSyntaxBroken, culprit.From.Name)
		}
		return res, ErrSyntaxBroken
	}

	// 7. Re-read the on-disk hash; reject if the file changed under us.
	current, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}
	if sha256.Sum256(current) != baseHash {
		return res, ErrStaleFile
	}

	// 8. Persist atomically: temp file in the same dir, then rename over target.
	if err := atomicWrite(path, buf); err != nil {
		return res, err
	}

	for i := range res.Edits {
		res.Edits[i].OK = true
	}
	res.Applied = true
	return res, nil
}

// outcomes seeds one (failed-by-default) EditOutcome per requested edit, in order.
func outcomes(edits []Edit) []EditOutcome {
	out := make([]EditOutcome, len(edits))
	for i, e := range edits {
		out[i] = EditOutcome{Target: e.Target}
	}
	return out
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
