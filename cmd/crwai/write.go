package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newWriteCmd surgically replaces one symbol with caller-supplied text. The new
// text comes either inline (--text) or from a file (--from); the edit is applied
// all-or-nothing by the library's write pipeline.
func newWriteCmd() *cobra.Command {
	var kind, name, container, text, from string
	cmd := &cobra.Command{
		Use:     "write <file>",
		Aliases: []string{"wr"},
		Short:   "Surgically replace a symbol with new source text (atomic)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			newText, err := resolveText(text, from)
			if err != nil {
				return err
			}

			edit := crwai.Edit{
				Target:  crwai.SymbolID{Kind: crwai.ParseKind(kind), Name: name, Container: container},
				NewText: newText,
			}
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			res, err := eng.Write(args[0], edit)
			if err != nil {
				return err
			}
			printWriteResult(cmd, res)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&name, "name", "n", "", "name of the target symbol (required)")
	f.StringVarP(&kind, "kind", "k", "func", "symbol kind: func, method, interface, or struct")
	f.StringVarP(&container, "container", "c", "", "enclosing receiver/class; empty for top-level")
	f.StringVarP(&text, "text", "t", "", "replacement source text, inline")
	f.StringVarP(&from, "from", "f", "", "read replacement source text from this file instead of --text")
	return cmd
}

// resolveText returns the replacement text from --text or --from, requiring
// exactly one of them.
func resolveText(text, from string) (string, error) {
	switch {
	case text != "" && from != "":
		return "", fmt.Errorf("use either --text or --from, not both")
	case from != "":
		b, err := os.ReadFile(from)
		return string(b), err
	case text != "":
		return text, nil
	default:
		return "", fmt.Errorf("provide replacement text via --text or --from")
	}
}

// printWriteResult renders the all-or-nothing outcome of a batch write.
func printWriteResult(cmd *cobra.Command, res crwai.WriteResult) {
	cmd.Println(ui.WriteResult(res))
}
