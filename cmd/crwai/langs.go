package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newLangsCmd lists the languages the server can parse, with their extensions.
func newLangsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "langs",
		Aliases: []string{"lng"},
		Short:   "List the supported languages and their file extensions",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			langs := crwai.New().Languages()
			nodes := make([]ui.Node, len(langs))
			for i, l := range langs {
				nodes[i] = ui.LangNode(l.Name, l.Extensions)
			}
			cmd.Println(ui.Tree("supported languages", nodes))
			return nil
		},
	}
}
