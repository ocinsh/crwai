package rust_test

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
	"github.com/ocinsh/crwai/internal/lang/rust"
)

// examplesDir is the shared corpus the server operates on.
const examplesDir = "../../../examples/rust"

// parse reads an example file and parses it; the returned Source is closed via
// t.Cleanup so each test reads from disk exactly once.
func parse(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := rust.Rust{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// TestParseExamples checks every example file parses without error nodes.
func TestParseExamples(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".rs" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			src := parse(t, e.Name())
			if src.Root().HasError() {
				t.Errorf("%s parsed with error nodes", e.Name())
			}
		})
	}
}

// TestListSignaturesCounts pins the symbol count of each example so accidental
// over/under-collection is caught.
func TestListSignaturesCounts(t *testing.T) {
	want := map[string]int{
		"basic.rs":    3,
		"methods.rs":  5,
		"traits.rs":   4,
		"generics.rs": 3,
		"edge.rs":     3,
		"complex.rs":  24,
	}
	for name, n := range want {
		t.Run(name, func(t *testing.T) {
			sigs, err := rust.Rust{}.ListSignatures(parse(t, name))
			if err != nil {
				t.Fatalf("ListSignatures: %v", err)
			}
			if len(sigs) != n {
				var got []string
				for _, s := range sigs {
					got = append(got, s.Name)
				}
				t.Errorf("count = %d, want %d (names: %v)", len(sigs), n, got)
			}
		})
	}
}

// TestSignatureFields spot-checks names, params, returns, and doc on a known file.
func TestSignatureFields(t *testing.T) {
	sigs, err := rust.Rust{}.ListSignatures(parse(t, "basic.rs"))
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	byName := map[string]core.Signature{}
	for _, s := range sigs {
		byName[s.Name] = s
	}

	add := byName["add"]
	if got := strings.Join(add.Params, ", "); got != "a: i32, b: i32" {
		t.Errorf("add params = %q", got)
	}
	if add.Returns != "i32" {
		t.Errorf("add returns = %q", add.Returns)
	}
	if add.Doc != "/// Adds two integers and returns the sum." {
		t.Errorf("add doc = %q", add.Doc)
	}
	if byName["no_doc"].Doc != "" {
		t.Errorf("no_doc doc = %q, want empty", byName["no_doc"].Doc)
	}
}

// TestDocMultiline verifies a multi-line doc comment is captured verbatim.
func TestDocMultiline(t *testing.T) {
	sigs, _ := rust.Rust{}.ListSignatures(parse(t, "basic.rs"))
	const want = "/// First line of the doc.\n/// Second line of the doc.\n/// Third line of the doc."
	for _, s := range sigs {
		if s.Name == "documented" {
			if s.Doc != want {
				t.Errorf("documented doc =\n%q\nwant\n%q", s.Doc, want)
			}
			return
		}
	}
	t.Fatal("symbol documented not found")
}

// TestFunctionAndBody checks whole-function and body-only extraction.
func TestFunctionAndBody(t *testing.T) {
	r := rust.Rust{}
	src := parse(t, "basic.rs")

	fn, err := r.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "add"})
	if err != nil {
		t.Fatalf("Function(add): %v", err)
	}
	const wantFn = "/// Adds two integers and returns the sum.\nfn add(a: i32, b: i32) -> i32 {\n    a + b\n}"
	if fn != wantFn {
		t.Errorf("Function(add) =\n%q\nwant\n%q", fn, wantFn)
	}

	body, err := r.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "documented"})
	if err != nil {
		t.Fatalf("FunctionBody(documented): %v", err)
	}
	if body != "{\n    true\n}" {
		t.Errorf("FunctionBody(documented) = %q", body)
	}
}

// TestReadStructAndInterface checks struct and trait (interface) extraction.
func TestReadStructAndInterface(t *testing.T) {
	r := rust.Rust{}

	st, err := r.ReadStruct(parse(t, "methods.rs"), core.SymbolID{Kind: core.KindStruct, Name: "Stack"})
	if err != nil {
		t.Fatalf("ReadStruct(Stack): %v", err)
	}
	const wantStruct = "/// A simple stack of integers.\nstruct Stack {\n    items: Vec<i32>,\n}"
	if st != wantStruct {
		t.Errorf("ReadStruct(Stack) =\n%q\nwant\n%q", st, wantStruct)
	}

	iface, err := r.ReadInterface(parse(t, "traits.rs"), core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if err != nil {
		t.Fatalf("ReadInterface(Shape): %v", err)
	}
	if !strings.HasPrefix(iface, "/// Shape can compute its own area.\ntrait Shape {") {
		t.Errorf("ReadInterface(Shape) = %q", iface)
	}
}

// TestDisambiguation proves two symbols that share a name but differ by Container
// are resolved independently.
func TestDisambiguation(t *testing.T) {
	r := rust.Rust{}

	// methods.rs: method size/Stack vs free function size.
	src := parse(t, "methods.rs")
	method, err := r.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "size", Container: "Stack"})
	if err != nil {
		t.Fatalf("Function(size/Stack): %v", err)
	}
	free, err := r.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "size"})
	if err != nil {
		t.Fatalf("Function(size): %v", err)
	}
	if method == free {
		t.Fatal("size/Stack and free size resolved to the same text")
	}
	if !strings.Contains(method, "self.items.len()") {
		t.Errorf("size/Stack body unexpected: %q", method)
	}
	if !strings.Contains(free, "fn size() -> usize") || strings.Contains(free, "self") {
		t.Errorf("free size unexpected: %q", free)
	}

	// traits.rs: area/Shape (declaration) vs area/Circle (implementation).
	tsrc := parse(t, "traits.rs")
	decl, err := r.Function(tsrc, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Shape"})
	if err != nil {
		t.Fatalf("Function(area/Shape): %v", err)
	}
	impl, err := r.Function(tsrc, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Circle"})
	if err != nil {
		t.Fatalf("Function(area/Circle): %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(decl), "fn area(&self) -> f64;") {
		t.Errorf("area/Shape should be a signature-only declaration: %q", decl)
	}
	if !strings.Contains(impl, "3.14159") {
		t.Errorf("area/Circle should be the implementation: %q", impl)
	}
}

// TestUnicodeSymbol resolves a function whose name is a Unicode identifier.
func TestUnicodeSymbol(t *testing.T) {
	fn, err := rust.Rust{}.Function(parse(t, "edge.rs"), core.SymbolID{Kind: core.KindFunc, Name: "寿司"})
	if err != nil {
		t.Fatalf("Function(寿司): %v", err)
	}
	if !strings.Contains(fn, `"🍣"`) {
		t.Errorf("Function(寿司) = %q", fn)
	}
}

// TestSymbolNotFound checks a missing symbol yields the typed sentinel.
func TestSymbolNotFound(t *testing.T) {
	_, err := rust.Rust{}.Function(parse(t, "basic.rs"), core.SymbolID{Kind: core.KindFunc, Name: "does_not_exist"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("err = %v, want ErrSymbolNotFound", err)
	}
}

// TestRelativeRangeRejected checks symbol-relative edits are refused with the
// typed sentinel rather than silently applied or panicking.
func TestRelativeRangeRejected(t *testing.T) {
	edits := []core.Edit{{
		Target: core.SymbolID{Kind: core.KindFunc, Name: "add"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}}
	_, err := rust.Rust{}.ResolveEdits(parse(t, "basic.rs"), edits)
	if !errors.Is(err, core.ErrRelativeRangeNotImplemented) {
		t.Errorf("err = %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// --- write pipeline (via core.BatchWrite on a temp copy) ---

// hashFile returns the SHA-256 of a file's contents.
func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return sha256.Sum256(b)
}

// tempCopy copies an example into a temp dir and returns the new path; writes in
// tests never touch the originals.
func tempCopy(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return tempCopyBytes(t, name, b)
}

func tempCopyBytes(t *testing.T, name string, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write temp %s: %v", name, err)
	}
	return path
}

// TestWriteParseRejection: a body that breaks the syntax must be rejected and the
// file left byte-for-byte intact.
func TestWriteParseRejection(t *testing.T) {
	path := tempCopy(t, "basic.rs")
	before := hashFile(t, path)

	edit := core.Edit{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "add"},
		NewText: "fn add(a: i32, b: i32) -> i32 { a + }", // missing operand
	}
	res, err := core.BatchWrite(rust.Rust{}, path, []core.Edit{edit})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("Applied = true, want false")
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite rejected write")
	}
}

// TestWriteIdempotent: rewriting a symbol with its own exact text must not change
// the file. no_doc has no doc comment, so Function returns its node text verbatim.
func TestWriteIdempotent(t *testing.T) {
	path := tempCopy(t, "basic.rs")
	before := hashFile(t, path)

	b, _ := os.ReadFile(path)
	src, _ := rust.Rust{}.Parse(b)
	original, err := rust.Rust{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "no_doc"})
	src.Close()
	if err != nil {
		t.Fatalf("Function(no_doc): %v", err)
	}

	edit := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "no_doc"}, NewText: original}
	res, err := core.BatchWrite(rust.Rust{}, path, []core.Edit{edit})
	if err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	if !res.Applied {
		t.Fatal("Applied = false")
	}
	if hashFile(t, path) != before {
		t.Error("idempotent rewrite changed the file")
	}
}

// TestWriteRoundTrip: a real edit then its inverse restores the original hash.
func TestWriteRoundTrip(t *testing.T) {
	path := tempCopy(t, "basic.rs")
	before := hashFile(t, path)

	add := core.SymbolID{Kind: core.KindFunc, Name: "add"}
	if _, err := core.BatchWrite(rust.Rust{}, path,
		[]core.Edit{{Target: add, NewText: "fn add(a: i32, b: i32) -> i32 {\n    a + b + 0\n}"}}); err != nil {
		t.Fatalf("forward write: %v", err)
	}
	if hashFile(t, path) == before {
		t.Fatal("forward write did not change the file")
	}
	if _, err := core.BatchWrite(rust.Rust{}, path,
		[]core.Edit{{Target: add, NewText: "fn add(a: i32, b: i32) -> i32 {\n    a + b\n}"}}); err != nil {
		t.Fatalf("restore write: %v", err)
	}
	if hashFile(t, path) != before {
		t.Error("round-trip did not restore the original file")
	}
}

// TestWriteAtomicBatch: a batch with one valid and one syntax-breaking edit must
// land none of them and leave the file intact.
func TestWriteAtomicBatch(t *testing.T) {
	path := tempCopy(t, "methods.rs")
	before := hashFile(t, path)

	edits := []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "size"}, NewText: "fn size() -> usize {\n    1\n}"},
		{Target: core.SymbolID{Kind: core.KindMethod, Name: "push", Container: "Stack"},
			NewText: "fn push(&mut self, value: i32) { self.items.push(value }"}, // broken
	}
	res, err := core.BatchWrite(rust.Rust{}, path, edits)
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("Applied = true, want false")
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite all-or-nothing rejection")
	}
}

// TestWriteUnknownSymbolRejected: a batch addressing a missing symbol fails whole.
func TestWriteUnknownSymbolRejected(t *testing.T) {
	path := tempCopy(t, "basic.rs")
	before := hashFile(t, path)

	edits := []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "no_doc"}, NewText: "fn no_doc() {}"},
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "ghost"}, NewText: "fn ghost() {}"},
	}
	if _, err := core.BatchWrite(rust.Rust{}, path, edits); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("err = %v, want ErrSymbolNotFound", err)
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite unresolved symbol")
	}
}

// --- encoding / line-ending edge cases ---

// TestWritePreservesBOM checks a UTF-8 BOM survives a surgical write that does not
// touch the file head.
func TestWritePreservesBOM(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join(examplesDir, "basic.rs"))
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, b...)
	path := tempCopyBytes(t, "bom.rs", withBOM)

	edit := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "no_doc"}, NewText: "fn no_doc() {}"}
	if _, err := core.BatchWrite(rust.Rust{}, path, []core.Edit{edit}); err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	out, _ := os.ReadFile(path)
	if len(out) < 3 || out[0] != 0xEF || out[1] != 0xBB || out[2] != 0xBF {
		t.Error("BOM was not preserved")
	}
}

// TestWritePreservesCRLF checks existing CRLF line endings are not corrupted by a
// surgical write.
func TestWritePreservesCRLF(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join(examplesDir, "basic.rs"))
	crlf := []byte(strings.ReplaceAll(string(b), "\n", "\r\n"))
	path := tempCopyBytes(t, "crlf.rs", crlf)
	wantCRLF := strings.Count(string(crlf), "\r\n")

	edit := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "no_doc"}, NewText: "fn no_doc() {}"}
	if _, err := core.BatchWrite(rust.Rust{}, path, []core.Edit{edit}); err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	out, _ := os.ReadFile(path)
	if got := strings.Count(string(out), "\r\n"); got != wantCRLF {
		t.Errorf("CRLF count = %d, want %d", got, wantCRLF)
	}
}

// TestWritePreservesNoFinalNewline checks a file without a trailing newline keeps
// that shape after a surgical write that does not touch the tail.
func TestWritePreservesNoFinalNewline(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join(examplesDir, "basic.rs"))
	nonl := []byte(strings.TrimRight(string(b), "\n"))
	path := tempCopyBytes(t, "nonl.rs", nonl)

	edit := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "no_doc"}, NewText: "fn no_doc() {}"}
	if _, err := core.BatchWrite(rust.Rust{}, path, []core.Edit{edit}); err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	out, _ := os.ReadFile(path)
	if len(out) == 0 || out[len(out)-1] == '\n' {
		t.Error("a trailing newline was added")
	}
}
