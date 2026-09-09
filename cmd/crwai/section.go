package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newSectionCmd prints one Markdown section: its heading and everything beneath
// it down to the next heading of equal or shallower level. The heading is
// addressed by the path the outline command prints, which disambiguates headings
// that repeat under different parents.
func newSectionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "section <file.md> <heading-path>",
		Aliases: []string{"sec"},
		Short:   "Print one section of a Markdown document by heading path",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			sec, err := eng.Section(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd, sec, ui.Section(args[0], sec))
		},
	}
}
