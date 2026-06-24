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
