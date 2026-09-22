package main

import (
	"strings"

	"github.com/spf13/cobra"
)

// newSeeFileCmd shows a file's structure and documentation without callable bodies.
func newSeeFileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "see_file <file>",
		Short: "View a source file with function bodies hidden",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			view, err := eng.SeeFile(args[0])
			if err != nil {
				return err
			}
			var human strings.Builder
			for _, sig := range view.Signatures {
				human.WriteString(sig.Type + " " + sig.Name)
				if sig.Container != "" {
					human.WriteString(" @ " + sig.Container)
				}
				human.WriteByte('\n')
			}
			human.WriteByte('\n')
			human.WriteString(view.Content)
			return emit(cmd, view, human.String())
		},
	}
}
