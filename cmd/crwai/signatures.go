package main

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// newSignaturesCmd maps a file: the cheapest entry point, no bodies loaded. The
// listing is a tree, with every method nested under the type it belongs to, so
// same-named methods on different types are told apart by where they sit rather
// than by a field the reader has to look up.
func newSignaturesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "signatures <file>",
		Aliases: []string{"sig", "ls"},
		Short:   "List the signatures of every top-level symbol in a file",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			sigs, err := eng.ListSignatures(args[0])
			if err != nil {
				return err
			}
			meta := ui.Meta(langLabel(cmd, args[0]), len(sigs), "symbol", "symbols")
			return emit(cmd, sigs, ui.Tree(args[0], meta, ui.SignatureNodes(sigs)))
		},
	}
}

// langLabel names the language a file was read as, for the meta line under a
// listing: the forced language when --lang is set, otherwise the one whose
// extensions claim the file. It is presentation only, so an unresolved name is
// simply omitted rather than reported as an error; the command that actually
// needs the language has already failed by this point if there is none.
func langLabel(cmd *cobra.Command, path string) string {
	if forced, _ := cmd.Flags().GetString(flagLang); forced != "" {
		return forced
	}
	eng, err := engineFor(cmd)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, l := range eng.Languages() {
		for _, e := range l.Extensions {
			if e == ext {
				return l.Name
			}
		}
	}
	return ""
}
