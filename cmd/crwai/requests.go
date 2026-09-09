package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newRequestsCmd maps a Postman collection export: one line per endpoint, no
// bodies loaded. It is the common-file counterpart of the signatures command. The
// tool is read-only: it never edits a collection and never sends a request.
func newRequestsCmd() *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:     "requests <collection.json>",
		Aliases: []string{"reqs"},
		Short:   "List the endpoints of a Postman collection export",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			reqs, err := eng.Requests(args[0], filter)
			if err != nil {
				return err
			}
			meta := ui.Meta("postman", len(reqs), "request", "requests")
			return emit(cmd, reqs, ui.Tree(args[0], meta, ui.RequestNodes(reqs)))
		},
	}
	cmd.Flags().StringVarP(&filter, "filter", "f", "",
		"keep only requests whose URL contains this text (case-insensitive)")
	return cmd
}
