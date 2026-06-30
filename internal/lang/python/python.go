// Package python implements the Python language for the tree-sitter code service
// (see lang/golang for the documented reference shape and lang/TEMPLATE.md for the
// recipe). It satisfies the core read capabilities plus the optional write
// capability (core.FunctionWriter).
//
// Python-specific notes:
//   - Documentation: the docstring is INSIDE the body — the first string-literal
//     statement of the function/class — NOT a preceding-sibling comment. This is
//     the one language whose doc handling differs from the comment-based others.
//   - SymbolID.Container: the enclosing class for a method; "" for a module-level
//     function.
//   - Symbols are located by walking the parse tree manually (module → top-level
//     function_definition / class_definition; class body → methods). Decorators
//     wrap a definition in a decorated_definition node, which is unwrapped so the
//     addressed symbol is always the bare function_definition / class_definition.
//   - Python has no interface concept, so ReadInterface never matches and returns
//     core.ErrSymbolNotFound; ReadStruct maps to class_definition.
package python

import (
	"fmt"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	ts "github.com/tree-sitter/go-tree-sitter"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// Python is the Python language implementation. The zero value is ready to use; it
// holds no per-call state (a fresh Source is parsed per call).
type Python struct{}

// Compile-time assertions: Python satisfies the read interfaces (via Language) and
// the optional write capability.
var (
	_ core.Language       = (*Python)(nil)
	_ core.FunctionWriter = (*Python)(nil)
)

// Node kinds and field names used while walking the Python parse tree.
const (
	kindModule        = "module"
	kindFunctionDef   = "function_definition"
	kindClassDef      = "class_definition"
	kindDecoratedDef  = "decorated_definition"
	kindExprStatement = "expression_statement"
	kindString        = "string"
	kindStringContent = "string_content"

	fieldName       = "name"
	fieldParameters = "parameters"
	fieldReturnType = "return_type"
	fieldBody       = "body"
	fieldDefinition = "definition"
)

// Name returns the canonical language name.
func (Python) Name() string { return "python" }

// Extensions returns the file extensions handled by this language.
func (Python) Extensions() []string { return []string{".py"} }

// Parse loads src into a parsed Source via the shared core helper. The caller owns
// the returned Source and must Close it (via defer).
func (Python) Parse(src []byte) (core.Source, error) {
	return core.Parse(ts.NewLanguage(tspython.Language()), src)
}

// ListSignatures returns one Signature per top-level symbol: module-level functions
// and classes, plus the methods declared directly inside each class. Bodies are not
// read; the Doc is the symbol's docstring (first string-literal statement).
func (Python) ListSignatures(src core.Source) ([]core.Signature, error) {
	bytes := src.Bytes()
	syms := collect(src)
	out := make([]core.Signature, 0, len(syms))
	for _, s := range syms {
		sig := signatureOf(s.node, bytes)
		sig.Kind = s.id.Kind
		sig.Container = s.id.Container
		// Callables carry a verbatim signature line (`def name(params) -> ret:`),
		// up to but not including the body suite; classes fall back to the name.
		if s.id.Kind == core.KindFunc || s.id.Kind == core.KindMethod {
			if body := s.node.ChildByFieldName(fieldBody); body != nil {
				sig.Text = strings.TrimSpace(string(bytes[s.node.StartByte():body.StartByte()]))
			}
		}
		out = append(out, sig)
	}
	return out, nil
}

// FunctionBody locates the function/method matching id and returns the text of its
// body block (the suite of statements under the `def`).
func (Python) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	node, ok := find(src, id)
	if !ok {
		return "", fmt.Errorf("%w: %s", core.ErrSymbolNotFound, id.Name)
	}
	body := node.ChildByFieldName(fieldBody)
	if body == nil {
		return "", fmt.Errorf("%w: %s has no body", core.ErrSymbolNotFound, id.Name)
	}
	return body.Utf8Text(src.Bytes()), nil
}

// Function locates the function/method matching id and returns its full text:
// signature + body (the docstring lives inside the body). Decorators are not part
// of the addressed symbol.
func (Python) Function(src core.Source, id core.SymbolID) (string, error) {
	node, ok := find(src, id)
	if !ok {
		return "", fmt.Errorf("%w: %s", core.ErrSymbolNotFound, id.Name)
	}
	return node.Utf8Text(src.Bytes()), nil
}

// ReadInterface always reports ErrSymbolNotFound: Python has no interface concept.
// The method exists only because core.Language aggregates the read capabilities.
func (Python) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	return "", fmt.Errorf("%w: python has no interfaces (%s)", core.ErrSymbolNotFound, id.Name)
}

// ReadStruct locates the class matching id (Python's nearest concept to a struct)
// and returns its full declaration text, methods included.
func (Python) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	node, ok := find(src, core.SymbolID{Kind: core.KindStruct, Name: id.Name, Container: id.Container})
	if !ok {
		return "", fmt.Errorf("%w: %s", core.ErrSymbolNotFound, id.Name)
	}
	return node.Utf8Text(src.Bytes()), nil
}

// ResolveEdits maps each Edit to the byte span of its target symbol. It does not
// mutate src or touch disk — core.BatchWrite orders, overlap-checks, applies,
// re-parses, and persists. RelativeRange edits are not implemented in v1.
func (Python) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, core.ErrRelativeRangeNotImplemented
		}
		node, ok := find(src, e.Target)
		if !ok {
			return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
		}
		out = append(out, core.ResolvedEdit{
			StartByte: node.StartByte(),
			EndByte:   node.EndByte(),
			NewText:   e.NewText,
			From:      e.Target,
		})
	}
	return out, nil
}

// symbol pairs a located symbol's identity with the tree node that defines it (the
// bare function_definition / class_definition, decorators already unwrapped).
type symbol struct {
	id   core.SymbolID
	node *ts.Node
}

// collect walks the module in source order and returns every addressable symbol:
// top-level functions and classes, and the methods declared directly in each class.
func collect(src core.Source) []symbol {
	bytes := src.Bytes()
	root := src.Root()
	var out []symbol
	for i := uint(0); i < root.NamedChildCount(); i++ {
		node := unwrap(root.NamedChild(i))
		if node == nil {
			continue
		}
		switch node.Kind() {
		case kindFunctionDef:
			out = append(out, symbol{
				id:   core.SymbolID{Kind: core.KindFunc, Name: nameOf(node, bytes)},
				node: node,
			})
		case kindClassDef:
			cname := nameOf(node, bytes)
			out = append(out, symbol{
				id:   core.SymbolID{Kind: core.KindStruct, Name: cname},
				node: node,
			})
			out = append(out, methodsOf(node, cname, bytes)...)
		}
	}
	return out
}

// methodsOf returns the methods declared directly inside a class body, each tagged
// with the class name as its Container.
func methodsOf(class *ts.Node, container string, bytes []byte) []symbol {
	body := class.ChildByFieldName(fieldBody)
	if body == nil {
		return nil
	}
	var out []symbol
	for i := uint(0); i < body.NamedChildCount(); i++ {
		node := unwrap(body.NamedChild(i))
		if node == nil || node.Kind() != kindFunctionDef {
			continue
		}
		out = append(out, symbol{
			id:   core.SymbolID{Kind: core.KindMethod, Name: nameOf(node, bytes), Container: container},
			node: node,
		})
	}
	return out
}

// find returns the node of the symbol whose identity equals id, if any.
func find(src core.Source, id core.SymbolID) (*ts.Node, bool) {
	for _, s := range collect(src) {
		if s.id == id {
			return s.node, true
		}
	}
	return nil, false
}

// unwrap returns the bare definition node, stepping through a decorated_definition
// wrapper so the addressed symbol is always the function/class itself.
func unwrap(node *ts.Node) *ts.Node {
	if node != nil && node.Kind() == kindDecoratedDef {
		return node.ChildByFieldName(fieldDefinition)
	}
	return node
}

// nameOf returns the declared identifier of a function/class node.
func nameOf(node *ts.Node, bytes []byte) string {
	if id := node.ChildByFieldName(fieldName); id != nil {
		return id.Utf8Text(bytes)
	}
	return ""
}

// signatureOf builds the light Signature for a function/class node: name, textual
// parameters, return type, and the docstring (see package doc).
func signatureOf(node *ts.Node, bytes []byte) core.Signature {
	sig := core.Signature{
		Name: nameOf(node, bytes),
		Doc:  docstring(node, bytes),
	}
	if params := node.ChildByFieldName(fieldParameters); params != nil {
		for i := uint(0); i < params.NamedChildCount(); i++ {
			sig.Params = append(sig.Params, params.NamedChild(i).Utf8Text(bytes))
		}
	}
	if ret := node.ChildByFieldName(fieldReturnType); ret != nil {
		sig.Returns = ret.Utf8Text(bytes)
	}
	return sig
}

// docstring returns the symbol's docstring — the inner text of the first
// string-literal statement of its body — or "" if the first statement is not a
// string. Quotes are excluded: the string's `string_content` child is returned
// verbatim (multi-line content preserved exactly).
func docstring(node *ts.Node, bytes []byte) string {
	body := node.ChildByFieldName(fieldBody)
	if body == nil || body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first.Kind() != kindExprStatement || first.NamedChildCount() == 0 {
		return ""
	}
	str := first.NamedChild(0)
	if str.Kind() != kindString {
		return ""
	}
	for i := uint(0); i < str.NamedChildCount(); i++ {
		if c := str.NamedChild(i); c.Kind() == kindStringContent {
			return c.Utf8Text(bytes)
		}
	}
	return ""
}
