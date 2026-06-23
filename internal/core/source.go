package core

import (
	"errors"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Source is a handle to a single file that has been loaded and parsed for the
// duration of ONE MCP call. It is opened once per call (via Language.Parse) and
// discarded at the end of the call — the server keeps no cache or session, so a
// Source never outlives the call that created it.
//
// The parse tree is an INTERNAL detail: it is available to language
// implementations (which walk it to satisfy the read/write interfaces) but is
// never serialized to or exposed to MCP clients.
//
// CGO hygiene: the concrete implementation owns C-allocated objects (the
// *sitter.Tree, and typically the *sitter.Parser). Close MUST be called — always
// via defer immediately after a successful Parse — to release them, because
// tree-sitter's CGO bindings cannot rely on finalizers.
type Source interface {
	// Bytes returns the file contents the tree was parsed from. Callers must
	// treat the slice as read-only; the write pipeline copies before mutating.
	Bytes() []byte

	// Root returns the root node of the internal parse tree. Intended for
	// language implementations only.
	Root() *sitter.Node

	// Close releases the underlying C-allocated resources (tree, parser).
	Close() error
}

// Parse configures a parser with lang, parses src exactly once, and returns a
// Source whose Close releases the parser and tree. Every language subpackage
// delegates its Parse to this helper (e.g. golang.Go.Parse). The caller owns the
// returned Source and MUST Close it, always via defer immediately after a
// successful Parse — tree-sitter's CGO bindings cannot rely on finalizers.
func Parse(lang *sitter.Language, src []byte) (Source, error) {
	p := sitter.NewParser()
	if err := p.SetLanguage(lang); err != nil {
		p.Close()
		return nil, err
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		p.Close()
		return nil, errors.New("tree-sitter: parse returned no tree")
	}
	return &treeSource{bytes: src, parser: p, tree: tree}, nil
}

// treeSource is the concrete Source: it owns the C-allocated parser and tree for
// the duration of one call and releases them in Close.
type treeSource struct {
	bytes  []byte
	parser *sitter.Parser
	tree   *sitter.Tree
}

// Bytes returns the file contents the tree was parsed from (read-only).
func (s *treeSource) Bytes() []byte { return s.bytes }

// Root returns the root node of the parse tree.
func (s *treeSource) Root() *sitter.Node { return s.tree.RootNode() }

// Close releases the tree then the parser. It is safe to call exactly once.
func (s *treeSource) Close() error {
	s.tree.Close()
	s.parser.Close()
	return nil
}
