package crwai

import (
	"os"

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
type Engine struct {
	reg *lang.Registry
	// forced, when non-nil, is the language every call uses instead of resolving
	// by file extension. Set only through Lang, which returns a copy.
	forced core.Language
}

// Compile-time assertion that Engine implements the full public surface.
var _ Service = (*Engine)(nil)

// New builds an Engine with the supported languages registered (TypeScript ships
// as two grammars: TypeScript for .ts and TSX for .tsx). The language
// set is fixed at construction; the registry is read-only thereafter.
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
// the re-parse and persists only if every edit lands.
func (e *Engine) Write(path string, edits ...Edit) (WriteResult, error) {
	l, ok := e.langFor(path)
	if !ok {
		return WriteResult{Path: path}, ErrUnsupportedLanguage
	}
	return core.BatchWrite(l, path, edits)
}

// open resolves the language for path and parses the file into a Source. The
// caller must Close the returned Source.
func (e *Engine) open(path string) (core.Language, core.Source, error) {
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

// langFor resolves the language for path: the language forced by Lang when set,
// otherwise the registered language for the file's extension.
func (e *Engine) langFor(path string) (core.Language, bool) {
	if e.forced != nil {
		return e.forced, true
	}
	return e.reg.ByExtension(path)
}

// funcID builds a SymbolID for a function/method: a non-empty container marks it
// as a method.
func funcID(name, container string) core.SymbolID {
	kind := core.KindFunc
	if container != "" {
		kind = core.KindMethod
	}
	return core.SymbolID{Kind: kind, Name: name, Container: container}
}
