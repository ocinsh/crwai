package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/internal/mcptool"
)

// newServeCmd is the explicit form of the root's default action: run the MCP
// server. Useful when a launcher needs a named subcommand instead of the bare
// binary.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "serve",
		Aliases: []string{"mcp"},
		Short:   "Run the MCP server over stdio (default with no subcommand)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), rootDir(cmd))
		},
	}
}

// serve constructs the MCP server over the public crwai engine, registers every
// decorated tool, and serves JSON-RPC over stdio until the transport closes.
//
// root, when non-empty, confines every tool call to that directory: a path
// outside it is refused with ErrPathOutsideRoot instead of being read or written.
// Without it the server addresses any path the process can reach, which is rarely
// what an MCP client wants, so pass --root when launching it.
func serve(ctx context.Context, root string) error {
	eng := crwai.New()
	if root != "" {
		confined, err := eng.Root(root)
		if err != nil {
			return err
		}
		eng = confined
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    crwai.Name,
		Version: crwai.Version,
	}, nil)

	registerTools(server, eng)

	return server.Run(ctx, &mcp.StdioTransport{})
}

// registerTools wires each capability to its decorated MCP tool. Every handler is
// a thin adapter from the tool's typed In struct to an engine method and back to
// the tool's Out struct — all parsing, language, and document logic lives in the
// engine.
//
// The code tools honour the per-call `lang` override; the common-file tools do
// not have one, because they are selected explicitly and never resolved by
// extension.
func registerTools(s *mcp.Server, eng *crwai.Engine) {
	registerCodeTools(s, eng)
	registerDocTools(s, eng)
}

// registerCodeTools wires the six tree-sitter tools.
func registerCodeTools(s *mcp.Server, eng *crwai.Engine) {
	mcptool.Register(s, mcptool.ListSignatures,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ListSignaturesIn) (*mcp.CallToolResult, mcptool.ListSignaturesOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.ListSignaturesOut{}, err
			}
			sigs, err := e.ListSignatures(in.Path)
			return nil, mcptool.ListSignaturesOut{Signatures: sigs}, err
		})

	mcptool.Register(s, mcptool.GetFunctionBody,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.GetFunctionBodyIn) (*mcp.CallToolResult, mcptool.GetFunctionBodyOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.GetFunctionBodyOut{}, err
			}
			body, err := e.FunctionBody(in.Path, in.Name, in.Container)
			return nil, mcptool.GetFunctionBodyOut{Body: body}, err
		})

	mcptool.Register(s, mcptool.GetFunction,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.GetFunctionIn) (*mcp.CallToolResult, mcptool.GetFunctionOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.GetFunctionOut{}, err
			}
			fn, err := e.Function(in.Path, in.Name, in.Container)
			return nil, mcptool.GetFunctionOut{Function: fn}, err
		})

	mcptool.Register(s, mcptool.ReadInterface,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ReadInterfaceIn) (*mcp.CallToolResult, mcptool.ReadInterfaceOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.ReadInterfaceOut{}, err
			}
			def, err := e.Interface(in.Path, in.Name)
			return nil, mcptool.ReadInterfaceOut{Definition: def}, err
		})

	mcptool.Register(s, mcptool.ReadStruct,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ReadStructIn) (*mcp.CallToolResult, mcptool.ReadStructOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.ReadStructOut{}, err
			}
			def, err := e.Struct(in.Path, in.Name)
			return nil, mcptool.ReadStructOut{Definition: def}, err
		})

	// write_function delegates to the engine's all-or-nothing pipeline, which
	// reports ErrReadOnlyLanguage if the resolved language has no write support
	// and ErrNoEdits if the batch is empty.
	mcptool.Register(s, mcptool.WriteFunction,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.WriteFunctionIn) (*mcp.CallToolResult, mcptool.WriteFunctionOut, error) {
			e, err := withLang(eng, in.Lang)
			if err != nil {
				return nil, mcptool.WriteFunctionOut{}, err
			}
			res, err := e.Write(in.Path, toEdits(in.Edits)...)
			return nil, mcptool.WriteFunctionOut{Result: res}, err
		})
}

// registerDocTools wires the five common-file tools: three for Markdown, two for
// a Postman export.
func registerDocTools(s *mcp.Server, eng *crwai.Engine) {
	mcptool.Register(s, mcptool.OutlineMarkdown,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.OutlineMarkdownIn) (*mcp.CallToolResult, mcptool.OutlineMarkdownOut, error) {
			heads, err := eng.Outline(in.Path)
			return nil, mcptool.OutlineMarkdownOut{Headings: heads}, err
		})

	mcptool.Register(s, mcptool.ReadSection,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ReadSectionIn) (*mcp.CallToolResult, mcptool.ReadSectionOut, error) {
			sec, err := eng.Section(in.Path, in.Heading)
			return nil, mcptool.ReadSectionOut{Section: sec}, err
		})

	mcptool.Register(s, mcptool.WriteSection,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.WriteSectionIn) (*mcp.CallToolResult, mcptool.WriteSectionOut, error) {
			res, err := eng.WriteSections(in.Path, toSectionEdits(in.Edits)...)
			return nil, mcptool.WriteSectionOut{Result: res}, err
		})

	mcptool.Register(s, mcptool.ListRequests,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ListRequestsIn) (*mcp.CallToolResult, mcptool.ListRequestsOut, error) {
			reqs, err := eng.Requests(in.Path, in.Filter)
			return nil, mcptool.ListRequestsOut{Requests: reqs}, err
		})

	mcptool.Register(s, mcptool.ReadRequest,
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcptool.ReadRequestIn) (*mcp.CallToolResult, mcptool.ReadRequestOut, error) {
			reqs, err := eng.Request(in.Path, in.Query)
			return nil, mcptool.ReadRequestOut{Requests: reqs}, err
		})
}

// withLang returns the engine to use for a call: the base engine when name is
// empty, otherwise a view forced to the named language (crwai.ErrUnsupportedLanguage
// if the name is not registered). The base engine's root confinement, if any, is
// carried over by the copy Lang returns.
func withLang(eng *crwai.Engine, name string) (*crwai.Engine, error) {
	if name == "" {
		return eng, nil
	}
	return eng.Lang(name)
}

// toEdits maps the MCP wire EditIn structs to public crwai.Edit values. It goes
// through TargetFor, so an edit that names a container without naming a kind
// resolves the method the read tools would have returned.
func toEdits(in []mcptool.EditIn) []crwai.Edit {
	edits := make([]crwai.Edit, 0, len(in))
	for _, e := range in {
		edits = append(edits, crwai.Edit{
			Target:  crwai.TargetFor(e.Kind, e.Name, e.Container),
			NewText: e.NewText,
		})
	}
	return edits
}

// toSectionEdits maps the MCP wire SectionEditIn structs to public
// crwai.SectionEdit values.
func toSectionEdits(in []mcptool.SectionEditIn) []crwai.SectionEdit {
	edits := make([]crwai.SectionEdit, 0, len(in))
	for _, e := range in {
		edits = append(edits, crwai.SectionEdit{Path: e.Heading, NewText: e.NewText})
	}
	return edits
}
