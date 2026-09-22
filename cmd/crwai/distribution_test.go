package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterClientUsesUserScopedMCPCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"codex", "claude"} {
		script := "#!/bin/sh\n" +
			"if [ \"$1 $2\" = \"mcp get\" ]; then exit 1; fi\n" +
			"printf '%s\\n' \"$*\" > \"$HOME/" + name + "-args\"\n"
		path := filepath.Join(home, name)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Register by client name so the command uses the matching argument form.
	t.Setenv("PATH", home)
	for _, name := range []string{"codex", "claude"} {
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := registerClient(cmd, name, "/tmp/crwai original", false); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		b, err := os.ReadFile(filepath.Join(home, name+"-args"))
		if err != nil {
			t.Fatal(err)
		}
		args := string(b)
		if !strings.Contains(args, "mcp add") || !strings.Contains(args, "serve --root") {
			t.Fatalf("%s arguments = %q", name, args)
		}
		if !strings.Contains(args, "/tmp/crwai original") || strings.Contains(args, ".local/bin") {
			t.Fatalf("%s does not use original executable: %q", name, args)
		}
		if name == "claude" && !strings.Contains(args, "--scope user") {
			t.Fatalf("Claude registration is not user scoped: %q", args)
		}
	}
}

func TestInstallWizardUsesCurrentExecutableWithoutCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"codex", "claude"} {
		script := "#!/bin/sh\n" +
			"if [ \"$1 $2\" = \"mcp get\" ]; then exit 1; fi\n" +
			"printf '%s\\n' \"$*\" > \"$HOME/" + name + "-args\"\n"
		if err := os.WriteFile(filepath.Join(home, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", home)
	cmd := newInstallCmd()
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader("y\nn\n"))
	var output, prompts bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&prompts)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(filepath.Join(home, "codex-args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), self) || !strings.Contains(output.String(), self) {
		t.Fatalf("original executable missing from registration or output: %q, %q", args, output.String())
	}
	if _, err := os.Stat(filepath.Join(home, "claude-args")); !os.IsNotExist(err) {
		t.Fatalf("Claude should not be configured: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "crwai")); !os.IsNotExist(err) {
		t.Fatalf("installer copied executable: %v", err)
	}
	if !strings.Contains(prompts.String(), "Install crwai for codex?") || !strings.Contains(prompts.String(), "Install crwai for claude?") {
		t.Fatalf("wizard prompts missing: %q", prompts.String())
	}
}
