package ui

import (
	"strings"
	"testing"

	"github.com/ocinsh/crwai"
)

// The tests below assert on the plain-text skeleton of the rendered output.
// lipgloss emits no escape codes when it is not writing to a terminal, which is
// the case under `go test`, so what these tests see is exactly what a user sees
// through a pipe or under NO_COLOR — the form that has to stay readable.

// TestSignatureNodesNestMethodsUnderTheirType is the central promise of the
// listing: three same-named methods on three different types must be
// distinguishable, and the only thing that distinguishes them is where they sit.
func TestSignatureNodesNestMethodsUnderTheirType(t *testing.T) {
	sigs := []crwai.Signature{
		{Kind: crwai.KindStruct, Name: "Circle"},
		{Kind: crwai.KindMethod, Name: "area", Container: "Circle", Text: "area(): number"},
		{Kind: crwai.KindStruct, Name: "Square"},
		{Kind: crwai.KindMethod, Name: "area", Container: "Square", Text: "area(): number"},
		{Kind: crwai.KindFunc, Name: "area", Text: "function area(w, h)"},
	}

	nodes := SignatureNodes(sigs)
	if len(nodes) != 3 {
		t.Fatalf("top level holds %d nodes, want 3 (two types and the free function)", len(nodes))
	}
	for i, want := range []int{1, 1, 0} {
		if got := len(nodes[i].Children); got != want {
			t.Errorf("node %d has %d children, want %d", i, got, want)
		}
	}

	out := Tree("shapes.ts", "typescript, 5 symbols", nodes)
	// A child row is indented past its parent's connector; a top-level row is not.
	if !strings.Contains(out, "\n├─ struct  Circle") {
		t.Errorf("Circle is not a top-level row:\n%s", out)
	}
	if !strings.Contains(out, "\n│  └─ method  area(): number") {
		t.Errorf("Circle.area is not nested under Circle:\n%s", out)
	}
	if !strings.Contains(out, "\n└─ func    function area(w, h)") {
		t.Errorf("the free function is not a top-level row:\n%s", out)
	}
}

// TestSignatureNodesGroupAnAbsentContainer covers a method whose type is declared
// in another file, as a C++ member defined outside its class is: it still belongs
// under its owner rather than floating at the top level.
func TestSignatureNodesGroupAnAbsentContainer(t *testing.T) {
	nodes := SignatureNodes([]crwai.Signature{
		{Kind: crwai.KindMethod, Name: "area", Container: "Circle", Text: "double Circle::area()"},
		{Kind: crwai.KindMethod, Name: "name", Container: "Circle", Text: "string Circle::name()"},
	})
	if len(nodes) != 1 {
		t.Fatalf("top level holds %d nodes, want 1 synthetic group", len(nodes))
	}
	if nodes[0].Tag != "scope" {
		t.Errorf("group tag = %q, want \"scope\"", nodes[0].Tag)
	}
	if len(nodes[0].Children) != 2 {
		t.Errorf("the group holds %d children, want 2", len(nodes[0].Children))
	}
}

// TestHeadingNodesMirrorTheDocument checks the outline nests by heading level, so
// two sections that share a title are told apart by their parent.
func TestHeadingNodesMirrorTheDocument(t *testing.T) {
	nodes := HeadingNodes([]crwai.Heading{
		{Level: 1, Text: "Guide", Path: "Guide", Line: 1},
		{Level: 2, Text: "Install", Path: "Guide/Install", Line: 5},
		{Level: 3, Text: "Notes", Path: "Guide/Install/Notes", Line: 9},
		{Level: 2, Text: "Usage", Path: "Guide/Usage", Line: 13},
	})

	if len(nodes) != 1 {
		t.Fatalf("top level holds %d nodes, want 1", len(nodes))
	}
	guide := nodes[0]
	if len(guide.Children) != 2 {
		t.Fatalf("Guide holds %d children, want 2", len(guide.Children))
	}
	if len(guide.Children[0].Children) != 1 {
		t.Errorf("Install holds %d children, want 1", len(guide.Children[0].Children))
	}
	if len(guide.Children[1].Children) != 0 {
		t.Errorf("Usage should hold no children")
	}
	// Appending to a nested slice reallocates it; a renderer that held pointers
	// across iterations would lose the deepest heading.
	if guide.Children[0].Children[0].Tag != "h3" {
		t.Error("the deepest heading was lost while building the tree")
	}
}

// TestDocSummaryStripsCommentSyntax guards the readability fix: a summary must be
// prose, not the punctuation the language wraps its docs in.
func TestDocSummaryStripsCommentSyntax(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"go line comment", "// Add returns the sum.", "Add returns the sum."},
		{"jsdoc opener", "/**\n * area returns the area.\n */", "area returns the area."},
		{"python docstring", `"""Return the area."""`, "Return the area."},
		{"rust doc comment", "/// Builds a stack.", "Builds a stack."},
		{"leading blank lines are skipped", "\n\n// Real text.", "Real text."},
		{"no doc", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := docSummary(c.doc); got != c.want {
				t.Errorf("docSummary = %q, want %q", got, c.want)
			}
		})
	}
}

func TestTruncateMarksTheCut(t *testing.T) {
	long := strings.Repeat("a", docWidth+20)
	got := docSummary("// " + long)
	if len([]rune(got)) != docWidth {
		t.Errorf("summary is %d runes, want %d", len([]rune(got)), docWidth)
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("a truncated summary does not mark the cut")
	}
}

// TestNoEmojiInRenderedOutput enforces the project rule at the only place that can
// break it. Box-drawing connectors are explicitly allowed; anything in an emoji
// range is not.
func TestNoEmojiInRenderedOutput(t *testing.T) {
	rendered := []string{
		Tree("f.go", "go, 1 symbol", SignatureNodes([]crwai.Signature{
			{Kind: crwai.KindStruct, Name: "T", Doc: "// A type."},
			{Kind: crwai.KindMethod, Name: "M", Container: "T", Text: "func (t T) M()"},
		})),
		Tree("langs", "1 language", LangNodes([]crwai.LanguageInfo{{Name: "go", Extensions: []string{".go"}}})),
		Symbol("f.go", crwai.KindMethod, "M", "T", "func (t T) M() {}"),
		WriteResult(crwai.WriteResult{Path: "f.go", Applied: true, Edits: []crwai.EditOutcome{
			{Target: crwai.SymbolID{Kind: crwai.KindFunc, Name: "F"}, OK: true},
		}}),
		Version("crwai", "0.1.0"),
	}
	for _, out := range rendered {
		for _, r := range out {
			if isEmoji(r) {
				t.Errorf("rendered output contains the emoji %q:\n%s", string(r), out)
			}
		}
	}
}

// isEmoji reports whether r falls in one of the Unicode blocks the ban covers.
// The box-drawing block (U+2500..U+257F) is deliberately not among them.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, emoticons, symbols, extensions
		return true
	case r >= 0x2600 && r <= 0x27BF: // miscellaneous symbols and dingbats
		return true
	case r == 0xFE0F: // variation selector 16, the emoji presentation marker
		return true
	default:
		return false
	}
}

// TestWriteResultReportsEachEdit checks a failed batch says which edit failed and
// why, since that is the only thing the caller can act on.
func TestWriteResultReportsEachEdit(t *testing.T) {
	out := WriteResult(crwai.WriteResult{
		Path:    "shapes.py",
		Applied: false,
		Edits: []crwai.EditOutcome{
			{Target: crwai.SymbolID{Kind: crwai.KindMethod, Name: "area", Container: "Circle"}, Reason: "overlaps edit on perimeter"},
		},
	})
	for _, want := range []string{"shapes.py", "no edits applied", "fail", "Circle.area", "overlaps edit on perimeter"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
}

func TestPluralAvoidsTheParenthesisedS(t *testing.T) {
	if got := plural(1, "edit", "edits"); got != "1 edit" {
		t.Errorf("plural(1) = %q, want \"1 edit\"", got)
	}
	if got := plural(3, "edit", "edits"); got != "3 edits" {
		t.Errorf("plural(3) = %q, want \"3 edits\"", got)
	}
	if got := Meta("go", 2, "symbol", "symbols"); got != "go, 2 symbols" {
		t.Errorf("Meta = %q, want \"go, 2 symbols\"", got)
	}
}
