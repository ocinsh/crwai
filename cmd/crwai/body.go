package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newBodyCmd returns only the body of a function or method — the densest form
// when the signature is already known.
func newBodyCmd() *cobra.Command {
	var container string
	cmd := &cobra.Command{
		Use:     "body <file> <name>",
		Aliases: []string{"bd"},
		Short:   ui.IconFunc + " Print only the body of a function or method",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := crwai.New().FunctionBody(args[0], args[1], container)
			if err != nil {
				return err
			}
			cmd.Println(ui.Code(args[1], body))
			return nil
		},
	}
	cmd.Flags().StringVarP(&container, "container", "c", "", "enclosing receiver/class for a method (empty for top-level)")
	return cmd
}
