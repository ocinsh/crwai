// Package typescript implements the TypeScript language for crwai. It follows the
// shape of the reference implementation in lang/golang: a TypeScript type that
// satisfies core.Language (the read capabilities) plus core.FunctionWriter (write
// support), walking a tree-sitter-typescript parse tree to answer by symbol
// identity.
//
// TypeScript-specific notes:
//   - Two grammars: the tree-sitter-typescript module ships a "typescript" grammar
//     (pure .ts) and a "tsx" grammar (.tsx, JSX-aware). They are registered as two
//     core.Language values — TypeScript (.ts) and TSX (.tsx) — sharing one
//     implementation; only Name/Extensions/Parse differ (see TSX). The shared
//     read/write logic is grammar-agnostic: the node kinds it targets exist in both.
//   - Documentation: contiguous preceding-sibling comments above the declaration,
//     primarily JSDoc `/** ... */` blocks, with `//` line comments as a fallback. A
//     blank line between a comment and the declaration severs the association.
//   - SymbolID.Container: the enclosing class/interface name for a method; the
//     namespace/module name for a function declared inside it; "" for a top-level
//     function. Top-level `const f = () => {}` / `function f() {}` are functions
//     with an empty container.
//   - SymbolID.Kind: TypeScript has real `interface` declarations (KindInterface).
//     A class maps to KindStruct, as does an object-typed `type` alias
//     (`type P = { ... }`). Every container-scoped callable is KindMethod, matching
//     how the engine addresses "name + container".
//   - Symbols handled: function_declaration, generator_function_declaration,
//     class_declaration / abstract_class_declaration and their method members,
//     interface_declaration and its method signatures, object-typed type aliases,
//     namespace/module bodies, and top-level lexical/var declarations whose value
//     is an arrow/function expression. Each is also unwrapped from a leading
//     `export` statement.
package typescript

import (
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// TypeScript is the TypeScript (.ts) language implementation. The zero value is
// ready to use; it holds no per-call state (a fresh Source is parsed per call).
type TypeScript struct{}

// TSX is the TypeScript JSX (.tsx) language implementation. It reuses the whole
// TypeScript read/write logic and only swaps the grammar, name, and extension —
// the tsx grammar is required to parse JSX expressions in .tsx files.
type TSX struct{ TypeScript }

// Compile-time assertions: both variants satisfy the read interfaces (via
// core.Language) and the optional write capability.
var (
	_ core.Language       = (*TypeScript)(nil)
	_ core.FunctionWriter = (*TypeScript)(nil)
	_ core.Language       = (*TSX)(nil)
	_ core.FunctionWriter = (*TSX)(nil)
)

// Name returns the canonical language name.
func (TypeScript) Name() string { return "typescript" }

// Extensions returns the file extensions handled by the pure TypeScript grammar.
func (TypeScript) Extensions() []string { return []string{".ts"} }

// Parse loads src into a parsed Source using the pure TypeScript grammar. The
// caller owns the returned Source and must Close it (via defer).
func (TypeScript) Parse(src []byte) (core.Source, error) {
	return core.Parse(sitter.NewLanguage(tsts.LanguageTypescript()), src)
}

// Name returns the canonical language name for the TSX variant.
func (TSX) Name() string { return "tsx" }

// Extensions returns the file extension handled by the TSX (JSX-aware) grammar.
func (TSX) Extensions() []string { return []string{".tsx"} }

// Parse loads src into a parsed Source using the TSX grammar, which understands
// JSX expressions. The caller owns the returned Source and must Close it.
func (TSX) Parse(src []byte) (core.Source, error) {
	return core.Parse(sitter.NewLanguage(tsts.LanguageTSX()), src)
}

// ListSignatures returns one Signature per addressable symbol (functions,
// methods, classes, interfaces, object-typed type aliases), each carrying its
// parameters, return-type annotation, and contiguous preceding-comment doc.
// Results are in source order so the output is deterministic.
func (TypeScript) ListSignatures(src core.Source) ([]core.Signature, error) {
	b := src.Bytes()
	syms := collect(src.Root(), b)
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		out = append(out, core.Signature{
			Kind:      s.id.Kind,
			Name:      s.id.Name,
			Container: s.id.Container,
			Text:      signatureText(s, b),
			Params:    s.params,
			Returns:   s.returns,
			Doc:       s.doc,
		})
	}
	return out, nil
}

// signatureText returns the verbatim signature line of a callable symbol: the
// source from the declaration start up to (but not including) its body, keeping
// the name, type parameters, parameters and return annotation as written.
// Classes, interfaces and body-less symbols yield "".
func signatureText(s sym, b []byte) string {
	if s.text == nil || s.id.Kind == core.KindStruct || s.id.Kind == core.KindInterface {
		return ""
	}
	// With a body, slice up to it; a body-less method signature (interface/abstract)
	// is the whole node, minus its trailing semicolon.
	end := s.text.EndByte()
	if s.body != nil {
		end = s.body.StartByte()
	}
	return strings.TrimRight(strings.TrimSpace(string(b[s.text.StartByte():end])), " \t\n;")
}

// FunctionBody locates the function/method matching id and returns the text of its
// body — the statement block (braces included) or, for an expression-bodied arrow,
// the expression text. A body-less symbol (e.g. an interface method signature)
// reports core.ErrSymbolNotFound.
func (TypeScript) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	s, ok := find(src, id)
	if !ok || s.body == nil {
		return "", core.ErrSymbolNotFound
	}
	return s.body.Utf8Text(src.Bytes()), nil
}

// Function locates the function/method matching id and returns its whole text:
// contiguous doc comments + signature + body.
func (TypeScript) Function(src core.Source, id core.SymbolID) (string, error) {
	s, ok := find(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(s.doc, s.text.Utf8Text(src.Bytes())), nil
}

// ReadInterface locates the interface matching id and returns its full
// declaration text (doc comments included).
func (TypeScript) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	want := core.SymbolID{Kind: core.KindInterface, Name: id.Name}
	for _, s := range collect(src.Root(), src.Bytes()) {
		if s.id == want {
			return withDoc(s.doc, s.text.Utf8Text(src.Bytes())), nil
		}
	}
	return "", core.ErrSymbolNotFound
}

// ReadStruct locates the class (or object-typed type alias) matching id —
// KindStruct maps to a TypeScript class — and returns its full declaration text.
func (TypeScript) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	want := core.SymbolID{Kind: core.KindStruct, Name: id.Name, Container: id.Container}
	for _, s := range collect(src.Root(), src.Bytes()) {
		if s.id == want {
			return withDoc(s.doc, s.text.Utf8Text(src.Bytes())), nil
		}
	}
	return "", core.ErrSymbolNotFound
}

// ResolveEdits maps each Edit to the [start, end) byte span of its whole target
// symbol on src. It does not mutate src or touch disk — core.BatchWrite orders,
// overlap-checks, applies, re-parses, and persists. Relative-range edits are not
// implemented in v1 (core.ErrRelativeRangeNotImplemented).
func (TypeScript) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
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
//   - body is the statement block / expression body (nil for a class, interface,
//     type alias, or body-less signature).
//   - doc is the extracted documentation ("" when none).
//   - params are the textual parameter declarations, in order.
//   - returns is the return-type annotation, leading colon stripped ("" when none).
type sym struct {
	id      core.SymbolID
	text    *sitter.Node
	body    *sitter.Node
	doc     string
	params  []string
	returns string
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

// collect walks the top level of the program and returns every addressable
// symbol, descending into classes, interfaces, and namespaces/modules.
func collect(root *sitter.Node, src []byte) []sym {
	var out []sym
	for i := uint(0); i < root.NamedChildCount(); i++ {
		out = append(out, fromStatement(root.NamedChild(i), src, "")...)
	}
	return out
}

// fromStatement turns one statement (reachable for doc/text via itself) into zero
// or more symbols, scoped to container. It unwraps `export` statements and the
// `expression_statement` wrapper that the grammar places around a `namespace`.
func fromStatement(child *sitter.Node, src []byte, container string) []sym {
	anchor := child
	inner := child
	switch child.Kind() {
	case "export_statement":
		decl := declarationOf(child)
		if decl == nil {
			return nil
		}
		inner = decl // keep the export statement as the doc/text anchor
	case "expression_statement":
		// `namespace Foo { ... }` parses as expression_statement > internal_module.
		if child.NamedChildCount() == 1 {
			inner = child.NamedChild(0)
		}
	}
	return symbolsFrom(anchor, inner, src, container)
}

// declarationOf returns the declaration carried by an export_statement, if any.
func declarationOf(export *sitter.Node) *sitter.Node {
	if d := export.ChildByFieldName("declaration"); d != nil {
		return d
	}
	for i := uint(0); i < export.NamedChildCount(); i++ {
		switch c := export.NamedChild(i); c.Kind() {
		case "function_declaration", "generator_function_declaration",
			"class_declaration", "abstract_class_declaration",
			"interface_declaration", "type_alias_declaration",
			"lexical_declaration", "variable_declaration",
			"internal_module", "module":
			return c
		}
	}
	return nil
}

// symbolsFrom turns one declaration (inner), reachable for text/doc via anchor,
// into zero or more symbols, scoped to container.
func symbolsFrom(anchor, inner *sitter.Node, src []byte, container string) []sym {
	switch inner.Kind() {
	case "function_declaration", "generator_function_declaration":
		name := inner.ChildByFieldName("name")
		if name == nil {
			return nil
		}
		return []sym{{
			id:      callableID(name.Utf8Text(src), container),
			text:    anchor,
			body:    inner.ChildByFieldName("body"),
			doc:     docFor(anchor, src),
			params:  paramTexts(inner, src),
			returns: returnText(inner, src),
		}}

	case "class_declaration", "abstract_class_declaration":
		return classSymbols(anchor, inner, src)

	case "interface_declaration":
		return interfaceSymbols(anchor, inner, src)

	case "type_alias_declaration":
		// Only object-typed aliases (`type P = { ... }`) map to a struct.
		if val := inner.ChildByFieldName("value"); val == nil || val.Kind() != "object_type" {
			return nil
		}
		name := inner.ChildByFieldName("name")
		if name == nil {
			return nil
		}
		return []sym{{
			id:   core.SymbolID{Kind: core.KindStruct, Name: name.Utf8Text(src)},
			text: anchor,
			doc:  docFor(anchor, src),
		}}

	case "lexical_declaration", "variable_declaration":
		return varSymbols(anchor, inner, src, container)

	case "internal_module", "module":
		return moduleSymbols(inner, src)
	}
	return nil
}

// classSymbols returns the class itself (KindStruct) plus every method member
// (KindMethod, container = class name).
func classSymbols(anchor, inner *sitter.Node, src []byte) []sym {
	name := nameText(inner, src)
	if name == "" {
		return nil
	}
	out := []sym{{
		id:   core.SymbolID{Kind: core.KindStruct, Name: name},
		text: anchor,
		doc:  docFor(anchor, src),
	}}
	body := inner.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		m := body.NamedChild(i)
		switch m.Kind() {
		case "method_definition", "method_signature", "abstract_method_signature":
			mname := m.ChildByFieldName("name")
			if mname == nil {
				continue
			}
			out = append(out, sym{
				id:      core.SymbolID{Kind: core.KindMethod, Name: mname.Utf8Text(src), Container: name},
				text:    m,
				body:    m.ChildByFieldName("body"),
				doc:     docFor(m, src),
				params:  paramTexts(m, src),
				returns: returnText(m, src),
			})
		}
	}
	return out
}

// interfaceSymbols returns the interface itself (KindInterface) plus each method
// signature (KindMethod, container = interface name, body-less).
func interfaceSymbols(anchor, inner *sitter.Node, src []byte) []sym {
	name := nameText(inner, src)
	if name == "" {
		return nil
	}
	out := []sym{{
		id:   core.SymbolID{Kind: core.KindInterface, Name: name},
		text: anchor,
		doc:  docFor(anchor, src),
	}}
	body := inner.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		m := body.NamedChild(i)
		if m.Kind() != "method_signature" {
			continue
		}
		mname := m.ChildByFieldName("name")
		if mname == nil {
			continue
		}
		out = append(out, sym{
			id:      core.SymbolID{Kind: core.KindMethod, Name: mname.Utf8Text(src), Container: name},
			text:    m,
			doc:     docFor(m, src),
			params:  paramTexts(m, src),
			returns: returnText(m, src),
		})
	}
	return out
}

// varSymbols returns one symbol per variable declarator whose value is a function
// form (arrow or function expression), scoped to container.
func varSymbols(anchor, inner *sitter.Node, src []byte, container string) []sym {
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
			id:      callableID(name.Utf8Text(src), container),
			text:    anchor,
			body:    val.ChildByFieldName("body"),
			doc:     docFor(anchor, src),
			params:  paramTexts(val, src),
			returns: returnText(val, src),
		})
	}
	return out
}

// moduleSymbols descends into a namespace/module body and collects its members,
// scoping each to the module name. The module itself is not an addressable symbol.
func moduleSymbols(inner *sitter.Node, src []byte) []sym {
	name := inner.ChildByFieldName("name")
	body := inner.ChildByFieldName("body")
	if name == nil || body == nil {
		return nil
	}
	container := name.Utf8Text(src)
	var out []sym
	for i := uint(0); i < body.NamedChildCount(); i++ {
		out = append(out, fromStatement(body.NamedChild(i), src, container)...)
	}
	return out
}

// callableID builds the identity of a callable: KindFunc when top-level, KindMethod
// when scoped to a container (class, interface, or namespace). This matches how the
// engine addresses a symbol — a non-empty container always means KindMethod.
func callableID(name, container string) core.SymbolID {
	if container == "" {
		return core.SymbolID{Kind: core.KindFunc, Name: name}
	}
	return core.SymbolID{Kind: core.KindMethod, Name: name, Container: container}
}

// isFunctionValue reports whether a variable-declarator value is a function form.
func isFunctionValue(kind string) bool {
	switch kind {
	case "arrow_function", "function_expression", "generator_function":
		return true
	}
	return false
}

// nameText returns the declared name of a type declaration: the "name" field if
// present, else the first identifier-like named child (covers the abstract-class
// form whose name is not exposed under the "name" field in every grammar build).
func nameText(decl *sitter.Node, src []byte) string {
	if n := decl.ChildByFieldName("name"); n != nil {
		return n.Utf8Text(src)
	}
	for i := uint(0); i < decl.NamedChildCount(); i++ {
		switch c := decl.NamedChild(i); c.Kind() {
		case "type_identifier", "identifier":
			return c.Utf8Text(src)
		}
	}
	return ""
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

// returnText returns the return-type annotation of a function-like node with the
// leading colon stripped ("" when the node has no explicit return type).
func returnText(fn *sitter.Node, src []byte) string {
	rt := fn.ChildByFieldName("return_type")
	if rt == nil {
		return ""
	}
	t := strings.TrimSpace(rt.Utf8Text(src))
	t = strings.TrimSpace(strings.TrimPrefix(t, ":"))
	return t
}

// docFor returns the documentation for the declaration anchored at node: the run
// of comment siblings immediately preceding it, with no blank line between any two
// of them or between the closest comment and the declaration. JSDoc `/** ... */`
// blocks and `//` line comments are both `comment` nodes, so both are captured.
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
