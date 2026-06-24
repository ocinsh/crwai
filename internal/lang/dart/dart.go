// Package dart implements the Dart language capabilities of the core: it parses
// a Dart file with tree-sitter and answers read/write requests addressed by
// symbol identity (see lang/golang for the documented reference shape and
// lang/TEMPLATE.md for the recipe).
//
// Dart-specific notes:
//   - Grammar: github.com/UserNobody14/tree-sitter-dart (community-maintained; no
//     official Dart grammar exists). Its Go binding returns an unsafe.Pointer
//     compatible with the official github.com/tree-sitter/go-tree-sitter runtime.
//     The module is PINNED in go.mod: never run `go mod tidy` (it follows the
//     longest-prefix match into a broken nested module and fails the build); the
//     pin is what keeps `go build`/`test`/`run` working.
//   - Documentation: preceding-sibling `///` doc comments (each `///` line is its
//     own documentation_comment node); they are joined with newlines.
//   - SymbolID.Container: the enclosing class name for a method; "" for a
//     top-level function.
//   - Interfaces vs structs: Dart has no distinct interface declaration, so
//     ReadInterface maps to an `abstract`/`interface` class and ReadStruct to a
//     concrete class.
//   - Tree shape: a function's signature and body are SEPARATE sibling nodes
//     (function_signature / method_signature / declaration, followed by an
//     optional function_body). The whole-symbol span runs from the leading doc
//     comments to the end of the body (or the signature when there is no body).
package dart

import (
	"fmt"
	"sort"
	"strings"

	tsdart "github.com/UserNobody14/tree-sitter-dart/bindings/go"
	"github.com/ocinsh/crwai/internal/core"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// Dart is the Dart language implementation. The zero value is ready to use; it
// holds no per-call state (a fresh Source is parsed per call).
type Dart struct{}

// Tree-sitter S-expression queries. The capture is always the symbol's name
// identifier; the enclosing signature/class node is reached via its Parent.
const (
	// queryFunctions matches every function and method by name. Top-level
	// functions, class methods, and abstract method declarations all nest a
	// function_signature with a `name` field, so one query covers them all.
	queryFunctions = `(function_signature name: (identifier) @name)`
	// queryClasses matches every class declaration by name. Abstract vs concrete
	// is distinguished by inspecting the node's children, not the query.
	queryClasses = `(class_definition name: (identifier) @name)`
)

// Compile-time assertions: Dart satisfies the read interfaces (via Language) and
// the optional write capability.
var (
	_ core.Language       = (*Dart)(nil)
	_ core.FunctionWriter = (*Dart)(nil)
)

// Name returns the canonical language name.
func (Dart) Name() string { return "dart" }

// Extensions returns the file extensions handled by this language.
func (Dart) Extensions() []string { return []string{".dart"} }

// dartLang wraps the grammar pointer in a runtime Language. It is cheap (a
// pointer wrap) and safe to call per query.
func dartLang() *ts.Language { return ts.NewLanguage(tsdart.Language()) }

// Parse loads src into a parsed Source via the shared core helper. The caller
// owns the returned Source and must Close it (via defer).
func (Dart) Parse(src []byte) (core.Source, error) {
	return core.Parse(dartLang(), src)
}

// ListSignatures returns one Signature per TOP-LEVEL symbol — top-level functions
// and classes — in source order. Methods are reachable via Function with a
// container; they are not listed here because Signature carries no container to
// disambiguate them. No bodies are read.
func (d Dart) ListSignatures(src core.Source) ([]core.Signature, error) {
	bytes := src.Bytes()

	type entry struct {
		start uint
		sig   core.Signature
	}
	var entries []entry

	fns, err := d.collectFunctions(src)
	if err != nil {
		return nil, err
	}
	for _, f := range fns {
		if p := f.anchor.Parent(); p == nil || p.Kind() != "program" {
			continue // only top-level functions
		}
		doc, _ := leadingDoc(f.anchor, bytes)
		entries = append(entries, entry{f.anchor.StartByte(), core.Signature{
			Name:    f.name,
			Params:  paramsOf(f.sig, bytes),
			Returns: returnsOf(f.sig, bytes),
			Doc:     doc,
		}})
	}

	classes, err := d.collectClasses(src)
	if err != nil {
		return nil, err
	}
	for _, c := range classes {
		if p := c.node.Parent(); p == nil || p.Kind() != "program" {
			continue
		}
		doc, _ := leadingDoc(c.node, bytes)
		entries = append(entries, entry{c.node.StartByte(), core.Signature{Name: c.name, Doc: doc}})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].start < entries[j].start })
	out := make([]core.Signature, len(entries))
	for i, e := range entries {
		out[i] = e.sig
	}
	return out, nil
}

// FunctionBody returns only the body text (the `{ … }` block or `=> …;` arrow) of
// the function/method matching id.
func (d Dart) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	f, ok := d.findFunction(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	if f.body == nil {
		return "", core.ErrSymbolNotFound // abstract / bodyless declaration
	}
	return f.body.Utf8Text(src.Bytes()), nil
}

// Function returns the whole function/method matching id: leading doc comments
// and annotations + signature + body.
func (d Dart) Function(src core.Source, id core.SymbolID) (string, error) {
	f, ok := d.findFunction(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	start, end := functionSpan(f, src.Bytes())
	return string(src.Bytes()[start:end]), nil
}

// ReadInterface returns the full definition of an abstract/interface class named
// id.Name (Dart's closest analogue to an interface), including leading doc.
func (d Dart) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	return d.readClass(src, id.Name, true)
}

// ReadStruct returns the full definition of a concrete (non-abstract) class named
// id.Name, including leading doc.
func (d Dart) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	return d.readClass(src, id.Name, false)
}

// ResolveEdits maps each Edit to a concrete byte span on src by symbolic
// identity. It does not mutate src or touch disk — core.BatchWrite orders,
// overlap-checks, applies, re-parses, and persists. Whole-symbol replacement is
// the only granularity; an Edit carrying a RelativeRange returns
// ErrRelativeRangeNotImplemented.
func (d Dart) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	bytes := src.Bytes()
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, ErrRelativeRangeNotImplemented
		}
		var start, end uint
		switch e.Target.Kind {
		case core.KindFunc, core.KindMethod:
			f, ok := d.findFunction(src, e.Target)
			if !ok {
				return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
			}
			start, end = functionSpan(f, bytes)
		case core.KindInterface:
			s, en, ok := d.classSpan(src, e.Target.Name, true)
			if !ok {
				return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
			}
			start, end = s, en
		case core.KindStruct:
			s, en, ok := d.classSpan(src, e.Target.Name, false)
			if !ok {
				return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
			}
			start, end = s, en
		default:
			return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
		}
		out = append(out, core.ResolvedEdit{StartByte: start, EndByte: end, NewText: e.NewText, From: e.Target})
	}
	return out, nil
}

// --- symbol collection ------------------------------------------------------

// fnSym is a located function/method: its name, its enclosing class ("" for
// top-level), and the tree nodes needed to compute spans.
type fnSym struct {
	name      string
	container string
	sig       *ts.Node // function_signature
	anchor    *ts.Node // function_signature, method_signature, or declaration
	body      *ts.Node // function_body, or nil for an abstract/bodyless declaration
}

// classSym is a located class: its name, its node, and whether it is abstract
// (mapped to an interface) or concrete (mapped to a struct).
type classSym struct {
	name     string
	node     *ts.Node
	abstract bool
}

// collectFunctions runs queryFunctions and returns every function/method in the
// Source, resolving each one's anchor (the writable node), body, and container.
func (Dart) collectFunctions(src core.Source) ([]fnSym, error) {
	q, qerr := ts.NewQuery(dartLang(), queryFunctions)
	if qerr != nil {
		return nil, fmt.Errorf("dart: bad function query: %s", qerr.Message)
	}
	defer q.Close()
	qc := ts.NewQueryCursor()
	defer qc.Close()

	bytes := src.Bytes()
	var out []fnSym
	matches := qc.Matches(q, src.Root(), bytes)
	for m := matches.Next(); m != nil; m = matches.Next() {
		for _, c := range m.Captures {
			node := c.Node
			nameNode := &node
			sig := nameNode.Parent()
			if sig == nil || sig.Kind() != "function_signature" {
				continue
			}
			anchor := sig
			if p := sig.Parent(); p != nil {
				if k := p.Kind(); k == "method_signature" || k == "declaration" {
					anchor = p
				}
			}
			out = append(out, fnSym{
				name:      nameNode.Utf8Text(bytes),
				container: containerOf(anchor, bytes),
				sig:       sig,
				anchor:    anchor,
				body:      bodyOf(anchor),
			})
		}
	}
	return out, nil
}

// collectClasses runs queryClasses and returns every class in the Source.
func (Dart) collectClasses(src core.Source) ([]classSym, error) {
	q, qerr := ts.NewQuery(dartLang(), queryClasses)
	if qerr != nil {
		return nil, fmt.Errorf("dart: bad class query: %s", qerr.Message)
	}
	defer q.Close()
	qc := ts.NewQueryCursor()
	defer qc.Close()

	bytes := src.Bytes()
	var out []classSym
	matches := qc.Matches(q, src.Root(), bytes)
	for m := matches.Next(); m != nil; m = matches.Next() {
		for _, c := range m.Captures {
			node := c.Node
			nameNode := &node
			cls := nameNode.Parent()
			if cls == nil || cls.Kind() != "class_definition" {
				continue
			}
			out = append(out, classSym{
				name:     nameNode.Utf8Text(bytes),
				node:     cls,
				abstract: isAbstract(cls),
			})
		}
	}
	return out, nil
}

// findFunction returns the function/method matching id by name and container.
func (d Dart) findFunction(src core.Source, id core.SymbolID) (fnSym, bool) {
	fns, err := d.collectFunctions(src)
	if err != nil {
		return fnSym{}, false
	}
	for _, f := range fns {
		if f.name == id.Name && f.container == id.Container {
			return f, true
		}
	}
	return fnSym{}, false
}

// classSpan returns the [start,end) byte span (doc-inclusive) of the class named
// name whose abstractness matches wantAbstract.
func (d Dart) classSpan(src core.Source, name string, wantAbstract bool) (uint, uint, bool) {
	classes, err := d.collectClasses(src)
	if err != nil {
		return 0, 0, false
	}
	bytes := src.Bytes()
	for _, c := range classes {
		if c.name == name && c.abstract == wantAbstract {
			_, start := leadingDoc(c.node, bytes)
			return start, c.node.EndByte(), true
		}
	}
	return 0, 0, false
}

// readClass returns the source text of the matching class (doc-inclusive).
func (d Dart) readClass(src core.Source, name string, wantAbstract bool) (string, error) {
	start, end, ok := d.classSpan(src, name, wantAbstract)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return string(src.Bytes()[start:end]), nil
}

// --- node helpers -----------------------------------------------------------

// containerOf returns the name of the nearest enclosing class, or "" if the node
// is top-level.
func containerOf(n *ts.Node, src []byte) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == "class_definition" {
			if nm := p.ChildByFieldName("name"); nm != nil {
				return nm.Utf8Text(src)
			}
			return ""
		}
	}
	return ""
}

// bodyOf returns the function_body that immediately follows anchor, or nil when
// the symbol has no body (an abstract method or a bodyless declaration).
func bodyOf(anchor *ts.Node) *ts.Node {
	if next := anchor.NextNamedSibling(); next != nil && next.Kind() == "function_body" {
		return next
	}
	return nil
}

// functionSpan returns the [start,end) byte span of the whole function: from its
// leading doc/annotations to the end of its body (or the signature when bodyless).
func functionSpan(f fnSym, src []byte) (uint, uint) {
	_, start := leadingDoc(f.anchor, src)
	end := f.anchor.EndByte()
	if f.body != nil {
		end = f.body.EndByte()
	}
	return start, end
}

// leadingDoc walks the contiguous run of doc comments and annotations directly
// preceding anchor. It returns the joined doc text (annotations excluded) and the
// start byte of the run (annotations included, so the whole-symbol span covers
// them).
func leadingDoc(anchor *ts.Node, src []byte) (doc string, start uint) {
	start = anchor.StartByte()
	var lines []string
	for prev := anchor.PrevNamedSibling(); prev != nil; prev = prev.PrevNamedSibling() {
		switch prev.Kind() {
		case "documentation_comment":
			// Only `///` doc comments count as documentation in Dart; a plain `//`
			// comment is a boundary (handled by the default case below).
			lines = append(lines, stripDoc(prev.Utf8Text(src)))
			start = prev.StartByte()
		case "annotation", "marker_annotation":
			start = prev.StartByte()
		default:
			return joinReversed(lines), start
		}
	}
	return joinReversed(lines), start
}

// stripDoc removes the `///` / `//` markers from each line of a comment node and
// trims surrounding whitespace.
func stripDoc(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		ln = strings.TrimSpace(ln)
		ln = strings.TrimPrefix(ln, "///")
		ln = strings.TrimPrefix(ln, "//")
		lines[i] = strings.TrimSpace(ln)
	}
	return strings.Join(lines, "\n")
}

// joinReversed reverses the nearest-first doc lines into source order and joins
// them with newlines.
func joinReversed(lines []string) string {
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n")
}

// isAbstract reports whether a class_definition is abstract or an interface
// class (both map to ReadInterface).
func isAbstract(cls *ts.Node) bool {
	for i := uint(0); i < cls.ChildCount(); i++ {
		switch cls.Child(i).Kind() {
		case "abstract", "interface":
			return true
		}
	}
	return false
}

// paramsOf returns the textual parameter declarations of a function_signature,
// in order.
func paramsOf(sig *ts.Node, src []byte) []string {
	fpl := childOfKind(sig, "formal_parameter_list")
	if fpl == nil {
		return nil
	}
	var ps []string
	for i := uint(0); i < fpl.NamedChildCount(); i++ {
		ps = append(ps, fpl.NamedChild(i).Utf8Text(src))
	}
	return ps
}

// returnsOf returns the textual return type of a function_signature: the text
// from the signature start up to the name identifier ("" if none).
func returnsOf(sig *ts.Node, src []byte) string {
	name := sig.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	start, end := sig.StartByte(), name.StartByte()
	if end < start || end > uint(len(src)) {
		return ""
	}
	return strings.TrimSpace(string(src[start:end]))
}

// childOfKind returns the first named child of n with the given kind, or nil.
func childOfKind(n *ts.Node, kind string) *ts.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if ch := n.NamedChild(i); ch.Kind() == kind {
			return ch
		}
	}
	return nil
}
