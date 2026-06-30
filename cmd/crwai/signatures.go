package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newSignaturesCmd maps a file: the cheapest entry point, no bodies loaded. Each
// listed symbol is labelled with its kind icon.
func newSignaturesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "signatures <file>",
		Aliases: []string{"sig", "ls"},
		Short:   "List the signatures of every top-level symbol in a file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			sigs, err := eng.ListSignatures(args[0])
			if err != nil {
				return err
			}
			nodes := make([]ui.Node, len(sigs))
			for i, s := range sigs {
				nodes[i] = ui.SignatureNode(s)
			}
			cmd.Println(ui.Tree(args[0], nodes))
			return nil
		},
	}
}
