package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newFunctionCmd returns the whole function — doc, signature, and body — for full
// context on one symbol before editing it.
func newFunctionCmd() *cobra.Command {
	var container string
	cmd := &cobra.Command{
		Use:     "function <file> <name>",
		Aliases: []string{"fn", "get-function"},
		Short:   "Print a whole function or method (doc, signature, body)",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			fn, err := eng.Function(args[0], args[1], container)
			if err != nil {
				return err
			}
			kind := crwai.KindFunc
			if container != "" {
				kind = crwai.KindMethod
			}
			cmd.Println(ui.Symbol(args[0], kind, args[1], fn))
			return nil
		},
	}
	cmd.Flags().StringVarP(&container, "container", "c", "", "enclosing receiver/class for a method (empty for top-level)")
	return cmd
}
