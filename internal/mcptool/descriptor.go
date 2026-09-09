// Package mcptool is the decorator layer between the naked core capabilities and
// the MCP wire. The core interfaces hold ONLY logic and carry zero agent-facing
// documentation; this package wraps each tool with the metadata an agent needs —
// mission (when/why to use it), per-parameter docs, and examples — and registers
// it with the official MCP SDK.
//
// The input/output JSON schema is NOT written by hand: the SDK infers it from the
// typed In/Out structs in schemas.go (via their `jsonschema` struct tags). A
// ToolDescriptor therefore documents the tool's purpose and parameters in one
// place; the struct tags carry the same parameter text to the wire schema.
package mcptool

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ParamDoc documents one input parameter. The Name must match the corresponding
// field's json name in the tool's In struct, so the human-readable docs and the
// generated schema stay in sync.
type ParamDoc struct {
	Name        string
	Description string
}

// ToolDescriptor is the agent-facing metadata that decorates a naked capability.
type ToolDescriptor struct {
	// Name is the MCP tool name (e.g. "list_signatures").
	Name string
	// Mission explains WHEN and WHY an agent reaches for this tool — the most
	// important field for steering tool selection.
	Mission string
	// Params documents each input parameter (mirrors the In struct fields).
	Params []ParamDoc
	// Examples are short usage illustrations folded into the tool description.
	Examples []string
}

// Register decorates a naked handler with its descriptor and registers it as an
// MCP tool. The In/Out type parameters drive automatic schema inference in the
// SDK, so callers pass a typed handler and never build a schema by hand.
//
// It builds a *mcp.Tool whose Name comes from the descriptor and whose Description
// is the Mission plus the rendered Params and Examples, then hands it to
// mcp.AddTool.
func Register[In, Out any](s *mcp.Server, d ToolDescriptor, h mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        d.Name,
		Description: d.render(),
	}, h)
}

// render folds the descriptor into a single description string: the Mission,
// then a "Parameters:" block (one line per ParamDoc), then an "Examples:" block.
// The per-field JSON schema is produced separately by the SDK from the In struct
// tags, so this text is purely the agent-facing prose.
func (d ToolDescriptor) render() string {
	var b strings.Builder
	b.WriteString(d.Mission)
	if len(d.Params) > 0 {
		b.WriteString("\n\nParameters:")
		for _, p := range d.Params {
			b.WriteString("\n  - ")
			b.WriteString(p.Name)
			b.WriteString(": ")
			b.WriteString(p.Description)
		}
	}
	if len(d.Examples) > 0 {
		b.WriteString("\n\nExamples:")
		for _, ex := range d.Examples {
			b.WriteString("\n  ")
			b.WriteString(ex)
		}
	}
	return b.String()
}
