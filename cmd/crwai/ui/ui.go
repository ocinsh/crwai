// Package ui is the presentation layer for the crwai CLI. It is the single place
// that decides how things LOOK in the terminal — colors, the tree structure, and
// the kind tags that label each symbol — so the command files stay logic-only and
// never embed escape codes or layout. Commands hand it plain data (a
// crwai.Signature, an error, a code string) and get back a ready-to-print string.
//
// The visual language is a tree: a bold root (usually a file path) with its
// symbols hanging off box-drawing connectors. The structure is drawn dim so it
// recedes; the content (names, signatures, source) stays bright. Box-drawing
// glyphs (├─ └─ │) are standard tree connectors, NOT emoji — emoji are banned
// from this CLI (see CLAUDE.md). No glyph here is decorative.
//
// All styling goes through lipgloss, which degrades gracefully: on a non-TTY or a
// NO_COLOR terminal the styles render as plain text, so piped output stays clean
// while the tree connectors (plain runes) keep the structure readable.
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

// Node is one row of a tree: a left tag column (kind, "lang", an outcome), a
// bright head (the name plus any signature), and an optional dim sub line shown
// underneath (a doc summary, a file's extensions, a failure reason).
type Node struct {
	Tag  string
	Head string
	Sub  string
}

// Tree renders a bold root with its nodes hanging off box-drawing connectors.
// The tag column is padded to a common width so the heads line up, and any Sub
// line is indented to sit under its head with the vertical guide continued (or
// closed, on the last node). It is the one assembler every listing goes through.
func Tree(root string, nodes []Node) string {
	var b strings.Builder
	b.WriteString(rootStyle.Render(root))

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
		tag := tagStyle.Render(n.Tag + strings.Repeat(" ", tagW-len(n.Tag)))
		b.WriteString("\n" + mutedStyle.Render(conn) + tag + "  " + n.Head)
		if n.Sub != "" {
			indent := mutedStyle.Render(cont) + strings.Repeat(" ", tagW+2)
			b.WriteString("\n" + indent + mutedStyle.Render(n.Sub))
		}
	}
	return b.String()
}

// SignatureNode turns one Signature into a tree node: the kind as its tag, the
// signature as its head, and the first line of its doc as the dim sub. Callables
// carry a verbatim signature line (Text) — receiver, type parameters and all — so
// it is shown as-is; interfaces and structs have no Text and render as a bare
// name (no empty "()").
func SignatureNode(s crwai.Signature) Node {
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
	return Node{Tag: s.Kind.String(), Head: head, Sub: firstLine(s.Doc)}
}

// LangNode turns one supported language into a tree node: a "lang" tag, the name,
// and its file extensions as the dim sub.
func LangNode(name string, exts []string) Node {
	return Node{Tag: "lang", Head: nameStyle.Render(name), Sub: strings.Join(exts, "  ")}
}

// Symbol renders a single located symbol as a one-branch tree: the file as root,
// a closing node labelled with the kind and name, and the source hanging under it
// behind a dim content bar. It is what the function/body/interface/struct
// commands print.
func Symbol(file string, kind crwai.SymbolKind, name, source string) string {
	head := mutedStyle.Render(last) + tagStyle.Render(kind.String()) + "  " + nameStyle.Render(name)
	var b strings.Builder
	b.WriteString(rootStyle.Render(file) + "\n" + head)
	for _, ln := range strings.Split(strings.TrimRight(source, "\n"), "\n") {
		b.WriteString("\n" + gap + mutedStyle.Render(bar) + ln)
	}
	return b.String()
}

// WriteResult renders the all-or-nothing outcome of a batch write as a tree: the
// file as root, a node summarising whether the batch applied, and one leaf per
// edit marked ok/fail with its reason.
func WriteResult(res crwai.WriteResult) string {
	summary := goodStyle.Render(fmt.Sprintf("applied %d edit(s)", len(res.Edits)))
	if !res.Applied {
		summary = badStyle.Render("no edits applied (file left untouched)")
	}
	nodes := make([]Node, 0, len(res.Edits))
	for _, e := range res.Edits {
		tag, style := "ok", goodStyle
		if !e.OK {
			tag, style = "fail", badStyle
		}
		head := style.Render(tag) + "  " + mutedStyle.Render(e.Target.Kind.String()) + " " + nameStyle.Render(e.Target.Name)
		nodes = append(nodes, Node{Tag: "", Head: head, Sub: e.Reason})
	}
	out := rootStyle.Render(res.Path) + "\n" + mutedStyle.Render(last) + summary
	if leaves := indentTree(nodes); leaves != "" {
		out += "\n" + leaves
	}
	return out
}

// Version renders the product banner: the bold name and its dim version.
func Version(name, version string) string {
	return rootStyle.Render(name) + " " + mutedStyle.Render(version)
}

// Fail renders an error as a red leaf.
func Fail(err error) string {
	return badStyle.Render("error  ") + err.Error()
}

// indentTree draws nodes one indent level in, under a last (└─) parent: the
// parent's vertical line is closed, so the level is prefixed with a blank gap and
// the same connector vocabulary as Tree. Used for the per-edit leaves under a
// write summary.
func indentTree(nodes []Node) string {
	var b strings.Builder
	for i, n := range nodes {
		conn, cont := branch, guide
		if i == len(nodes)-1 {
			conn, cont = last, gap
		}
		b.WriteString(mutedStyle.Render(gap+conn) + n.Head)
		if n.Sub != "" {
			b.WriteString("\n" + mutedStyle.Render(gap+cont) + mutedStyle.Render(n.Sub))
		}
		if i < len(nodes)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// firstLine returns the first non-empty, trimmed line of s.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}
