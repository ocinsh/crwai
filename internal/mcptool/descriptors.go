package mcptool

// The six v1 tool descriptors. Each carries the mission, parameter docs, and
// examples for one naked core capability. Logic lives in the language
// implementations; these values only describe how an agent should use the tool.

// ListSignatures is the agent's default, cheapest entry point: a map of the file.
var ListSignatures = ToolDescriptor{
	Name: "list_signatures",
	Mission: "List the signatures of every top-level symbol in a file. Use this " +
		"first to map a file without loading any bodies, then fetch only the one " +
		"symbol you need with get_function / read_interface / read_struct.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file to scan"},
	},
	Examples: []string{
		`list_signatures{"path":"core/write.go"}`,
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
		{Name: "container", Description: "enclosing receiver/class for a method; empty for a top-level function"},
	},
	Examples: []string{
		`get_function_body{"path":"core/write.go","name":"BatchWrite"}`,
	},
}

// GetFunction fetches the whole function (doc + signature + body).
var GetFunction = ToolDescriptor{
	Name: "get_function",
	Mission: "Return the whole function or method — doc, signature, and body. Use " +
		"when you need full context for one symbol before editing it.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the function or method"},
		{Name: "container", Description: "enclosing receiver/class for a method; empty for a top-level function"},
	},
	Examples: []string{
		`get_function{"path":"lang/golang/golang.go","name":"Parse","container":"Go"}`,
	},
}

// ReadInterface fetches a full interface definition.
var ReadInterface = ToolDescriptor{
	Name:    "read_interface",
	Mission: "Return the full definition of an interface/protocol/trait by name.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the interface/protocol/trait"},
	},
	Examples: []string{
		`read_interface{"path":"core/interfaces.go","name":"Language"}`,
	},
}

// ReadStruct fetches a full struct definition.
var ReadStruct = ToolDescriptor{
	Name:    "read_struct",
	Mission: "Return the full definition of a struct/class/record by name.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file"},
		{Name: "name", Description: "name of the struct/class/record"},
	},
	Examples: []string{
		`read_struct{"path":"core/types.go","name":"SymbolID"}`,
	},
}

// WriteFunction is the scalpel: replace one or more symbols atomically. The
// server validates the re-parse and rejects anything that breaks syntax; it never
// generates code.
var WriteFunction = ToolDescriptor{
	Name: "write_function",
	Mission: "Surgically replace one or more functions/methods by name, supplying " +
		"the new source text yourself. Edits are applied all-or-nothing: if any " +
		"edit overlaps another or breaks the file's syntax, the whole batch is " +
		"rejected and the file is left untouched.",
	Params: []ParamDoc{
		{Name: "path", Description: "path to the source file to modify"},
		{Name: "edits", Description: "one or more edits (kind, name, container, new_text) applied atomically"},
	},
	Examples: []string{
		`write_function{"path":"core/write.go","edits":[{"kind":"func","name":"BatchWrite","new_text":"func BatchWrite(...) (...) { ... }"}]}`,
	},
}

// V1 lists the descriptors registered in v1, in a stable order. main wires each
// to its handler via Register.
var V1 = []ToolDescriptor{
	ListSignatures,
	GetFunctionBody,
	GetFunction,
	ReadInterface,
	ReadStruct,
	WriteFunction,
}
