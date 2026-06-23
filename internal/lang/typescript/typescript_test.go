package typescript

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

const examples = "../../../examples/typescript/"

// lang returns the right implementation for a file: the TSX grammar for .tsx,
// the pure TypeScript grammar otherwise.
func langFor(name string) core.Language {
	if filepath.Ext(name) == ".tsx" {
		return TSX{}
	}
	return TypeScript{}
}

// parse loads an example file into a Source. The caller must Close it.
func parse(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(examples + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := langFor(name).Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return src
}

// TestParseAllExamples parses every example — including the byte-sensitive edge
// files (CRLF, no trailing newline, BOM, unicode) and the .tsx file — and asserts
// none produce error nodes. The .tsx file is parsed with the TSX grammar.
func TestParseAllExamples(t *testing.T) {
	entries, err := os.ReadDir(examples)
	if err != nil {
		t.Fatal(err)
	}
	var ts, tsx int
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".ts" && ext != ".tsx") {
			continue
		}
		if ext == ".tsx" {
			tsx++
		} else {
			ts++
		}
		src := parse(t, e.Name())
		if src.Root().HasError() {
			t.Errorf("%s: parse tree has error nodes", e.Name())
		}
		src.Close()
	}
	if ts == 0 {
		t.Fatal("no .ts examples found")
	}
	if tsx == 0 {
		t.Fatal("no .tsx examples found")
	}
}

// TestListSignatures checks the symbol names, count, and ordering per file.
func TestListSignatures(t *testing.T) {
	cases := []struct {
		file  string
		count int
		names []string
	}{
		{"math.ts", 5, []string{"add", "subtract", "multiply", "square", "identity"}},
		{"shapes.ts", 10, []string{"Shape", "area", "Circle", "constructor", "area", "unit", "Square", "constructor", "area", "area"}},
		{"closures.ts", 3, []string{"makeCounter", "identity", "pair"}},
		{"namespace.ts", 4, []string{"perimeter", "diagonalSquared", "shout", "perimeter"}},
		{"generics.ts", 9, []string{"Container", "get", "set", "Box", "constructor", "get", "set", "firstOf", "mapList"}},
		{"complex.ts", 23, nil},
		{"unicode.ts", 2, []string{"café", "Ωmega"}},
		{"crlf.ts", 2, []string{"greet", "farewell"}},
		{"nonewline.ts", 1, []string{"onlyOne"}},
		{"bom.ts", 1, []string{"withBom"}},
		{"widget.tsx", 4, []string{"Greeting", "Badge", "Panel", "render"}},
	}
	for _, tc := range cases {
		src := parse(t, tc.file)
		sigs, err := langFor(tc.file).ListSignatures(src)
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

// TestReturnTypes verifies return-type annotations are captured (a TS-specific
// signature field plain JavaScript lacks).
func TestReturnTypes(t *testing.T) {
	src := parse(t, "math.ts")
	defer src.Close()
	sigs, _ := (TypeScript{}).ListSignatures(src)
	got := map[string]string{}
	for _, s := range sigs {
		got[s.Name] = s.Returns
	}
	if got["multiply"] != "number" {
		t.Errorf("multiply return = %q, want %q", got["multiply"], "number")
	}
	if got["identity"] != "T" {
		t.Errorf("identity return = %q, want %q", got["identity"], "T")
	}
}

// TestFunctionAndBody compares extracted function and body text exactly, across a
// declaration, an arrow const, and an exported arrow const.
func TestFunctionAndBody(t *testing.T) {
	src := parse(t, "math.ts")
	defer src.Close()
	ts := TypeScript{}

	body, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "multiply"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  return a * b;\n}"; body != want {
		t.Errorf("multiply body = %q, want %q", body, want)
	}

	// Expression-bodied arrow returns the expression text, not a block.
	sq, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "square"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "n * n"; sq != want {
		t.Errorf("square body = %q, want %q", sq, want)
	}

	fn, err := ts.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "multiply"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/**\n * multiply returns the product of two numbers.\n" +
		" * @param a the first factor\n * @param b the second factor\n" +
		" * @returns the product a * b\n */\n" +
		"function multiply(a: number, b: number): number {\n  return a * b;\n}"
	if fn != want {
		t.Errorf("multiply function = %q, want %q", fn, want)
	}

	// An exported arrow const: the whole-symbol text keeps the `export` keyword.
	id, err := ts.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "export const identity = <T>(value: T): T => value;"; id != want {
		t.Errorf("identity function = %q, want %q", id, want)
	}
}

// TestDocJSDoc asserts the multi-line JSDoc (description + @param + @returns) is
// captured verbatim.
func TestDocJSDoc(t *testing.T) {
	src := parse(t, "math.ts")
	defer src.Close()
	sigs, _ := (TypeScript{}).ListSignatures(src)
	for _, s := range sigs {
		if s.Name != "multiply" {
			continue
		}
		want := "/**\n * multiply returns the product of two numbers.\n" +
			" * @param a the first factor\n * @param b the second factor\n" +
			" * @returns the product a * b\n */"
		if s.Doc != want {
			t.Errorf("multiply doc = %q, want %q", s.Doc, want)
		}
		return
	}
	t.Fatal("multiply not found")
}

// TestClassDisambiguation proves three methods named "area" in two different
// classes plus a free function named "area" are extracted distinctly.
func TestClassDisambiguation(t *testing.T) {
	src := parse(t, "shapes.ts")
	defer src.Close()
	ts := TypeScript{}

	circle, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	square, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Square"})
	if err != nil {
		t.Fatal(err)
	}
	free, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	if circle == square || circle == free || square == free {
		t.Fatalf("area bodies not distinct:\n Circle=%q\n Square=%q\n free=%q", circle, square, free)
	}
	if want := "{\n    return 3.14159 * this.r * this.r;\n  }"; circle != want {
		t.Errorf("Circle.area body = %q, want %q", circle, want)
	}
	if want := "{\n  return w * h;\n}"; free != want {
		t.Errorf("free area body = %q, want %q", free, want)
	}
}

// TestNamespaceDisambiguation proves a namespace-scoped function and a free
// function sharing the name "perimeter" are addressed distinctly via Container.
func TestNamespaceDisambiguation(t *testing.T) {
	src := parse(t, "namespace.ts")
	defer src.Close()
	ts := TypeScript{}

	scoped, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "perimeter", Container: "Geometry"})
	if err != nil {
		t.Fatal(err)
	}
	free, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "perimeter"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n    return 2 * (w + h);\n  }"; scoped != want {
		t.Errorf("Geometry.perimeter body = %q, want %q", scoped, want)
	}
	if want := "{\n  return 4 * side;\n}"; free != want {
		t.Errorf("free perimeter body = %q, want %q", free, want)
	}

	// A module-scoped arrow const is reachable the same way.
	shout, err := ts.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "shout", Container: "Strings"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "s.toUpperCase()"; shout != want {
		t.Errorf("Strings.shout body = %q, want %q", shout, want)
	}
}

// TestReadInterface extracts a known interface, doc included.
func TestReadInterface(t *testing.T) {
	src := parse(t, "shapes.ts")
	defer src.Close()
	def, err := (TypeScript{}).ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/**\n * Shape is anything with a measurable area.\n" +
		" * @returns the area when area() is called\n */\n" +
		"interface Shape {\n  /** area returns the shape's area. */\n" +
		"  area(): number;\n  name: string;\n}"
	if def != want {
		t.Errorf("Shape interface = %q, want %q", def, want)
	}
}

// TestReadInterfaceGeneric extracts a generic interface.
func TestReadInterfaceGeneric(t *testing.T) {
	src := parse(t, "generics.ts")
	defer src.Close()
	def, err := (TypeScript{}).ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Container"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(def, "interface Container<T> {") || !contains(def, "get(): T;") {
		t.Errorf("Container interface missing expected text:\n%s", def)
	}
}

// TestReadStructClass extracts a known class definition, doc included.
func TestReadStructClass(t *testing.T) {
	src := parse(t, "shapes.ts")
	defer src.Close()
	def, err := (TypeScript{}).ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Square"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/** Square is a four-sided shape. */\n" +
		"class Square implements Shape {\n  name: string;\n\n" +
		"  constructor(private side: number) {\n    this.name = \"square\";\n  }\n\n" +
		"  area(): number {\n    return this.side * this.side;\n  }\n}"
	if def != want {
		t.Errorf("Square class = %q, want %q", def, want)
	}
}

// TestReadStructTypeAlias extracts an object-typed `type` alias as a struct.
func TestReadStructTypeAlias(t *testing.T) {
	src := parse(t, "complex.ts")
	defer src.Close()
	def, err := (TypeScript{}).ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Point"})
	if err != nil {
		t.Fatal(err)
	}
	want := "/** Point is a 2D coordinate. */\ntype Point = { x: number; y: number };"
	if def != want {
		t.Errorf("Point type = %q, want %q", def, want)
	}
}

// TestTSXBody asserts a JSX-returning component body is returned faithfully, and
// that the .tsx file requires the TSX grammar (the pure grammar misparses JSX).
func TestTSXBody(t *testing.T) {
	src := parse(t, "widget.tsx")
	defer src.Close()
	body, err := (TSX{}).FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "Greeting"})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  return <div className=\"greeting\">Hello, {props.name}!</div>;\n}"
	if body != want {
		t.Errorf("Greeting body = %q, want %q", body, want)
	}

	method, err := (TSX{}).FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "render", Container: "Panel"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(method, "<section className=\"panel\">") {
		t.Errorf("Panel.render body missing JSX:\n%s", method)
	}

	// The pure TypeScript grammar cannot parse this JSX cleanly.
	b, _ := os.ReadFile(examples + "widget.tsx")
	pure, err := (TypeScript{}).Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if !pure.Root().HasError() {
		t.Error("expected the pure TypeScript grammar to misparse JSX, but it parsed cleanly")
	}
	pure.Close()
}

// TestBOMAndUnicode asserts a BOM-prefixed file and unicode identifiers resolve.
func TestBOMAndUnicode(t *testing.T) {
	bom := parse(t, "bom.ts")
	defer bom.Close()
	body, err := (TypeScript{}).FunctionBody(bom, core.SymbolID{Kind: core.KindFunc, Name: "withBom"})
	if err != nil {
		t.Fatalf("withBom: %v", err)
	}
	if want := "{\n  return 7;\n}"; body != want {
		t.Errorf("withBom body = %q, want %q", body, want)
	}

	uni := parse(t, "unicode.ts")
	defer uni.Close()
	caf, err := (TypeScript{}).FunctionBody(uni, core.SymbolID{Kind: core.KindFunc, Name: "café"})
	if err != nil {
		t.Fatalf("café: %v", err)
	}
	if want := "{\n  return \"café ☕\";\n}"; caf != want {
		t.Errorf("café body = %q, want %q", caf, want)
	}
}

// TestInterfaceMethodHasNoBody asserts a body-less interface method signature
// reports ErrSymbolNotFound from FunctionBody (it has no body to return).
func TestInterfaceMethodHasNoBody(t *testing.T) {
	src := parse(t, "shapes.ts")
	defer src.Close()
	_, err := (TypeScript{}).FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Shape"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("Shape.area body error = %v, want ErrSymbolNotFound", err)
	}
}

// TestSymbolNotFound asserts a missing symbol yields ErrSymbolNotFound.
func TestSymbolNotFound(t *testing.T) {
	src := parse(t, "math.ts")
	defer src.Close()
	_, err := (TypeScript{}).FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "nope"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("error = %v, want ErrSymbolNotFound", err)
	}
}

// TestRelativeRangeRejected asserts a relative-range edit is rejected, not panicked.
func TestRelativeRangeRejected(t *testing.T) {
	src := parse(t, "math.ts")
	defer src.Close()
	_, err := (TypeScript{}).ResolveEdits(src, []core.Edit{{
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
	path, before := tempCopy(t, "math.ts")
	res, err := core.BatchWrite(TypeScript{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "multiply"},
		NewText: "function multiply(a: number, b: number): number { return a *", // broken
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

// TestWriteRoundTrip applies two valid edits then restores both in one batch,
// asserting the file returns to its exact original bytes (reversibility).
func TestWriteRoundTrip(t *testing.T) {
	path, before := tempCopy(t, "math.ts")
	ts := TypeScript{}

	addOrig := "function add(a: number, b: number): number {\n  return a + b;\n}"
	subOrig := "function subtract(a: number, b: number): number {\n  return a - b;\n}"

	if _, err := core.BatchWrite(ts, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "add"},
		NewText: "function add(a: number, b: number): number {\n  return b + a;\n}",
	}}); err != nil {
		t.Fatalf("write #1: %v", err)
	}
	if hash(t, path) == before {
		t.Error("file unchanged after write #1")
	}
	if _, err := core.BatchWrite(ts, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "subtract"},
		NewText: "function subtract(a: number, b: number): number {\n  return -(b - a);\n}",
	}}); err != nil {
		t.Fatalf("write #2: %v", err)
	}
	if _, err := core.BatchWrite(ts, path, []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "add"}, NewText: addOrig},
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "subtract"}, NewText: subOrig},
	}); err != nil {
		t.Fatalf("write #3 (restore): %v", err)
	}
	if hash(t, path) != before {
		t.Error("file not restored to original bytes after round-trip")
	}
}

// TestAtomicBatch asserts a batch with one good and one broken edit is rejected
// wholesale and the file is left intact (all-or-nothing).
func TestAtomicBatch(t *testing.T) {
	path, before := tempCopy(t, "math.ts")
	res, err := core.BatchWrite(TypeScript{}, path, []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "subtract"}, NewText: "function subtract(a: number, b: number): number {\n  return a - b - 1;\n}"},
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "add"}, NewText: "function add(a: number, b: number): number { return a +"},
	})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Errorf("error = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("atomic batch reported Applied")
	}
	if h := hash(t, path); h != before {
		t.Error("file changed despite rejected atomic batch")
	}
}

// TestWriteIdempotent rewrites a symbol with its own exact text (incl. CRLF line
// endings and a no-trailing-newline file) and asserts the file hash is unchanged.
func TestWriteIdempotent(t *testing.T) {
	for _, name := range []string{"math.ts", "crlf.ts", "nonewline.ts"} {
		path, before := tempCopy(t, name)

		b, _ := os.ReadFile(path)
		src, _ := (TypeScript{}).Parse(b)
		var target core.SymbolID
		var same string
		for _, s := range collect(src.Root(), src.Bytes()) {
			target = s.id
			same = s.text.Utf8Text(src.Bytes())
			break // first symbol is enough
		}
		src.Close()
		if same == "" {
			t.Fatalf("%s: no symbol found", name)
		}

		res, err := core.BatchWrite(TypeScript{}, path, []core.Edit{{Target: target, NewText: same}})
		if err != nil {
			t.Fatalf("%s: idempotent write: %v", name, err)
		}
		if !res.Applied {
			t.Errorf("%s: idempotent write not applied", name)
		}
		if h := hash(t, path); h != before {
			t.Errorf("%s: file hash changed after writing identical text", name)
		}
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

// contains reports whether s contains sub.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
