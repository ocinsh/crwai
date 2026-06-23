// Package lang holds the per-language subpackages and the registry that maps a
// file (by extension) or a language name to its core.Language implementation.
// Each target language lives in its own subpackage (lang/golang, lang/rust, ...)
// and is registered here so the server can pick the reference language for a call.
package lang

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/ocinsh/crwai/internal/core"
)

// Registry maps file extensions and language names to core.Language values. It is
// built once at startup and read-only thereafter (the server is stateless per
// call but the language set is fixed).
type Registry struct {
	byExt  map[string]core.Language
	byName map[string]core.Language
}

// NewRegistry returns an empty Registry ready for Register calls.
func NewRegistry() *Registry {
	return &Registry{
		byExt:  make(map[string]core.Language),
		byName: make(map[string]core.Language),
	}
}

// Register adds a language, indexing it by its Name and each of its Extensions.
// Extensions are matched case-insensitively and must include the leading dot.
func (r *Registry) Register(l core.Language) {
	r.byName[strings.ToLower(l.Name())] = l
	for _, ext := range l.Extensions() {
		r.byExt[strings.ToLower(ext)] = l
	}
}

// ByExtension resolves the language for a file path using its extension.
func (r *Registry) ByExtension(path string) (core.Language, bool) {
	l, ok := r.byExt[strings.ToLower(filepath.Ext(path))]
	return l, ok
}

// ByName resolves a language by its canonical name (case-insensitive).
func (r *Registry) ByName(name string) (core.Language, bool) {
	l, ok := r.byName[strings.ToLower(name)]
	return l, ok
}

// Languages returns every registered language, sorted by name. Used by the CLI to
// show which languages the server supports.
func (r *Registry) Languages() []core.Language {
	out := make([]core.Language, 0, len(r.byName))
	for _, l := range r.byName {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
