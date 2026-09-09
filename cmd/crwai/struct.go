package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newStructCmd prints a full struct, class, or record definition.
func newStructCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "struct <file> <name>",
		Aliases: []string{"st"},
		Short:   "Print the full definition of a struct, class or record",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			def, err := eng.Struct(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd,
				symbolOut{Path: args[0], Kind: crwai.KindStruct.String(), Name: args[1], Source: def},
				ui.Symbol(args[0], crwai.KindStruct, args[1], "", def))
		},
	}
}
