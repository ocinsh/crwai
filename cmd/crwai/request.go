package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newRequestCmd prints the full detail of the Postman endpoints whose URL matches
// a query: method, URL, body, and documentation. The result is a list because one
// endpoint is commonly duplicated across folders, for instance once per region.
func newRequestCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "request <collection.json> <query>",
		Aliases: []string{"req"},
		Short:   "Print method, URL, body and documentation of matching endpoints",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			reqs, err := eng.Request(args[0], args[1])
			if err != nil {
				return err
			}
			return emit(cmd, reqs, ui.Requests(args[0], reqs))
		},
	}
}
