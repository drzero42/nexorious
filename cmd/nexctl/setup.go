package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Bootstrap a fresh instance (create admin / restore backup) — pre-auth",
		Long: "Bootstrap a fresh, user-less Nexorious instance by driving the\n" +
			"unauthenticated setup-zone endpoints over HTTP: create the first admin\n" +
			"user, or restore from a backup (disaster recovery). Intended to run from\n" +
			"a workstation or via `kubectl exec` into a fresh instance.",
	}
	cmd.AddCommand(newSetupAdminCmd())
	return cmd
}

func newSetupAdminCmd() *cobra.Command {
	var username string
	var passwordStdin, login bool
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Create the first admin user on a fresh instance",
		Long: "Create the first admin user by driving POST /api/auth/setup/admin.\n" +
			"Pending database migrations are applied automatically first. Pass --login\n" +
			"to also log in and store an API key so subsequent commands are ready.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetupAdmin(cmd, username, passwordStdin, login)
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	cmd.Flags().StringVar(&username, "username", "", "Admin username (prompted if omitted; required with --password-stdin)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin instead of prompting")
	cmd.Flags().BoolVar(&login, "login", false, "After creating the admin, log in with the same credentials and store an API key")
	return cmd
}

func runSetupAdmin(cmd *cobra.Command, username string, passwordStdin, login bool) error {
	out := cmd.OutOrStdout()
	src := cmd.InOrStdin()
	in := bufio.NewReader(src)

	stdinIsTTY := false
	if f, ok := src.(*os.File); ok {
		stdinIsTTY = term.IsTerminal(int(f.Fd()))
	}
	if !passwordStdin && !stdinIsTTY {
		return fmt.Errorf("no password: pass --password-stdin to read it from stdin, or run interactively")
	}
	if passwordStdin && username == "" {
		return fmt.Errorf("--username is required with --password-stdin")
	}

	url := resolveServerURL(cmd)
	client := cliclient.New(url)

	if err := cliauth.Preflight(out, client, url); err != nil {
		return err
	}

	if username == "" {
		var err error
		username, err = cliui.Prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username is required")
		}
	}

	password, err := resolveSetupPassword(in, src, out, passwordStdin)
	if err != nil {
		return err
	}

	res, err := client.SetupAdmin(username, password)
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	if err := cliauth.ReportSetupResult(out, username, res); err != nil {
		return err
	}

	if login {
		cfg, err := clicfg.Load()
		if err != nil {
			return fmt.Errorf("admin created, but loading CLI config for --login failed (run \"nexctl account login\"): %w", err)
		}
		if err := cliauth.LoginAndStoreKey(out, client, cfg, profileName(cmd, cfg), url, username, password); err != nil {
			return fmt.Errorf("admin created, but --login failed (run \"nexctl account login\"): %w", err)
		}
	}
	return nil
}

// resolveSetupPassword returns the admin password. With passwordStdin it reads a
// single line from in. Otherwise it prompts twice (no echo on a TTY) and requires
// the entries to match.
func resolveSetupPassword(in *bufio.Reader, src io.Reader, out io.Writer, passwordStdin bool) (string, error) {
	if passwordStdin {
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pw := strings.TrimSpace(line)
		if pw == "" {
			return "", fmt.Errorf("empty password on stdin")
		}
		return pw, nil
	}
	read := func(label string) (string, error) {
		return cliui.ReadPassword(in, src, out, label)
	}
	return confirmInteractivePassword(read)
}

// confirmInteractivePassword prompts for the password twice via read and returns
// it only if both entries match and are non-empty.
func confirmInteractivePassword(read func(label string) (string, error)) (string, error) {
	first, err := read("Password: ")
	if err != nil {
		return "", err
	}
	second, err := read("Confirm password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("passwords do not match")
	}
	if first == "" {
		return "", fmt.Errorf("password is required")
	}
	return first, nil
}
