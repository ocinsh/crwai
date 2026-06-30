// Package golang is the REFERENCE language implementation. It shows the exact
// shape every language subpackage follows: a type that satisfies core.Language
// (the read capabilities) plus, optionally, core.FunctionWriter (write support).
//
// It walks the tree-sitter-go parse tree to answer by symbol identity: list the
// signatures of a file, read a whole function/interface/struct or just a function
// body, and resolve a batch of symbolic edits to concrete byte spans for the
// language-agnostic write pipeline in core.
//
// Go-specific notes:
//   - Documentation: preceding-sibling line/block comments immediately above the
//     declaration (the standard Go doc-comment convention). A blank line between
//     the comment and the declaration breaks the association.
//   - SymbolID.Container: the receiver type for a method (e.g. "Registry" for
//     `func (r *Registry) Register(...)`, pointer stripped), and "" for a
//     top-level function.
//   - The subpackage is named `golang` (not `go`) to avoid colliding with the
//     host language keyword.
package golang

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/ocinsh/crwai/internal/core"
)

// ErrRelativeRangeNotImplemented is returned when an Edit carries a RelativeRange
// (Edit.Rel != nil): the v1 write path only replaces whole symbols. The contract
// is present so the signature never changes when relative edits are implemented.
var ErrRelativeRangeNotImplemented = errors.New("golang: relative-range edits not implemented")

// Go is the Go language implementation. The zero value is ready to use; it holds
// no per-call state (the server is stateless — a fresh Source is parsed per call).
type Go struct{}

// Tree-sitter S-expression queries this language uses. Declared here as the single
// source of truth for the node kinds each capability targets.
const (
	// queryFunctions matches top-level functions and methods. The receiver field
	// of a method_declaration yields SymbolID.Container.
	queryFunctions = `
		(function_declaration name: (identifier) @name) @func
		(method_declaration
			receiver: (parameter_list) @receiver
			name: (field_identifier) @name) @method
	`
	// queryInterfaces matches interface type declarations.
	queryInterfaces = `(type_declaration (type_spec name: (type_identifier) @name type: (interface_type))) @iface`
	// queryStructs matches struct type declarations.
	queryStructs = `(type_declaration (type_spec name: (type_identifier) @name type: (struct_type))) @struct`
)

// Compile-time assertions: Go satisfies the read interfaces (via Language) and the
// optional write capability.
var (
	_ core.Language       = (*Go)(nil)
	_ core.FunctionWriter = (*Go)(nil)
)

// Name returns the canonical language name.
func (Go) Name() string { return "go" }

// Extensions returns the file extensions handled by this language.
func (Go) Extensions() []string { return []string{".go"} }

// Parse loads src into a parsed Source via the shared core parse helper. The
// caller owns the returned Source and must Close it (via defer).
func (Go) Parse(src []byte) (core.Source, error) {
	return core.Parse(ts.NewLanguage(tsgo.Language()), src)
}

// ListSignatures returns one Signature per top-level symbol (functions, methods,
// interfaces, structs), each with its preceding-comment Doc attached. Results are
// ordered by position in the file so the output is deterministic. No bodies are
// read for interfaces/structs and parameter/return text is taken verbatim.
func (Go) ListSignatures(src core.Source) ([]core.Signature, error) {
	b := src.Bytes()
	type entry struct {
		start uint
		sig   core.Signature
	}
	var entries []entry
	add := func(start uint, sig core.Signature) { entries = append(entries, entry{start, sig}) }

	if err := runQuery(src, queryFunctions, func(q *ts.Query, m *ts.QueryMatch) {
		caps := captures(q, m)
		name, ok := caps["name"]
		if !ok {
			return
		}
		kind := core.KindFunc
		decl, ok := caps["func"]
		if !ok {
			decl, ok = caps["method"]
			if !ok {
				return
			}
			kind = core.KindMethod
		}
		container := ""
		if recv, hasRecv := caps["receiver"]; hasRecv {
			container = receiverType(recv, b)
		}
		add(decl.StartByte(), core.Signature{
			Kind:      kind,
			Name:      name.Utf8Text(b),
			Container: container,
			Text:      signatureText(decl, b),
			Params:    paramsOf(decl, b),
			Returns:   returnsOf(decl, b),
			Doc:       docOf(decl, b),
		})
	}); err != nil {
		return nil, err
	}

	for _, qt := range []struct {
		query, capture string
		kind           core.SymbolKind
	}{
		{queryInterfaces, "iface", core.KindInterface},
		{queryStructs, "struct", core.KindStruct},
	} {
		if err := runQuery(src, qt.query, func(q *ts.Query, m *ts.QueryMatch) {
			caps := captures(q, m)
			name, ok := caps["name"]
			if !ok {
				return
			}
			decl, ok := caps[qt.capture]
			if !ok {
				return
			}
			add(decl.StartByte(), core.Signature{Kind: qt.kind, Name: name.Utf8Text(b), Doc: docOf(decl, b)})
		}); err != nil {
			return nil, err
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].start < entries[j].start })
	out := make([]core.Signature, len(entries))
	for i, e := range entries {
		out[i] = e.sig
	}
	return out, nil
}

// FunctionBody locates the function/method matching id and returns only the text
// inside its body block (braces stripped, surrounding whitespace trimmed).
func (g Go) FunctionBody(src core.Source, id core.SymbolID) (string, error) {
	decl, ok := g.locateFunc(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	body := decl.ChildByFieldName("body")
	if body == nil {
		return "", core.ErrSymbolNotFound
	}
	text := strings.TrimSpace(body.Utf8Text(src.Bytes()))
	text = strings.TrimSuffix(strings.TrimPrefix(text, "{"), "}")
	return strings.TrimSpace(text), nil
}

// Function locates the function/method matching id and returns its full text:
// preceding doc comments + signature + body.
func (g Go) Function(src core.Source, id core.SymbolID) (string, error) {
	decl, ok := g.locateFunc(src, id)
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(decl, src.Bytes()), nil
}

// ReadInterface locates the interface type matching id and returns its full
// declaration text (doc comments included).
func (Go) ReadInterface(src core.Source, id core.SymbolID) (string, error) {
	decl, ok := locateType(src, id.Name, queryInterfaces, "iface")
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(decl, src.Bytes()), nil
}

// ReadStruct locates the struct type matching id and returns its full declaration
// text (doc comments included).
func (Go) ReadStruct(src core.Source, id core.SymbolID) (string, error) {
	decl, ok := locateType(src, id.Name, queryStructs, "struct")
	if !ok {
		return "", core.ErrSymbolNotFound
	}
	return withDoc(decl, src.Bytes()), nil
}

// ResolveEdits maps each Edit to a concrete byte span on src: it locates the
// target symbol by identity and returns the [start, end) span of the WHOLE symbol
// (the declaration, doc comments excluded). It does not mutate src or touch disk —
// core.BatchWrite orders, overlap-checks, applies, re-parses, and persists.
func (g Go) ResolveEdits(src core.Source, edits []core.Edit) ([]core.ResolvedEdit, error) {
	out := make([]core.ResolvedEdit, 0, len(edits))
	for _, e := range edits {
		if e.Rel != nil {
			return nil, ErrRelativeRangeNotImplemented
		}
		decl, ok := g.locate(src, e.Target)
		if !ok {
			return nil, fmt.Errorf("%w: %s", core.ErrSymbolNotFound, e.Target.Name)
		}
		start, end := decl.ByteRange()
		out = append(out, core.ResolvedEdit{StartByte: start, EndByte: end, NewText: e.NewText, From: e.Target})
	}
	return out, nil
}

// locate dispatches to the right finder by symbol kind.
func (g Go) locate(src core.Source, id core.SymbolID) (ts.Node, bool) {
	switch id.Kind {
	case core.KindInterface:
		return locateType(src, id.Name, queryInterfaces, "iface")
	case core.KindStruct:
		return locateType(src, id.Name, queryStructs, "struct")
	default:
		return g.locateFunc(src, id)
	}
}

// locateFunc finds the function/method declaration node matching id (by name and
// container). The returned Node value is a copy, safe to use after the query
// cursor advances.
func (Go) locateFunc(src core.Source, id core.SymbolID) (ts.Node, bool) {
	b := src.Bytes()
	var found ts.Node
	var ok bool
	_ = runQuery(src, queryFunctions, func(q *ts.Query, m *ts.QueryMatch) {
		if ok {
			return
		}
		caps := captures(q, m)
		name, hasName := caps["name"]
		if !hasName {
			return
		}
		var decl ts.Node
		var container string
		if fn, isFn := caps["func"]; isFn {
			decl = fn
		} else if md, isMethod := caps["method"]; isMethod {
			decl = md
			if recv, hasRecv := caps["receiver"]; hasRecv {
				container = receiverType(recv, b)
			}
		} else {
			return
		}
		if name.Utf8Text(b) == id.Name && container == id.Container {
			found, ok = decl, true
		}
	})
	return found, ok
}

// locateType finds a type declaration (interface or struct) by name using the
// given query and capture name for the declaration node.
func locateType(src core.Source, name, query, capture string) (ts.Node, bool) {
	b := src.Bytes()
	var found ts.Node
	var ok bool
	_ = runQuery(src, query, func(q *ts.Query, m *ts.QueryMatch) {
		if ok {
			return
		}
		caps := captures(q, m)
		nameNode, hasName := caps["name"]
		decl, hasDecl := caps[capture]
		if hasName && hasDecl && nameNode.Utf8Text(b) == name {
			found, ok = decl, true
		}
	})
	return found, ok
}

// runQuery compiles query, runs it against the whole tree, and calls visit once
// per match. It owns the Query/QueryCursor and closes them (CGO hygiene).
func runQuery(src core.Source, query string, visit func(q *ts.Query, m *ts.QueryMatch)) error {
	root := src.Root()
	q, qerr := ts.NewQuery(root.Language(), query)
	if qerr != nil {
		return fmt.Errorf("golang: invalid query: %s", qerr.Message)
	}
	defer q.Close()
	qc := ts.NewQueryCursor()
	defer qc.Close()
	matches := qc.Matches(q, root, src.Bytes())
	for m := matches.Next(); m != nil; m = matches.Next() {
		visit(q, m)
	}
	return nil
}

// captures maps each capture name to a COPY of its node. The copy is required
// because the query cursor reuses the match buffer on the next iteration; a Node
// value is self-contained and stays valid as long as the tree is alive.
func captures(q *ts.Query, m *ts.QueryMatch) map[string]ts.Node {
	names := q.CaptureNames()
	out := make(map[string]ts.Node, len(m.Captures))
	for _, c := range m.Captures {
		out[names[c.Index]] = c.Node
	}
	return out
}

// withDoc returns the declaration text prefixed with its contiguous doc comments.
func withDoc(decl ts.Node, src []byte) string {
	body := decl.Utf8Text(src)
	if doc := docOf(decl, src); doc != "" {
		return doc + "\n" + body
	}
	return body
}

// docOf returns the contiguous preceding-sibling comments immediately above decl,
// joined by newlines. A blank line between a comment and the declaration breaks
// the chain (standard Go doc-comment association).
func docOf(decl ts.Node, src []byte) string {
	var lines []string
	cur := decl
	for {
		prev := cur.PrevSibling()
		if prev == nil || prev.Kind() != "comment" {
			break
		}
		if cur.StartPosition().Row == 0 || prev.EndPosition().Row != cur.StartPosition().Row-1 {
			break
		}
		lines = append([]string{prev.Utf8Text(src)}, lines...)
		cur = *prev
	}
	return strings.Join(lines, "\n")
}

// receiverType extracts the receiver type name from a method's parameter_list,
// stripping a leading pointer and any generic type arguments (so both
// `(r *Registry)` and `(r Registry[T])` yield "Registry").
func receiverType(recv ts.Node, src []byte) string {
	var t string
	for i := uint(0); i < recv.NamedChildCount(); i++ {
		c := recv.NamedChild(i)
		if c.Kind() == "parameter_declaration" {
			if tn := c.ChildByFieldName("type"); tn != nil {
				t = tn.Utf8Text(src)
			}
			break
		}
	}
	t = strings.TrimPrefix(strings.TrimSpace(t), "*")
	if i := strings.IndexByte(t, '['); i >= 0 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// signatureText returns the verbatim signature line of a function/method: the
// source from the declaration start up to (but not including) the body block, so
// the receiver, type parameters and return type are preserved exactly as written
// (e.g. "func (s *Stack[T]) Len() int"). When no body is present the whole
// declaration text is returned, trimmed.
func signatureText(decl ts.Node, src []byte) string {
	if body := decl.ChildByFieldName("body"); body != nil {
		return strings.TrimSpace(string(src[decl.StartByte():body.StartByte()]))
	}
	return strings.TrimSpace(decl.Utf8Text(src))
}

// paramsOf returns the textual parameter declarations of a function/method, in
// order ("" slice if none).
func paramsOf(fn ts.Node, src []byte) []string {
	p := fn.ChildByFieldName("parameters")
	if p == nil {
		return nil
	}
	var out []string
	for i := uint(0); i < p.NamedChildCount(); i++ {
		c := p.NamedChild(i)
		out = append(out, strings.TrimSpace(c.Utf8Text(src)))
	}
	return out
}

// returnsOf returns the textual result type/list of a function/method ("" if it
// returns nothing).
func returnsOf(fn ts.Node, src []byte) string {
	r := fn.ChildByFieldName("result")
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Utf8Text(src))
}
