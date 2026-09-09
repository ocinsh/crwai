package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newWriteCmd is the CLI's single mutation verb: it replaces one target with text
// the caller supplies, all-or-nothing, and never generates anything itself. The
// target is a code symbol addressed by --name, or a Markdown section addressed by
// --heading; the two are mutually exclusive because they are different contracts
// on different files, and keeping one command means the atomic-replacement
// discipline is stated once.
//
// The replacement text comes either inline (--text) or from a file (--from), and
// --from is what you want for anything with newlines in it.
func newWriteCmd() *cobra.Command {
	var kind, name, container, heading, text, from string
	cmd := &cobra.Command{
		Use:     "write <file>",
		Aliases: []string{"wr"},
		Short:   "Replace a symbol, or a Markdown section, with new text (atomic)",
		Long: "Replace one target with caller-supplied text, all-or-nothing: if the\n" +
			"replacement fails to resolve or breaks the file's syntax, nothing is written\n" +
			"and the file is left untouched.\n\n" +
			"Address a code symbol with --name (plus --container for a method, and --kind\n" +
			"when the symbol is an interface or a struct), or a Markdown section with\n" +
			"--heading, using the heading path the outline command prints.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case name == "" && heading == "":
				return fmt.Errorf("name the target with --name (a code symbol) or --heading (a Markdown section)")
			case name != "" && heading != "":
				return fmt.Errorf("use either --name or --heading, not both")
			}
			newText, err := resolveText(text, from)
			if err != nil {
				return err
			}
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}

			var res crwai.WriteResult
			if heading != "" {
				res, err = eng.WriteSections(args[0], crwai.SectionEdit{Path: heading, NewText: newText})
			} else {
				res, err = eng.Write(args[0], crwai.Edit{
					Target:  crwai.TargetFor(kind, name, container),
					NewText: newText,
				})
			}
			if err != nil {
				return err
			}
			return emit(cmd, res, ui.WriteResult(res))
		},
	}
	f := cmd.Flags()
	f.StringVarP(&name, "name", "n", "", "name of the target code symbol")
	f.StringVarP(&kind, "kind", "k", "func", "symbol kind: func, method, interface, or struct")
	f.StringVarP(&container, "container", "c", "", "enclosing receiver/class; empty for top-level")
	f.StringVar(&heading, "heading", "", "heading path of the target Markdown section, e.g. Usage/Flags")
	f.StringVarP(&text, "text", "t", "", "replacement text, inline")
	f.StringVarP(&from, "from", "f", "", "read the replacement text from this file instead of --text")
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
		return "", fmt.Errorf("provide the replacement text with --text or --from")
	}
}
