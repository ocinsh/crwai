package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newLangsCmd lists the languages the engine can parse and the extensions each
// one claims. It is also the reference for the --lang override: the names printed
// here are the names that flag accepts.
func newLangsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "langs",
		Aliases: []string{"lng"},
		Short:   "List the supported languages and their file extensions",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			langs := eng.Languages()
			meta := ui.Meta("", len(langs), "language", "languages")
			return emit(cmd, langs, ui.Tree("supported languages", meta, ui.LangNodes(langs)))
		},
	}
}
