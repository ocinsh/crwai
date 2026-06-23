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
		Short:   ui.IconLang + " List the supported languages and their file extensions",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(ui.Heading(ui.IconLang, "Supported languages"))
			for _, l := range crwai.New().Languages() {
				cmd.Println(ui.Lang(l.Name, l.Extensions))
			}
			return nil
		},
	}
}
