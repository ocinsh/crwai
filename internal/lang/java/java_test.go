package java

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

// examplesDir is the shared, deterministic Java corpus, relative to this package.
const examplesDir = "../../../examples/java"

// parseFile reads and parses an example file, returning the open Source. The
// caller must Close it.
func parseFile(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := Java{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return src
}

// TestParseAllExamples parses every file in the corpus and asserts the tree has
// no syntax errors — covering the BOM, CRLF, no-trailing-newline, and unicode
// edge cases, which must all parse cleanly.
func TestParseAllExamples(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var seen int
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".java" {
			continue
		}
		seen++
		src := parseFile(t, e.Name())
		if src.Root().HasError() {
			t.Errorf("%s: parse tree reports a syntax error", e.Name())
		}
		src.Close()
	}
	if seen == 0 {
		t.Fatal("no .java examples found")
	}
}

// TestListSignaturesGreeter checks the names and count returned for Greeter.java:
// the class itself, its two-arg-less identity, the single constructor, and three
// methods.
func TestListSignaturesGreeter(t *testing.T) {
	src := parseFile(t, "Greeter.java")
	defer src.Close()

	sigs, err := Java{}.ListSignatures(src)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	var names []string
	for _, s := range sigs {
		names = append(names, s.Name)
	}
	want := []string{"Greeter", "Greeter", "greet", "identity", "farewell"}
	if len(names) != len(want) {
		t.Fatalf("got %d signatures %v, want %d %v", len(names), names, len(want), want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("signature %d = %q, want %q", i, names[i], want[i])
		}
	}
}

// TestListSignaturesCatalogCount asserts the large file exposes at least 20
// addressable symbols, the realistic-file requirement.
func TestListSignaturesCatalogCount(t *testing.T) {
	src := parseFile(t, "Catalog.java")
	defer src.Close()

	sigs, err := Java{}.ListSignatures(src)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 20 {
		t.Errorf("Catalog.java exposes %d symbols, want >= 20", len(sigs))
	}
}

// TestFunctionAndBody checks exact extraction of a known method, both the whole
// function (doc + signature + body) and the body alone.
func TestFunctionAndBody(t *testing.T) {
	src := parseFile(t, "Greeter.java")
	defer src.Close()

	id := core.SymbolID{Kind: core.KindMethod, Name: "greet", Container: "Greeter"}

	wantBody := "{\n        return name + \" greets \" + who;\n    }"
	body, err := Java{}.FunctionBody(src, id)
	if err != nil {
		t.Fatalf("FunctionBody: %v", err)
	}
	if body != wantBody {
		t.Errorf("body =\n%q\nwant\n%q", body, wantBody)
	}

	wantFn := "// greet returns a salutation; this line comment is the doc.\n" +
		"public String greet(String who) {\n" +
		"        return name + \" greets \" + who;\n" +
		"    }"
	fn, err := Java{}.Function(src, id)
	if err != nil {
		t.Fatalf("Function: %v", err)
	}
	if fn != wantFn {
		t.Errorf("function =\n%q\nwant\n%q", fn, wantFn)
	}
}

// TestReadInterfaceAndStruct checks interface, record, and enum extraction.
func TestReadInterfaceAndStruct(t *testing.T) {
	shapes := parseFile(t, "Shapes.java")
	defer shapes.Close()

	iface, err := Java{}.ReadInterface(shapes, core.SymbolID{Kind: core.KindInterface, Name: "Shape"})
	if err != nil {
		t.Fatalf("ReadInterface: %v", err)
	}
	if !contains(iface, "interface Shape") || !contains(iface, "double area();") {
		t.Errorf("interface Shape extraction unexpected:\n%s", iface)
	}

	point, err := Java{}.ReadStruct(shapes, core.SymbolID{Kind: core.KindStruct, Name: "Point"})
	if err != nil {
		t.Fatalf("ReadStruct(record): %v", err)
	}
	if !contains(point, "record Point(int x, int y)") {
		t.Errorf("record Point extraction unexpected:\n%s", point)
	}

	catalog := parseFile(t, "Catalog.java")
	defer catalog.Close()
	enum, err := Java{}.ReadStruct(catalog, core.SymbolID{Kind: core.KindStruct, Name: "Category"})
	if err != nil {
		t.Fatalf("ReadStruct(enum): %v", err)
	}
	if !contains(enum, "enum Category") || !contains(enum, "perishable()") {
		t.Errorf("enum Category extraction unexpected:\n%s", enum)
	}

	// A missing symbol must report ErrSymbolNotFound, not a panic or empty success.
	_, missErr := Java{}.ReadStruct(catalog, core.SymbolID{Kind: core.KindStruct, Name: "Nope"})
	if missErr != core.ErrSymbolNotFound {
		t.Errorf("missing struct error = %v, want ErrSymbolNotFound", missErr)
	}
}

// TestDisambiguation verifies two methods named "greet" in different containers
// are not confused: Greeter.greet takes one parameter, Town.greet takes none.
func TestDisambiguation(t *testing.T) {
	src := parseFile(t, "Shapes.java")
	defer src.Close()

	town, err := Java{}.Function(src, core.SymbolID{Kind: core.KindMethod, Name: "greet", Container: "Town"})
	if err != nil {
		t.Fatalf("Function(Town.greet): %v", err)
	}
	if !contains(town, `public String greet() {`) || !contains(town, "welcome to town") {
		t.Errorf("Town.greet unexpected:\n%s", town)
	}

	greeter := parseFile(t, "Greeter.java")
	defer greeter.Close()
	g, err := Java{}.Function(greeter, core.SymbolID{Kind: core.KindMethod, Name: "greet", Container: "Greeter"})
	if err != nil {
		t.Fatalf("Function(Greeter.greet): %v", err)
	}
	if !contains(g, "greet(String who)") {
		t.Errorf("Greeter.greet unexpected:\n%s", g)
	}
}

// TestDocMultiline asserts the multi-line Javadoc above identity is extracted
// exactly (delimiters and interior whitespace preserved).
func TestDocMultiline(t *testing.T) {
	src := parseFile(t, "Greeter.java")
	defer src.Close()

	sigs, err := Java{}.ListSignatures(src)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	wantDoc := "/**\n" +
		"     * identity returns its argument unchanged.\n" +
		"     * It is generic to exercise type parameters.\n" +
		"     */"
	var got string
	for _, s := range sigs {
		if s.Name == "identity" {
			got = s.Doc
		}
	}
	if got != wantDoc {
		t.Errorf("identity doc =\n%q\nwant\n%q", got, wantDoc)
	}
}

// TestParseRejection feeds a syntactically broken body through the all-or-nothing
// pipeline and asserts the batch is rejected (ErrSyntaxBroken) and the file on
// disk is left byte-for-byte intact.
func TestParseRejection(t *testing.T) {
	path := copyToTemp(t, "Greeter.java")
	before := hashFile(t, path)

	edit := core.Edit{
		Target:  core.SymbolID{Kind: core.KindMethod, Name: "greet", Container: "Greeter"},
		NewText: "public String greet(String who) { return @@@ not valid",
	}
	res, err := core.BatchWrite(Java{}, path, []core.Edit{edit})
	if err == nil {
		t.Fatalf("expected an error, got nil (res.Applied=%v)", res.Applied)
	}
	if res.Applied {
		t.Errorf("res.Applied = true, want false on rejected batch")
	}
	if after := hashFile(t, path); after != before {
		t.Errorf("file changed on rejected write: %s != %s", after, before)
	}
}

// TestWriteIdempotent writes a symbol's original text back to itself and asserts
// the file content (hash) is unchanged.
func TestWriteIdempotent(t *testing.T) {
	path := copyToTemp(t, "Greeter.java")
	before := hashFile(t, path)

	original := "public String greet(String who) {\n" +
		"        return name + \" greets \" + who;\n" +
		"    }"
	edit := core.Edit{
		Target:  core.SymbolID{Kind: core.KindMethod, Name: "greet", Container: "Greeter"},
		NewText: original,
	}
	res, err := core.BatchWrite(Java{}, path, []core.Edit{edit})
	if err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	if !res.Applied {
		t.Fatalf("res.Applied = false, want true")
	}
	if after := hashFile(t, path); after != before {
		t.Errorf("rewriting the identical body changed the file: %s != %s", after, before)
	}
}

// --- test helpers ---

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func copyToTemp(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
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
