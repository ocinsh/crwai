# Install crwai for Codex

This guide configures the local crwai MCP server for Codex in this repository.
Codex reads the project-level `.codex/config.toml` when the project is trusted.
The configuration launches `dist/crwai` and confines file access to the Git
repository root. It does not require a global Go installation of crwai.

## Requirements

- Go 1.26 and a C toolchain. The tree-sitter grammars require CGO.
- Codex CLI or the Codex desktop app.
- A trusted Codex project opened from this repository.

## Build and activate

Run these commands from the repository root:

```sh
make build
./dist/crwai version
codex mcp get crwai
```

For a user-level installation usable across projects, run `./dist/crwai install`.
The wizard asks whether to configure each detected client, including Codex and
Claude Code. On macOS it detects the Codex CLI bundled with the desktop app even
when `codex` is absent from `PATH`. Codex is registered at user level; a
project-level entry may take precedence. It records the resolved path to
`dist/crwai` and does not copy the binary. Keep that file in place; `update`
replaces it at the same path. The
wizard asks before replacing an existing entry. See `docs/releases.md` for
release updates.

The project configuration is already in `.codex/config.toml`. Its shell command
finds the Git root at startup, so it works from a directory inside this
repository without storing a machine-specific path. It starts crwai with
`serve --root` set to that root. Keep `dist/crwai` available after building.
The build output is ignored by Git, so build again on each new checkout or after
updating the source.

Restart Codex after changing the configuration or rebuilding the binary. In the
Codex terminal interface, `/mcp` shows active servers. `codex mcp list` and
`codex mcp get crwai` show the saved configuration. Ask Codex to list symbols in
`examples/golang/basic.go` to check that the MCP tool is usable.

## Troubleshooting

- If crwai is absent, open the repository as a trusted project and restart
  Codex. Project-level configuration is ignored for untrusted projects.
- If the server cannot start, run `make build` again and check that
  `./dist/crwai version` succeeds.
- If a request reports a path outside the configured root, use a path inside
  this repository. The server is intentionally confined to this checkout.
- If Go reports a Dart module error, do not run `go mod tidy`. The pinned Dart
  grammar in `go.mod` is required for the build.

For Codex MCP configuration details, see the
[official OpenAI documentation](https://developers.openai.com/codex/mcp).
