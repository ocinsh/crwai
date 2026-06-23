package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newRoot builds the root command. Run with no subcommand it starts the MCP
// server over stdio (so an MCP client can launch the bare binary); the
// subcommands below expose each capability for manual testing.
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           crwai.Name,
		Short:         "Tree-sitter code reader/writer — MCP server and CLI",
		Version:       crwai.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: crwai.Name + " parses source files with tree-sitter and answers by symbol\n" +
			"identity: list signatures, read a function/interface/struct, or surgically\n" +
			"rewrite a symbol. With no subcommand it speaks MCP over stdio.",
		// Default action (no subcommand) == run the MCP server.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context())
		},
	}

	root.AddCommand(
		newServeCmd(),
		newSignaturesCmd(),
		newBodyCmd(),
		newFunctionCmd(),
		newInterfaceCmd(),
		newStructCmd(),
		newWriteCmd(),
		newLangsCmd(),
		newVersionCmd(),
	)
	return root
}

// newVersionCmd prints the product version (the same crwai.Version constant the
// MCP handshake and the --version flag report).
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"ver"},
		Short:   ui.IconServer + " Print the product version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(ui.Heading(ui.IconServer, crwai.Name+" "+crwai.Version))
			return nil
		},
	}
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, crwai.Name+": "+err.Error())
		os.Exit(1)
	}
}
