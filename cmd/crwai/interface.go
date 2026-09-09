package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newInterfaceCmd prints a full interface, protocol, or trait definition.
// Languages without the construct report that the symbol was not found; read the
// abstract class with the struct command instead.
func newInterfaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "interface <file> <name>",
		Aliases: []string{"iface"},
		Short:   "Print the full definition of an interface, protocol or trait",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			def, err := eng.Interface(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd,
				symbolOut{Path: args[0], Kind: crwai.KindInterface.String(), Name: args[1], Source: def},
				ui.Symbol(args[0], crwai.KindInterface, args[1], "", def))
		},
	}
}
