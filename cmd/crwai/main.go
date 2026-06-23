// Command crwai is the single entry point for the tree-sitter code service. With
// no subcommand it runs the MCP server over stdio (the root IS the server, for
// launch by an MCP client); each capability is also exposed as a one-word
// subcommand so every function can be exercised by hand from a clean CLI.
//
// This file is the WIRING only — no language logic lives here. The handlers
// resolve a core.Language by file extension and delegate to it (and to
// core.BatchWrite for edits), exactly as the MCP handlers do.
package main

func main() {
	Execute()
}
