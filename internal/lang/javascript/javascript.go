// Package javascript implements the JavaScript language for crwai. It follows the
// shape of the reference skeleton in lang/golang: a JavaScript type that satisfies
// core.Language (the read capabilities) plus core.FunctionWriter (write support).
//
// JavaScript-specific notes:
//   - Documentation: preceding-sibling `//` line comments or `/* ... */` (incl.
//     `/** ... */` JSDoc) blocks that sit contiguously above the declaration. A
//     blank line between a comment and the declaration severs the association.
//   - SymbolID.Container: the enclosing class name for a method; "" for a
//     top-level function. Top-level `const f = () => {}` / `function f() {}` are
//     functions with an empty container.
//   - JavaScript has no interface construct, so ReadInterface always reports
//     core.ErrSymbolNotFound; the method exists only because core.Language
//     mandates it.
//   - Symbols handled: function_declaration, generator_function_declaration,
//     class_declaration and its method_definition members, and top-level
//     lexical/var declarations whose value is an arrow/function expression. Each
//     of these is also unwrapped from a leading `export` statement.
package javascript

import (
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

// JavaScript is the JavaScript language implementation. The zero value is ready to
// use; it holds no per-call state (a fresh Source is parsed per call).
type JavaScript struct{}

// Compile-time assertions: JavaScript satisfies the read interfaces (via
// core.Language) and the optional write capability.
var (
	_ core.Language       = (*JavaScript)(nil)
	_ core.FunctionWriter = (*JavaScript)(nil)
)

// Name returns the canonical language name.
func (JavaScript) Name() string { return "javascript" }

// Extensions returns the file extensions handled by this language.
func (JavaScript) Extensions() []string { return []string{".js", ".jsx", ".mjs", ".cjs"} }

// Parse loads src into a parsed Source via the shared core parse helper. The
// caller owns the returned Source and must Close it (via defer).
func (JavaScript) Parse(src []byte) (core.Source, error) {
	return core.Parse(sitter.NewLanguage(tsjs.Language()), src)
}

// ListSignatures returns one Signature per addressable symbol (functions,
// methods, and classes), attaching contiguous preceding-comment docs. No bodies
// are loaded beyond what the signature needs.
func (JavaScript) ListSignatures(src core.Source) ([]core.Signature, error) {
	syms := collect(src.Root(), src.Bytes())
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		out = append(out, core.Signature{
			Name:    s.id.Name,
			Params:  s.params,
			Returns: "", // plain JavaScript carries no return-type annotations
			Doc:     s.doc,
		})
	}
	return out, nil
}

// FunctionBody locates the function/method matching id and returns the text of its
// body — the statement block (braces included) or, for an expression-bodied arrow,
// the expression text.
func (JavaScript) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	s, ok := find(src, id)
	if !ok || s.body == nil {
		return "", core.ErrSymbolNotFound
	}
	return s.body.Utf8Text(src.Bytes()), nil
}

// Function locates the function/method matching id and returns its whole text:
// contiguous doc comments + signature + body.
func (JavaScript) Function(src core.Source, id core.SymbolID) (string, error) {
	s, ok := find(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(s.doc, s.text.Utf8Text(src.Bytes())), nil
}

// ReadInterface always reports core.ErrSymbolNotFound: JavaScript has no interface
// construct. The method exists only to satisfy core.Language.
func (JavaScript) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	return "", core.ErrSymbolNotFound
}

// ReadStruct locates the class matching id (KindStruct maps to a JavaScript class)
// and returns its full declaration text.
func (JavaScript) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	want := core.SymbolID{Kind: core.KindStruct, Name: id.Name, Container: id.Container}
	for _, s := range collect(src.Root(), src.Bytes()) {
		if s.id == want {
			return s.text.Utf8Text(src.Bytes()), nil
		}
	}
	return "", core.ErrSymbolNotFound
}

// ResolveEdits maps each Edit to the [start, end) byte span of its whole target
// symbol on src. It does not mutate src or touch disk — core.BatchWrite orders,
// overlap-checks, applies, re-parses, and persists. Relative-range edits are not
// implemented in v1 (core.ErrRelativeRangeNotImplemented).
func (JavaScript) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	syms := collect(src.Root(), src.Bytes())
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, core.ErrRelativeRangeNotImplemented
		}
		s, ok := lookup(syms, e.Target)
		if !ok {
			return nil, core.ErrSymbolNotFound
		}
		out = append(out, core.ResolvedEdit{
			StartByte: s.text.StartByte(),
			EndByte:   s.text.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return out, nil
}

// sym is the internal descriptor of a located symbol.
//
//   - text is the node whose Utf8Text is the WHOLE symbol (and whose preceding
//     siblings hold the doc comments) — the declaration itself or its wrapping
//     `export` statement.
//   - body is the statement block / expression body (nil for a class).
//   - doc is the extracted documentation ("" when none).
//   - params are the textual parameter declarations, in order.
type sym struct {
	id     core.SymbolID
	text   *sitter.Node
	body   *sitter.Node
	doc    string
	params []string
}

// find parses the symbols of src and returns the one matching id.
func find(src core.Source, id core.SymbolID) (sym, bool) {
	return lookup(collect(src.Root(), src.Bytes()), id)
}

// lookup returns the symbol whose identity equals id.
func lookup(syms []sym, id core.SymbolID) (sym, bool) {
	for _, s := range syms {
		if s.id == id {
			return s, true
		}
	}
	return sym{}, false
}

// collect walks the top level of the program and returns every addressable symbol,
// descending into classes to gather their methods.
func collect(root *sitter.Node, src []byte) []sym {
	var out []sym
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		anchor := child // doc + whole-text anchor (the statement itself)
		inner := child
		if child.Kind() == "export_statement" {
			if decl := declarationOf(child); decl != nil {
				inner = decl // unwrap `export <decl>`, keep the export as the anchor
			}
		}
		out = append(out, symbolsFrom(anchor, inner, src)...)
	}
	return out
}

// declarationOf returns the declaration carried by an export_statement, if any.
func declarationOf(export *sitter.Node) *sitter.Node {
	if d := export.ChildByFieldName("declaration"); d != nil {
		return d
	}
	for i := uint(0); i < export.NamedChildCount(); i++ {
		switch c := export.NamedChild(i); c.Kind() {
		case "function_declaration", "generator_function_declaration",
			"class_declaration", "lexical_declaration", "variable_declaration":
			return c
		}
	}
	return nil
}

// symbolsFrom turns one top-level declaration (inner), reachable for text/doc via
// anchor, into zero or more symbols.
func symbolsFrom(anchor, inner *sitter.Node, src []byte) []sym {
	switch inner.Kind() {
	case "function_declaration", "generator_function_declaration":
		name := inner.ChildByFieldName("name")
		if name == nil {
			return nil
		}
		return []sym{{
			id:     core.SymbolID{Kind: core.KindFunc, Name: name.Utf8Text(src)},
			text:   anchor,
			body:   inner.ChildByFieldName("body"),
			doc:    docFor(anchor, src),
			params: paramTexts(inner, src),
		}}

	case "class_declaration":
		name := inner.ChildByFieldName("name")
		if name == nil {
			return nil
		}
		className := name.Utf8Text(src)
		out := []sym{{
			id:   core.SymbolID{Kind: core.KindStruct, Name: className},
			text: anchor,
			doc:  docFor(anchor, src),
		}}
		if body := inner.ChildByFieldName("body"); body != nil {
			for i := uint(0); i < body.NamedChildCount(); i++ {
				m := body.NamedChild(i)
				if m.Kind() != "method_definition" {
					continue
				}
				mname := m.ChildByFieldName("name")
				if mname == nil {
					continue
				}
				out = append(out, sym{
					id:     core.SymbolID{Kind: core.KindMethod, Name: mname.Utf8Text(src), Container: className},
					text:   m,
					body:   m.ChildByFieldName("body"),
					doc:    docFor(m, src),
					params: paramTexts(m, src),
				})
			}
		}
		return out

	case "lexical_declaration", "variable_declaration":
		var out []sym
		for i := uint(0); i < inner.NamedChildCount(); i++ {
			d := inner.NamedChild(i)
			if d.Kind() != "variable_declarator" {
				continue
			}
			val := d.ChildByFieldName("value")
			if val == nil || !isFunctionValue(val.Kind()) {
				continue
			}
			name := d.ChildByFieldName("name")
			if name == nil {
				continue
			}
			out = append(out, sym{
				id:     core.SymbolID{Kind: core.KindFunc, Name: name.Utf8Text(src)},
				text:   anchor,
				body:   val.ChildByFieldName("body"),
				doc:    docFor(anchor, src),
				params: paramTexts(val, src),
			})
		}
		return out
	}
	return nil
}

// isFunctionValue reports whether a variable-declarator value is a function form.
func isFunctionValue(kind string) bool {
	switch kind {
	case "arrow_function", "function_expression", "generator_function":
		return true
	}
	return false
}

// paramTexts returns the textual parameter declarations of a function-like node,
// in order. It handles both a formal_parameters list and the single-identifier
// parameter form of an arrow function (`x => x`).
func paramTexts(fn *sitter.Node, src []byte) []string {
	if list := fn.ChildByFieldName("parameters"); list != nil {
		out := make([]string, 0, list.NamedChildCount())
		for i := uint(0); i < list.NamedChildCount(); i++ {
			out = append(out, list.NamedChild(i).Utf8Text(src))
		}
		return out
	}
	if single := fn.ChildByFieldName("parameter"); single != nil {
		return []string{single.Utf8Text(src)}
	}
	return nil
}

// docFor returns the documentation for the declaration anchored at node: the run
// of comment siblings immediately preceding it, with no blank line between any two
// of them or between the closest comment and the declaration.
func docFor(node *sitter.Node, src []byte) string {
	var comments []*sitter.Node
	cur := node
	for {
		prev := cur.PrevSibling()
		if prev == nil || prev.Kind() != "comment" {
			break
		}
		if int(cur.StartPosition().Row)-int(prev.EndPosition().Row) > 1 {
			break // blank line severs the association
		}
		comments = append([]*sitter.Node{prev}, comments...)
		cur = prev
	}
	if len(comments) == 0 {
		return ""
	}
	parts := make([]string, len(comments))
	for i, c := range comments {
		parts[i] = c.Utf8Text(src)
	}
	return strings.Join(parts, "\n")
}

// withDoc prefixes doc (if any) to body, separated by a newline.
func withDoc(doc, body string) string {
	if doc == "" {
		return body
	}
	return doc + "\n" + body
}
