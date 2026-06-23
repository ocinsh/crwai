// Package java implements the core.Language (read) and core.FunctionWriter
// (write) capabilities for Java, using the tree-sitter-java grammar. It follows
// the shape of the reference subpackage lang/golang: compile-time assertions, a
// type with no per-call state, and methods that operate on an already-parsed
// core.Source.
//
// Java-specific notes:
//   - Documentation: contiguous preceding-sibling `/** ... */` Javadoc blocks (or
//     `//` line comments) immediately above the declaration.
//   - SymbolID.Container: the enclosing class/interface/record/enum name for a
//     method or constructor; "" only for the rare top-level case (Java requires
//     methods to live inside a type).
//   - Node kinds used: method_declaration, constructor_declaration,
//     class_declaration, interface_declaration, record_declaration,
//     enum_declaration; the `name`/`type`/`parameters`/`body` fields name the
//     relevant children.
package java

import (
	"errors"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"github.com/ocinsh/crwai/internal/core"
)

// Java-specific typed errors. ErrNotSupportedByLanguage exists for the contract's
// sake (Java does support interfaces and classes, so it is never returned here);
// ErrRelativeRangeNotImplemented marks the v1 gap for symbol-relative edits.
var (
	// ErrNotSupportedByLanguage means a capability has no meaning in Java.
	ErrNotSupportedByLanguage = errors.New("capability not supported by language")
	// ErrRelativeRangeNotImplemented means an Edit carried a RelativeRange, which
	// is not resolved in v1.
	ErrRelativeRangeNotImplemented = errors.New("relative-range edits not implemented")
)

// Java is the Java language implementation. The zero value is ready to use; it
// holds no per-call state (the server is stateless — a fresh Source per call).
type Java struct{}

// Compile-time assertions: Java satisfies the read interfaces (via Language) and
// the optional write capability.
var (
	_ core.Language       = (*Java)(nil)
	_ core.FunctionWriter = (*Java)(nil)
)

// Node kinds this implementation targets, named once for clarity.
const (
	kindMethod       = "method_declaration"
	kindConstructor  = "constructor_declaration"
	kindClass        = "class_declaration"
	kindInterface    = "interface_declaration"
	kindRecord       = "record_declaration"
	kindEnum         = "enum_declaration"
	kindLineComment  = "line_comment"
	kindBlockComment = "block_comment"
)

// Name returns the canonical language name.
func (Java) Name() string { return "java" }

// Extensions returns the file extensions handled by this language.
func (Java) Extensions() []string { return []string{".java"} }

// Parse loads src into a parsed Source via the shared core helper. The caller
// owns the returned Source and must Close it (via defer).
func (Java) Parse(src []byte) (core.Source, error) {
	return core.Parse(ts.NewLanguage(tsjava.Language()), src)
}

// ListSignatures walks the tree and returns one Signature per method,
// constructor, and type declaration (class/interface/record/enum), attaching the
// contiguous preceding comments as Doc. No bodies are read.
//
// Note: core.Signature carries no Kind/Container, so same-named symbols in
// different containers appear as separate entries with the same Name; they are
// disambiguated only when addressed by SymbolID through the reader/writer methods.
func (Java) ListSignatures(src core.Source) ([]core.Signature, error) {
	source := src.Bytes()
	var out []core.Signature
	walk(src.Root(), func(n *ts.Node) {
		switch n.Kind() {
		case kindMethod, kindConstructor:
			out = append(out, methodSignature(n, source))
		case kindClass, kindInterface, kindRecord, kindEnum:
			out = append(out, typeSignature(n, source))
		}
	})
	return out, nil
}

// FunctionBody locates the method/constructor matching id and returns the text of
// its body block (braces included). An abstract interface method has no body and
// yields ErrSymbolNotFound.
func (Java) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	n := findMethod(src.Root(), src.Bytes(), id)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	body := methodBody(n)
	if body == nil {
		return "", core.ErrSymbolNotFound
	}
	return body.Utf8Text(src.Bytes()), nil
}

// Function locates the method/constructor matching id and returns its full text:
// contiguous preceding doc comments + signature + body.
func (Java) Function(src core.Source, id core.SymbolID) (string, error) {
	n := findMethod(src.Root(), src.Bytes(), id)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(n, src.Bytes()), nil
}

// ReadInterface locates the interface declaration matching id and returns its
// full declaration text (doc + body).
func (Java) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	n := findType(src.Root(), src.Bytes(), id.Name, kindInterface)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(n, src.Bytes()), nil
}

// ReadStruct locates the struct-like declaration matching id — a Java class,
// record, or enum — and returns its full declaration text (doc + body).
func (Java) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	n := findType(src.Root(), src.Bytes(), id.Name, kindClass, kindRecord, kindEnum)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(n, src.Bytes()), nil
}

// ResolveEdits (core.FunctionWriter) maps each Edit to a concrete byte span on
// src by locating the target symbol by identity. The replaced span is the whole
// declaration node (signature + body, doc comments excluded) — the agent supplies
// a complete replacement symbol. It does not mutate src or touch disk;
// core.BatchWrite orders, overlap-checks, applies, re-parses, and persists.
//
// RelativeRange edits are a v1 gap: an Edit carrying Rel != nil yields
// ErrRelativeRangeNotImplemented (never a panic).
func (Java) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	source := src.Bytes()
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, ErrRelativeRangeNotImplemented
		}
		var n *ts.Node
		switch e.Target.Kind {
		case core.KindFunc, core.KindMethod:
			n = findMethod(src.Root(), source, e.Target)
		case core.KindInterface:
			n = findType(src.Root(), source, e.Target.Name, kindInterface)
		case core.KindStruct:
			n = findType(src.Root(), source, e.Target.Name, kindClass, kindRecord, kindEnum)
		}
		if n == nil {
			return nil, core.ErrSymbolNotFound
		}
		out = append(out, core.ResolvedEdit{
			StartByte: n.StartByte(),
			EndByte:   n.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return out, nil
}

// --- tree helpers -----------------------------------------------------------

// walk invokes fn on every named node in the subtree rooted at n, pre-order.
func walk(n *ts.Node, fn func(*ts.Node)) {
	count := n.NamedChildCount()
	for i := uint(0); i < count; i++ {
		c := n.NamedChild(i)
		fn(c)
		walk(c, fn)
	}
}

// findMethod returns the method/constructor node matching id (name + container),
// or nil. Container "" matches a symbol whose enclosing type is also absent.
func findMethod(root *ts.Node, source []byte, id core.SymbolID) *ts.Node {
	var found *ts.Node
	walk(root, func(n *ts.Node) {
		if found != nil {
			return
		}
		if n.Kind() != kindMethod && n.Kind() != kindConstructor {
			return
		}
		if nodeName(n, source) != id.Name {
			return
		}
		if enclosingTypeName(n, source) != id.Container {
			return
		}
		found = n
	})
	return found
}

// findType returns the first type declaration of one of kinds whose name matches,
// or nil.
func findType(root *ts.Node, source []byte, name string, kinds ...string) *ts.Node {
	var found *ts.Node
	walk(root, func(n *ts.Node) {
		if found != nil {
			return
		}
		k := n.Kind()
		match := false
		for _, want := range kinds {
			if k == want {
				match = true
				break
			}
		}
		if !match {
			return
		}
		if nodeName(n, source) == name {
			found = n
		}
	})
	return found
}

// nodeName returns the text of a node's `name` field, or "" if absent.
func nodeName(n *ts.Node, source []byte) string {
	name := n.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	return name.Utf8Text(source)
}

// enclosingTypeName walks parents from n up to the first type declaration and
// returns its name; "" if n is not nested in a type.
func enclosingTypeName(n *ts.Node, source []byte) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.Kind() {
		case kindClass, kindInterface, kindRecord, kindEnum:
			return nodeName(p, source)
		}
	}
	return ""
}

// methodBody returns the body node of a method/constructor (a `block` or
// `constructor_body`), or nil for an abstract method.
func methodBody(n *ts.Node) *ts.Node {
	if b := n.ChildByFieldName("body"); b != nil {
		return b
	}
	// Constructors expose the body without a field name in some grammar versions.
	count := n.NamedChildCount()
	for i := uint(0); i < count; i++ {
		c := n.NamedChild(i)
		if c.Kind() == "block" || c.Kind() == "constructor_body" {
			return c
		}
	}
	return nil
}

// methodSignature builds a Signature for a method/constructor: its declared name,
// each formal parameter's text, the return type (if any), and preceding doc.
func methodSignature(n *ts.Node, source []byte) core.Signature {
	sig := core.Signature{
		Name: nodeName(n, source),
		Doc:  precedingDoc(n, source),
	}
	if params := n.ChildByFieldName("parameters"); params != nil {
		pc := params.NamedChildCount()
		for i := uint(0); i < pc; i++ {
			p := params.NamedChild(i)
			if p.Kind() == "formal_parameter" || p.Kind() == "spread_parameter" {
				sig.Params = append(sig.Params, strings.TrimSpace(p.Utf8Text(source)))
			}
		}
	}
	if rt := n.ChildByFieldName("type"); rt != nil {
		sig.Returns = rt.Utf8Text(source)
	}
	return sig
}

// typeSignature builds a Signature for a type declaration: its name and doc. A
// record's component list is surfaced as Params; classes/interfaces/enums carry
// none.
func typeSignature(n *ts.Node, source []byte) core.Signature {
	sig := core.Signature{
		Name: nodeName(n, source),
		Doc:  precedingDoc(n, source),
	}
	if n.Kind() == kindRecord {
		if params := n.ChildByFieldName("parameters"); params != nil {
			pc := params.NamedChildCount()
			for i := uint(0); i < pc; i++ {
				p := params.NamedChild(i)
				if p.Kind() == "formal_parameter" {
					sig.Params = append(sig.Params, strings.TrimSpace(p.Utf8Text(source)))
				}
			}
		}
	}
	return sig
}

// precedingDoc collects the contiguous comment siblings immediately above n
// (block or line comments, no blank-line gap) and returns them joined by newline.
// Returns "" when there is no attached documentation.
func precedingDoc(n *ts.Node, source []byte) string {
	var comments []*ts.Node
	prev := n.PrevSibling()
	below := n
	for prev != nil && isComment(prev) {
		// Require adjacency: the comment must end on the line directly above the
		// node it documents (no intervening blank line).
		if below.StartPosition().Row-prev.EndPosition().Row > 1 {
			break
		}
		comments = append([]*ts.Node{prev}, comments...)
		below = prev
		prev = prev.PrevSibling()
	}
	if len(comments) == 0 {
		return ""
	}
	parts := make([]string, len(comments))
	for i, c := range comments {
		parts[i] = c.Utf8Text(source)
	}
	return strings.Join(parts, "\n")
}

// withDoc returns the node's text prefixed by its contiguous preceding doc
// comments (separated by a newline), or just the node text when undocumented.
func withDoc(n *ts.Node, source []byte) string {
	doc := precedingDoc(n, source)
	body := n.Utf8Text(source)
	if doc == "" {
		return body
	}
	return doc + "\n" + body
}

// isComment reports whether n is a line or block comment node.
func isComment(n *ts.Node) bool {
	return n.Kind() == kindLineComment || n.Kind() == kindBlockComment
}
