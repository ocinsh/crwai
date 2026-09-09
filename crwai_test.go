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
