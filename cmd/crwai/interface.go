package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newInterfaceCmd returns a full interface/protocol/trait definition by name.
func newInterfaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "interface <file> <name>",
		Aliases: []string{"iface"},
		Short:   "Print the full definition of an interface/protocol/trait",
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
			cmd.Println(ui.Symbol(args[0], crwai.KindInterface, args[1], def))
			return nil
		},
	}
}
