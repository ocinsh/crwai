// Package crwai_test exercises the public facade: the parts a consumer of the
// library actually touches, and the parts the two front-ends rely on. It covers
// the rules that live nowhere else — how a free-text kind and a container combine
// into a symbol identity, and what the root confinement does and does not allow —
// rather than re-testing the language implementations, which have their own
// suites.
package crwai_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ocinsh/crwai"
)

// TestTargetForInfersMethodFromContainer pins the rule that keeps the read and
// write paths addressing the same symbol. Before it existed, reading a method by
// container and writing it back with the default kind resolved to two different
// identities and the write failed with ErrSymbolNotFound.
func TestTargetForInfersMethodFromContainer(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		container string
		want      crwai.SymbolKind
	}{
		{"empty kind, no container, is a function", "", "", crwai.KindFunc},
		{"empty kind with a container is a method", "", "Circle", crwai.KindMethod},
		{"default func kind with a container is a method", "func", "Circle", crwai.KindMethod},
		{"an explicit method stays a method", "method", "Circle", crwai.KindMethod},
		{"an explicit struct is never promoted", "struct", "Outer", crwai.KindStruct},
		{"an explicit interface is never promoted", "interface", "Outer", crwai.KindInterface},
		{"an unknown kind falls back to func", "widget", "", crwai.KindFunc},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := crwai.TargetFor(c.kind, "area", c.container)
			if got.Kind != c.want {
				t.Errorf("Kind = %q, want %q", got.Kind, c.want)
			}
			if got.Name != "area" || got.Container != c.container {
				t.Errorf("identity = %+v, want name area and container %q", got, c.container)
			}
		})
	}
}

// TestReadAndWriteAgreeOnAMethod is the end-to-end version of the rule above: a
// method read by container must be writable by the same container, with no kind
// supplied.
func TestReadAndWriteAgreeOnAMethod(t *testing.T) {
	path := copyFixture(t, "examples/python/shapes.py")
	svc := crwai.New()

	if _, err := svc.Function(path, "area", "Circle"); err != nil {
		t.Fatalf("reading Circle.area: %v", err)
	}
	res, err := svc.Write(path, crwai.Edit{
		Target:  crwai.TargetFor("", "area", "Circle"),
		NewText: "    def area(self):\n        return 0",
	})
	if err != nil {
		t.Fatalf("writing Circle.area: %v", err)
	}
	if !res.Applied {
		t.Fatal("the write did not apply")
	}
	if got := read(t, path); !strings.Contains(got, "return 0") {
		t.Error("the replacement text is not in the file")
	}
}

func TestWriteRejectsAnEmptyBatch(t *testing.T) {
	path := copyFixture(t, "examples/golang/basic.go")
	before := read(t, path)

	if _, err := crwai.New().Write(path); !errors.Is(err, crwai.ErrNoEdits) {
		t.Fatalf("err = %v, want ErrNoEdits", err)
	}
	if read(t, path) != before {
		t.Error("an empty batch rewrote the file")
	}
}

// TestRootConfinesEveryPath covers the workspace boundary: inside is allowed,
// outside is refused, and refusal applies to reads and writes alike.
func TestRootConfinesEveryPath(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "inside.go")
	if err := os.WriteFile(inside, []byte("package p\n\nfunc F() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package p\n\nfunc G() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := crwai.New().Root(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ListSignatures(inside); err != nil {
		t.Errorf("a path inside the root was refused: %v", err)
	}
	if _, err := svc.ListSignatures(outside); !errors.Is(err, crwai.ErrPathOutsideRoot) {
		t.Errorf("reading outside the root: err = %v, want ErrPathOutsideRoot", err)
	}
	if _, err := svc.Write(outside, crwai.Edit{
		Target:  crwai.TargetFor("func", "G", ""),
		NewText: "func G() int { return 3 }",
	}); !errors.Is(err, crwai.ErrPathOutsideRoot) {
		t.Errorf("writing outside the root: err = %v, want ErrPathOutsideRoot", err)
	}
	// A traversal that climbs out and comes back is still outside.
	escape := filepath.Join(dir, "..", filepath.Base(filepath.Dir(outside)), "outside.go")
	if _, err := svc.ListSignatures(escape); !errors.Is(err, crwai.ErrPathOutsideRoot) {
		t.Errorf("traversal out of the root: err = %v, want ErrPathOutsideRoot", err)
	}
}

// TestRootRefusesASymlinkEscape checks the confinement resolves symlinks rather
// than comparing strings: a link inside the root pointing out of it must not be
// a way through.
func TestRootRefusesASymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.go")
	if err := os.WriteFile(target, []byte("package p\n\nfunc H() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	svc, err := crwai.New().Root(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListSignatures(link); !errors.Is(err, crwai.ErrPathOutsideRoot) {
		t.Errorf("err = %v, want ErrPathOutsideRoot", err)
	}
}

// TestNoRootAllowsAnyPath states the default plainly: an unconfined engine is
// exactly that, which is why the front-ends offer --root.
func TestNoRootAllowsAnyPath(t *testing.T) {
	path := copyFixture(t, "examples/golang/basic.go")
	if _, err := crwai.New().ListSignatures(path); err != nil {
		t.Errorf("an unconfined engine refused a path: %v", err)
	}
}

func TestLangForcesTheLanguage(t *testing.T) {
	svc, err := crwai.New().Lang("cpp")
	if err != nil {
		t.Fatal(err)
	}
	// A .c file read as C++ still parses; the point is that the override wins
	// over the extension, which would have selected C.
	if _, err := svc.ListSignatures("examples/c/simple.c"); err != nil {
		t.Errorf("forced language failed to read the file: %v", err)
	}
	if _, err := crwai.New().Lang("klingon"); !errors.Is(err, crwai.ErrUnsupportedLanguage) {
		t.Errorf("err = %v, want ErrUnsupportedLanguage", err)
	}
}

// TestLanguagesAreRegistered guards the registry against a language quietly
// dropping out of engine.New.
func TestLanguagesAreRegistered(t *testing.T) {
	want := []string{"c", "cpp", "dart", "go", "java", "javascript", "python", "rust", "tsx", "typescript"}
	got := map[string]bool{}
	for _, l := range crwai.New().Languages() {
		got[l.Name] = true
		if len(l.Extensions) == 0 {
			t.Errorf("language %q claims no extensions", l.Name)
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("language %q is not registered", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("registered %d languages, want %d", len(got), len(want))
	}
}

// TestMarkdownSectionRoundTrip checks the common-file surface keeps the same
// promise as the code surface: what a read returns, written straight back, leaves
// the file byte-identical.
func TestMarkdownSectionRoundTrip(t *testing.T) {
	path := copyFixture(t, "examples/markdown/sample.md")
	before := read(t, path)
	svc := crwai.New()

	heads, err := svc.Outline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) == 0 {
		t.Fatal("the fixture has no headings")
	}

	for _, h := range heads {
		sec, err := svc.Section(path, h.Path)
		if err != nil {
			t.Fatalf("reading section %q: %v", h.Path, err)
		}
		if _, err := svc.WriteSections(path, crwai.SectionEdit{Path: h.Path, NewText: sec.Content}); err != nil {
			t.Fatalf("writing section %q back: %v", h.Path, err)
		}
		if got := read(t, path); got != before {
			t.Fatalf("writing section %q back changed the document", h.Path)
		}
	}
}

func TestSectionReportsAnUnknownHeading(t *testing.T) {
	path := copyFixture(t, "examples/markdown/sample.md")
	if _, err := crwai.New().Section(path, "No/Such/Heading"); !errors.Is(err, crwai.ErrSymbolNotFound) {
		t.Errorf("err = %v, want ErrSymbolNotFound", err)
	}
}

func TestPostmanReadsTheCollection(t *testing.T) {
	const path = "examples/postman/sample.postman_collection.json"
	svc := crwai.New()

	all, err := svc.Requests(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("the fixture collection lists no requests")
	}

	filtered, err := svc.Requests(path, "/token")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) == 0 || len(filtered) >= len(all) {
		t.Errorf("filter kept %d of %d requests, want a strict non-empty subset", len(filtered), len(all))
	}

	details, err := svc.Request(path, "/token")
	if err != nil {
		t.Fatal(err)
	}
	if len(details) != len(filtered) {
		t.Errorf("the reader matched %d requests and the lister %d", len(details), len(filtered))
	}
}

// TestEverySymbolListedCanBeOpened is the promise the listing makes, checked
// across every language and every bundled fixture: if list_signatures names a
// symbol, get_declaration must return it. Before the const/var/type kinds existed
// the listing was silent about them; now that it names them, nothing may be named
// and unopenable.
func TestEverySymbolListedCanBeOpened(t *testing.T) {
	svc := crwai.New()
	files, err := filepath.Glob("examples/*/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}

	checked := 0
	for _, path := range files {
		sigs, err := svc.ListSignatures(path)
		if err != nil {
			continue // a fixture of a format with no language, such as Markdown
		}
		for _, s := range sigs {
			if _, err := svc.Declaration(path, string(s.Kind), s.Name, s.Container); err != nil {
				t.Errorf("%s: listed %s %q (container %q) but could not open it: %v",
					path, s.Kind, s.Name, s.Container, err)
			}
			checked++
		}
	}
	if checked < 200 {
		t.Errorf("only %d symbols checked; the corpus should be much larger", checked)
	}
}

// TestListingReportsBodylessDeclarations pins the gap this fixed: a file that
// declares nothing but constants used to report zero symbols.
func TestListingReportsBodylessDeclarations(t *testing.T) {
	sigs, err := crwai.New().ListSignatures("internal/core/version.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d symbols, want the 2 constants the file declares", len(sigs))
	}
	for _, s := range sigs {
		if s.Kind != crwai.KindConst {
			t.Errorf("%s is listed as %q, want %q", s.Name, s.Kind, crwai.KindConst)
		}
		// The declaration line is the light form: it is where the value and any
		// declared type are written.
		if !strings.Contains(s.Text, s.Name) {
			t.Errorf("%s has no declaration line: %q", s.Name, s.Text)
		}
	}
}

// TestDeclarationFindsASymbolWithoutItsKind covers the shape a caller actually
// has: a name copied out of a listing, and no wish to choose a reader for it.
func TestDeclarationFindsASymbolWithoutItsKind(t *testing.T) {
	svc := crwai.New()
	cases := []struct {
		name string
		want string
	}{
		{"ErrStaleFile", "errors.New"},    // a var inside a grouped declaration
		{"BatchWrite", "func BatchWrite"}, // a function
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := "internal/core/errors.go"
			if c.name == "BatchWrite" {
				path = "internal/core/write.go"
			}
			got, err := svc.Declaration(path, "", c.name, "")
			if err != nil {
				t.Fatalf("Declaration: %v", err)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("declaration does not contain %q:\n%s", c.want, got)
			}
		})
	}
}

// TestDeclarationHonoursAnExplicitKind checks the kind is a filter and not a hint:
// asking for a const named like a function must not return the function.
func TestDeclarationHonoursAnExplicitKind(t *testing.T) {
	svc := crwai.New()
	if _, err := svc.Declaration("internal/core/write.go", "const", "BatchWrite", ""); !errors.Is(err, crwai.ErrSymbolNotFound) {
		t.Errorf("err = %v, want ErrSymbolNotFound: BatchWrite is a function, not a constant", err)
	}
}

// TestStripDocsClearsOnlyTheDocumentation checks the opt-in listing keeps
// everything a map needs and drops only the part a map does not.
func TestStripDocsClearsOnlyTheDocumentation(t *testing.T) {
	full, err := crwai.New().ListSignatures("internal/core/version.go")
	if err != nil {
		t.Fatal(err)
	}
	documented := false
	for _, s := range full {
		if s.Doc != "" {
			documented = true
		}
	}
	if !documented {
		t.Fatal("the fixture carries no documentation, so the test proves nothing")
	}

	stripped := crwai.StripDocs(full)
	if len(stripped) != len(full) {
		t.Fatalf("stripping changed the symbol count: %d, want %d", len(stripped), len(full))
	}
	for i, s := range stripped {
		if s.Doc != "" {
			t.Errorf("%s kept its documentation", s.Name)
		}
		if s.Name != full[i].Name || s.Kind != full[i].Kind || s.Text != full[i].Text {
			t.Errorf("%s lost more than its documentation", s.Name)
		}
	}
	// The original must be untouched: a caller may still want the full form.
	if full[0].Doc == "" {
		t.Error("StripDocs mutated the slice it was given")
	}
}

// TestBodylessDeclarationsAcrossLanguages checks the same promise in all nine
// languages against a fixture per language that holds nothing but declarations
// without a body. It is one table rather than nine near-identical unit tests
// because the behaviour under test is a cross-language contract: what a listing
// covers, and what each kind means.
//
// The mapping is not uniform, and the exceptions are the point:
//   - Python has no constant, so every module-level binding is a variable.
//   - Dart's `final` binds once at run time and is a variable; only `const` is a
//     constant.
//   - Java has no file-level declaration: its class body is the top level, and a
//     static field is what binds once per program.
//   - An enum reads as a struct wherever the language has one, because
//     read_struct already returns it whole.
func TestBodylessDeclarationsAcrossLanguages(t *testing.T) {
	cases := []struct {
		path string
		want map[string]crwai.SymbolKind
	}{
		{"examples/golang/declarations.go", map[string]crwai.SymbolKind{
			"MaxRetries": crwai.KindConst, "KindLine": crwai.KindConst, "KindBlock": crwai.KindConst,
			"ErrEmpty": crwai.KindVar, "ErrTooBig": crwai.KindVar, "ErrTooSmall": crwai.KindVar,
			"Table": crwai.KindVar, "Handler": crwai.KindType, "Weight": crwai.KindType,
			"Pair": crwai.KindStruct, "Sum": crwai.KindMethod,
		}},
		{"examples/python/declarations.py", map[string]crwai.SymbolKind{
			"MAX_RETRIES": crwai.KindVar, "TIMEOUT": crwai.KindVar, "TABLE": crwai.KindVar,
			"described": crwai.KindFunc,
		}},
		{"examples/rust/declarations.rs", map[string]crwai.SymbolKind{
			"MAX": crwai.KindConst, "NAME": crwai.KindVar, "Id": crwai.KindType,
			"Op": crwai.KindStruct, "Pair": crwai.KindStruct, "sum": crwai.KindFunc,
		}},
		{"examples/typescript/declarations.ts", map[string]crwai.SymbolKind{
			"MAX": crwai.KindConst, "counter": crwai.KindVar, "Id": crwai.KindType,
			"Pair": crwai.KindStruct, "sum": crwai.KindFunc,
		}},
		{"examples/javascript/declarations.js", map[string]crwai.SymbolKind{
			"MAX": crwai.KindConst, "TABLE": crwai.KindConst,
			"counter": crwai.KindVar, "legacy": crwai.KindVar, "sum": crwai.KindFunc,
		}},
		{"examples/java/Declarations.java", map[string]crwai.SymbolKind{
			"Declarations": crwai.KindStruct, "MAX": crwai.KindConst,
			"counter": crwai.KindVar, "sum": crwai.KindMethod,
		}},
		{"examples/c/declarations.c", map[string]crwai.SymbolKind{
			"MAX": crwai.KindConst, "counter": crwai.KindVar, "Id": crwai.KindType,
			"Point": crwai.KindStruct, "sum": crwai.KindFunc,
		}},
		{"examples/cpp/declarations.cpp", map[string]crwai.SymbolKind{
			"kMax": crwai.KindConst, "counter": crwai.KindVar,
			"Id": crwai.KindType, "Legacy": crwai.KindType, "sum": crwai.KindFunc,
		}},
		{"examples/dart/declarations.dart", map[string]crwai.SymbolKind{
			"kMax": crwai.KindConst, "greeting": crwai.KindVar, "counter": crwai.KindVar,
			"Handler": crwai.KindType, "sum": crwai.KindFunc,
		}},
	}

	svc := crwai.New()
	for _, c := range cases {
		t.Run(filepath.Base(c.path), func(t *testing.T) {
			sigs, err := svc.ListSignatures(c.path)
			if err != nil {
				t.Fatalf("ListSignatures: %v", err)
			}
			got := map[string]crwai.SymbolKind{}
			for _, s := range sigs {
				got[s.Name] = s.Kind
			}
			for name, kind := range c.want {
				if got[name] != kind {
					t.Errorf("%s is listed as %q, want %q", name, got[name], kind)
				}
			}
			if len(got) != len(c.want) {
				t.Errorf("listed %d symbols, want %d: %v", len(got), len(c.want), got)
			}

			// Every listed symbol opens, and a bodyless one shows the declaration
			// line that carries its value or its declared type.
			for _, s := range sigs {
				text, err := svc.Declaration(c.path, string(s.Kind), s.Name, s.Container)
				if err != nil {
					t.Errorf("%s %s: %v", s.Kind, s.Name, err)
					continue
				}
				if !strings.Contains(text, s.Name) {
					t.Errorf("%s %s: declaration does not mention it:\n%s", s.Kind, s.Name, text)
				}
				switch s.Kind {
				case crwai.KindConst, crwai.KindVar, crwai.KindType:
					if s.Text == "" {
						t.Errorf("%s %s has no declaration line", s.Kind, s.Name)
					}
				case crwai.KindStruct, crwai.KindInterface:
					if s.Text != "" {
						t.Errorf("%s %s should render as a bare name, got %q", s.Kind, s.Name, s.Text)
					}
				}
			}
		})
	}
}

// copyFixture copies a repository fixture into a temp dir so a test that writes
// never mutates the corpus, and returns the copy's path.
func copyFixture(t *testing.T, src string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

// read returns a file's contents or fails the test.
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
