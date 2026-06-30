// Package rust implements the Rust language capabilities (see lang/golang for the
// documented reference and lang/TEMPLATE.md for the recipe). It walks a
// tree-sitter parse tree to satisfy the core read interfaces and the optional
// write capability, addressing symbols purely by identity (never by file offset).
//
// Rust-specific notes:
//   - Documentation: the contiguous preceding-sibling line/block comments
//     immediately above an item (`///` outer docs, `//!` inner docs, `/** */`
//     block docs, and plain `//` comments are all captured verbatim).
//   - SymbolID.Container: the type of the enclosing `impl` block for a method
//     (e.g. "Calculator" for `impl Calculator { fn ... }`, or "Wrapper<T>" for a
//     generic impl), or the trait name for a trait method; "" for a free function.
//   - Mapped kinds: a `trait` is reported as KindInterface and a `struct`/`union`
//     as KindStruct, so the generic interface/struct tools work on Rust.
package rust

import (
	"bytes"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"

	"github.com/ocinsh/crwai/internal/core"
)

// rustLang is the tree-sitter grammar handle. It is a static, read-only pointer
// (no Close needed) and is safe to share across the stateless, per-call parses.
var rustLang = ts.NewLanguage(tsrust.Language())

// Rust is the Rust language implementation. The zero value is ready to use; it
// holds no per-call state (the server is stateless — a fresh Source per call).
type Rust struct{}

// Rust is a write target, so it satisfies both the read aggregate (core.Language)
// and the optional write capability (core.FunctionWriter).
var (
	_ core.Language       = (*Rust)(nil)
	_ core.FunctionWriter = (*Rust)(nil)
)

// Name returns the canonical language name.
func (Rust) Name() string { return "rust" }

// Extensions returns the file extensions handled by this language.
func (Rust) Extensions() []string { return []string{".rs"} }

// Parse loads src into a parsed Source via the shared core helper. The caller owns
// the returned Source and must Close it (via defer).
func (Rust) Parse(src []byte) (core.Source, error) {
	return core.Parse(rustLang, src)
}

// ListSignatures returns one Signature per addressable symbol (free functions,
// impl/trait methods, structs/unions, and traits), in document order, attaching
// the contiguous preceding-sibling comments as Doc. No bodies are read.
func (Rust) ListSignatures(src core.Source) ([]core.Signature, error) {
	b := src.Bytes()
	syms := collect(src.Root(), b)
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		out = append(out, core.Signature{
			Kind:      s.id.Kind,
			Name:      s.id.Name,
			Container: s.id.Container,
			Text:      signatureText(s.id.Kind, s.node, b),
			Params:    params(s.node, b),
			Returns:   fieldText(s.node, "return_type", b),
			Doc:       docText(s.node, b),
		})
	}
	return out, nil
}

// signatureText returns the verbatim signature line of a callable symbol: the
// source from the function_item start up to (but not including) its body block,
// so generic parameters, where-clause and return type are preserved exactly. A
// body-less trait method declaration yields its whole text; structs and traits
// yield "".
func signatureText(kind core.SymbolKind, node *ts.Node, b []byte) string {
	if kind != core.KindFunc && kind != core.KindMethod {
		return ""
	}
	if body := node.ChildByFieldName("body"); body != nil {
		return strings.TrimSpace(string(b[node.StartByte():body.StartByte()]))
	}
	return strings.TrimRight(strings.TrimSpace(node.Utf8Text(b)), " \t\n;")
}

// FunctionBody returns only the block (braces included) of the function or method
// matching id.
func (Rust) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	b := src.Bytes()
	n := find(src.Root(), b, id)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	body := n.ChildByFieldName("body")
	if body == nil {
		return "", core.ErrSymbolNotFound
	}
	return body.Utf8Text(b), nil
}

// Function returns the whole function or method matching id: preceding doc
// comments + signature + body.
func (Rust) Function(src core.Source, id core.SymbolID) (string, error) {
	return wholeSymbol(src, id)
}

// ReadInterface returns the full definition (doc + declaration) of the trait
// matching id. Rust traits are reported as KindInterface.
func (Rust) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	return wholeSymbol(src, id)
}

// ReadStruct returns the full definition (doc + declaration) of the struct/union
// matching id.
func (Rust) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	return wholeSymbol(src, id)
}

// ResolveEdits (core.FunctionWriter) maps each Edit to the concrete byte span of
// its whole target symbol (the declaration node, excluding preceding doc
// comments). It does not mutate src or touch disk — core.BatchWrite orders,
// overlap-checks, applies, re-parses, and persists. Symbol-relative edits
// (Edit.Rel != nil) are not implemented in v1.
func (Rust) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	b := src.Bytes()
	root := src.Root()
	resolved := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, core.ErrRelativeRangeNotImplemented
		}
		n := find(root, b, e.Target)
		if n == nil {
			return nil, core.ErrSymbolNotFound
		}
		resolved = append(resolved, core.ResolvedEdit{
			StartByte: n.StartByte(),
			EndByte:   n.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return resolved, nil
}

// wholeSymbol returns "doc + declaration" for the symbol matching id: the bytes
// from the start of the contiguous preceding comments through the end of the
// declaration node.
func wholeSymbol(src core.Source, id core.SymbolID) (string, error) {
	b := src.Bytes()
	n := find(src.Root(), b, id)
	if n == nil {
		return "", core.ErrSymbolNotFound
	}
	return string(b[docStart(n, b):n.EndByte()]), nil
}

// located pairs a resolved SymbolID with the declaration node that produced it.
type located struct {
	id   core.SymbolID
	node *ts.Node
}

// collect walks the tree in document order and returns every addressable symbol:
// free functions and methods (function_item / function_signature_item), structs
// and unions (KindStruct), and traits (KindInterface).
func collect(root *ts.Node, b []byte) []located {
	var out []located
	var visit func(n *ts.Node)
	visit = func(n *ts.Node) {
		count := n.NamedChildCount()
		for i := uint(0); i < count; i++ {
			c := n.NamedChild(i)
			switch c.Kind() {
			case "function_item", "function_signature_item":
				if name := fieldText(c, "name", b); name != "" {
					container := containerOf(c, b)
					kind := core.KindFunc
					if container != "" {
						kind = core.KindMethod
					}
					out = append(out, located{core.SymbolID{Kind: kind, Name: name, Container: container}, c})
				}
			case "struct_item", "union_item":
				if name := fieldText(c, "name", b); name != "" {
					out = append(out, located{core.SymbolID{Kind: core.KindStruct, Name: name}, c})
				}
			case "trait_item":
				if name := fieldText(c, "name", b); name != "" {
					out = append(out, located{core.SymbolID{Kind: core.KindInterface, Name: name}, c})
				}
			}
			visit(c)
		}
	}
	visit(root)
	return out
}

// find returns the declaration node whose resolved identity equals id, or nil.
func find(root *ts.Node, b []byte, id core.SymbolID) *ts.Node {
	for _, l := range collect(root, b) {
		if l.id == id {
			return l.node
		}
	}
	return nil
}

// containerOf resolves SymbolID.Container for a function node: the type of the
// nearest enclosing `impl` block, the name of the nearest enclosing `trait`, or ""
// when the function is free or nested inside another function/closure.
func containerOf(n *ts.Node, b []byte) string {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.Kind() {
		case "impl_item":
			return fieldText(p, "type", b)
		case "trait_item":
			return fieldText(p, "name", b)
		case "function_item", "closure_expression":
			// Nested function/closure: treat as top-level (no container).
			return ""
		}
	}
	return ""
}

// params returns the textual parameter declarations of a function node, in order
// (including a leading `self` receiver when present), or nil if it has none.
func params(n *ts.Node, b []byte) []string {
	pl := n.ChildByFieldName("parameters")
	if pl == nil {
		return nil
	}
	count := pl.NamedChildCount()
	if count == 0 {
		return nil
	}
	out := make([]string, 0, count)
	for i := uint(0); i < count; i++ {
		out = append(out, pl.NamedChild(i).Utf8Text(b))
	}
	return out
}

// fieldText returns the text of n's named field, or "" if the field is absent.
func fieldText(n *ts.Node, field string, b []byte) string {
	if c := n.ChildByFieldName(field); c != nil {
		return c.Utf8Text(b)
	}
	return ""
}

// docText returns the contiguous preceding-comment block of n as text, with any
// trailing whitespace trimmed; "" when there is no doc comment.
func docText(n *ts.Node, b []byte) string {
	ds := docStart(n, b)
	if ds == n.StartByte() {
		return ""
	}
	return strings.TrimRight(string(b[ds:n.StartByte()]), " \t\r\n")
}

// docStart returns the start byte of the contiguous block of line/block comments
// immediately preceding n (no blank line between them and n), or n's own start
// byte when n has no preceding comment. A comment node already includes its own
// trailing newline, so any further newline in the gap below a comment marks a
// blank line and ends the doc block.
func docStart(n *ts.Node, b []byte) uint {
	start := n.StartByte()
	for prev := n.PrevSibling(); prev != nil; prev = prev.PrevSibling() {
		if k := prev.Kind(); k != "line_comment" && k != "block_comment" {
			break
		}
		if bytes.IndexByte(b[prev.EndByte():start], '\n') >= 0 {
			break // blank line gap: not part of this doc block
		}
		start = prev.StartByte()
	}
	return start
}
