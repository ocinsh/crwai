package golang

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

// examplesDir is the path to the deterministic Go sources, relative to this
// package directory (internal/lang/golang).
const examplesDir = "../../../examples/golang"

func example(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(examplesDir, name)
}

// parse reads and parses an example file, registering Close via t.Cleanup.
func parse(t *testing.T, path string) core.Source {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src, err := Go{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// TestParseAllExamples parses every example and asserts the tree is error-free.
func TestParseAllExamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(examplesDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob examples: %v (found %d)", err, len(files))
	}
	for _, f := range files {
		src := parse(t, f)
		if src.Root().HasError() {
			t.Errorf("%s: parse tree has errors", f)
		}
	}
}

func sigNames(sigs []core.Signature) []string {
	names := make([]string, len(sigs))
	for i, s := range sigs {
		names[i] = s.Name
	}
	return names
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// TestListSignaturesBasic checks names, params, and returns on basic.go.
func TestListSignaturesBasic(t *testing.T) {
	src := parse(t, example(t, "basic.go"))
	sigs, err := Go{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	names := sigNames(sigs)
	for _, want := range []string{"Add", "Greet", "noDoc", "Variadic", "MultiReturn"} {
		if !contains(names, want) {
			t.Errorf("missing signature %q; got %v", want, names)
		}
	}

	var add core.Signature
	for _, s := range sigs {
		if s.Name == "Add" {
			add = s
		}
	}
	if got, want := add.Params, []string{"a int", "b int"}; !equalStrings(got, want) {
		t.Errorf("Add params = %v, want %v", got, want)
	}
	if add.Returns != "int" {
		t.Errorf("Add returns = %q, want %q", add.Returns, "int")
	}
}

// TestListSignaturesBigCountAndOrder verifies the big file lists >= 22 symbols in
// file order (deterministic).
func TestListSignaturesBigCountAndOrder(t *testing.T) {
	src := parse(t, example(t, "big.go"))
	sigs, err := Go{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) < 22 {
		t.Fatalf("big.go: got %d signatures, want >= 22", len(sigs))
	}
	if sigs[0].Name != "BigConst01" {
		t.Errorf("first symbol = %q, want BigConst01 (file order)", sigs[0].Name)
	}
	names := sigNames(sigs)
	for _, want := range []string{"BigConst01", "BigConst22"} {
		if !contains(names, want) {
			t.Errorf("missing %q in big.go", want)
		}
	}
}

// TestListSignaturesKind verifies the cheap listing labels each symbol with its
// kind, so a caller can tell a struct/interface from a function without already
// knowing which it is.
func TestListSignaturesKind(t *testing.T) {
	src := parse(t, example(t, "types.go"))
	sigs, err := Go{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]core.SymbolKind{}
	for _, s := range sigs {
		kinds[s.Name] = s.Kind
	}
	for name, want := range map[string]core.SymbolKind{
		"Point":    core.KindStruct,
		"Shape":    core.KindInterface,
		"Stringer": core.KindInterface,
	} {
		if got, ok := kinds[name]; !ok {
			t.Errorf("missing signature %q", name)
		} else if got != want {
			t.Errorf("%s kind = %v, want %v", name, got, want)
		}
	}
}

// TestListSignaturesVerbatim checks that callables carry the full verbatim
// signature line (receiver and type parameters included) and the machine-readable
// Container, so a method is never mistaken for a free function.
func TestListSignaturesVerbatim(t *testing.T) {
	src := parse(t, example(t, "generics.go"))
	sigs, err := Go{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]core.Signature{}
	for _, s := range sigs {
		byName[s.Name] = s
	}
	if got := byName["Len"]; got.Text != "func (s *Stack[T]) Len() int" || got.Container != "Stack" {
		t.Errorf("Len: Text=%q Container=%q, want the full receiver signature bound to Stack", got.Text, got.Container)
	}
	if got := byName["MapSlice"]; got.Text != "func MapSlice[T any, U any](in []T, f func(T) U) []U" || got.Container != "" {
		t.Errorf("MapSlice: Text=%q Container=%q, want the type-parameter signature with no container", got.Text, got.Container)
	}
}

// TestFunctionAndBody checks full-function and body-only extraction.
func TestFunctionAndBody(t *testing.T) {
	src := parse(t, example(t, "basic.go"))
	fn, err := Go{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "Add"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fn, "func Add(a int, b int) int") {
		t.Errorf("Function(Add) missing signature:\n%s", fn)
	}
	if !strings.HasPrefix(fn, "// Add returns the sum") {
		t.Errorf("Function(Add) should start with its doc:\n%s", fn)
	}

	body, err := Go{}.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "Add"})
	if err != nil {
		t.Fatal(err)
	}
	if body != "return a + b" {
		t.Errorf("FunctionBody(Add) = %q, want %q", body, "return a + b")
	}
}

// TestReadInterfaceAndStruct checks type extraction on types.go.
func TestReadInterfaceAndStruct(t *testing.T) {
	src := parse(t, example(t, "types.go"))

	iface, err := Go{}.ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(iface, "interface") || !strings.Contains(iface, "Area() float64") {
		t.Errorf("ReadInterface(Shape) unexpected:\n%s", iface)
	}

	st, err := Go{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Point"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st, "type Point struct") || !strings.Contains(st, "X is the horizontal") {
		t.Errorf("ReadStruct(Point) unexpected:\n%s", st)
	}

	if _, err := (Go{}).ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Nope"}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("ReadStruct(Nope) err = %v, want ErrSymbolNotFound", err)
	}
}

// TestContainerDisambiguation verifies same-named symbols with different
// containers are not confused.
func TestContainerDisambiguation(t *testing.T) {
	basic := parse(t, example(t, "basic.go"))
	methods := parse(t, example(t, "methods.go"))

	topGreet, err := Go{}.Function(basic, core.SymbolID{Kind: core.KindFunc, Name: "Greet"})
	if err != nil {
		t.Fatal(err)
	}
	methGreet, err := Go{}.Function(methods, core.SymbolID{Kind: core.KindMethod, Name: "Greet", Container: "Greeter"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(topGreet, "func Greet(name string) string") {
		t.Errorf("top-level Greet wrong:\n%s", topGreet)
	}
	if !strings.Contains(methGreet, "func (g *Greeter) Greet(name string) string") {
		t.Errorf("Greeter.Greet wrong:\n%s", methGreet)
	}
	if topGreet == methGreet {
		t.Errorf("top-level Greet and Greeter.Greet resolved to the same text")
	}

	// Two methods named Reset on different receivers.
	counterReset, err := Go{}.Function(methods, core.SymbolID{Kind: core.KindMethod, Name: "Reset", Container: "Counter"})
	if err != nil {
		t.Fatal(err)
	}
	greeterReset, err := Go{}.Function(methods, core.SymbolID{Kind: core.KindMethod, Name: "Reset", Container: "Greeter"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(counterReset, "func (c *Counter) Reset()") {
		t.Errorf("Counter.Reset wrong:\n%s", counterReset)
	}
	if !strings.Contains(greeterReset, "func (g *Greeter) Reset()") {
		t.Errorf("Greeter.Reset wrong:\n%s", greeterReset)
	}
}

// TestGenericReceiverContainer checks that a generic receiver (Stack[T]) resolves
// to the bare container name "Stack".
func TestGenericReceiverContainer(t *testing.T) {
	src := parse(t, example(t, "generics.go"))
	push, err := Go{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "Push", Container: "Stack"})
	if err != nil {
		t.Fatalf("locate Stack.Push: %v", err)
	}
	if !strings.Contains(push, "func (s *Stack[T]) Push(x T)") {
		t.Errorf("Stack.Push wrong:\n%s", push)
	}
}

// TestDocExtractionMultiline verifies the exact multi-line doc for Greet.
func TestDocExtractionMultiline(t *testing.T) {
	src := parse(t, example(t, "basic.go"))
	decl, ok := Go{}.locateFunc(src, core.SymbolID{Kind: core.KindFunc, Name: "Greet"})
	if !ok {
		t.Fatal("locate Greet")
	}
	want := "// Greet builds a greeting for name. It has a multi-line doc comment so the\n" +
		"// doc-extraction logic can be verified exactly:\n" +
		"//\n" +
		"// The second paragraph is part of the doc too, including this list:\n" +
		"//   - one\n" +
		"//   - two"
	if got := docOf(decl, src.Bytes()); got != want {
		t.Errorf("docOf(Greet) mismatch:\n got: %q\nwant: %q", got, want)
	}

	// noDoc has no preceding comment.
	nd, ok := Go{}.locateFunc(src, core.SymbolID{Kind: core.KindFunc, Name: "noDoc"})
	if !ok {
		t.Fatal("locate noDoc")
	}
	if got := docOf(nd, src.Bytes()); got != "" {
		t.Errorf("docOf(noDoc) = %q, want empty", got)
	}
}

// TestUnicodeSymbols verifies non-ASCII identifiers are handled.
func TestUnicodeSymbols(t *testing.T) {
	src := parse(t, example(t, "unicode.go"))
	sigs, err := Go{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	names := sigNames(sigs)
	for _, want := range []string{"Café", "Σ"} {
		if !contains(names, want) {
			t.Errorf("missing unicode symbol %q; got %v", want, names)
		}
	}
}

// --- write pipeline (via core.BatchWrite) ---

func copyToTemp(t *testing.T, srcPath string) (string, [32]byte, []byte) {
	t.Helper()
	b, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(srcPath))
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst, sha256.Sum256(b), b
}

func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

// TestWriteParseRejection: a write that breaks syntax is rejected and the file is
// left byte-identical.
func TestWriteParseRejection(t *testing.T) {
	path, before, _ := copyToTemp(t, example(t, "basic.go"))
	_, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: "func Add(a int, b int) int { return a + ", // broken
	}})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite rejected write")
	}
}

// TestWriteSymbolNotFound: an unknown symbol fails the batch, file untouched.
func TestWriteSymbolNotFound(t *testing.T) {
	path, before, _ := copyToTemp(t, example(t, "basic.go"))
	_, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "DoesNotExist"},
		NewText: "func DoesNotExist() {}",
	}})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("err = %v, want ErrSymbolNotFound", err)
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite failed write")
	}
}

// declText returns the exact source text of a symbol's declaration (doc excluded).
func declText(t *testing.T, src core.Source, id core.SymbolID) string {
	t.Helper()
	decl, ok := Go{}.locateFunc(src, id)
	if !ok {
		t.Fatalf("locate %v", id)
	}
	return decl.Utf8Text(src.Bytes())
}

// TestWriteIdempotence: writing a symbol's own current text back leaves the file
// hash unchanged.
func TestWriteIdempotence(t *testing.T) {
	path, before, _ := copyToTemp(t, example(t, "basic.go"))
	src := parse(t, path)
	orig := declText(t, src, core.SymbolID{Kind: core.KindFunc, Name: "Add"})

	res, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: orig,
	}})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !res.Applied {
		t.Fatal("write not applied")
	}
	if hashFile(t, path) != before {
		t.Error("idempotent write changed the file hash")
	}
}

// TestWriteRoundTrip: edit a function, then restore it; final hash equals original.
func TestWriteRoundTrip(t *testing.T) {
	path, before, _ := copyToTemp(t, example(t, "basic.go"))

	src := parse(t, path)
	orig := declText(t, src, core.SymbolID{Kind: core.KindFunc, Name: "Add"})

	res, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: "func Add(a int, b int) int {\n\treturn b + a\n}",
	}})
	if err != nil || !res.Applied {
		t.Fatalf("first write: %v applied=%v", err, res.Applied)
	}
	if hashFile(t, path) == before {
		t.Fatal("file unchanged after a real edit")
	}

	res, err = core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: orig,
	}})
	if err != nil || !res.Applied {
		t.Fatalf("restore write: %v applied=%v", err, res.Applied)
	}
	if hashFile(t, path) != before {
		t.Error("file hash differs after round-trip restore")
	}
}

// TestWriteAtomicBatch: a batch with one valid and one broken edit is rejected
// whole; the file is untouched.
func TestWriteAtomicBatch(t *testing.T) {
	path, before, _ := copyToTemp(t, example(t, "basic.go"))
	_, err := core.BatchWrite(Go{}, path, []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "Add"}, NewText: "func Add(a int, b int) int { return a + b }"},
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "Variadic"}, NewText: "func Variadic(nums ...int) int { return "}, // broken
	})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite rejected batch")
	}
}

// TestRelativeRangeRejected: an Edit carrying a RelativeRange is rejected with the
// typed sentinel (v1 contract).
func TestRelativeRangeRejected(t *testing.T) {
	src := parse(t, example(t, "basic.go"))
	_, err := Go{}.ResolveEdits(src, []core.Edit{{
		Target: core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}})
	if !errors.Is(err, ErrRelativeRangeNotImplemented) {
		t.Fatalf("err = %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// --- robustness: line endings, BOM, missing final newline ---

// TestWritePreservesCRLF: editing a CRLF file must not introduce lone LFs.
func TestWritePreservesCRLF(t *testing.T) {
	b, err := os.ReadFile(example(t, "basic.go"))
	if err != nil {
		t.Fatal(err)
	}
	crlf := bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	path := filepath.Join(t.TempDir(), "crlf.go")
	if err := os.WriteFile(path, crlf, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "noDoc"},
		NewText: "func noDoc() bool {\r\n\treturn false\r\n}",
	}})
	if err != nil || !res.Applied {
		t.Fatalf("write: %v applied=%v", err, res.Applied)
	}
	after, _ := os.ReadFile(path)
	if bytes.Count(after, []byte("\n")) != bytes.Count(after, []byte("\r\n")) {
		t.Error("CRLF corrupted: found lone LF bytes after write")
	}
}

// TestWritePreservesNoFinalNewline: a file without a trailing newline keeps that
// property after an edit elsewhere.
func TestWritePreservesNoFinalNewline(t *testing.T) {
	b, err := os.ReadFile(example(t, "basic.go"))
	if err != nil {
		t.Fatal(err)
	}
	trimmed := bytes.TrimRight(b, "\n")
	path := filepath.Join(t.TempDir(), "nonl.go")
	if err := os.WriteFile(path, trimmed, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: "func Add(a int, b int) int {\n\treturn a + b\n}",
	}})
	if err != nil || !res.Applied {
		t.Fatalf("write: %v applied=%v", err, res.Applied)
	}
	after, _ := os.ReadFile(path)
	if len(after) == 0 || after[len(after)-1] == '\n' {
		t.Error("a spurious final newline was added")
	}
}

// TestBOMReadAndWrite: a UTF-8 BOM file parses, reads, and round-trips cleanly.
func TestBOMReadAndWrite(t *testing.T) {
	b, err := os.ReadFile(example(t, "basic.go"))
	if err != nil {
		t.Fatal(err)
	}
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, b...)
	path := filepath.Join(t.TempDir(), "bom.go")
	if err := os.WriteFile(path, withBOM, 0o644); err != nil {
		t.Fatal(err)
	}
	before := hashFile(t, path)

	src := parse(t, path)
	if src.Root().HasError() {
		t.Fatal("BOM file parsed with errors")
	}
	if _, err := (Go{}).Function(src, core.SymbolID{Kind: core.KindFunc, Name: "Add"}); err != nil {
		t.Fatalf("read Add from BOM file: %v", err)
	}
	orig := declText(t, src, core.SymbolID{Kind: core.KindFunc, Name: "Add"})

	res, err := core.BatchWrite(Go{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "Add"},
		NewText: orig,
	}})
	if err != nil || !res.Applied {
		t.Fatalf("write BOM file: %v applied=%v", err, res.Applied)
	}
	if hashFile(t, path) != before {
		t.Error("idempotent write changed the BOM file")
	}
	if after, _ := os.ReadFile(path); !bytes.HasPrefix(after, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("BOM was stripped by the write")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
