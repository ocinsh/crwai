package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newDeclarationCmd prints the full source of any symbol, whatever its kind. It
// is the only way to open a constant, a variable or a named type, which the
// listing reports but no dedicated reader accepts, and it works for functions,
// interfaces and structs too.
//
// The kind is optional on purpose: a caller that copied a name out of a listing
// has a name, and the language searches every kind for it.
func newDeclarationCmd() *cobra.Command {
	var kind, container string
	cmd := &cobra.Command{
		Use:     "declaration <file> <name>",
		Aliases: []string{"decl"},
		Short:   "Print the full source of any symbol, whatever its kind",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			src, err := eng.Declaration(args[0], kind, args[1], container)
			if err != nil {
				return err
			}
			shown := crwai.SymbolKind(kind)
			if kind == "" {
				shown = "symbol"
			}
			return emit(cmd,
				symbolOut{Path: args[0], Kind: shown.String(), Name: args[1], Container: container, Source: src},
				ui.Symbol(args[0], shown, args[1], container, src))
		},
	}
	f := cmd.Flags()
	f.StringVarP(&kind, "kind", "k", "",
		"symbol kind: func, method, interface, struct, const, var, or type; omit to search every kind")
	f.StringVarP(&container, "container", "c", "",
		"enclosing receiver/class for a method; empty for a top-level symbol")
	return cmd
}
