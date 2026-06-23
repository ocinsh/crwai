package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newSignaturesCmd maps a file: the cheapest entry point, no bodies loaded. Each
// listed symbol is labelled with its kind icon.
func newSignaturesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "signatures <file>",
		Aliases: []string{"sig", "ls"},
		Short:   ui.IconFunc + " List the signatures of every top-level symbol in a file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sigs, err := crwai.New().ListSignatures(args[0])
			if err != nil {
				return err
			}
			cmd.Println(ui.Heading(ui.IconFunc, args[0]))
			for _, s := range sigs {
				cmd.Println(ui.Signature(ui.IconFunc, s))
			}
			return nil
		},
	}
}
