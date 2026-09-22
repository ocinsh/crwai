# Releases and updates

crwai releases use exact tags of the form `vX.X.X`. The `Version` constant in
`internal/core/version.go` must match the tag. No release tag is present in this
checkout. Only a tagged push activates GitHub Actions; an ordinary branch push
does not run a workflow.

## Publish a release

1. Change `internal/core/version.go` to the intended tag, for example `v1.2.3`.
2. Update `README.md` and `AGENTS.md` for any behavior changes, then run
   `make check` and `make build`.
3. Commit the release-ready state using the repository commit style.
4. Create and push an annotated tag with the exact same version:

   ```sh
   git tag -a v1.2.3 -m v1.2.3
   git push origin v1.2.3
   ```

The tag workflow validates the version, runs the full check on Linux and macOS,
and creates four CGO builds: Linux and macOS, each for amd64 and arm64. The
publish job checks out the tagged repository before calling `gh release create`. It
publishes `crwai_vX.X.X_<os>_<arch>.tar.gz` packages and `checksums.txt` in a
GitHub Release. A failed check prevents publication. No tag or release is
created by the commands in this guide until you run them. If publication fails
after a tag push, push the workflow fix to the default branch and use Actions >
release > Run workflow with the existing tag as the `tag` input. This uses the
fixed workflow without moving or recreating the tag.

## Install and update

Run `crwai install` from the binary you intend to keep. The wizard detects the
Codex and Claude Code command line clients, including the CLI bundled with the
Codex desktop app on macOS, and asks separately whether to configure each
user-level crwai MCP entry. Each accepted entry uses the resolved path of that
same binary. The wizard prints the path and makes no copy.
Keep the binary at that path and restart the client after installation.

The commands below query published GitHub Releases:

```sh
crwai check-update
crwai update
crwai update --to v1.2.3
```

`check-update` compares the current binary with the latest stable release.
`update` downloads the package for the current OS and architecture, verifies
its SHA-256 value against `checksums.txt`, and atomically replaces the binary
that ran the command. `--to` selects an existing exact release tag and also
permits an intentional downgrade. Restart any MCP client to use the new binary.
The update target must be writable by the current user.

The repository's `.codex/config.toml` is a separate project-level setup that
launches `dist/crwai`. It takes precedence while working in this repository.
If you run the wizard from `dist/crwai`, `update` replaces that file, so the
registration continues to use the updated version. A later `make build` can
overwrite it with the version in the source checkout.
