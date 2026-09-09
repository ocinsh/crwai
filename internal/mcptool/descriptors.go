package mcptool

// The tool descriptors. Each carries the mission, parameter docs, and examples for
// one naked core capability. Logic lives in the language implementations and the
// common-file packages; these values only describe how an agent should use a tool.
//
// Two families are described here and they are not interchangeable. The first six
// are the CODE tools: they are backed by tree-sitter, address a symbol by identity
// (kind, name, container), and resolve their language from the file extension. The
// last five are the COMMON-FILE tools: no tree-sitter, no language resolution, and
// a target addressed by the document's own identity — a heading path, a request
// URL. An agent picks a common-file tool explicitly; nothing infers one.
//
// Params must mirror the fields of the matching In struct, including the optional
// ones, so the prose an agent reads and the generated schema never disagree.

// langParam is the language override documented on every code tool.
var langParam = ParamDoc{
	Name:        "lang",
	Description: "optional: force a language by name (see the langs listing), overriding file-extension detection; use it for an ambiguous .h header or a file with no extension",
}

// ListSignatures is the agent's default, cheapest entry point: a map of the file.
var ListSignatures = ToolDescriptor{
	Name: "list_signatures",
	Mission: "List the signatures of every top-level symbol in a file. Use this " +
		"first to map a file without loading any bodies, then fetch only the one " +
		"symbol you need with get_function / read_interface / read_struct. Each " +
		"entry carries its kind, so the listing also tells you which reader to " +
		"call, and its container, so a method tells you which type it belongs to.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file to scan"},
		langParam,
	},
	Examples: []string{
		`list_signatures{"path":"core/write.go"}`,
		`list_signatures{"path":"widget.h","lang":"cpp"}`,
	},
}

// GetFunctionBody fetches just the body — the densest form when the signature is
// already known.
var GetFunctionBody = ToolDescriptor{
	Name: "get_function_body",
	Mission: "Return only the body of a function or method, addressed by name " +
		"(and container for a method). Use when you already know the signature " +
		"and want the minimum context.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the function or method"},
		{Name: "container", Description: "optional: enclosing receiver/class for a method; empty for a top-level function"},
		langParam,
	},
	Examples: []string{
		`get_function_body{"path":"core/write.go","name":"BatchWrite"}`,
	},
}

// GetFunction fetches the whole function (doc + signature + body).
var GetFunction = ToolDescriptor{
	Name: "get_function",
	Mission: "Return a whole function or method — doc comment, signature, and " +
		"body — addressed by name (and container for a method). Use before " +
		"rewriting it, so you edit against the full text rather than a guess.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the function or method"},
		{Name: "container", Description: "optional: enclosing receiver/class for a method; empty for a top-level function"},
		langParam,
	},
	Examples: []string{
		`get_function{"path":"engine.go","name":"Write","container":"Engine"}`,
	},
}

// ReadInterface fetches a full interface / protocol / trait definition.
var ReadInterface = ToolDescriptor{
	Name: "read_interface",
	Mission: "Return the full definition of an interface, protocol, or trait. " +
		"Use it to learn the contract a type must satisfy. Languages without the " +
		"construct (C, C++, JavaScript, Python) report that the symbol was not " +
		"found; read the abstract class as a struct instead.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the interface/protocol/trait"},
		langParam,
	},
	Examples: []string{
		`read_interface{"path":"core/interfaces.go","name":"Language"}`,
	},
}

// ReadStruct fetches a full struct / class / record definition.
var ReadStruct = ToolDescriptor{
	Name: "read_struct",
	Mission: "Return the full definition of a struct, class, or record — its " +
		"fields and, where the language declares them inside the type, its " +
		"methods. Use it to learn a type's shape before constructing or editing " +
		"it.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the struct/class/record"},
		langParam,
	},
	Examples: []string{
		`read_struct{"path":"core/types.go","name":"Signature"}`,
	},
}

// WriteFunction is the scalpel: it places text the agent supplies, atomically.
var WriteFunction = ToolDescriptor{
	Name: "write_function",
	Mission: "Surgically replace one or more symbols by identity, supplying the " +
		"new source text yourself — this tool never generates code. Edits are " +
		"applied all-or-nothing: if any edit fails to resolve, overlaps another, " +
		"or breaks the file's syntax, the whole batch is rejected and the file is " +
		"left untouched. Read the symbol first, then send back the whole " +
		"replacement, including its signature line.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file to modify"},
		{Name: "edits", Description: "one or more edits (name, new_text, and optionally kind and container), applied atomically; an empty list is rejected"},
		langParam,
	},
	Examples: []string{
		`write_function{"path":"core/write.go","edits":[{"name":"BatchWrite","new_text":"func BatchWrite(...) (...) { ... }"}]}`,
		`write_function{"path":"shapes.py","edits":[{"name":"area","container":"Circle","new_text":"    def area(self):\n        return 0"}]}`,
	},
}

// OutlineMarkdown is the common-file counterpart of list_signatures.
var OutlineMarkdown = ToolDescriptor{
	Name: "outline_markdown",
	Mission: "List the headings of a Markdown document in order, with no section " +
		"content loaded. It is the cheap map of a document: read it first, then " +
		"pull the single section you need with read_section. Each heading carries " +
		"the slash-joined path that addresses it, which disambiguates headings " +
		"that repeat under different parents.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the Markdown file to outline"},
	},
	Examples: []string{
		`outline_markdown{"path":"README.md"}`,
	},
}

// ReadSection is the common-file counterpart of get_function.
var ReadSection = ToolDescriptor{
	Name: "read_section",
	Mission: "Return one section of a Markdown document: its heading plus the " +
		"content beneath it, down to the next heading of equal or shallower level " +
		"(nested subsections included). Address it by the heading path reported by " +
		"outline_markdown.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the Markdown file"},
		{Name: "heading", Description: "heading path of the section, e.g. \"Usage/Flags\""},
	},
	Examples: []string{
		`read_section{"path":"README.md","heading":"Usage"}`,
	},
}

// WriteSection is the common-file counterpart of write_function.
var WriteSection = ToolDescriptor{
	Name: "write_section",
	Mission: "Replace one or more Markdown sections with text you supply, " +
		"addressed by heading path. It goes through the same all-or-nothing write " +
		"path as the code writer: the file is read once, every edit is resolved, " +
		"overlapping edits are rejected, and the result is renamed into place " +
		"atomically. Include the heading line in the replacement block.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the Markdown file to modify"},
		{Name: "edits", Description: "one or more section rewrites (heading, new_text), applied atomically; an empty list is rejected"},
	},
	Examples: []string{
		`write_section{"path":"README.md","edits":[{"heading":"Status","new_text":"## Status\n\nAll nine languages are implemented."}]}`,
	},
}

// ListRequests is the cheap map of a Postman collection export.
var ListRequests = ToolDescriptor{
	Name: "list_requests",
	Mission: "List the endpoints of a Postman collection export (Collection " +
		"Format v2.1), one line each, optionally filtered by URL. It is the cheap " +
		"map: read it first, then pull the full detail of the endpoints you care " +
		"about with read_request. Read-only; it never edits a collection and never " +
		"sends a request.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the collection export"},
		{Name: "filter", Description: "optional: keep only requests whose URL contains this text (case-insensitive)"},
	},
	Examples: []string{
		`list_requests{"path":"api.postman_collection.json"}`,
		`list_requests{"path":"api.postman_collection.json","filter":"/token"}`,
	},
}

// ReadRequest is the full read of the Postman endpoints matching a query.
var ReadRequest = ToolDescriptor{
	Name: "read_request",
	Mission: "Return method, URL, body, and documentation for every endpoint of a " +
		"Postman collection whose URL matches a query. The result is a list " +
		"because one endpoint is commonly duplicated across folders, for instance " +
		"once per region; no match is an empty list, not an error.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the collection export"},
		{Name: "query", Description: "match requests whose URL contains this text (case-insensitive)"},
	},
	Examples: []string{
		`read_request{"path":"api.postman_collection.json","query":"/token/oauth"}`,
	},
}
