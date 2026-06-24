package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newStructCmd returns a full struct/class/record definition by name.
func newStructCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "struct <file> <name>",
		Aliases: []string{"st"},
		Short:   ui.IconStruct + " Print the full definition of a struct/class/record",
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
			cmd.Println(ui.Heading(ui.IconStruct, args[1]))
			cmd.Println(def)
			return nil
		},
	}
}
