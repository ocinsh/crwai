package mcptool

import "github.com/ocinsh/crwai/internal/core"

// Typed input/output structs, one pair per v1 tool. The MCP SDK infers each
// tool's JSON schema from these types; the `json` tags name the wire fields and
// the `jsonschema` tags provide the per-field descriptions shown to the agent.
//
// SymbolID is flattened into agent-friendly scalar fields (Kind/Name/Container)
// rather than exposed as a nested struct, and the handler reassembles a
// core.SymbolID. File offsets are never part of any input — symbols are addressed
// purely by identity.

// --- list_signatures ---

type ListSignaturesIn struct {
	Path string `json:"path" jsonschema:"path to the source file to scan"`
}

type ListSignaturesOut struct {
	Signatures []core.Signature `json:"signatures" jsonschema:"the signatures of every top-level symbol in the file"`
}

// --- get_function_body ---

type GetFunctionBodyIn struct {
	Path      string `json:"path" jsonschema:"path to the source file"`
	Name      string `json:"name" jsonschema:"name of the function or method"`
	Container string `json:"container,omitempty" jsonschema:"enclosing receiver/class for a method; empty for a top-level function"`
}

type GetFunctionBodyOut struct {
	Body string `json:"body" jsonschema:"the function body only, without signature or doc"`
}

// --- get_function ---

type GetFunctionIn struct {
	Path      string `json:"path" jsonschema:"path to the source file"`
	Name      string `json:"name" jsonschema:"name of the function or method"`
	Container string `json:"container,omitempty" jsonschema:"enclosing receiver/class for a method; empty for a top-level function"`
}

type GetFunctionOut struct {
	Function string `json:"function" jsonschema:"the whole function: doc, signature, and body"`
}

// --- read_interface ---

type ReadInterfaceIn struct {
	Path string `json:"path" jsonschema:"path to the source file"`
	Name string `json:"name" jsonschema:"name of the interface/protocol/trait"`
}

type ReadInterfaceOut struct {
	Definition string `json:"definition" jsonschema:"the full interface definition"`
}

// --- read_struct ---

type ReadStructIn struct {
	Path string `json:"path" jsonschema:"path to the source file"`
	Name string `json:"name" jsonschema:"name of the struct/class/record"`
}

type ReadStructOut struct {
	Definition string `json:"definition" jsonschema:"the full struct definition"`
}

// --- write_function ---

// EditIn is one requested edit, addressed by symbolic identity. Rel is optional
// and, when present, narrows the edit to a span relative to the symbol start.
type EditIn struct {
	Kind      string              `json:"kind" jsonschema:"symbol kind: func, method, interface, or struct"`
	Name      string              `json:"name" jsonschema:"name of the target symbol"`
	Container string              `json:"container,omitempty" jsonschema:"enclosing receiver/class; empty for top-level"`
	NewText   string              `json:"new_text" jsonschema:"replacement source text supplied by the agent"`
	Rel       *core.RelativeRange `json:"rel,omitempty" jsonschema:"optional span relative to the symbol start; omit to replace the whole symbol"`
}

type WriteFunctionIn struct {
	Path  string   `json:"path" jsonschema:"path to the source file to modify"`
	Edits []EditIn `json:"edits" jsonschema:"one or more edits applied atomically (all-or-nothing) in a single call"`
}

type WriteFunctionOut struct {
	Result core.WriteResult `json:"result" jsonschema:"the all-or-nothing outcome of the batch"`
}
