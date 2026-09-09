// Package ui is the presentation layer for the crwai CLI. It is the single place
// that decides how things LOOK in the terminal — colors, the tree structure, and
// the kind tags that label each row — so the command files stay logic-only and
// never embed escape codes or layout. Commands hand it plain data (a
// crwai.Signature, an error, a code string) and get back a ready-to-print string.
//
// The visual language is one idea, applied everywhere: a bold root naming what is
// being shown, a dim meta line summarising it, and the content hanging off
// box-drawing connectors. Structure is drawn dim so it recedes; the content
// (names, signatures, source) stays bright. Every listing is nested where the
// data is nested — a method sits UNDER the type it belongs to, a Markdown
// subsection under its parent heading — because that nesting is the fact the
// reader most needs and the flat form loses: three methods called `area` are
// indistinguishable in a flat list and obvious in a tree.
//
// Box-drawing glyphs (the standard tree connectors) are NOT emoji, and emoji are
// banned from this CLI (see CLAUDE.md). No glyph here is decorative: the tag
// column carries the kind as a word, never as an icon.
//
// All styling goes through lipgloss, which degrades gracefully: on a non-TTY or a
// NO_COLOR terminal the styles render as plain text, so piped and redirected
// output stays clean while the connectors keep the structure readable.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ocinsh/crwai"
)

// Tree connectors — standard box-drawing runes, the same vocabulary `tree` and
// `eza` use. branch/last prefix a node; guide/gap continue (or close) the
// vertical line under it.
const (
	branch = "├─ "
	last   = "└─ "
	guide  = "│  "
	gap    = "   "
	bar    = "│ " // content bar: the dim gutter that fronts a source block
)

// docWidth is the column at which a doc summary is cut. It keeps a listing
// scannable on a narrow terminal without wrapping, and it is a summary, not the
// documentation: the full text comes back with the symbol itself.
const docWidth = 96

// Palette — a small, restrained set of semantic colors. Structure is grey so it
// recedes; names are bright; everything secondary is muted.
var (
	accent = lipgloss.Color("63")  // violet — root + kind tags
	good   = lipgloss.Color("42")  // green — success
	bad    = lipgloss.Color("203") // red — failure
	muted  = lipgloss.Color("245") // grey — structure, docs, secondary
	name   = lipgloss.Color("81")  // cyan — symbol names

	rootStyle  = lipgloss.NewStyle().Bold(true)
	tagStyle   = lipgloss.NewStyle().Foreground(accent)
	nameStyle  = lipgloss.NewStyle().Bold(true).Foreground(name)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	goodStyle  = lipgloss.NewStyle().Bold(true).Foreground(good)
	badStyle   = lipgloss.NewStyle().Bold(true).Foreground(bad)
)

// Node is one row of a tree: a left tag column (the kind, or whatever word
// classifies the row), a bright head (the name plus any signature), an optional
// dim sub line shown underneath (a doc summary, a file's extensions, a failure
// reason), and any children nested beneath it.
type Node struct {
	Tag      string
	Head     string
	Sub      string
	Tone     Tone
	Children []Node
}

// Tone selects the color of a node's tag. It exists so a caller can say what a
// row MEANS (this edit succeeded, this one failed) and leave which color that is
// to this package, which is the only place allowed to know.
type Tone int

const (
	// ToneNeutral is the default: the tag is a classifier, not a verdict.
	ToneNeutral Tone = iota
	// ToneGood marks a success.
	ToneGood
	// ToneBad marks a failure.
	ToneBad
)

// style returns the style a tone is rendered in.
func (t Tone) style() lipgloss.Style {
	switch t {
	case ToneGood:
		return goodStyle
	case ToneBad:
		return badStyle
	default:
		return tagStyle
	}
}

// Tree renders a bold root, an optional dim meta line, and the nodes hanging off
// box-drawing connectors, recursing into children. Within each group of siblings
// the tag column is padded to a common width so the heads line up; a Sub line is
// indented to sit under its head with the vertical guide continued (or closed, on
// the last node). It is the one assembler every listing goes through.
func Tree(root, meta string, nodes []Node) string {
	var b strings.Builder
	b.WriteString(rootStyle.Render(root))
	if meta != "" {
		b.WriteString("\n" + mutedStyle.Render(meta))
	}
	writeNodes(&b, nodes, "")
	return b.String()
}

// writeNodes renders one level of siblings under the given prefix, then recurses.
// prefix is the already-drawn structure to the left of this level (dim guides and
// gaps), so a child knows nothing about its depth beyond that string.
func writeNodes(b *strings.Builder, nodes []Node, prefix string) {
	tagW := 0
	for _, n := range nodes {
		if len(n.Tag) > tagW {
			tagW = len(n.Tag)
		}
	}
	for i, n := range nodes {
		conn, cont := branch, guide
		if i == len(nodes)-1 {
			conn, cont = last, gap
		}
		tag := ""
		if tagW > 0 {
			tag = n.Tone.style().Render(n.Tag+strings.Repeat(" ", tagW-len(n.Tag))) + "  "
		}
		b.WriteString("\n" + mutedStyle.Render(prefix+conn) + tag + n.Head)
		if n.Sub != "" {
			pad := ""
			if tagW > 0 {
				pad = strings.Repeat(" ", tagW+2)
			}
			b.WriteString("\n" + mutedStyle.Render(prefix+cont) + pad + mutedStyle.Render(n.Sub))
		}
		writeNodes(b, n.Children, prefix+cont)
	}
}

// SignatureNodes turns a file's signatures into a tree: top-level symbols at the
// first level, and every method nested under the type it belongs to. The nesting
// is the point — a flat listing renders three same-named methods identically,
// while the tree says which type each one is bound to without the reader having
// to read a container field.
//
// A method whose container is not declared in this file (a C++ member defined
// outside its class, a Go method on a type from another file) gets a synthetic
// "scope" group named after the container, so it is still shown under its owner
// rather than floating at the top level.
func SignatureNodes(sigs []crwai.Signature) []Node {
	var out []Node
	index := map[string]int{} // container name -> index in out

	for _, s := range sigs {
		node := signatureNode(s)
		if s.Container == "" {
			if s.Kind == crwai.KindStruct || s.Kind == crwai.KindInterface {
				index[s.Name] = len(out)
			}
			out = append(out, node)
			continue
		}
		i, ok := index[s.Container]
		if !ok {
			i = len(out)
			index[s.Container] = i
			out = append(out, Node{Tag: "scope", Head: nameStyle.Render(s.Container)})
		}
		out[i].Children = append(out[i].Children, node)
	}
	return out
}

// signatureNode turns one Signature into a tree node: the kind as its tag, the
// signature as its head, and a summary of its doc as the dim sub. Callables carry
// a verbatim signature line (Text) — receiver, type parameters and all — so it is
// shown as-is; interfaces and structs have no Text and render as a bare name (no
// empty "()").
func signatureNode(s crwai.Signature) Node {
	var head string
	switch {
	case s.Text != "":
		// Text is verbatim and may span lines (a multi-line parameter list); the
		// tree wants one line per symbol, so collapse internal whitespace runs.
		head = nameStyle.Render(strings.Join(strings.Fields(s.Text), " "))
	case s.Kind == crwai.KindStruct || s.Kind == crwai.KindInterface:
		head = nameStyle.Render(s.Name)
	default:
		head = nameStyle.Render(s.Name) + mutedStyle.Render("("+strings.Join(s.Params, ", ")+")")
		if s.Returns != "" {
			head += " " + mutedStyle.Render(s.Returns)
		}
	}
	return Node{Tag: s.Kind.String(), Head: head, Sub: docSummary(s.Doc)}
}

// LangNodes turns the supported languages into tree nodes: the language name in
// the tag column, its file extensions as the head. Name-in-the-tag-column is what
// aligns the extensions into a readable second column.
func LangNodes(langs []crwai.LanguageInfo) []Node {
	out := make([]Node, 0, len(langs))
	for _, l := range langs {
		out = append(out, Node{Tag: l.Name, Head: mutedStyle.Render(strings.Join(l.Extensions, "  "))})
	}
	return out
}

// HeadingNodes turns a Markdown outline into a tree that mirrors the document's
// own nesting: an h2 sits under the h1 above it, an h3 under that h2. The tag
// column carries the heading level, and the head carries the heading path segment
// that addresses the section, which is exactly the string `read_section` and the
// `section` command take.
func HeadingNodes(headings []crwai.Heading) []Node {
	var roots []Node
	// path is the index chain to the currently open ancestor, levels the heading
	// level at each of its depths. The chain is walked afresh for every heading:
	// appending to a nested slice can reallocate it, so holding a pointer to a
	// node across iterations would dangle.
	var path []int
	var levels []int

	for _, h := range headings {
		n := Node{
			Tag:  fmt.Sprintf("h%d", h.Level),
			Head: nameStyle.Render(h.Text),
			Sub:  fmt.Sprintf("line %d", h.Line),
		}
		for len(levels) > 0 && levels[len(levels)-1] >= h.Level {
			levels = levels[:len(levels)-1]
			path = path[:len(path)-1]
		}
		siblings := &roots
		for _, i := range path {
			siblings = &(*siblings)[i].Children
		}
		*siblings = append(*siblings, n)
		path = append(path, len(*siblings)-1)
		levels = append(levels, h.Level)
	}
	return roots
}

// RequestNodes turns a Postman listing into tree nodes: the HTTP method in the
// tag column so the verbs align into a readable column, the URL as the head, and
// the collection folder as the dim sub — the folder being what tells two copies
// of the same endpoint apart.
func RequestNodes(reqs []crwai.Request) []Node {
	out := make([]Node, 0, len(reqs))
	for _, r := range reqs {
		sub := r.Name
		if r.Folder != "" {
			sub = r.Folder + "/" + r.Name
		}
		out = append(out, Node{Tag: r.Method, Head: nameStyle.Render(r.URL), Sub: sub})
	}
	return out
}

// Symbol renders a single located symbol as a one-branch tree: the file as root,
// a closing node labelled with the kind and name (qualified by its container when
// it has one), and the source hanging under it behind a dim content bar. It is
// what the function/body/interface/struct commands print.
func Symbol(file string, kind crwai.SymbolKind, name, container, source string) string {
	label := name
	if container != "" {
		label = container + "." + name
	}
	head := mutedStyle.Render(last) + tagStyle.Render(kind.String()) + "  " + nameStyle.Render(label)
	return rootStyle.Render(file) + "\n" + head + "\n" + block(source)
}

// Section renders one Markdown section: the file as root, the heading path as the
// single branch, and the section's Markdown behind the content bar.
func Section(file string, s crwai.Section) string {
	head := mutedStyle.Render(last) + tagStyle.Render(fmt.Sprintf("h%d", s.Heading.Level)) +
		"  " + nameStyle.Render(s.Heading.Path) +
		"  " + mutedStyle.Render(fmt.Sprintf("line %d", s.Heading.Line))
	return rootStyle.Render(file) + "\n" + head + "\n" + block(s.Content)
}

// Requests renders the full detail of the Postman endpoints matching a query: one
// branch per endpoint, with method and URL on the branch and the body and
// documentation behind content bars beneath it.
func Requests(file string, reqs []crwai.RequestDetail) string {
	var b strings.Builder
	b.WriteString(rootStyle.Render(file))
	b.WriteString("\n" + mutedStyle.Render(plural(len(reqs), "request", "requests")))
	for i, r := range reqs {
		conn, cont := branch, guide
		if i == len(reqs)-1 {
			conn, cont = last, gap
		}
		b.WriteString("\n" + mutedStyle.Render(conn) + tagStyle.Render(r.Method) + "  " + nameStyle.Render(r.URL))
		if r.Folder != "" {
			b.WriteString("\n" + mutedStyle.Render(cont+r.Folder+"/"+r.Name))
		}
		for _, part := range []struct{ label, text string }{{"doc", r.Doc}, {"body", r.Body}} {
			if strings.TrimSpace(part.text) == "" {
				continue
			}
			b.WriteString("\n" + mutedStyle.Render(cont) + tagStyle.Render(part.label))
			for _, ln := range lines(part.text) {
				b.WriteString("\n" + cont + mutedStyle.Render(bar) + ln)
			}
		}
	}
	return b.String()
}

// WriteResult renders the all-or-nothing outcome of a batch write as a tree: the
// file as root, a node summarising whether the batch applied, and one leaf per
// edit marked ok/fail with its reason.
func WriteResult(res crwai.WriteResult) string {
	summary := goodStyle.Render("applied " + plural(len(res.Edits), "edit", "edits"))
	if !res.Applied {
		summary = badStyle.Render("no edits applied, file left untouched")
	}
	nodes := make([]Node, 0, len(res.Edits))
	for _, e := range res.Edits {
		tag, tone := "ok", ToneGood
		if !e.OK {
			tag, tone = "fail", ToneBad
		}
		label := e.Target.Name
		if e.Target.Container != "" {
			label = e.Target.Container + "." + e.Target.Name
		}
		nodes = append(nodes, Node{
			Tag:  tag,
			Tone: tone,
			Head: mutedStyle.Render(e.Target.Kind.String()) + " " + nameStyle.Render(label),
			Sub:  e.Reason,
		})
	}
	var b strings.Builder
	b.WriteString(rootStyle.Render(res.Path) + "\n" + mutedStyle.Render(last) + summary)
	writeNodes(&b, nodes, gap)
	return b.String()
}

// Version renders the product banner: the bold name and its dim version.
func Version(name, version string) string {
	return rootStyle.Render(name) + " " + mutedStyle.Render(version)
}

// Fail renders an error as a red leaf. It is what the CLI prints to stderr when a
// command returns an error, so a failure looks like the rest of the tool rather
// than like a stray panic.
func Fail(err error) string {
	return badStyle.Render("error") + "  " + err.Error()
}

// Meta builds the dim summary line under a tree root: a count of what was found,
// optionally prefixed by the language or format it was read as.
func Meta(kind string, n int, unit, units string) string {
	count := plural(n, unit, units)
	if kind == "" {
		return count
	}
	return kind + ", " + count
}

// block renders arbitrary text behind the dim content bar, indented one level, so
// a source body or a Markdown section reads as content hanging off the tree
// rather than as output the shell printed.
func block(text string) string {
	var b strings.Builder
	for i, ln := range lines(text) {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(gap + mutedStyle.Render(bar) + ln)
	}
	return b.String()
}

// lines splits text into display lines, dropping the trailing newline so a block
// never ends in an empty bar.
func lines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

// plural renders "1 edit" / "3 edits" without the "(s)" that makes CLI output
// look machine-generated.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// docSummary reduces a doc comment to one line of prose for the tree's sub row:
// the first line that carries text once the comment syntax is stripped, cut to
// docWidth. Without the stripping a TypeScript symbol's summary would read
// "/**" and a Python one would read '"""', which tells the reader nothing.
func docSummary(doc string) string {
	for _, ln := range strings.Split(doc, "\n") {
		if t := stripCommentMarkers(ln); t != "" {
			return truncate(t, docWidth)
		}
	}
	return ""
}

// commentOpeners and commentClosers are the punctuation a language wraps a doc
// comment in. Order matters within each list: the longest match must come first,
// so "/**" is not mistaken for "/*" and "///" not for "//".
var (
	commentOpeners = []string{"/**", "/*", "///", "//!", "//", "#", `"""`, "'''", "*/", "*"}
	commentClosers = []string{"*/", `"""`, "'''"}
)

// stripCommentMarkers removes the comment punctuation a language wraps its docs
// in, leaving the prose. It is deliberately syntactic and cheap: it runs on a
// single line that has already been extracted as documentation. Both ends are
// handled, because a one-line docstring or block comment carries its closing
// delimiter on the same line as its text.
func stripCommentMarkers(line string) string {
	s := strings.TrimSpace(line)
	for _, marker := range commentOpeners {
		if strings.HasPrefix(s, marker) {
			s = strings.TrimSpace(strings.TrimPrefix(s, marker))
			break
		}
	}
	for _, marker := range commentClosers {
		if strings.HasSuffix(s, marker) {
			s = strings.TrimSpace(strings.TrimSuffix(s, marker))
			break
		}
	}
	return s
}

// truncate cuts s to at most n runes, marking the cut with a trailing ellipsis of
// three plain dots (no single-glyph ellipsis: the CLI stays ASCII-safe).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-3]), " ") + "..."
}
