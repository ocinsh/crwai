package python

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

// examplesDir is the deterministic Python corpus, relative to this package dir.
const examplesDir = "../../../examples/python"

// load parses an example file into a Source, registering Close for cleanup.
func load(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := Python{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// TestParsesEveryExample confirms each corpus file parses without error nodes.
func TestParsesEveryExample(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".py" {
			continue
		}
		src := load(t, e.Name())
		if src.Root().HasError() {
			t.Errorf("%s: parse tree has errors", e.Name())
		}
	}
}

// TestListSignaturesCounts checks the expected number of top-level + method symbols.
func TestListSignaturesCounts(t *testing.T) {
	cases := map[string]int{
		"shapes.py":  10, // 3 funcs + 2 classes + 5 methods
		"unicode.py": 4,  // 2 funcs + 1 class + 1 method
		"complex.py": 21, // 18 funcs + 1 class + 2 methods
	}
	for file, want := range cases {
		src := load(t, file)
		sigs, err := Python{}.ListSignatures(src)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if len(sigs) != want {
			names := make([]string, len(sigs))
			for i, s := range sigs {
				names[i] = s.Name
			}
			t.Errorf("%s: got %d signatures %v, want %d", file, len(sigs), names, want)
		}
	}
}

// TestSignatureFields checks params, returns, and single-line docstrings.
func TestSignatureFields(t *testing.T) {
	src := load(t, "shapes.py")
	sigs, err := Python{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]core.Signature{}
	for _, s := range sigs {
		// NOTE: area appears as a func and a method; keep the first (the func).
		if _, ok := byName[s.Name]; !ok {
			byName[s.Name] = s
		}
	}
	area := byName["area"]
	if len(area.Params) != 2 || area.Params[0] != "width" || area.Params[1] != "height" {
		t.Errorf("area params = %v, want [width height]", area.Params)
	}
	if area.Doc != "Return the area of a rectangle." {
		t.Errorf("area doc = %q", area.Doc)
	}
	describe := byName["describe"]
	if describe.Doc != "" {
		t.Errorf("describe should have no doc, got %q", describe.Doc)
	}
}

// TestMultiLineDocstring checks exact extraction of a multi-line docstring.
func TestMultiLineDocstring(t *testing.T) {
	src := load(t, "shapes.py")
	sigs, err := Python{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Return the rectangle perimeter.\n\n    The perimeter is twice the sum of the two sides.\n    "
	var got string
	for _, s := range sigs {
		if s.Name == "perimeter" {
			got = s.Doc
		}
	}
	if got != want {
		t.Errorf("perimeter doc mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestFunctionAndBody checks full-function and body-only extraction.
func TestFunctionAndBody(t *testing.T) {
	src := load(t, "shapes.py")
	fn, err := Python{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	const wantFn = "def area(width, height):\n    \"\"\"Return the area of a rectangle.\"\"\"\n    return width * height"
	if fn != wantFn {
		t.Errorf("Function(area):\n got %q\nwant %q", fn, wantFn)
	}
	body, err := Python{}.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	const wantBody = "\"\"\"Return the area of a rectangle.\"\"\"\n    return width * height"
	if body != wantBody {
		t.Errorf("FunctionBody(area):\n got %q\nwant %q", body, wantBody)
	}
}

// TestDisambiguation checks that the three `area` symbols (a top-level func and two
// methods) are resolved independently by Container.
func TestDisambiguation(t *testing.T) {
	src := load(t, "shapes.py")
	topFn, err := Python{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	rectArea, err := Python{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Rectangle"})
	if err != nil {
		t.Fatal(err)
	}
	circArea, err := Python{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	if topFn == rectArea || topFn == circArea || rectArea == circArea {
		t.Errorf("three area symbols should differ:\n top=%q\n rect=%q\n circ=%q", topFn, rectArea, circArea)
	}
	const wantRect = "def area(self):\n        \"\"\"Return the rectangle area.\"\"\"\n        return self.width * self.height"
	if rectArea != wantRect {
		t.Errorf("Rectangle.area:\n got %q\nwant %q", rectArea, wantRect)
	}
}

// TestReadStruct checks class extraction and that interfaces are unsupported.
func TestReadStruct(t *testing.T) {
	src := load(t, "shapes.py")
	def, err := Python{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	if len(def) == 0 || def[:len("class Circle:")] != "class Circle:" {
		t.Errorf("ReadStruct(Circle) unexpected: %q", def)
	}
	py := Python{}
	if _, err := py.ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Circle"}); err == nil {
		t.Error("ReadInterface should fail: Python has no interfaces")
	}
}

// TestSymbolNotFound checks the typed error for an unknown symbol.
func TestSymbolNotFound(t *testing.T) {
	src := load(t, "shapes.py")
	_, err := Python{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "nope"})
	if err == nil {
		t.Fatal("want error for unknown symbol")
	}
}

// hashFile returns the SHA-256 of a file's contents.
func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return sha256.Sum256(b)
}

// copyExample copies a corpus file into a temp dir and returns its path.
func copyExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
	return dst
}

// TestWriteParseRejection: a write whose new body is broken syntax is rejected and
// leaves the file byte-for-byte intact.
func TestWriteParseRejection(t *testing.T) {
	path := copyExample(t, "shapes.py")
	before := hashFile(t, path)
	edit := core.Edit{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "area"},
		NewText: "def area(width, height):\n    return (",
	}
	res, err := core.BatchWrite(Python{}, path, []core.Edit{edit})
	if err == nil {
		t.Fatal("want error for broken syntax")
	}
	if res.Applied {
		t.Error("Applied should be false on rejection")
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite rejected write")
	}
}

// TestWriteAllOrNothing: a batch with one valid and one broken edit is rejected
// whole; the file is untouched.
func TestWriteAllOrNothing(t *testing.T) {
	path := copyExample(t, "shapes.py")
	before := hashFile(t, path)
	edits := []core.Edit{
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "describe"}, NewText: "def describe(name):\n    return name"},
		{Target: core.SymbolID{Kind: core.KindFunc, Name: "area"}, NewText: "def area(width, height):\n    return ("},
	}
	res, err := core.BatchWrite(Python{}, path, edits)
	if err == nil {
		t.Fatal("want error for broken batch")
	}
	if res.Applied {
		t.Error("Applied should be false")
	}
	if hashFile(t, path) != before {
		t.Error("file changed despite rejected batch")
	}
}

// TestWriteIdempotent: rewriting a symbol with its own current text leaves the file
// hash unchanged.
func TestWriteIdempotent(t *testing.T) {
	path := copyExample(t, "shapes.py")
	before := hashFile(t, path)

	src := load(t, "shapes.py")
	original, err := Python{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	edit := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "area"}, NewText: original}
	res, err := core.BatchWrite(Python{}, path, []core.Edit{edit})
	if err != nil {
		t.Fatalf("idempotent write failed: %v", err)
	}
	if !res.Applied {
		t.Error("Applied should be true")
	}
	if hashFile(t, path) != before {
		t.Error("idempotent write changed the file hash")
	}
}

// TestWriteRoundTrip: change a body, confirm the file changed and still parses, then
// restore it and confirm the original hash is recovered.
func TestWriteRoundTrip(t *testing.T) {
	path := copyExample(t, "shapes.py")
	before := hashFile(t, path)

	src := load(t, "shapes.py")
	original, err := Python{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "describe"})
	if err != nil {
		t.Fatal(err)
	}

	change := core.Edit{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "describe"},
		NewText: "def describe(name):\n    return \"name: \" + name",
	}
	if _, err := core.BatchWrite(Python{}, path, []core.Edit{change}); err != nil {
		t.Fatalf("change write failed: %v", err)
	}
	if hashFile(t, path) == before {
		t.Fatal("file should have changed")
	}

	restore := core.Edit{Target: core.SymbolID{Kind: core.KindFunc, Name: "describe"}, NewText: original}
	if _, err := core.BatchWrite(Python{}, path, []core.Edit{restore}); err != nil {
		t.Fatalf("restore write failed: %v", err)
	}
	if hashFile(t, path) != before {
		t.Error("restore did not recover the original file")
	}
}
