package cpp

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

// exampleDir is the shared corpus of deterministic C++ sources. The tests parse
// them read-only; write tests operate on copies under t.TempDir().
const exampleDir = "../../../examples/cpp"

// parse loads and parses an example file, registering Close as cleanup.
func parse(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(exampleDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := Cpp{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// copyExample copies an example file into t.TempDir and returns the copy's path.
func copyExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(exampleDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("copy %s: %v", name, err)
	}
	return dst
}

func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return sha256.Sum256(b)
}

// TestParseExamples parses every example and asserts a clean tree (no error nodes).
func TestParseExamples(t *testing.T) {
	entries, err := os.ReadDir(exampleDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			src := parse(t, e.Name())
			if src.Root().HasError() {
				t.Errorf("%s: parse tree contains error nodes", e.Name())
			}
		})
	}
}

// TestListSignatures checks the symbol inventory (names, containers, counts) on a
// small file and on a realistic file with >= 20 symbols.
func TestListSignatures(t *testing.T) {
	t.Run("shapes", func(t *testing.T) {
		sigs, err := Cpp{}.ListSignatures(parse(t, "shapes.cpp"))
		if err != nil {
			t.Fatal(err)
		}
		if len(sigs) != 8 {
			t.Fatalf("got %d signatures, want 8", len(sigs))
		}
		names := map[string]int{}
		for _, s := range sigs {
			names[s.Name]++
		}
		if names["area"] != 3 { // free area + Circle::area + Rectangle::area
			t.Errorf("got %d 'area' symbols, want 3", names["area"])
		}
	})

	t.Run("big_has_at_least_20", func(t *testing.T) {
		sigs, err := Cpp{}.ListSignatures(parse(t, "big.cpp"))
		if err != nil {
			t.Fatal(err)
		}
		if len(sigs) < 20 {
			t.Fatalf("got %d signatures, want >= 20", len(sigs))
		}
	})
}

// TestFunctionAndBody extracts a specific function and its body and compares with
// exact expected strings.
func TestFunctionAndBody(t *testing.T) {
	src := parse(t, "big.cpp")
	id := core.SymbolID{Kind: core.KindFunc, Name: "add"}

	body, err := Cpp{}.FunctionBody(src, id)
	if err != nil {
		t.Fatal(err)
	}
	if body != "return a + b;" {
		t.Errorf("body = %q, want %q", body, "return a + b;")
	}

	fn, err := Cpp{}.Function(src, id)
	if err != nil {
		t.Fatal(err)
	}
	want := "// add returns a + b.\nint add(int a, int b) { return a + b; }"
	if fn != want {
		t.Errorf("function = %q, want %q", fn, want)
	}
}

// TestReadStruct extracts a known class definition.
func TestReadStruct(t *testing.T) {
	def, err := Cpp{}.ReadStruct(parse(t, "shapes.cpp"), core.SymbolID{Kind: core.KindStruct, Name: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(def, "class Circle {") {
		t.Errorf("struct def does not start with the class header:\n%s", def)
	}
	if !strings.Contains(def, "double area() const") {
		t.Errorf("struct def missing member function")
	}
}

// TestReadInterfaceUnsupported asserts C++ reports interfaces as unsupported.
func TestReadInterfaceUnsupported(t *testing.T) {
	_, err := Cpp{}.ReadInterface(parse(t, "shapes.cpp"), core.SymbolID{Kind: core.KindInterface, Name: "Circle"})
	if !errors.Is(err, ErrInterfacesUnsupported) {
		t.Fatalf("got %v, want ErrInterfacesUnsupported", err)
	}
}

// TestDisambiguation confirms that the free function area(double) and the member
// function Circle::area() are extracted independently and never confused.
func TestDisambiguation(t *testing.T) {
	src := parse(t, "shapes.cpp")

	free, err := Cpp{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "area"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(free, "PI * radius * radius") {
		t.Errorf("free area() resolved to the wrong symbol:\n%s", free)
	}

	method, err := Cpp{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Circle"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(method, "radius_ * radius_") {
		t.Errorf("Circle::area() resolved to the wrong symbol:\n%s", method)
	}
	if free == method {
		t.Error("free area and Circle::area resolved to identical text")
	}

	rect, err := Cpp{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Rectangle"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rect, "width * height") {
		t.Errorf("Rectangle::area() resolved to the wrong symbol:\n%s", rect)
	}
}

// TestOutOfLineMethodContainer checks the namespace::class container of an
// out-of-line member definition.
func TestOutOfLineMethodContainer(t *testing.T) {
	fn, err := Cpp{}.Function(parse(t, "math_utils.cpp"),
		core.SymbolID{Kind: core.KindMethod, Name: "add", Container: "mind::math::Accumulator"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fn, "sum_ += value;") {
		t.Errorf("out-of-line add() resolved to the wrong symbol:\n%s", fn)
	}
}

// TestDocMultiline checks that a multi-line doc comment is extracted exactly.
func TestDocMultiline(t *testing.T) {
	sigs, err := Cpp{}.ListSignatures(parse(t, "math_utils.cpp"))
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, s := range sigs {
		if s.Name == "clamp" {
			got = s.Doc
		}
	}
	want := "// clamp restricts v to the inclusive range [lo, hi].\n" +
		"// This is a multi-line doc comment spanning\n" +
		"// three source lines on purpose, so doc extraction\n" +
		"// can be asserted exactly."
	if got != want {
		t.Errorf("clamp doc =\n%q\nwant\n%q", got, want)
	}
}

// TestWriteParseRejection feeds a syntactically broken replacement and asserts the
// batch is rejected and the file on disk is untouched.
func TestWriteParseRejection(t *testing.T) {
	path := copyExample(t, "shapes.cpp")
	before := hashFile(t, path)

	broken := core.Edit{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "area"},
		NewText: "double area(double radius { return 0; }", // missing ')'
	}
	res, err := core.BatchWrite(Cpp{}, path, []core.Edit{broken})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("got err %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Error("Applied = true on a rejected write")
	}
	if hashFile(t, path) != before {
		t.Error("file changed on disk after a rejected write")
	}
}

// TestWriteIdempotent writes a symbol's own text back to itself and asserts the
// file content is byte-identical (hash unchanged).
func TestWriteIdempotent(t *testing.T) {
	path := copyExample(t, "shapes.cpp")
	before := hashFile(t, path)

	src := parse(t, "shapes.cpp")
	id := core.SymbolID{Kind: core.KindFunc, Name: "area"}
	orig, err := Cpp{}.Function(src, id)
	if err != nil {
		t.Fatal(err)
	}

	res, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: orig}})
	if err != nil {
		t.Fatalf("idempotent write failed: %v", err)
	}
	if !res.Applied {
		t.Fatal("idempotent write not applied")
	}
	if hashFile(t, path) != before {
		t.Error("idempotent write changed the file hash")
	}
}

// TestWriteRoundTrip modifies a symbol, then restores its original text, and
// asserts the file returns to its original hash.
func TestWriteRoundTrip(t *testing.T) {
	path := copyExample(t, "shapes.cpp")
	before := hashFile(t, path)

	src := parse(t, "shapes.cpp")
	id := core.SymbolID{Kind: core.KindFunc, Name: "area"}
	orig, err := Cpp{}.Function(src, id)
	if err != nil {
		t.Fatal(err)
	}

	modified := "// area (modified)\ndouble area(double radius) {\n    return 2.0 * PI * radius * radius;\n}"
	if _, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: modified}}); err != nil {
		t.Fatalf("modify write failed: %v", err)
	}
	if hashFile(t, path) == before {
		t.Fatal("file unchanged after a modifying write")
	}

	if _, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: orig}}); err != nil {
		t.Fatalf("restore write failed: %v", err)
	}
	if hashFile(t, path) != before {
		t.Error("round-trip did not restore the original file")
	}
}

// TestWriteRejectsRelativeRange asserts the unimplemented RelativeRange path
// returns a typed error rather than panicking.
func TestWriteRejectsRelativeRange(t *testing.T) {
	src := parse(t, "shapes.cpp")
	_, err := Cpp{}.ResolveEdits(src, []core.Edit{{
		Target: core.SymbolID{Kind: core.KindFunc, Name: "area"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}})
	if !errors.Is(err, ErrRelativeRangeNotImplemented) {
		t.Fatalf("got %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// TestPreserveLineEndings modifies the first function of a CRLF file and asserts
// the CRLF endings elsewhere in the file are preserved.
func TestPreserveLineEndings(t *testing.T) {
	path := copyExample(t, "crlf.cpp")

	id := core.SymbolID{Kind: core.KindFunc, Name: "crlfFn"}
	modified := "int crlfFn(int a) { return a * 3; }"
	if _, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: modified}}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("int other(int b) { return b - 1; }\r\n")) {
		t.Error("CRLF line ending outside the edit was not preserved")
	}
}

// TestPreserveNoTrailingNewline modifies the only function in a file that has no
// trailing newline and asserts the file still ends without one.
func TestPreserveNoTrailingNewline(t *testing.T) {
	path := copyExample(t, "no_newline.cpp")

	id := core.SymbolID{Kind: core.KindFunc, Name: "tailFn"}
	modified := "int tailFn(int a) { return a - 7; }" // no trailing newline in NewText
	if _, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: modified}}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	b, _ := os.ReadFile(path)
	if len(b) == 0 || b[len(b)-1] == '\n' {
		t.Error("a trailing newline was added to a file that had none")
	}
}

// TestPreserveBOM modifies a function in a UTF-8 BOM file and asserts the BOM is
// preserved at the start of the file.
func TestPreserveBOM(t *testing.T) {
	path := copyExample(t, "bom.cpp")

	id := core.SymbolID{Kind: core.KindFunc, Name: "bomFn"}
	modified := "int bomFn(int a) { return a + 9; }"
	if _, err := core.BatchWrite(Cpp{}, path, []core.Edit{{Target: id, NewText: modified}}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	b, _ := os.ReadFile(path)
	if !bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("UTF-8 BOM was not preserved")
	}
}

// TestSymbolNotFound asserts a clean typed error for a missing symbol.
func TestSymbolNotFound(t *testing.T) {
	_, err := Cpp{}.Function(parse(t, "shapes.cpp"), core.SymbolID{Kind: core.KindFunc, Name: "doesNotExist"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("got %v, want ErrSymbolNotFound", err)
	}
}
