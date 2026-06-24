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
			return serve(cmd.Context())
		},
	}
}

// serve constructs the MCP server over the public crwai engine, registers every
// decorated tool, and serves JSON-RPC over stdio until the transport closes.
func serve(ctx context.Context) error {
	eng := crwai.New()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    crwai.Name,
		Version: crwai.Version,
	}, nil)

	registerTools(server, eng)

	return server.Run(ctx, &mcp.StdioTransport{})
}

// registerTools wires each v1 capability to its decorated MCP tool. Every handler
// is a thin adapter from the tool's typed In struct to an engine method and back
// to the tool's Out struct — all parsing and language logic lives in the engine.
func registerTools(s *mcp.Server, eng *crwai.Engine) {
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
	// reports ErrReadOnlyLanguage if the resolved language has no write support.
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

// withLang returns the engine to use for a call: the base engine when name is
// empty, otherwise a view forced to the named language (crwai.ErrUnsupportedLanguage
// if the name is not registered).
func withLang(eng *crwai.Engine, name string) (*crwai.Engine, error) {
	if name == "" {
		return eng, nil
	}
	return eng.Lang(name)
}

// toEdits maps the MCP wire EditIn structs to public crwai.Edit values.
func toEdits(in []mcptool.EditIn) []crwai.Edit {
	edits := make([]crwai.Edit, 0, len(in))
	for _, e := range in {
		edits = append(edits, crwai.Edit{
			Target:  crwai.SymbolID{Kind: crwai.ParseKind(e.Kind), Name: e.Name, Container: e.Container},
			NewText: e.NewText,
			Rel:     e.Rel,
		})
	}
	return edits
}
