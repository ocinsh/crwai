package mcptool

import (
	"github.com/ocinsh/crwai/internal/common/markdown"
	"github.com/ocinsh/crwai/internal/common/postman"
	"github.com/ocinsh/crwai/internal/core"
)

// Typed input/output structs, one pair per tool. The MCP SDK infers each tool's
// JSON schema from these types; the `json` tags name the wire fields and the
// `jsonschema` tags provide the per-field descriptions shown to the agent.
//
// SymbolID is flattened into agent-friendly scalar fields (Kind/Name/Container)
// rather than exposed as a nested struct, and the handler reassembles a
// core.SymbolID. File offsets are never part of any input — symbols are addressed
// purely by identity.
//
// Only fields a tool can actually honour appear here. core.Edit carries a Rel
// (RelativeRange) that no language resolves in v1, so EditIn deliberately omits
// it: a schema must not offer an agent a parameter that always fails.

// --- list_signatures ---

type ListSignaturesIn struct {
	Path string `json:"path" jsonschema:"path to the source file to scan"`
	Lang string `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type ListSignaturesOut struct {
	Signatures []core.Signature `json:"signatures" jsonschema:"the signatures of every top-level symbol in the file"`
}

// --- get_function_body ---

type GetFunctionBodyIn struct {
	Path      string `json:"path" jsonschema:"path to the source file"`
	Name      string `json:"name" jsonschema:"name of the function or method"`
	Container string `json:"container,omitempty" jsonschema:"enclosing receiver/class for a method; empty for a top-level function"`
	Lang      string `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type GetFunctionBodyOut struct {
	Body string `json:"body" jsonschema:"the function body only, without signature or doc"`
}

// --- get_function ---

type GetFunctionIn struct {
	Path      string `json:"path" jsonschema:"path to the source file"`
	Name      string `json:"name" jsonschema:"name of the function or method"`
	Container string `json:"container,omitempty" jsonschema:"enclosing receiver/class for a method; empty for a top-level function"`
	Lang      string `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type GetFunctionOut struct {
	Function string `json:"function" jsonschema:"the whole function: doc, signature, and body"`
}

// --- read_interface ---

type ReadInterfaceIn struct {
	Path string `json:"path" jsonschema:"path to the source file"`
	Name string `json:"name" jsonschema:"name of the interface/protocol/trait"`
	Lang string `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type ReadInterfaceOut struct {
	Definition string `json:"definition" jsonschema:"the full interface definition"`
}

// --- read_struct ---

type ReadStructIn struct {
	Path string `json:"path" jsonschema:"path to the source file"`
	Name string `json:"name" jsonschema:"name of the struct/class/record"`
	Lang string `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type ReadStructOut struct {
	Definition string `json:"definition" jsonschema:"the full struct definition"`
}

// --- write_function ---

// EditIn is one requested edit, addressed by symbolic identity. Kind may be left
// empty: a non-empty container then means a method, matching how the read tools
// address one.
type EditIn struct {
	Kind      string `json:"kind,omitempty" jsonschema:"symbol kind: func, method, interface, or struct; omit to infer func, or method when container is set"`
	Name      string `json:"name" jsonschema:"name of the target symbol"`
	Container string `json:"container,omitempty" jsonschema:"enclosing receiver/class; empty for top-level"`
	NewText   string `json:"new_text" jsonschema:"replacement source text supplied by the agent"`
}

type WriteFunctionIn struct {
	Path  string   `json:"path" jsonschema:"path to the source file to modify"`
	Edits []EditIn `json:"edits" jsonschema:"one or more edits applied atomically (all-or-nothing) in a single call; an empty list is rejected"`
	Lang  string   `json:"lang,omitempty" jsonschema:"force a language by name (e.g. cpp), overriding file-extension detection; omit to detect from the extension"`
}

type WriteFunctionOut struct {
	Result core.WriteResult `json:"result" jsonschema:"the all-or-nothing outcome of the batch"`
}

// --- common-file tools ---
//
// These address a document by its own identity (a heading path, a request URL)
// rather than by code symbol, and are never routed by file extension: the agent
// picks the tool explicitly, so none of them takes a lang override.

// --- outline_markdown ---

type OutlineMarkdownIn struct {
	Path string `json:"path" jsonschema:"path to the Markdown file to outline"`
}

type OutlineMarkdownOut struct {
	Headings []markdown.Heading `json:"headings" jsonschema:"every heading in document order, each with the heading path that addresses its section"`
}

// --- read_section ---

type ReadSectionIn struct {
	Path    string `json:"path" jsonschema:"path to the Markdown file"`
	Heading string `json:"heading" jsonschema:"heading path identifying the section, as reported by outline_markdown (e.g. Usage/Flags)"`
}

type ReadSectionOut struct {
	Section markdown.Section `json:"section" jsonschema:"the heading and the Markdown beneath it, down to the next heading of equal or shallower level"`
}

// --- write_section ---

// SectionEditIn is one requested Markdown rewrite, addressed by heading path.
type SectionEditIn struct {
	Heading string `json:"heading" jsonschema:"heading path of the section to replace, as reported by outline_markdown"`
	NewText string `json:"new_text" jsonschema:"replacement Markdown block supplied by the agent, heading line included"`
}

type WriteSectionIn struct {
	Path  string          `json:"path" jsonschema:"path to the Markdown file to modify"`
	Edits []SectionEditIn `json:"edits" jsonschema:"one or more section rewrites applied atomically (all-or-nothing); an empty list is rejected"`
}

type WriteSectionOut struct {
	Result core.WriteResult `json:"result" jsonschema:"the all-or-nothing outcome of the batch"`
}

// --- list_requests ---

type ListRequestsIn struct {
	Path   string `json:"path" jsonschema:"path to the Postman collection export (Collection Format v2.1)"`
	Filter string `json:"filter,omitempty" jsonschema:"keep only requests whose URL contains this text (case-insensitive); omit to list them all"`
}

type ListRequestsOut struct {
	Requests []postman.Request `json:"requests" jsonschema:"one entry per endpoint, in collection order, with the folder path that disambiguates duplicates"`
}

// --- read_request ---

type ReadRequestIn struct {
	Path  string `json:"path" jsonschema:"path to the Postman collection export"`
	Query string `json:"query" jsonschema:"match requests whose URL contains this text (case-insensitive)"`
}

type ReadRequestOut struct {
	Requests []postman.RequestDetail `json:"requests" jsonschema:"the matching endpoints with method, URL, body and documentation; a slice because one endpoint is commonly duplicated per folder"`
}
