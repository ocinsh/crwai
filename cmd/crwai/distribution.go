package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ocinsh/crwai"
	"github.com/ocinsh/crwai/internal/release"
)

// newInstallCmd offers to register this executable with detected MCP clients.
func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Configure detected Codex and Claude Code clients",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			binary, err := filepath.EvalSymlinks(self)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "crwai executable: %s\n", binary)
			configured := []string{}
			detected := []string{}
			input := bufio.NewReader(cmd.InOrStdin())
			for _, client := range []string{"codex", "claude"} {
				if _, err := exec.LookPath(client); err != nil {
					continue
				}
				detected = append(detected, client)
				existing := clientRegistered(cmd, client)
				question := "Install crwai for " + client + "? [y/N]: "
				if existing {
					question = "Replace the existing crwai entry for " + client + "? [y/N]: "
				}
				confirmed, err := confirmInstall(cmd, input, question)
				if err != nil {
					return err
				}
				if !confirmed {
					continue
				}
				if err := registerClient(cmd, client, binary, existing); err != nil {
					return fmt.Errorf("%s registration failed: %w", client, err)
				}
				configured = append(configured, client)
			}
			message := "crwai " + crwai.Version + " executable: " + binary
			if len(configured) > 0 {
				message += "; configured: " + strings.Join(configured, ", ")
			} else if len(detected) > 0 {
				message += "; no MCP client configured"
			} else {
				message += "; no Codex or Claude Code executable found"
			}
			return emit(cmd, map[string]any{"version": crwai.Version, "binary": binary, "clients": configured}, message)
		},
	}
}

func confirmInstall(cmd *cobra.Command, input *bufio.Reader, question string) (bool, error) {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), question); err != nil {
		return false, err
	}
	answer, err := input.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if err == io.EOF && answer == "" {
		return false, fmt.Errorf("interactive input required for installation")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

func clientRegistered(cmd *cobra.Command, client string) bool {
	return exec.CommandContext(cmd.Context(), client, "mcp", "get", "crwai").Run() == nil
}

// registerClient points the user-level MCP entry at the current executable.
func registerClient(cmd *cobra.Command, client, binary string, replace bool) error {
	if replace {
		if out, err := exec.CommandContext(cmd.Context(), client, "mcp", "remove", "crwai").CombinedOutput(); err != nil {
			return fmt.Errorf("remove existing entry: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	var args []string
	switch client {
	case "codex":
		args = []string{"mcp", "add", "crwai", "--", binary, "serve", "--root", "."}
	case "claude":
		args = []string{"mcp", "add", "--scope", "user", "crwai", "--", "sh", "-c", "exec " + shellQuote(binary) + ` serve --root "${CLAUDE_PROJECT_DIR:-.}"`}
	default:
		return fmt.Errorf("unsupported MCP client %q", client)
	}
	out, err := exec.CommandContext(cmd.Context(), client, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// newCheckUpdateCmd compares this binary with the latest published release.
func newCheckUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-update",
		Short: "Check GitHub for a newer crwai release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			latest, err := release.NewClient().Latest(cmd.Context())
			if errors.Is(err, release.ErrReleaseNotFound) {
				return emit(cmd, map[string]any{"current": crwai.Version, "latest": nil, "update_available": false}, "no published crwai release found")
			}
			if err != nil {
				return err
			}
			cmp, err := release.Compare(crwai.Version, latest)
			if err != nil {
				return err
			}
			available := cmp < 0
			message := "crwai " + crwai.Version + " is up to date"
			if available {
				message = "crwai " + latest + " is available; run crwai update"
			}
			return emit(cmd, map[string]any{"current": crwai.Version, "latest": latest, "update_available": available}, message)
		},
	}
}

// newUpdateCmd installs a verified GitHub release over this executable.
func newUpdateCmd() *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update crwai to the latest or a specified release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := release.NewClient()
			var tag string
			var err error
			if to == "" {
				tag, err = client.Latest(cmd.Context())
			} else {
				tag, err = client.Version(cmd.Context(), to)
			}
			if err != nil {
				return err
			}
			if tag == crwai.Version {
				return emit(cmd, map[string]any{"version": tag, "updated": false}, "crwai "+tag+" is already installed")
			}
			binary, err := client.Download(cmd.Context(), tag)
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			target, err := filepath.EvalSymlinks(self)
			if err != nil {
				return err
			}
			if err := release.Replace(target, binary); err != nil {
				return err
			}
			return emit(cmd, map[string]any{"version": tag, "updated": true}, "crwai updated to "+tag)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "install a specific vX.X.X release")
	return cmd
}
