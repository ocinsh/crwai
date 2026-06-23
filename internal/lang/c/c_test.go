package c

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

const examplesDir = "../../../examples/c"

// load parses an example file into a Source. The caller must Close it.
func load(t *testing.T, name string) core.Source {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	src, err := C{}.Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return src
}

// TestParseExamples parses every example file and requires a clean parse tree.
func TestParseExamples(t *testing.T) {
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	var seen int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".c" {
			continue
		}
		seen++
		src := load(t, e.Name())
		if src.Root().HasError() {
			t.Errorf("%s: parse tree reports a syntax error", e.Name())
		}
		src.Close()
	}
	if seen == 0 {
		t.Fatal("no .c example files found")
	}
}

func TestListSignaturesSimple(t *testing.T) {
	src := load(t, "simple.c")
	defer src.Close()
	sigs, err := C{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(sigs))
	for i, s := range sigs {
		got[i] = s.Name
	}
	want := []string{"add", "sub", "mul"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", got, want)
	}
	add := sigs[0]
	if add.Returns != "int" {
		t.Errorf("add.Returns = %q, want int", add.Returns)
	}
	if strings.Join(add.Params, "|") != "int a|int b" {
		t.Errorf("add.Params = %v, want [int a int b]", add.Params)
	}
	if add.Doc != "// add returns the sum of a and b." {
		t.Errorf("add.Doc = %q", add.Doc)
	}
	if sigs[2].Doc != "" {
		t.Errorf("mul.Doc = %q, want empty (separated by a blank line)", sigs[2].Doc)
	}
}

func TestListSignaturesComplex(t *testing.T) {
	src := load(t, "complex.c")
	defer src.Close()
	sigs, err := C{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) < 20 {
		t.Fatalf("complex.c has %d symbols, want >= 20", len(sigs))
	}
	names := map[string]bool{}
	for _, s := range sigs {
		names[s.Name] = true
	}
	for _, n := range []string{"Vec2", "Stats", "imax", "vec2_dot", "stats_mean", "fib", "sign"} {
		if !names[n] {
			t.Errorf("missing expected symbol %q", n)
		}
	}
}

func TestFunctionAndBody(t *testing.T) {
	src := load(t, "simple.c")
	defer src.Close()

	body, err := C{}.FunctionBody(src, core.SymbolID{Kind: core.KindFunc, Name: "add"})
	if err != nil {
		t.Fatal(err)
	}
	if body != "{\n    return a + b;\n}" {
		t.Errorf("add body = %q", body)
	}

	full, err := C{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "add"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(full, "// add returns the sum of a and b.\n") {
		t.Errorf("Function(add) missing doc prefix: %q", full)
	}
	if !strings.Contains(full, "int add(int a, int b)") {
		t.Errorf("Function(add) missing signature: %q", full)
	}
}

func TestFunctionNotFound(t *testing.T) {
	src := load(t, "simple.c")
	defer src.Close()
	_, err := C{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "nope"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("err = %v, want ErrSymbolNotFound", err)
	}
}

func TestDocMultiline(t *testing.T) {
	src := load(t, "simple.c")
	defer src.Close()
	full, err := C{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	wantDoc := "/*\n * sub returns the difference of a and b.\n" +
		" * It uses a multi-line block comment so the\n" +
		" * doc-extraction has more than one line to join.\n */"
	if !strings.HasPrefix(full, wantDoc+"\n") {
		t.Errorf("multi-line doc not extracted exactly.\n got: %q", full)
	}
}

func TestReadStruct(t *testing.T) {
	src := load(t, "shapes.c")
	defer src.Close()

	point, err := C{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Point"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(point, "struct Point {") || !strings.Contains(point, "int x;") {
		t.Errorf("Point struct = %q", point)
	}

	size, err := C{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "Size"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(size, "typedef struct {") || !strings.Contains(size, "} Size;") {
		t.Errorf("Size typedef = %q", size)
	}
}

func TestReadInterfaceUnsupported(t *testing.T) {
	src := load(t, "shapes.c")
	defer src.Close()
	_, err := C{}.ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: "Anything"})
	if !errors.Is(err, core.ErrSymbolNotFound) {
		t.Fatalf("err = %v, want ErrSymbolNotFound", err)
	}
}

// TestDisambiguation verifies a struct and a function sharing the name "list"
// are not confused: each is located by its SymbolKind.
func TestDisambiguation(t *testing.T) {
	src := load(t, "disambig.c")
	defer src.Close()

	st, err := C{}.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(st, "struct list {") {
		t.Errorf("struct list = %q", st)
	}

	fn, err := C{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fn, "struct list *list(void)") {
		t.Errorf("function list = %q", fn)
	}
	if st == fn {
		t.Error("struct and function with the same name resolved to identical text")
	}
}

func TestResolveEditsRelativeRange(t *testing.T) {
	src := load(t, "simple.c")
	defer src.Close()
	_, err := C{}.ResolveEdits(src, []core.Edit{{
		Target: core.SymbolID{Kind: core.KindFunc, Name: "add"},
		Rel:    &core.RelativeRange{StartLine: 1},
	}})
	if !errors.Is(err, core.ErrRelativeRangeNotImplemented) {
		t.Fatalf("err = %v, want ErrRelativeRangeNotImplemented", err)
	}
}

// wholeSymbolText returns the exact source text of the whole `add` function
// (signature + body, without the doc comment), derived from the public reads so
// the test stays at the interface level.
func wholeSymbolText(t *testing.T, src core.Source) string {
	t.Helper()
	full, err := C{}.Function(src, core.SymbolID{Kind: core.KindFunc, Name: "add"})
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := C{}.ListSignatures(src)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(full, sigs[0].Doc+"\n")
}

// copyExample copies an example file into dir and returns its path and hash.
func copyExample(t *testing.T, name, dir string) (string, [32]byte) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst, sha256.Sum256(b)
}

// TestWriteRejectsBrokenSyntax checks that an edit producing invalid syntax is
// rejected and the file on disk is left byte-identical.
func TestWriteRejectsBrokenSyntax(t *testing.T) {
	path, want := copyExample(t, "simple.c", t.TempDir())
	_, err := core.BatchWrite(C{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "add"},
		NewText: "int add(int a, int b) { return a +", // missing operand and closing brace
	}})
	if !errors.Is(err, core.ErrSyntaxBroken) {
		t.Fatalf("err = %v, want ErrSyntaxBroken", err)
	}
	got, _ := os.ReadFile(path)
	if sha256.Sum256(got) != want {
		t.Error("file changed on disk after a rejected write")
	}
}

// TestWriteIdempotent checks that replacing a symbol with its own exact text
// leaves the file hash unchanged.
func TestWriteIdempotent(t *testing.T) {
	dir := t.TempDir()
	path, want := copyExample(t, "simple.c", dir)

	src := load(t, "simple.c")
	original := wholeSymbolText(t, src)
	src.Close()

	res, err := core.BatchWrite(C{}, path, []core.Edit{{
		Target:  core.SymbolID{Kind: core.KindFunc, Name: "add"},
		NewText: original,
	}})
	if err != nil {
		t.Fatalf("BatchWrite: %v", err)
	}
	if !res.Applied {
		t.Fatalf("write not applied: %+v", res)
	}
	got, _ := os.ReadFile(path)
	if sha256.Sum256(got) != want {
		t.Error("rewriting a symbol with its own text changed the file")
	}
}
