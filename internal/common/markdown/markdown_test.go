package markdown

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocinsh/crwai/internal/core"
)

const examplesDir = "../../../examples/markdown"

func load(t *testing.T, name string) *Doc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	d, err := Parse(b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return d
}

func paths(hs []Heading) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Path
	}
	return out
}

// TestOutline checks levels, ancestor paths, line numbers, and that a `#` line
// inside a fenced code block is NOT treated as a heading.
func TestOutline(t *testing.T) {
	d := load(t, "sample.md")
	got := paths(d.Outline())
	want := []string{
		"Guide",
		"Guide/Installation",
		"Guide/Installation/Notes",
		"Guide/Usage",
		"Guide/Usage/Notes",
		"Guide/Code",
	}
	if len(got) != len(want) {
		t.Fatalf("outline = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("heading[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// The "## Not A Heading" line lives inside a ```go fence and must be ignored.
	for _, p := range got {
		if strings.Contains(p, "Not A Heading") {
			t.Errorf("fenced code heading leaked into outline: %v", got)
		}
	}

	// Spot-check level and 1-based line number of the top heading.
	top := d.Outline()[0]
	if top.Level != 1 || top.Text != "Guide" || top.Line != 1 {
		t.Errorf("top heading = %+v, want level 1 'Guide' line 1", top)
	}
}

// TestSectionBoundaries checks that a section spans to the next heading of
// equal-or-shallower level and includes its nested subsections.
func TestSectionBoundaries(t *testing.T) {
	d := load(t, "sample.md")

	sec, err := d.Section("Guide/Installation")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sec.Content, "## Installation") {
		t.Errorf("section should start with its heading line:\n%s", sec.Content)
	}
	for _, want := range []string{"Install intro.", "### Notes", "Install notes."} {
		if !strings.Contains(sec.Content, want) {
			t.Errorf("Installation section missing %q:\n%s", want, sec.Content)
		}
	}
	if strings.Contains(sec.Content, "Usage intro.") {
		t.Errorf("Installation section bled into Usage:\n%s", sec.Content)
	}
}

// TestDuplicateHeadingDisambiguation verifies two headings sharing the same Text
// are addressed independently by their full path.
func TestDuplicateHeadingDisambiguation(t *testing.T) {
	d := load(t, "sample.md")

	install, err := d.Section("Guide/Installation/Notes")
	if err != nil {
		t.Fatal(err)
	}
	usage, err := d.Section("Guide/Usage/Notes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(install.Content, "Install notes.") {
		t.Errorf("Installation/Notes wrong:\n%s", install.Content)
	}
	if !strings.Contains(usage.Content, "Usage notes.") {
		t.Errorf("Usage/Notes wrong:\n%s", usage.Content)
	}
	if install.Content == usage.Content {
		t.Error("duplicate-named sections resolved to the same content")
	}
}

// TestSectionNotFound checks the typed error for an unknown heading path.
func TestSectionNotFound(t *testing.T) {
	d := load(t, "sample.md")
	if _, err := d.Section("Guide/Nope"); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("err = %v, want ErrSymbolNotFound", err)
	}
}

// TestResolveSectionEdits checks the resolved byte span matches the section text
// and that an unknown path is rejected.
func TestResolveSectionEdits(t *testing.T) {
	d := load(t, "sample.md")

	edits, err := d.ResolveSectionEdits([]SectionEdit{{Path: "Guide/Usage", NewText: "## Usage\n\nReplaced.\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("got %d resolved edits, want 1", len(edits))
	}
	e := edits[0]
	span := string(d.src[e.StartByte:e.EndByte])
	if !strings.HasPrefix(span, "## Usage") {
		t.Errorf("resolved span should start at the heading:\n%s", span)
	}
	if strings.Contains(span, "## Code") {
		t.Errorf("resolved span should stop before the next L2 heading:\n%s", span)
	}
	if !strings.Contains(span, "Usage notes.") {
		t.Errorf("resolved span should include the nested subsection:\n%s", span)
	}

	if _, err := d.ResolveSectionEdits([]SectionEdit{{Path: "Guide/Nope"}}); !errors.Is(err, core.ErrSymbolNotFound) {
		t.Errorf("err = %v, want ErrSymbolNotFound", err)
	}
}

// TestCRLF verifies headings are detected and their text is clean on a CRLF file.
func TestCRLF(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(examplesDir, "sample.md"))
	if err != nil {
		t.Fatal(err)
	}
	crlf := bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
	d, err := Parse(crlf)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range d.Outline() {
		if strings.ContainsAny(h.Text, "\r") {
			t.Errorf("heading text retains CR: %q", h.Text)
		}
	}
	if got := paths(d.Outline()); len(got) != 6 {
		t.Errorf("CRLF outline count = %d, want 6", len(got))
	}
}

// TestWriteSectionsIsAtomic covers the write half of the tool: it goes through
// the shared pipeline, so it must refuse an empty batch and an unknown heading
// without touching the file, and a rewrite must leave the document's spacing
// intact rather than welding the next heading onto the new text.
func TestWriteSectionsIsAtomic(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "doc.md")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	const doc = "# Top\n\nIntro.\n\n## One\n\nFirst.\n\n## Two\n\nSecond.\n"

	t.Run("an empty batch is refused and changes nothing", func(t *testing.T) {
		path := write(t, doc)
		if _, err := WriteSections(path, nil); !errors.Is(err, core.ErrNoEdits) {
			t.Fatalf("err = %v, want ErrNoEdits", err)
		}
		if got := readFile(t, path); got != doc {
			t.Error("an empty batch rewrote the file")
		}
	})

	t.Run("an unknown heading is refused and changes nothing", func(t *testing.T) {
		path := write(t, doc)
		_, err := WriteSections(path, []SectionEdit{{Path: "Top/Missing", NewText: "## Missing\n"}})
		if !errors.Is(err, core.ErrSymbolNotFound) {
			t.Fatalf("err = %v, want ErrSymbolNotFound", err)
		}
		if got := readFile(t, path); got != doc {
			t.Error("a batch naming an unknown heading rewrote the file")
		}
	})

	t.Run("overlapping edits are refused", func(t *testing.T) {
		path := write(t, doc)
		// "Top" contains "Top/One", so the two spans are nested.
		_, err := WriteSections(path, []SectionEdit{
			{Path: "Top", NewText: "# Top\n"},
			{Path: "Top/One", NewText: "## One\n"},
		})
		if !errors.Is(err, core.ErrOverlappingEdits) {
			t.Fatalf("err = %v, want ErrOverlappingEdits", err)
		}
		if got := readFile(t, path); got != doc {
			t.Error("a rejected overlapping batch rewrote the file")
		}
	})

	t.Run("a rewrite keeps the separation from the next heading", func(t *testing.T) {
		path := write(t, doc)
		res, err := WriteSections(path, []SectionEdit{{Path: "Top/One", NewText: "## One\n\nReplaced."}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied {
			t.Fatal("the write did not apply")
		}
		want := "# Top\n\nIntro.\n\n## One\n\nReplaced.\n\n## Two\n\nSecond.\n"
		if got := readFile(t, path); got != want {
			t.Errorf("document =\n%q\nwant\n%q", got, want)
		}
		if res.Edits[0].Target.Kind != core.KindSection {
			t.Errorf("target kind = %q, want %q", res.Edits[0].Target.Kind, core.KindSection)
		}
	})
}

// readFile returns a file's contents or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
