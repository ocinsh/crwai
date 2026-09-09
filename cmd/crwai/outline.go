package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newOutlineCmd maps a Markdown document: every heading, no section content. It
// is the common-file counterpart of the signatures command, and the paths it
// prints are what the section and write commands address a section by.
func newOutlineCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "outline <file.md>",
		Aliases: []string{"ol"},
		Short:   "List the heading outline of a Markdown document",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			heads, err := eng.Outline(args[0])
			if err != nil {
				return err
			}
			meta := ui.Meta("markdown", len(heads), "heading", "headings")
			return emit(cmd, heads, ui.Tree(args[0], meta, ui.HeadingNodes(heads)))
		},
	}
}
