// Package markdown is a COMMON-FILE tool: it reads, and surgically rewrites, a
// Markdown document at the granularity of its heading sections. It lives
// alongside the programming-language layer but is deliberately NOT a
// core.Language:
//
//   - It is not routed by file extension through the language registry. The tool
//     is selected explicitly (a CLI subcommand / a named MCP tool), the same way
//     an agent picks `list_signatures` rather than having it inferred.
//   - A section is addressed by HEADING IDENTITY (a heading path), never by byte
//     offset across the tool boundary — mirroring the symbol-identity rule of the
//     code tools.
//   - Parsing is a line-oriented block scan, NOT tree-sitter. At the
//     outline/section granularity a heading is just a line beginning with `#`
//     (outside a fenced code block), and Markdown has no breakable syntax that a
//     re-parse would need to validate. This keeps the tool free of CGO and of the
//     tree-sitter dependency that the code languages carry.
//
// The tool never authors prose: it only places text the caller supplies, exactly
// as the code writer only places caller-provided source.
package markdown

import (
	"fmt"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
)

// Heading is one node of a document's outline: the cheap, body-free descriptor an
// agent reads first (the Markdown analogue of core.Signature).
type Heading struct {
	// Level is the ATX heading depth, 1..6 (`#` == 1, `##` == 2, ...).
	Level int `json:"level"`
	// Text is the heading text with the leading `#`s and surrounding spaces
	// stripped (e.g. "Prerequisites").
	Text string `json:"text"`
	// Path is the slash-joined chain of ancestor headings ending in this one
	// (e.g. "Installation/Prerequisites"). It is the STABLE IDENTITY used to
	// address a section, disambiguating headings that share the same Text under
	// different parents.
	Path string `json:"path"`
	// Line is the 1-based line number of the heading line in the file.
	Line uint `json:"line"`
}

// Section is a heading together with the content beneath it — the full read of
// one slice of a document (the Markdown analogue of get_function).
type Section struct {
	// Heading is this section's own heading.
	Heading Heading `json:"heading"`
	// Content is the raw Markdown spanning from the heading down to (but not
	// including) the next heading of equal-or-shallower level. It INCLUDES the
	// heading line itself, so the returned text is a self-contained, re-insertable
	// block; nested deeper subsections are part of the content. Trailing blank
	// lines before the next heading are trimmed.
	Content string `json:"content"`
}

// SectionEdit is a single requested rewrite, addressed by heading Path. The
// caller supplies the full replacement block (heading line included); the tool
// never generates it. It is the Markdown analogue of core.Edit.
type SectionEdit struct {
	// Path is the heading identity of the section to replace (see Heading.Path).
	Path string `json:"path"`
	// NewText is the replacement Markdown block supplied by the caller.
	NewText string `json:"new_text"`
}

// Doc is a parsed Markdown document, valid only for the duration of one call. The
// tool is stateless: every call re-reads the file from disk and re-parses it, so
// a Doc never outlives the call that produced it.
type Doc struct {
	// src is the original file content; sections index into it by byte offset.
	src []byte
	// heads are the document's headings in document order, each carrying the byte
	// offset of its heading line (used to compute section spans).
	heads []headRec
}

// headRec is a heading plus the byte offset at which its line begins.
type headRec struct {
	h     Heading
	start int
}

// Parse scans raw Markdown into a Doc by walking its ATX headings, skipping any
// `#` lines that fall inside a fenced code block (``` or ~~~). It performs no
// validation beyond reading the bytes: every input is valid Markdown.
func Parse(src []byte) (*Doc, error) {
	d := &Doc{src: src}
	var ancestors []Heading // open ancestor chain, by level
	offset := 0
	lineNo := uint(0)
	inFence := false
	for _, raw := range strings.SplitAfter(string(src), "\n") {
		if raw == "" {
			break
		}
		lineStart := offset
		offset += len(raw)
		lineNo++
		line := strings.TrimRight(raw, "\r\n")

		if isFence(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		level, text, ok := atxHeading(line)
		if !ok {
			continue
		}
		for len(ancestors) > 0 && ancestors[len(ancestors)-1].Level >= level {
			ancestors = ancestors[:len(ancestors)-1]
		}
		path := text
		if len(ancestors) > 0 {
			parts := make([]string, 0, len(ancestors)+1)
			for _, a := range ancestors {
				parts = append(parts, a.Text)
			}
			parts = append(parts, text)
			path = strings.Join(parts, "/")
		}
		h := Heading{Level: level, Text: text, Path: path, Line: lineNo}
		d.heads = append(d.heads, headRec{h: h, start: lineStart})
		ancestors = append(ancestors, h)
	}
	return d, nil
}

// isFence reports whether a line opens or closes a fenced code block.
func isFence(line string) bool {
	s := strings.TrimLeft(line, " ")
	return strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~")
}

// atxHeading parses an ATX heading line, returning its level (1..6) and text. The
// trailing closing `#` sequence, if any, is stripped.
func atxHeading(line string) (int, string, bool) {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return 0, "", false
	}
	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return 0, "", false // e.g. "#tag" is not a heading
	}
	text := strings.TrimSpace(line[i:])
	text = strings.TrimRight(text, "#")
	text = strings.TrimSpace(text)
	return i, text, true
}

// Outliner lists the heading outline of a document — the cheap map an agent reads
// first, the Markdown analogue of core.SignatureLister / `list_signatures`.
type Outliner interface {
	// Outline returns every heading in document order. An empty slice means the
	// document has no headings (valid, not an error).
	Outline() []Heading
}

// SectionReader returns the content of a single section, addressed by heading
// Path. The Markdown analogue of core.FunctionReader / `get_function`.
type SectionReader interface {
	// Section returns the section whose Heading.Path equals path. It returns
	// core.ErrSymbolNotFound when no heading matches the path.
	Section(path string) (Section, error)
}

// SectionWriter is the OPTIONAL write capability. It maps a batch of section
// edits onto concrete byte spans on the in-memory Doc; it does NOT touch the disk
// and does NOT order or apply the edits — ordering, overlap checks, the atomic
// apply, and the staleness defense are the job of the shared all-or-nothing write
// path, exactly as core.FunctionWriter hands spans to core.BatchWrite. A
// read-only document tool would simply omit this interface.
type SectionWriter interface {
	// ResolveSectionEdits resolves each SectionEdit (addressed by heading Path)
	// to a concrete [start, end) byte span on the Doc. It returns
	// core.ErrSymbolNotFound if any edit targets a heading that does not exist.
	ResolveSectionEdits(edits []SectionEdit) ([]core.ResolvedEdit, error)
}

// Compile-time assertions: a parsed Doc satisfies the read capabilities and the
// optional write capability.
var (
	_ Outliner      = (*Doc)(nil)
	_ SectionReader = (*Doc)(nil)
	_ SectionWriter = (*Doc)(nil)
)

// Outline returns the document's headings in order.
func (d *Doc) Outline() []Heading {
	out := make([]Heading, len(d.heads))
	for i, r := range d.heads {
		out[i] = r.h
	}
	return out
}

// Section returns the section addressed by heading path.
func (d *Doc) Section(path string) (Section, error) {
	i, ok := d.indexOf(path)
	if !ok {
		return Section{}, fmt.Errorf("markdown: %w: heading %q", core.ErrSymbolNotFound, path)
	}
	start, end := d.span(i)
	content := strings.TrimRight(string(d.src[start:end]), "\n")
	return Section{Heading: d.heads[i].h, Content: content}, nil
}

// ResolveSectionEdits maps section edits to byte spans for the shared write path.
func (d *Doc) ResolveSectionEdits(edits []SectionEdit) ([]core.ResolvedEdit, error) {
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		i, ok := d.indexOf(e.Path)
		if !ok {
			return nil, fmt.Errorf("markdown: %w: heading %q", core.ErrSymbolNotFound, e.Path)
		}
		start, end := d.span(i)
		out = append(out, core.ResolvedEdit{
			StartByte: uint(start),
			EndByte:   uint(end),
			NewText:   e.NewText,
			From:      core.SymbolID{Name: e.Path},
		})
	}
	return out, nil
}

// indexOf returns the index of the first heading whose Path matches.
func (d *Doc) indexOf(path string) (int, bool) {
	for i, r := range d.heads {
		if r.h.Path == path {
			return i, true
		}
	}
	return 0, false
}

// span returns the [start, end) byte range of the section at index i: from its
// heading line to the start of the next heading of equal-or-shallower level (or
// end of file).
func (d *Doc) span(i int) (int, int) {
	start := d.heads[i].start
	level := d.heads[i].h.Level
	for j := i + 1; j < len(d.heads); j++ {
		if d.heads[j].h.Level <= level {
			return start, d.heads[j].start
		}
	}
	return start, len(d.src)
}
