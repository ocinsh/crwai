package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/cmd/crwai/ui"
)

// Persistent flag names, declared once so the commands and the helpers below
// cannot drift apart.
const (
	flagLang = "lang"
	flagRoot = "root"
	flagJSON = "json"
)

// Command groups. The help listing separates the two families the tool actually
// has — symbols in code, and non-code documents — because they are different
// contracts on different files, and a single alphabetical list hides that.
const (
	groupCode = "code"
	groupDocs = "documents"
	groupTool = "tool"
)

// newRoot builds the root command. Run with no subcommand it starts the MCP
// server over stdio (so an MCP client can launch the bare binary); the
// subcommands expose each capability for use by hand.
//
// Output discipline: every successful result goes to STDOUT and every error to
// STDERR, so `crwai sig f.go > map.txt` and `crwai sig f.go | grep` behave the way
// a shell user expects. The commands write through cmd.OutOrStdout() rather than
// cobra's Print helpers, which target stderr.
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           crwai.Name,
		Short:         "Tree-sitter code reader and writer: MCP server and CLI",
		Version:       crwai.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: crwai.Name + " parses source files with tree-sitter and answers by symbol\n" +
			"identity: list signatures, read a function, interface or struct, or surgically\n" +
			"rewrite a symbol. A second family of commands reads non-code documents\n" +
			"(Markdown sections, Postman endpoints). With no subcommand it speaks MCP over\n" +
			"stdio.",
		// Default action (no subcommand) == run the MCP server.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), rootDir(cmd))
		},
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	f := root.PersistentFlags()
	f.StringP(flagLang, "l", "",
		"force a language by name, overriding file-extension detection; see the langs command")
	f.StringP(flagRoot, "r", "",
		"confine every path to this directory; anything outside it is rejected")
	f.Bool(flagJSON, false,
		"print the machine-readable JSON form instead of the tree")

	root.AddGroup(
		&cobra.Group{ID: groupCode, Title: "Code (tree-sitter, addressed by symbol):"},
		&cobra.Group{ID: groupDocs, Title: "Documents (no tree-sitter, addressed by heading or URL):"},
		&cobra.Group{ID: groupTool, Title: "Tool:"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add(groupCode,
		newSignaturesCmd(), newFunctionCmd(), newBodyCmd(),
		newInterfaceCmd(), newStructCmd(),
	)
	add(groupDocs,
		newOutlineCmd(), newSectionCmd(), newRequestsCmd(), newRequestCmd(),
	)
	add(groupTool,
		newWriteCmd(), newServeCmd(), newLangsCmd(), newVersionCmd(),
	)
	return root
}

// engineFor builds the engine for a command, applying the global --root and
// --lang overrides in that order. With --lang empty the engine resolves a file's
// language by extension; an unknown language name returns
// crwai.ErrUnsupportedLanguage, and an unreadable --root directory returns the
// filesystem error.
func engineFor(cmd *cobra.Command) (*crwai.Engine, error) {
	eng := crwai.New()
	if dir := rootDir(cmd); dir != "" {
		confined, err := eng.Root(dir)
		if err != nil {
			return nil, err
		}
		eng = confined
	}
	name, _ := cmd.Flags().GetString(flagLang)
	if name == "" {
		return eng, nil
	}
	return eng.Lang(name)
}

// rootDir reads the --root flag.
func rootDir(cmd *cobra.Command) string {
	dir, _ := cmd.Flags().GetString(flagRoot)
	return dir
}

// emit writes a command's result to stdout in the form the caller asked for: the
// indented JSON of data under --json, otherwise the rendered tree. It is the only
// place a command produces output, so the two forms can never diverge and neither
// can leak onto stderr.
func emit(cmd *cobra.Command, data any, human string) error {
	out := cmd.OutOrStdout()
	machine, _ := cmd.Flags().GetBool(flagJSON)
	if machine {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	_, err := fmt.Fprintln(out, human)
	return err
}

// newVersionCmd prints the product version (the same crwai.Version constant the
// MCP handshake and the --version flag report).
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"ver"},
		Short:   "Print the product version",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			type version struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			return emit(cmd,
				version{Name: crwai.Name, Version: crwai.Version},
				ui.Version(crwai.Name, crwai.Version))
		},
	}
}

// Execute runs the root command and exits non-zero on error. The failure is
// rendered by the ui package and printed to stderr, so a script sees a clean
// stdout and a non-zero status while a human sees the same visual language as a
// successful run.
func Execute() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.Fail(err))
		os.Exit(1)
	}
}
