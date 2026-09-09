package main

import (
	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newFunctionCmd prints a whole function or method: doc, signature, and body. It
// is what to read before rewriting a symbol, so the replacement is written
// against the real text.
func newFunctionCmd() *cobra.Command {
	var container string
	cmd := &cobra.Command{
		Use:     "function <file> <name>",
		Aliases: []string{"fn", "get-function"},
		Short:   "Print a whole function or method (doc, signature, body)",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			fn, err := eng.Function(args[0], args[1], container)
			if err != nil {
				return err
			}
			kind := crwai.TargetFor("func", args[1], container).Kind
			return emit(cmd,
				symbolOut{Path: args[0], Kind: kind.String(), Name: args[1], Container: container, Source: fn},
				ui.Symbol(args[0], kind, args[1], container, fn))
		},
	}
	cmd.Flags().StringVarP(&container, "container", "c", "",
		"enclosing receiver/class for a method (empty for top-level)")
	return cmd
}

// symbolOut is the machine form of a single located symbol, shared by the
// function, body, interface, and struct commands so --json returns one shape for
// all four.
type symbolOut struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Container string `json:"container,omitempty"`
	Source    string `json:"source"`
}
