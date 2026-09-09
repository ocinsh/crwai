package crwai

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
	"github.com/ocinsh/crwai/internal/lang"
	"github.com/ocinsh/crwai/internal/lang/c"
	"github.com/ocinsh/crwai/internal/lang/cpp"
	"github.com/ocinsh/crwai/internal/lang/dart"
	"github.com/ocinsh/crwai/internal/lang/golang"
	"github.com/ocinsh/crwai/internal/lang/java"
	"github.com/ocinsh/crwai/internal/lang/javascript"
	"github.com/ocinsh/crwai/internal/lang/python"
	"github.com/ocinsh/crwai/internal/lang/rust"
	"github.com/ocinsh/crwai/internal/lang/typescript"
)

// Engine is the canonical Service implementation: it owns the language registry
// and resolves each call's language by file extension (or by a language forced
// via Lang). It holds no per-call state — every method re-parses the file from
// disk — so a single Engine is safe for concurrent use and a package-level
// instance is fine.
//
// Concurrency has one caveat, documented on Write: concurrent writes to the SAME
// file can still lose an edit, because the staleness check is a defense rather
// than a lock. Concurrent calls on different files are always safe.
type Engine struct {
	reg *lang.Registry
	// forced, when non-nil, is the language every call uses instead of resolving
	// by file extension. Set only through Lang, which returns a copy.
	forced core.Language
	// root, when non-empty, is the absolute directory every addressed path must
	// live under. Set only through Root, which returns a copy.
	root string
}

// Compile-time assertion that Engine implements the full public surface.
var (
	_ Service    = (*Engine)(nil)
	_ DocService = (*Engine)(nil)
)

// New builds an Engine with the supported languages registered (TypeScript ships
// as two grammars: TypeScript for .ts and TSX for .tsx). The language
// set is fixed at construction; the registry is read-only thereafter. The engine
// is unconfined by default: use Root to restrict it to a workspace.
func New() *Engine {
	reg := lang.NewRegistry()
	reg.Register(golang.Go{})
	reg.Register(rust.Rust{})
	reg.Register(dart.Dart{})
	reg.Register(python.Python{})
	reg.Register(typescript.TypeScript{})
	reg.Register(typescript.TSX{})
	reg.Register(javascript.JavaScript{})
	reg.Register(java.Java{})
	reg.Register(c.C{})
	reg.Register(cpp.Cpp{})
	return &Engine{reg: reg}
}

// Lang returns a view of the Engine that forces every subsequent call to use the
// named language, bypassing file-extension detection. It is the override for files
// whose extension is ambiguous (a C++ header named .h, which resolves to C by
// extension) or absent. The name is matched case-insensitively against the
// registered languages (see Languages); an unknown name yields
// ErrUnsupportedLanguage. The receiver is left unchanged, so extension-based
// resolution stays available through the original Engine.
func (e *Engine) Lang(name string) (*Engine, error) {
	l, ok := e.reg.ByName(name)
	if !ok {
		return nil, ErrUnsupportedLanguage
	}
	cp := *e
	cp.forced = l
	return &cp, nil
}

// Root returns a view of the Engine confined to dir: every subsequent call, read
// or write, rejects a path that does not resolve inside that directory with
// ErrPathOutsideRoot. It is the workspace boundary an MCP client wants, since the
// server otherwise addresses any path the process can reach.
//
// dir is made absolute and symlink-resolved once, here; each addressed path is
// resolved the same way before comparison, so neither "../" segments nor a symlink
// pointing outside can escape. An empty dir clears the confinement. The receiver
// is left unchanged.
func (e *Engine) Root(dir string) (*Engine, error) {
	cp := *e
	if dir == "" {
		cp.root = ""
		return &cp, nil
	}
	abs, err := resolve(dir)
	if err != nil {
		return nil, err
	}
	cp.root = abs
	return &cp, nil
}

// Languages reports the registered languages, sorted by name.
func (e *Engine) Languages() []LanguageInfo {
	ls := e.reg.Languages()
	out := make([]LanguageInfo, 0, len(ls))
	for _, l := range ls {
		out = append(out, LanguageInfo{Name: l.Name(), Extensions: l.Extensions()})
	}
	return out
}

// ListSignatures returns the signature of every top-level symbol in the file.
func (e *Engine) ListSignatures(path string) ([]Signature, error) {
	l, src, err := e.open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return l.ListSignatures(src)
}

// FunctionBody returns only the body of the named function or method.
func (e *Engine) FunctionBody(path, name, container string) (string, error) {
	l, src, err := e.open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	return l.FunctionBody(src, funcID(name, container))
}

// Function returns the whole named function or method (doc, signature, body).
func (e *Engine) Function(path, name, container string) (string, error) {
	l, src, err := e.open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	return l.Function(src, funcID(name, container))
}

// Interface returns the full definition of the named interface/protocol/trait.
func (e *Engine) Interface(path, name string) (string, error) {
	l, src, err := e.open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	return l.ReadInterface(src, core.SymbolID{Kind: core.KindInterface, Name: name})
}

// Struct returns the full definition of the named struct/class/record.
func (e *Engine) Struct(path, name string) (string, error) {
	l, src, err := e.open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	return l.ReadStruct(src, core.SymbolID{Kind: core.KindStruct, Name: name})
}

// Write applies a batch of edits atomically. It resolves the language by
// extension and delegates to the all-or-nothing core pipeline, which validates
// the re-parse and persists only if every edit lands. An empty batch is rejected
// with ErrNoEdits rather than rewriting the file for nothing.
//
// Concurrency: the pipeline defends against a lost update with a content-hash
// check, but that is not a lock. Two writers racing on the SAME file can still
// lose an edit; serialise them yourself.
func (e *Engine) Write(path string, edits ...Edit) (WriteResult, error) {
	if err := e.allow(path); err != nil {
		return WriteResult{Path: path}, err
	}
	l, ok := e.langFor(path)
	if !ok {
		return WriteResult{Path: path}, ErrUnsupportedLanguage
	}
	return core.BatchWrite(l, path, edits)
}

// open resolves the language for path and parses the file into a Source. The
// caller must Close the returned Source.
func (e *Engine) open(path string) (core.Language, core.Source, error) {
	if err := e.allow(path); err != nil {
		return nil, nil, err
	}
	l, ok := e.langFor(path)
	if !ok {
		return nil, nil, ErrUnsupportedLanguage
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	src, err := l.Parse(b)
	if err != nil {
		return nil, nil, err
	}
	return l, src, nil
}

// allow enforces the configured root: it reports ErrPathOutsideRoot unless path
// resolves inside it. With no root configured every path is allowed, which is the
// default and keeps the library usable as a plain package.
func (e *Engine) allow(path string) error {
	if e.root == "" {
		return nil
	}
	abs, err := resolve(path)
	if err != nil {
		return err
	}
	if abs == e.root || strings.HasPrefix(abs, e.root+string(filepath.Separator)) {
		return nil
	}
	return ErrPathOutsideRoot
}

// resolve makes path absolute and follows symlinks in the part of it that exists,
// so a symlink cannot smuggle a path outside the root. A path that does not exist
// yet is resolved through its nearest existing ancestor.
func resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	dir, base := filepath.Split(abs)
	realDir, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return filepath.Clean(abs), nil
	}
	return filepath.Join(realDir, base), nil
}

// langFor resolves the language for path: the language forced by Lang when set,
// otherwise the registered language for the file's extension.
func (e *Engine) langFor(path string) (core.Language, bool) {
	if e.forced != nil {
		return e.forced, true
	}
	return e.reg.ByExtension(path)
}

// funcID builds a SymbolID for a function/method: a non-empty container marks it
// as a method. It is the read-path counterpart of TargetFor, which applies the
// same rule to the free-text kind a front-end passes on the write path.
func funcID(name, container string) core.SymbolID {
	kind := core.KindFunc
	if container != "" {
		kind = core.KindMethod
	}
	return core.SymbolID{Kind: kind, Name: name, Container: container}
}
