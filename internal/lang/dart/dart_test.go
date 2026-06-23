package dart

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

const examplesDir = "../../../examples/dart"

// parseFile reads and parses an example file, registering Close on cleanup.
func parseFile(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := Dart{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// TestParseExamples parses every example file and asserts a clean tree, covering
// the UTF-8 BOM, CRLF, and no-trailing-newline edge files.
func TestParseExamples(t *testing.T) {
	for _, name := range []string{
		"shapes.dart", "math_rich.dart",
		"edge_bom.dart", "edge_crlf.dart", "edge_nonewline.dart",
	} {
		src := parseFile(t, name)
		if src.Root().HasError() {
			t.Errorf("%s: parse tree has errors", name)
		}
	}
}

func TestListSignatures(t *testing.T) {
	src := parseFile(t, "shapes.dart")
	sigs, err := Dart{}.ListSignatures(src)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	wantNames := []string{"describe", "Shape", "Rectangle", "doubleAll"}
	if len(sigs) != len(wantNames) {
		t.Fatalf("got %d signatures, want %d: %+v", len(sigs), len(wantNames), sigs)
	}
	for i, w := range wantNames {
		if sigs[i].Name != w {
			t.Errorf("signature[%d].Name = %q, want %q", i, sigs[i].Name, w)
		}
	}
	// The nested local function `twice` inside doubleAll must NOT be listed.
	for _, s := range sigs {
		if s.Name == "twice" {
			t.Errorf("nested local function 'twice' leaked into top-level signatures")
		}
	}

	// Rich file: a realistic, symbol-dense file (>= 20 top-level symbols).
	rich := parseFile(t, "math_rich.dart")
	rsigs, err := Dart{}.ListSignatures(rich)
	if err != nil {
		t.Fatalf("ListSignatures rich: %v", err)
	}
	if len(rsigs) < 20 {
		t.Errorf("rich file: got %d signatures, want >= 20", len(rsigs))
	}
	// Spot-check a generic return type is captured.
	if got := findSig(rsigs, "singleton").Returns; got != "List<T>" {
		t.Errorf("singleton Returns = %q, want %q", got, "List<T>")
	}
	if got := findSig(rsigs, "add").Params; len(got) != 2 || got[0] != "int a" || got[1] != "int b" {
		t.Errorf("add Params = %v, want [int a int b]", got)
	}
}

func findSig(sigs []core.Signature, name string) core.Signature {
	for _, s := range sigs {
		if s.Name == name {
			return s
		}
	}
	return core.Signature{}
}

// TestFunctionDisambiguation proves two symbols with the same name but different
// containers resolve independently.
func TestFunctionDisambiguation(t *testing.T) {
	src := parseFile(t, "shapes.dart")

	topLevel := `/// Returns a generic, module-level description.
/// Shares its name with Shape.describe and Rectangle.describe so the
/// SymbolID.Container is what disambiguates them.
String describe() {
  return "a shape";
}`
	method := `/// Describes this rectangle.
  @override
  String describe() {
    return "rectangle";
  }`

	gotTop, err := Dart{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "describe"})
	if err != nil {
		t.Fatalf("Function top-level describe: %v", err)
	}
	if gotTop != topLevel {
		t.Errorf("top-level describe:\n got %q\nwant %q", gotTop, topLevel)
	}

	gotMethod, err := Dart{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "describe", Container: "Rectangle"})
	if err != nil {
		t.Fatalf("Function Rectangle.describe: %v", err)
	}
	if gotMethod != method {
		t.Errorf("Rectangle.describe:\n got %q\nwant %q", gotMethod, method)
	}

	if gotTop == gotMethod {
		t.Errorf("disambiguation failed: top-level and method describe returned identical text")
	}
}

func TestFunctionBody(t *testing.T) {
	src := parseFile(t, "shapes.dart")
	want := `{
    return width * height;
  }`
	got, err := Dart{}.FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Rectangle"})
	if err != nil {
		t.Fatalf("FunctionBody: %v", err)
	}
	if got != want {
		t.Errorf("Rectangle.area body:\n got %q\nwant %q", got, want)
	}

	// A bodyless (abstract) method has no body.
	if _, err := (Dart{}).FunctionBody(src, core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Shape"}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("abstract Shape.area body: err = %v, want ErrSymbolNotFound", err)
	}
}

func TestDocExtractionMultiline(t *testing.T) {
	src := parseFile(t, "shapes.dart")
	sigs, err := Dart{}.ListSignatures(src)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	want := "Returns a generic, module-level description.\n" +
		"Shares its name with Shape.describe and Rectangle.describe so the\n" +
		"SymbolID.Container is what disambiguates them."
	got := findSig(sigs, "describe").Doc
	if got != want {
		t.Errorf("describe Doc:\n got %q\nwant %q", got, want)
	}
}

func TestReadInterfaceAndStruct(t *testing.T) {
	src := parseFile(t, "shapes.dart")

	iface, err := Dart{}.ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if err != nil {
		t.Fatalf("ReadInterface Shape: %v", err)
	}
	wantIface := `/// The base shape contract.
///
/// Maps to ReadInterface because it is an abstract class.
abstract class Shape {
  /// Computes the area of the shape.
  double area();

  /// Describes the shape in words.
  String describe();
}`
	if iface != wantIface {
		t.Errorf("ReadInterface Shape:\n got %q\nwant %q", iface, wantIface)
	}

	st, err := Dart{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Rectangle"})
	if err != nil {
		t.Fatalf("ReadStruct Rectangle: %v", err)
	}
	if want := "/// A rectangle"; len(st) < len(want) || st[:len(want)] != want {
		t.Errorf("ReadStruct Rectangle should start with doc, got prefix %q", st[:len(want)])
	}

	// Mapping is strict: an abstract class is not a struct, and vice versa.
	if _, err := (Dart{}).ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Shape"}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("ReadStruct on abstract Shape: err = %v, want ErrSymbolNotFound", err)
	}
	if _, err := (Dart{}).ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Rectangle"}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("ReadInterface on concrete Rectangle: err = %v, want ErrSymbolNotFound", err)
	}
}

func TestRelativeRangeRejected(t *testing.T) {
	src := parseFile(t, "shapes.dart")
	edits := []core.Edit{{
		Target: core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Rectangle"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}}
	if _, err := (Dart{}).ResolveEdits(src, edits); !errors.Is(err, ErrRelativeRangeNotImplemented) {
		t.Errorf("ResolveEdits with Rel: err = %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// --- write pipeline (via core.BatchWrite) -----------------------------------

func copyExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
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

func readFunction(t *testing.T, path string, id core.SymbolID) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	src, err := Dart{}.Parse(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer src.Close()
	fn, err := Dart{}.Function(src, id)
	if err != nil {
		t.Fatalf("Function %v: %v", id, err)
	}
	return fn
}

var (
	areaID     = core.SymbolID{Kind: core.KindMethod, Name: "area", Container: "Rectangle"}
	describeID = core.SymbolID{Kind: core.KindMethod, Name: "describe", Container: "Rectangle"}
)

func TestWriteIdempotent(t *testing.T) {
	path := copyExample(t, "shapes.dart")
	before := hashFile(t, path)

	orig := readFunction(t, path, areaID)
	res, err := core.BatchWrite(Dart{}, path, []core.Edit{{Target: areaID, NewText: orig}})
	if err != nil {
		t.Fatalf("BatchWrite (no-op): %v", err)
	}
	if !res.Applied {
		t.Fatalf("BatchWrite (no-op) not applied: %+v", res)
	}
	if hashFile(t, path) != before {
		t.Errorf("writing a symbol back to itself changed the file hash")
	}
}

func TestWriteReversible(t *testing.T) {
	path := copyExample(t, "shapes.dart")
	orig := hashFile(t, path)

	origArea := readFunction(t, path, areaID)
	origDesc := readFunction(t, path, describeID)

	// Write #1: modify area (still valid Dart, doc preserved).
	newArea := replaceOnce(origArea, "width * height", "height * width")
	mustApply(t, path, []core.Edit{{Target: areaID, NewText: newArea}})
	if hashFile(t, path) == orig {
		t.Fatalf("write #1 did not change the file")
	}

	// Write #2: modify describe.
	newDesc := replaceOnce(origDesc, `"rectangle"`, `"rect"`)
	mustApply(t, path, []core.Edit{{Target: describeID, NewText: newDesc}})

	// Write #3: restore both in a single batch.
	mustApply(t, path, []core.Edit{
		{Target: areaID, NewText: origArea},
		{Target: describeID, NewText: origDesc},
	})

	if hashFile(t, path) != orig {
		t.Errorf("after restore, file hash differs from original")
	}
}

func TestParseRejectionSingle(t *testing.T) {
	path := copyExample(t, "shapes.dart")
	before := hashFile(t, path)

	_, err := core.BatchWrite(Dart{}, path, []core.Edit{
		{Target: areaID, NewText: "@@@ this is not valid dart @@@"},
	})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("broken write: err = %v, want ErrSyntaxBroken", err)
	}
	if hashFile(t, path) != before {
		t.Errorf("file changed despite rejected write")
	}
}

func TestBatchAtomicity(t *testing.T) {
	path := copyExample(t, "shapes.dart")
	before := hashFile(t, path)

	origArea := readFunction(t, path, areaID)
	res, err := core.BatchWrite(Dart{}, path, []core.Edit{
		{Target: areaID, NewText: replaceOnce(origArea, "width * height", "height * width")}, // valid
		{Target: describeID, NewText: "@@@ broken @@@"},                                      // invalid
	})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("mixed batch: err = %v, want ErrSyntaxBroken", err)
	}
	if res.Applied {
		t.Errorf("mixed batch reported Applied=true")
	}
	if hashFile(t, path) != before {
		t.Errorf("file changed despite all-or-nothing rejection")
	}
	// The result must identify the offending edit (describe), not the valid one.
	var describeReason, areaReason string
	for _, o := range res.Edits {
		switch o.Target {
		case describeID:
			describeReason = o.Reason
		case areaID:
			areaReason = o.Reason
		}
	}
	if describeReason == "" {
		t.Errorf("WriteResult did not flag the broken 'describe' edit: %+v", res.Edits)
	}
	if areaReason != "" {
		t.Errorf("WriteResult wrongly flagged the valid 'area' edit: %q", areaReason)
	}
}

func TestWriteSymbolNotFound(t *testing.T) {
	path := copyExample(t, "shapes.dart")
	_, err := core.BatchWrite(Dart{}, path, []core.Edit{
		{Target: core.SymbolID{Kind: core.KindMethod, Name: "nope", Container: "Rectangle"}, NewText: "void nope() {}"},
	})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("missing symbol: err = %v, want ErrSymbolNotFound", err)
	}
}

// --- small helpers ----------------------------------------------------------

func mustApply(t *testing.T, path string, edits []core.Edit) {
	t.Helper()
	res, err := core.BatchWrite(Dart{}, path, edits)
	if err != nil {
		t.Fatalf("BatchWrite: %v (%+v)", err, res)
	}
	if !res.Applied {
		t.Fatalf("BatchWrite not applied: %+v", res)
	}
}

func replaceOnce(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
