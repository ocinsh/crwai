package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newBodyCmd prints only the body of a function or method: the densest read, for
// when the signature is already known.
func newBodyCmd() *cobra.Command {
	var container string
	cmd := &cobra.Command{
		Use:     "body <file> <name>",
		Aliases: []string{"bd"},
		Short:   "Print only the body of a function or method",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			body, err := eng.FunctionBody(args[0], args[1], container)
			if err != nil {
				return err
			}
			kind := crwai.TargetFor("func", args[1], container).Kind
			return emit(cmd,
				symbolOut{Path: args[0], Kind: kind.String(), Name: args[1], Container: container, Source: body},
				ui.Symbol(args[0], kind, args[1], container, body))
		},
	}
	cmd.Flags().StringVarP(&container, "container", "c", "",
		"enclosing receiver/class for a method (empty for top-level)")
	return cmd
}
