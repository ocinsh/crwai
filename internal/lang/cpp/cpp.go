// Package cpp implements the C++ language capabilities of the server: it parses a
// C++ source file with tree-sitter and answers queries by symbol identity (list
// signatures, read a function/struct, surgically rewrite a function).
//
// It is the first fully-implemented language; see lang/golang for the documented
// skeleton shape and lang/TEMPLATE.md for the recipe.
//
// C++-specific notes:
//   - Documentation: contiguous preceding-sibling `//`, `///`, or `/* ... */`
//     comments immediately above the declaration (no blank line in between).
//   - SymbolID.Container: the enclosing class/struct for a member function (for an
//     out-of-line definition `Ns::Cls::m` the container is `Ns::Cls`); "" for a
//     free function. A class or struct is addressed as KindStruct.
//   - C++ has no `interface` construct, so ReadInterface always reports
//     ErrInterfacesUnsupported (the Language aggregate forces the method to exist,
//     so it cannot simply be omitted via type assertion).
package cpp

import (
	"errors"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tscpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"

	"github.com/ocinsh/crwai/internal/core"
)

// Package-level typed errors for the conditions the contracts promise. Callers can
// branch with errors.Is; core.ErrSymbolNotFound is reused for "no such symbol".
var (
	// ErrInterfacesUnsupported is returned by ReadInterface: C++ has no interface
	// construct (the nearest idiom, an abstract class, is read as a struct/class).
	ErrInterfacesUnsupported = errors.New("cpp: interfaces are not a C++ construct; read the abstract class as a struct")

	// ErrRelativeRangeNotImplemented is returned when an Edit carries a non-nil
	// RelativeRange: v1 resolves only whole-symbol replacement.
	ErrRelativeRangeNotImplemented = errors.New("cpp: relative-range edits are not implemented")
)

// Cpp is the C++ language implementation. The zero value is ready to use; it holds
// no per-call state (the server is stateless — a fresh Source is parsed per call).
type Cpp struct{}

// Compile-time assertions: Cpp satisfies the read interfaces (via Language) and the
// optional write capability.
var (
	_ core.Language       = (*Cpp)(nil)
	_ core.FunctionWriter = (*Cpp)(nil)
)

// Name returns the canonical language name.
func (Cpp) Name() string { return "cpp" }

// Extensions returns the file extensions handled by this language.
func (Cpp) Extensions() []string {
	return []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"}
}

// Parse loads src into a parsed Source, delegating to the shared core helper. The
// caller owns the returned Source and must Close it (via defer).
func (Cpp) Parse(src []byte) (core.Source, error) {
	return core.Parse(sitter.NewLanguage(tscpp.Language()), src)
}

// symbol is a located declaration: its identity plus the byte spans the read and
// write capabilities need. It is computed by a single tree walk per call.
type symbol struct {
	id   core.SymbolID
	decl *sitter.Node // the declaration node (function_definition / template_declaration / class/struct_specifier)
	body *sitter.Node // the compound_statement for a function; nil for a type
	// docStart is the byte offset of the first contiguous doc comment above decl,
	// or decl.StartByte() when there is no doc. The "whole symbol" span used by
	// Function and ResolveEdits is [docStart, decl.EndByte()).
	docStart uint
}

// ListSignatures walks the tree once and returns one Signature per discovered
// symbol (free functions, methods, classes/structs), attaching contiguous
// preceding-comment documentation. No bodies are read.
func (c Cpp) ListSignatures(src core.Source) ([]core.Signature, error) {
	b := src.Bytes()
	syms := c.symbols(src)
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		out = append(out, c.signatureOf(s, b))
	}
	return out, nil
}

// FunctionBody locates the function/method matching id and returns only the text
// of its body block (the braces' interior, trimmed).
func (c Cpp) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	s, ok := c.find(src, id)
	if !ok || s.body == nil {
		return "", core.ErrSymbolNotFound
	}
	b := src.Bytes()
	start, end := s.body.StartByte(), s.body.EndByte()
	if end-start < 2 {
		return "", nil
	}
	return strings.TrimSpace(string(b[start+1 : end-1])), nil
}

// Function locates the function/method matching id and returns its full text:
// contiguous preceding doc comments + signature + body.
func (c Cpp) Function(src core.Source, id core.SymbolID) (string, error) {
	s, ok := c.find(src, id)
	if !ok || s.body == nil {
		return "", core.ErrSymbolNotFound
	}
	b := src.Bytes()
	return string(b[s.docStart:s.decl.EndByte()]), nil
}

// ReadInterface always reports ErrInterfacesUnsupported: C++ has no interface
// construct. (The method exists because core.Language mandates InterfaceReader.)
func (Cpp) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	return "", ErrInterfacesUnsupported
}

// ReadStruct locates the class/struct matching id by name and returns its full
// declaration text (the class_specifier/struct_specifier node).
func (c Cpp) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	b := src.Bytes()
	for _, s := range c.symbols(src) {
		if s.id.Kind == core.KindStruct && s.id.Name == id.Name {
			return string(b[s.decl.StartByte():s.decl.EndByte()]), nil
		}
	}
	return "", core.ErrSymbolNotFound
}

// ResolveEdits maps each Edit to a concrete byte span on src: it locates the target
// symbol by identity and returns the [docStart, declEnd) span to replace (the whole
// symbol, doc comments included, so that get_function output round-trips through a
// write). It does not mutate src or touch disk — core.BatchWrite orders,
// overlap-checks, applies, re-parses, and persists.
func (c Cpp) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, ErrRelativeRangeNotImplemented
		}
		s, ok := c.find(src, e.Target)
		if !ok {
			return nil, core.ErrSymbolNotFound
		}
		out = append(out, core.ResolvedEdit{
			StartByte: s.docStart,
			EndByte:   s.decl.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return out, nil
}

// find returns the symbol matching id (by kind, name and container). Functions and
// methods are matched by Name+Container; Kind disambiguates funcs/methods from
// types but KindFunc and KindMethod are treated interchangeably when the container
// already disambiguates (a method has a non-empty container).
func (c Cpp) find(src core.Source, id core.SymbolID) (symbol, bool) {
	for _, s := range c.symbols(src) {
		if s.id.Name != id.Name || s.id.Container != id.Container {
			continue
		}
		if isCallable(s.id.Kind) && isCallable(id.Kind) {
			return s, true
		}
		if s.id.Kind == id.Kind {
			return s, true
		}
	}
	return symbol{}, false
}

// isCallable reports whether kind addresses a function or method.
func isCallable(k core.SymbolKind) bool { return k == core.KindFunc || k == core.KindMethod }

// symbols walks the parse tree once and collects every addressable symbol. It
// descends only through structural containers (translation units, namespaces,
// class/struct bodies, template and linkage wrappers) — never into function bodies
// — so nested lambdas and local declarations are not surfaced as top-level symbols.
func (c Cpp) symbols(src core.Source) []symbol {
	b := src.Bytes()
	var out []symbol

	var visit func(n *sitter.Node, container string)
	visit = func(n *sitter.Node, container string) {
		count := n.ChildCount()
		for i := uint(0); i < count; i++ {
			child := n.Child(i)
			switch child.Kind() {
			case "function_definition":
				if s, ok := mkFunc(child, child, container, b); ok {
					out = append(out, s)
				}

			case "template_declaration":
				// A template wraps either a function or a class/struct. Use the
				// template_declaration as the whole-symbol node so the template
				// header is part of the signature and of any write.
				if inner := childOfKind(child, "function_definition"); inner != nil {
					if s, ok := mkFunc(inner, child, container, b); ok {
						out = append(out, s)
					}
				}
				for _, k := range []string{"class_specifier", "struct_specifier"} {
					if t := childOfKind(child, k); t != nil {
						out = append(out, mkType(t, child, b))
						if body := t.ChildByFieldName("body"); body != nil {
							visit(body, typeName(t, b))
						}
					}
				}

			case "class_specifier", "struct_specifier":
				out = append(out, mkType(child, child, b))
				if body := child.ChildByFieldName("body"); body != nil {
					visit(body, typeName(child, b))
				}

			case "namespace_definition":
				// A namespace does not turn its free functions into methods, so the
				// container is carried through unchanged.
				if body := child.ChildByFieldName("body"); body != nil {
					visit(body, container)
				}

			case "translation_unit", "declaration_list", "field_declaration_list", "linkage_specification":
				visit(child, container)
			}
		}
	}
	visit(src.Root(), "")
	return out
}

// mkFunc builds a function/method symbol from a function_definition node. whole is
// the node that delimits the whole symbol (the function_definition itself, or its
// enclosing template_declaration). Reports ok=false if no name can be extracted.
func mkFunc(fn, whole *sitter.Node, container string, b []byte) (symbol, bool) {
	decl := unwrapDeclarator(fn.ChildByFieldName("declarator"))
	if decl == nil || decl.Kind() != "function_declarator" {
		return symbol{}, false
	}
	name := decl.ChildByFieldName("declarator")
	if name == nil {
		return symbol{}, false
	}

	id := core.SymbolID{Name: nodeText(name, b), Container: container, Kind: core.KindFunc}
	switch name.Kind() {
	case "identifier":
		// Free function (also covers a function declared inside a namespace).
	case "field_identifier":
		// In-class member definition; container is the enclosing class name.
		id.Kind = core.KindMethod
	case "qualified_identifier":
		// Out-of-line member definition: split Ns::Cls::method into name+container.
		scope, leaf := splitQualified(name, b)
		id.Name, id.Container, id.Kind = leaf, scope, core.KindMethod
	default:
		return symbol{}, false
	}

	return symbol{
		id:       id,
		decl:     whole,
		body:     fn.ChildByFieldName("body"),
		docStart: docStart(whole, b),
	}, true
}

// mkType builds a KindStruct symbol from a class_specifier/struct_specifier. whole
// is the delimiting node (the specifier itself, or its template_declaration).
func mkType(spec, whole *sitter.Node, b []byte) symbol {
	return symbol{
		id:       core.SymbolID{Kind: core.KindStruct, Name: typeName(spec, b)},
		decl:     whole,
		docStart: docStart(whole, b),
	}
}

// signatureOf renders the "light" signature of a symbol: declared name, textual
// parameters and return type for callables, and the contiguous doc comment.
func (c Cpp) signatureOf(s symbol, b []byte) core.Signature {
	sig := core.Signature{Name: s.id.Name, Doc: docText(s, b)}
	if s.body == nil {
		return sig // a type has no params/returns
	}
	if decl := unwrapDeclarator(s.body.Parent().ChildByFieldName("declarator")); decl != nil {
		if params := decl.ChildByFieldName("parameters"); params != nil {
			pc := params.NamedChildCount()
			for i := uint(0); i < pc; i++ {
				p := params.NamedChild(i)
				if p.Kind() == "parameter_declaration" || p.Kind() == "optional_parameter_declaration" {
					sig.Params = append(sig.Params, nodeText(p, b))
				}
			}
		}
	}
	if ret := s.body.Parent().ChildByFieldName("type"); ret != nil {
		sig.Returns = nodeText(ret, b)
	}
	return sig
}

// docText returns the contiguous preceding-comment documentation for s, trimmed.
func docText(s symbol, b []byte) string {
	if s.docStart >= s.decl.StartByte() {
		return ""
	}
	return strings.TrimSpace(string(b[s.docStart:s.decl.StartByte()]))
}

// docStart returns the byte offset of the first comment in the run of contiguous
// `comment` siblings immediately above n, or n.StartByte() when there is none. A
// blank line (a row gap greater than one) breaks contiguity.
func docStart(n *sitter.Node, b []byte) uint {
	start := n.StartByte()
	prevRow := n.StartPosition().Row
	for sib := n.PrevSibling(); sib != nil && sib.Kind() == "comment"; sib = sib.PrevSibling() {
		if prevRow-sib.EndPosition().Row > 1 {
			break // blank line between this comment and what follows it
		}
		start = sib.StartByte()
		prevRow = sib.StartPosition().Row
	}
	return start
}

// unwrapDeclarator descends through pointer/reference declarators to reach the
// underlying declarator (typically a function_declarator).
func unwrapDeclarator(n *sitter.Node) *sitter.Node {
	for n != nil {
		switch n.Kind() {
		case "pointer_declarator", "reference_declarator":
			n = n.ChildByFieldName("declarator")
		default:
			return n
		}
	}
	return nil
}

// splitQualified splits a qualified_identifier (e.g. geo::Circle::area) into its
// scope ("geo::Circle") and the leaf name ("area").
func splitQualified(n *sitter.Node, b []byte) (scope, leaf string) {
	var parts []string
	cur := n
	for cur != nil && cur.Kind() == "qualified_identifier" {
		if s := cur.ChildByFieldName("scope"); s != nil {
			parts = append(parts, nodeText(s, b))
		}
		cur = cur.ChildByFieldName("name")
	}
	if cur != nil {
		leaf = nodeText(cur, b)
	}
	return strings.Join(parts, "::"), leaf
}

// typeName returns the declared name of a class/struct specifier ("" if anonymous).
func typeName(spec *sitter.Node, b []byte) string {
	if name := spec.ChildByFieldName("name"); name != nil {
		return nodeText(name, b)
	}
	return ""
}

// childOfKind returns the first direct child of n with the given kind, or nil.
func childOfKind(n *sitter.Node, kind string) *sitter.Node {
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		if child := n.Child(i); child.Kind() == kind {
			return child
		}
	}
	return nil
}

// nodeText returns the source text spanned by n.
func nodeText(n *sitter.Node, b []byte) string {
	return string(b[n.StartByte():n.EndByte()])
}
