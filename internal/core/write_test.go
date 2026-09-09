// Package core_test exercises the write pipeline — the most safety-critical code
// in the repository and the part every language depends on. It is an EXTERNAL
// test package so it can drive the pipeline through a real language
// (internal/lang/golang) without creating an import cycle: the point is to test
// the pipeline's contract end to end, including the tree-sitter re-parse, not a
// stubbed approximation of it.
//
// The guarantees under test are the ones the contract promises and nothing else
// covered: an empty batch changes nothing, overlapping edits are refused, a
// broken re-parse is refused, a file that moved under the call is refused, and in
// every refusal the bytes on disk are exactly what they were.
package core_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
	"github.com/ocinsh/crwai/internal/lang/golang"
)

// src is the fixture every test writes into a fresh temp file. Two top-level
// functions at different offsets let a batch exercise the bottom-up apply.
const src = `package fixture

// Add returns the sum.
func Add(a, b int) int {
	return a + b
}

// Sub returns the difference.
func Sub(a, b int) int {
	return a - b
}
`

// fixture writes src to a temp file and returns its path.
func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// mustRead reads a file or fails the test.
func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// edit builds a whole-symbol replacement for a top-level function.
func edit(name, text string) core.Edit {
	return core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: name}, NewText: text}
}

func TestBatchWriteAppliesEveryEdit(t *testing.T) {
	path := fixture(t)

	res, err := core.BatchWrite(golang.Go{}, path,
		[]core.Edit{
			edit("Add", "func Add(a, b int) int { return a + b + 1 }"),
			edit("Sub", "func Sub(a, b int) int { return a - b - 1 }"),
		})
	if err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	if !res.Applied {
		t.Fatal("Applied is false on a successful batch")
	}
	for _, e := range res.Edits {
		if !e.OK {
			t.Errorf("edit on %s reported not OK: %s", e.Target.Name, e.Reason)
		}
	}

	got := mustRead(t, path)
	// Both edits landed, and the edit lower in the file did not shift the one
	// above it: that is what the bottom-up apply of step 5 buys.
	for _, want := range []string{"return a + b + 1", "return a - b - 1", "// Add returns the sum."} {
		if !strings.Contains(got, want) {
			t.Errorf("result is missing %q:\n%s", want, got)
		}
	}
}

func TestBatchWriteRejectsEmptyBatch(t *testing.T) {
	path := fixture(t)
	before := mustRead(t, path)

	res, err := core.BatchWrite(golang.Go{}, path, nil)
	if !errors.Is(err, core.ErrNoEdits) {
		t.Fatalf("err = %v, want ErrNoEdits", err)
	}
	if res.Applied {
		t.Error("Applied is true for an empty batch")
	}
	if got := mustRead(t, path); got != before {
		t.Error("an empty batch rewrote the file")
	}
}

func TestBatchWriteRejectsOverlappingEdits(t *testing.T) {
	path := fixture(t)
	before := mustRead(t, path)

	// Two edits on the same symbol resolve to identical spans, which is the
	// simplest overlap the checker must catch.
	res, err := core.BatchWrite(golang.Go{}, path, []core.Edit{
		edit("Add", "func Add(a, b int) int { return 1 }"),
		edit("Add", "func Add(a, b int) int { return 2 }"),
	})
	if !errors.Is(err, core.ErrOverlappingEdits) {
		t.Fatalf("err = %v, want ErrOverlappingEdits", err)
	}
	if res.Applied {
		t.Error("Applied is true for a rejected batch")
	}
	if got := mustRead(t, path); got != before {
		t.Error("a rejected overlapping batch touched the file")
	}
	for _, e := range res.Edits {
		if e.Reason == "" {
			t.Errorf("edit on %s carries no reason for the rejection", e.Target.Name)
		}
	}
}

func TestBatchWriteRejectsBrokenSyntax(t *testing.T) {
	path := fixture(t)
	before := mustRead(t, path)

	res, err := core.BatchWrite(golang.Go{}, path, []core.Edit{
		edit("Add", "func Add(a, b int) int { return a +"),
	})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("Applied is true for a rejected batch")
	}
	if got := mustRead(t, path); got != before {
		t.Error("a batch that broke the syntax reached the disk")
	}
	// The outcome must name WHICH edit broke it, not just that something did:
	// with a multi-edit batch that is the only way a caller knows what to fix.
	if res.Edits[0].Reason == "" {
		t.Error("the outcome does not say which edit produced the invalid syntax")
	}
}

func TestBatchWriteRejectsMissingSymbol(t *testing.T) {
	path := fixture(t)
	before := mustRead(t, path)

	if _, err := core.BatchWrite(golang.Go{}, path, []core.Edit{
		edit("Nope", "func Nope() {}"),
	}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("err = %v, want ErrSymbolNotFound", err)
	}
	if got := mustRead(t, path); got != before {
		t.Error("a batch naming an unknown symbol touched the file")
	}
}

func TestBatchWriteRejectsReadOnlyLanguage(t *testing.T) {
	path := fixture(t)

	if _, err := core.BatchWrite(readOnlyGo{}, path, []core.Edit{
		edit("Add", "func Add(a, b int) int { return 0 }"),
	}); !errors.Is(err, core.ErrReadOnlyLanguage) {
		t.Fatalf("err = %v, want ErrReadOnlyLanguage", err)
	}
}

// TestApplyResolvedRejectsStaleFile drives the staleness defense directly, which
// is the only way to reach it deterministically: the pipeline compares the bytes
// it started from against the bytes on disk just before the rename, so the test
// changes the file after the read and hands the pipeline the now-stale original.
func TestApplyResolvedRejectsStaleFile(t *testing.T) {
	path := fixture(t)
	original := []byte(src)

	// An external writer gets there first.
	external := src + "\n// touched by someone else\n"
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}

	res := core.WriteResult{Path: path, Edits: core.Outcomes([]core.SymbolID{{Kind: core.KindFunc, Name: "Add"}})}
	resolved := []core.ResolvedEdit{{
		StartByte: 0,
		EndByte:   uint(len("package fixture")),
		NewText:   "package fixture",
		From:      core.SymbolID{Kind: core.KindFunc, Name: "Add"},
	}}

	out, err := core.ApplyResolved(path, original, resolved, &res, nil)
	if !errors.Is(err, core.ErrStaleFile) {
		t.Fatalf("err = %v, want ErrStaleFile", err)
	}
	if out.Applied {
		t.Error("Applied is true for a stale write")
	}
	if got := mustRead(t, path); got != external {
		t.Error("a stale write overwrote the external change")
	}
}

// TestApplyResolvedPreservesFileMode checks the atomic rename of step 8 does not
// silently reset permissions: the file is replaced, not recreated with defaults.
func TestApplyResolvedPreservesFileMode(t *testing.T) {
	path := fixture(t)
	const mode os.FileMode = 0o600
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}

	if _, err := core.BatchWrite(golang.Go{}, path, []core.Edit{
		edit("Add", "func Add(a, b int) int { return 0 }"),
	}); err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != mode {
		t.Errorf("mode = %v, want %v", got, mode)
	}
}

// TestBatchWriteLeavesNoTempFiles guards the atomic write's cleanup: a rejected
// batch must not leave its scratch file behind in the target's directory.
func TestBatchWriteLeavesNoTempFiles(t *testing.T) {
	path := fixture(t)
	dir := filepath.Dir(path)

	_, _ = core.BatchWrite(golang.Go{}, path, []core.Edit{
		edit("Add", "func Add(a, b int) int { return a +"),
	})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the fixture", len(entries))
	}
}

// readOnlyGo is a Language WITHOUT the optional write capability. It forwards
// every read method to the real Go implementation and deliberately does not
// declare ResolveEdits, which is what makes the pipeline report
// ErrReadOnlyLanguage. Embedding golang.Go would promote ResolveEdits and defeat
// the test, so each method is forwarded by hand.
type readOnlyGo struct{}

func (readOnlyGo) Name() string         { return "readonly-go" }
func (readOnlyGo) Extensions() []string { return []string{".rogo"} }

func (readOnlyGo) Parse(b []byte) (core.Source, error) { return golang.Go{}.Parse(b) }

func (readOnlyGo) ListSignatures(s core.Source) ([]core.Signature, error) {
	return golang.Go{}.ListSignatures(s)
}

func (readOnlyGo) FunctionBody(s core.Source, id core.SymbolID) (string, error) {
	return golang.Go{}.FunctionBody(s, id)
}

func (readOnlyGo) Function(s core.Source, id core.SymbolID) (string, error) {
	return golang.Go{}.Function(s, id)
}

func (readOnlyGo) ReadInterface(s core.Source, id core.SymbolID) (string, error) {
	return golang.Go{}.ReadInterface(s, id)
}

func (readOnlyGo) ReadStruct(s core.Source, id core.SymbolID) (string, error) {
	return golang.Go{}.ReadStruct(s, id)
}

// Compile-time assertion that readOnlyGo satisfies the read contract. That it does
// NOT satisfy the write one is asserted at run time, below, because a missing
// interface cannot be checked at compile time.
var _ core.Language = readOnlyGo{}

// TestReadOnlyLanguageHasNoWriter pins the premise the read-only test rests on:
// if readOnlyGo ever gained a ResolveEdits method, that test would pass for the
// wrong reason.
func TestReadOnlyLanguageHasNoWriter(t *testing.T) {
	var l core.Language = readOnlyGo{}
	if _, isWriter := l.(core.FunctionWriter); isWriter {
		t.Fatal("readOnlyGo implements FunctionWriter; the read-only test would be vacuous")
	}
}
