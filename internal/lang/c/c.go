// Package c implements the C language for the crwai server (see lang/golang for
// the documented reference and lang/TEMPLATE.md for the recipe). It reads and
// rewrites C source at the symbol level using the tree-sitter-c grammar.
//
// C-specific notes:
//   - Documentation: contiguous preceding-sibling `//` or `/* ... */` comments
//     immediately above the declaration (a blank line breaks the association).
//   - SymbolID.Container: always "" — C has no methods or enclosing types for
//     functions, and structs are top-level. C has no interfaces, so ReadInterface
//     always reports core.ErrSymbolNotFound.
//   - Symbols recognised: top-level `function_definition` (KindFunc) and
//     struct definitions (KindStruct), both as bare `struct_specifier` and as
//     `typedef struct { ... } Name;` (indexed by the typedef name).
package c

import (
	"fmt"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

// C is the C language implementation. The zero value is ready to use; it holds
// no per-call state (the server is stateless and re-parses every call).
type C struct{}

var (
	_ core.Language       = (*C)(nil)
	_ core.FunctionWriter = (*C)(nil)
)

// Name returns the canonical language name used as a registry key.
func (C) Name() string { return "c" }

// Extensions returns the file extensions handled by the C language.
func (C) Extensions() []string { return []string{".c", ".h"} }

// Parse configures a tree-sitter parser with the C grammar and parses src into a
// Source. The caller must Close the returned Source via defer.
func (C) Parse(src []byte) (core.Source, error) {
	return core.Parse(sitter.NewLanguage(tree_sitter_c.Language()), src)
}

// symbol is a located top-level symbol: its identity, the whole-symbol node, and
// the documentation comment block extracted above it.
type symbol struct {
	id   core.SymbolID
	node *sitter.Node
	doc  string
}

// ListSignatures returns the signature of every top-level symbol in the file:
// functions (with parameters and return type) and struct definitions.
func (c C) ListSignatures(src core.Source) ([]core.Signature, error) {
	b := src.Bytes()
	syms := c.symbols(src)
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		sig := core.Signature{Kind: s.id.Kind, Name: s.id.Name, Container: s.id.Container, Doc: s.doc}
		if s.id.Kind == core.KindFunc {
			fd := functionDeclarator(s.node.ChildByFieldName("declarator"))
			sig.Params = params(fd, b)
			sig.Returns = returnType(s.node, b)
			// Verbatim signature: the function_definition up to its body, so the
			// full return type (pointer included) and parameters read as written.
			if body := s.node.ChildByFieldName("body"); body != nil {
				sig.Text = strings.TrimSpace(string(b[s.node.StartByte():body.StartByte()]))
			}
		}
		out = append(out, sig)
	}
	return out, nil
}

// FunctionBody returns only the body (the brace-delimited compound statement) of
// the named function, without its signature or documentation.
func (c C) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	s, ok := c.find(src, core.KindFunc, id.Name)
	if !ok {
		return "", fmt.Errorf("%w: function %s", core.ErrSymbolNotFound, id.Name)
	}
	body := s.node.ChildByFieldName("body")
	if body == nil {
		return "", fmt.Errorf("%w: body of %s", core.ErrSymbolNotFound, id.Name)
	}
	return body.Utf8Text(src.Bytes()), nil
}

// Function returns the whole named function: its documentation (if any) followed
// by the signature and body.
func (c C) Function(src core.Source, id core.SymbolID) (string, error) {
	s, ok := c.find(src, core.KindFunc, id.Name)
	if !ok {
		return "", fmt.Errorf("%w: function %s", core.ErrSymbolNotFound, id.Name)
	}
	text := s.node.Utf8Text(src.Bytes())
	if s.doc != "" {
		return s.doc + "\n" + text, nil
	}
	return text, nil
}

// ReadInterface always reports core.ErrSymbolNotFound: C has no interface
// construct. The capability exists only to satisfy the aggregate Language
// interface; the read tool surfaces the typed error to the caller.
func (C) ReadInterface(_ core.Source, id core.SymbolID) (string, error) {
	return "", fmt.Errorf("%w: C has no interface %q", core.ErrSymbolNotFound, id.Name)
}

// ReadStruct returns the full definition of the named struct, covering both a
// bare `struct Name { ... }` and a `typedef struct { ... } Name;` (located by the
// typedef name).
func (c C) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	s, ok := c.find(src, core.KindStruct, id.Name)
	if !ok {
		return "", fmt.Errorf("%w: struct %s", core.ErrSymbolNotFound, id.Name)
	}
	return s.node.Utf8Text(src.Bytes()), nil
}

// ResolveEdits maps each Edit to the byte span of its whole target symbol. It
// performs no I/O and no ordering — the core write pipeline orders, overlap-checks
// and applies the spans. A relative-range edit reports
// core.ErrRelativeRangeNotImplemented; an unresolved symbol fails the whole batch.
func (c C) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	syms := c.symbols(src)
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, fmt.Errorf("%w: %s", core.ErrRelativeRangeNotImplemented, e.Target.Name)
		}
		var found *symbol
		for i := range syms {
			if syms[i].id.Kind == e.Target.Kind && syms[i].id.Name == e.Target.Name {
				found = &syms[i]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("%w: %s %q", core.ErrSymbolNotFound, e.Target.Kind, e.Target.Name)
		}
		out = append(out, core.ResolvedEdit{
			StartByte: found.node.StartByte(),
			EndByte:   found.node.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return out, nil
}

// symbols walks the top-level named children of the parse tree and collects the
// functions and struct definitions, each with its documentation block. The walk
// is read-only; it never re-parses.
func (C) symbols(src core.Source) []symbol {
	root := src.Root()
	b := src.Bytes()
	var out []symbol
	for i := uint(0); i < root.NamedChildCount(); i++ {
		n := root.NamedChild(i)
		switch n.Kind() {
		case "function_definition":
			fd := functionDeclarator(n.ChildByFieldName("declarator"))
			if fd == nil {
				continue
			}
			name := fd.ChildByFieldName("declarator")
			if name == nil {
				continue
			}
			out = append(out, symbol{
				id:   core.SymbolID{Kind: core.KindFunc, Name: name.Utf8Text(b)},
				node: n,
				doc:  precedingDoc(n, b),
			})
		case "struct_specifier", "union_specifier", "enum_specifier":
			if name := n.ChildByFieldName("name"); name != nil {
				out = append(out, symbol{
					id:   core.SymbolID{Kind: core.KindStruct, Name: name.Utf8Text(b)},
					node: n,
					doc:  precedingDoc(n, b),
				})
			}
		case "type_definition":
			t := n.ChildByFieldName("type")
			if t == nil {
				continue
			}
			switch t.Kind() {
			case "struct_specifier", "union_specifier", "enum_specifier":
				if name := n.ChildByFieldName("declarator"); name != nil {
					out = append(out, symbol{
						id:   core.SymbolID{Kind: core.KindStruct, Name: name.Utf8Text(b)},
						node: n,
						doc:  precedingDoc(n, b),
					})
				}
			}
		}
	}
	return out
}

// find returns the first top-level symbol matching kind and name.
func (c C) find(src core.Source, kind core.SymbolKind, name string) (symbol, bool) {
	for _, s := range c.symbols(src) {
		if s.id.Kind == kind && s.id.Name == name {
			return s, true
		}
	}
	return symbol{}, false
}

// returnType renders a function's return type, recovering the pointer levels that
// C parks on the declarator rather than on the `type` field. For
// `struct list *list(void)` the `type` field is just `struct list` while the `*`
// sits on the pointer_declarator wrapping the function_declarator; reading the
// type field alone would silently drop the pointer from the listed signature.
func returnType(fn *sitter.Node, b []byte) string {
	t := fn.ChildByFieldName("type")
	if t == nil {
		return ""
	}
	ret := strings.TrimSpace(t.Utf8Text(b))
	stars := 0
	for decl := fn.ChildByFieldName("declarator"); decl != nil; {
		switch decl.Kind() {
		case "pointer_declarator":
			stars++
			decl = decl.ChildByFieldName("declarator")
		case "parenthesized_declarator":
			decl = decl.ChildByFieldName("declarator")
		default:
			decl = nil
		}
	}
	if stars > 0 {
		ret += " " + strings.Repeat("*", stars)
	}
	return ret
}

// functionDeclarator descends through pointer/parenthesized declarators to reach
// the function_declarator that carries the function's name and parameter list
// (e.g. `struct list *list(void)` wraps the function_declarator in a
// pointer_declarator). It returns nil if no function_declarator is found.
func functionDeclarator(decl *sitter.Node) *sitter.Node {
	for decl != nil {
		switch decl.Kind() {
		case "function_declarator":
			return decl
		case "pointer_declarator", "parenthesized_declarator":
			decl = decl.ChildByFieldName("declarator")
		default:
			return nil
		}
	}
	return nil
}

// params returns the textual parameter declarations of a function_declarator, in
// order (e.g. ["int a", "int b"]). A `(void)` parameter list yields ["void"].
func params(fd *sitter.Node, src []byte) []string {
	if fd == nil {
		return nil
	}
	pl := fd.ChildByFieldName("parameters")
	if pl == nil {
		return nil
	}
	var ps []string
	for i := uint(0); i < pl.NamedChildCount(); i++ {
		c := pl.NamedChild(i)
		switch c.Kind() {
		case "parameter_declaration", "variadic_parameter":
			ps = append(ps, strings.TrimSpace(c.Utf8Text(src)))
		}
	}
	return ps
}

// precedingDoc collects the block of comment nodes immediately above n. Comments
// are gathered upward as long as each is contiguous (no blank line) with the line
// below it; a non-comment sibling or a blank-line gap stops the walk. The result
// preserves source order and original comment text, joined by newlines.
func precedingDoc(n *sitter.Node, src []byte) string {
	var comments []*sitter.Node
	below := n.StartPosition().Row
	for cur := n.PrevSibling(); cur != nil && cur.Kind() == "comment"; cur = cur.PrevSibling() {
		if below-cur.EndPosition().Row > 1 {
			break // blank line between this comment and the block below it
		}
		comments = append(comments, cur)
		below = cur.StartPosition().Row
	}
	if len(comments) == 0 {
		return ""
	}
	parts := make([]string, 0, len(comments))
	for i := len(comments) - 1; i >= 0; i-- { // reverse into source order
		parts = append(parts, comments[i].Utf8Text(src))
	}
	return strings.Join(parts, "\n")
}
