package javascript

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

const examples = "../../../examples/javascript/"

// parse loads an example file into a Source. The caller must Close it.
func parse(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(examples + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := (JavaScript{}).Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return src
}

// TestParseAllExamples parses every example, including the byte-sensitive edge
// files (CRLF, no trailing newline, unicode), and asserts none produce error nodes.
func TestParseAllExamples(t *testing.T) {
	entries, err := os.ReadDir(examples)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".js" {
			continue
		}
		count++
		src := parse(t, e.Name())
		if src.Root().HasError() {
			t.Errorf("%s: parse tree has error nodes", e.Name())
		}
		src.Close()
	}
	if count == 0 {
		t.Fatal("no .js examples found")
	}
}

// TestListSignatures checks symbol names, params and counts per file.
func TestListSignatures(t *testing.T) {
	cases := []struct {
		file  string
		count int
		names []string
	}{
		{"math.js", 4, []string{"add", "subtract", "multiply", "square"}},
		{"shapes.js", 7, []string{"Shape", "area", "describe", "Circle", "constructor", "area", "area"}},
		{"closures.js", 3, []string{"makeCounter", "identity", "pair"}},
		{"complex.js", 22, nil},
		{"unicode.js", 2, []string{"café", "Ωmega"}},
		{"crlf.js", 2, []string{"greet", "farewell"}},
		{"nonewline.js", 1, []string{"onlyOne"}},
	}
	for _, tc := range cases {
		src := parse(t, tc.file)
		sigs, err := (JavaScript{}).ListSignatures(src)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if len(sigs) != tc.count {
			t.Errorf("%s: got %d signatures, want %d", tc.file, len(sigs), tc.count)
		}
		for i, want := range tc.names {
			if i < len(sigs) && sigs[i].Name != want {
				t.Errorf("%s: sig[%d] = %q, want %q", tc.file, i, sigs[i].Name, want)
			}
		}
		src.Close()
	}
}

// TestParamsAndArrowForms verifies parameter extraction across function forms,
// including the single-identifier arrow parameter (`n => ...`).
func TestParamsAndArrowForms(t *testing.T) {
	src := parse(t, "math.js")
	defer src.Close()
	sigs, _ := (JavaScript{}).ListSignatures(src)
	got := map[string][]string{}
	for _, s := range sigs {
		got[s.Name] = s.Params
	}
	if want := []string{"a", "b"}; !eq(got["add"], want) {
		t.Errorf("add params = %v, want %v", got["add"], want)
	}
	if want := []string{"n"}; !eq(got["square"], want) {
		t.Errorf("square params = %v, want %v", got["square"], want)
	}
}

// TestFunctionAndBody compares extracted function and body text exactly.
func TestFunctionAndBody(t *testing.T) {
	src := parse(t, "math.js")
	defer src.Close()
	js := JavaScript{}

	body, err := js.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "multiply"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  return a * b;\n}"; body != want {
		t.Errorf("multiply body = %q, want %q", body, want)
	}

	// Expression-bodied arrow returns the expression text, not a block.
	sq, err := js.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "square"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "n * n"; sq != want {
		t.Errorf("square body = %q, want %q", sq, want)
	}

	fn, err := js.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "multiply"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/**\n * multiply returns the product of two numbers.\n" +
		" * It does not mutate its arguments.\n */\n" +
		"function multiply(a, b) {\n  return a * b;\n}"
	if fn != want {
		t.Errorf("multiply function = %q, want %q", fn, want)
	}
}

// TestDocMultiline asserts the multi-line JSDoc is captured verbatim.
func TestDocMultiline(t *testing.T) {
	src := parse(t, "math.js")
	defer src.Close()
	sigs, _ := (JavaScript{}).ListSignatures(src)
	for _, s := range sigs {
		if s.Name != "multiply" {
			continue
		}
		want := "/**\n * multiply returns the product of two numbers.\n" +
			" * It does not mutate its arguments.\n */"
		if s.Doc != want {
			t.Errorf("multiply doc = %q, want %q", s.Doc, want)
		}
		return
	}
	t.Fatal("multiply not found")
}

// TestDisambiguation proves three symbols named "area" — two methods with
// different containers and one free function — are extracted distinctly.
func TestDisambiguation(t *testing.T) {
	src := parse(t, "shapes.js")
	defer src.Close()
	js := JavaScript{}

	shapeArea, err := js.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Shape"})
	if err != nil {
		t.Fatal(err)
	}
	circleArea, err := js.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	freeArea, err := js.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	if shapeArea == circleArea || shapeArea == freeArea || circleArea == freeArea {
		t.Fatalf("area bodies not distinct:\n Shape=%q\n Circle=%q\n free=%q", shapeArea, circleArea, freeArea)
	}
	if want := "{\n    return 0;\n  }"; shapeArea != want {
		t.Errorf("Shape.area body = %q, want %q", shapeArea, want)
	}
	if want := "{\n  return w * h;\n}"; freeArea != want {
		t.Errorf("free area body = %q, want %q", freeArea, want)
	}
}

// TestReadStruct extracts a known class definition.
func TestReadStruct(t *testing.T) {
	src := parse(t, "shapes.js")
	defer src.Close()
	def, err := (JavaScript{}).ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	want := "class Circle {\n  constructor(r) {\n    this.r = r;\n  }\n\n" +
		"  // area returns the circle's area.\n" +
		"  area() {\n    return 3.14159 * this.r * this.r;\n  }\n}"
	if def != want {
		t.Errorf("Circle struct = %q, want %q", def, want)
	}
}

// TestReadInterfaceUnsupported asserts JavaScript reports no interfaces.
func TestReadInterfaceUnsupported(t *testing.T) {
	src := parse(t, "shapes.js")
	defer src.Close()
	_, err := (JavaScript{}).ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("ReadInterface error = %v, want ErrSymbolNotFound", err)
	}
}

// TestSymbolNotFound asserts a missing symbol yields ErrSymbolNotFound.
func TestSymbolNotFound(t *testing.T) {
	src := parse(t, "math.js")
	defer src.Close()
	_, err := (JavaScript{}).FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "nope"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("error = %v, want ErrSymbolNotFound", err)
	}
}

// TestRelativeRangeRejected asserts a relative-range edit is rejected, not panicked.
func TestRelativeRangeRejected(t *testing.T) {
	src := parse(t, "math.js")
	defer src.Close()
	_, err := (JavaScript{}).ResolveEdits(src, []core.Edit{{
		Target: core.SymbolID{Kind: core.KindFunc, Name: "add"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}})
	if !errors.Is(err, core.ErrRelativeRangeNotImplemented) {
		t.Errorf("error = %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// TestParseRejectionViaBatchWrite drives the core write pipeline: an edit whose
// new body breaks the syntax must be rejected and the file left byte-for-byte
// intact.
func TestParseRejectionViaBatchWrite(t *testing.T) {
	path, before := tempCopy(t, "math.js")
	res, err := core.BatchWrite(JavaScript{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "multiply"},
		NewText: "function multiply(a, b) { return a + ", // unbalanced — broken
	}})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Errorf("error = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("result reported Applied on a rejected batch")
	}
	if h := hash(t, path); h != before {
		t.Error("file changed despite rejected write")
	}
}

// TestWriteIdempotent rewrites a symbol with its own exact text and asserts the
// file hash is unchanged.
func TestWriteIdempotent(t *testing.T) {
	path, before := tempCopy(t, "math.js")

	// Recover the symbol's exact current span text (no doc) to feed back in.
	b, _ := os.ReadFile(path)
	src, _ := (JavaScript{}).Parse(b)
	var same string
	for _, s := range collect(src.Root(), src.Bytes()) {
		if s.id.Name == "subtract" {
			same = s.text.Utf8Text(src.Bytes())
		}
	}
	src.Close()
	if same == "" {
		t.Fatal("subtract not found")
	}

	res, err := core.BatchWrite(JavaScript{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "subtract"},
		NewText: same,
	}})
	if err != nil {
		t.Fatalf("idempotent write: %v", err)
	}
	if !res.Applied {
		t.Error("idempotent write not applied")
	}
	if h := hash(t, path); h != before {
		t.Error("file hash changed after writing identical text")
	}
}

// tempCopy copies an example into a temp dir and returns its path and content hash.
func tempCopy(t *testing.T, name string) (string, [32]byte) {
	t.Helper()
	b, err := os.ReadFile(examples + name)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, sha256.Sum256(b)
}

// hash returns the SHA-256 of the file at path.
func hash(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
