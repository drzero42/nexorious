package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

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
	cmd.AddCommand(newSetupAdminCmd(), newSetupBackupsCmd(), newSetupRestoreCmd())
	return cmd
}

func newSetupBackupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backups",
		Short: "List on-disk backups available for restore on a fresh instance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			url := resolveServerURL(cmd)
			client := cliclient.New(url)
			// The setup zone is only reachable once the instance is Ready; bring a
			// fresh instance up (running pending migrations) first, like setup admin.
			if err := cliauth.Preflight(out, client, url); err != nil {
				return err
			}
			entries, err := client.SetupListBackups()
			if err != nil {
				return fmt.Errorf("list setup backups: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, entries)
			}
			if flagBool(cmd, "quiet") {
				for i := range entries {
					fmt.Fprintln(out, entries[i].Filename)
				}
				return nil
			}
			if len(entries) == 0 {
				fmt.Fprintln(out, "No backups found.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "FILENAME\tSIZE\tMODIFIED\tRESTORABLE\tREASON")
			for i := range entries {
				e := &entries[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
					e.Filename, humanBackupBytes(e.SizeBytes), e.ModTime, e.Restorable, e.Reason)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	return cmd
}

func newSetupRestoreCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "restore [<name>]",
		Short: "Restore a fresh instance from a backup (on-disk name or uploaded --file)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			hasName := len(args) == 1
			hasFile := cmd.Flags().Changed("file")
			if hasName == hasFile {
				return fmt.Errorf("specify exactly one of <name> or --file")
			}

			confirmMsg := "WARNING: Restoring will overwrite the database on this instance. Proceed?"
			ok, err := cliui.Confirm(in, out, confirmMsg, flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			url := resolveServerURL(cmd)
			c := cliclient.New(url)
			// The setup zone is only reachable once the instance is Ready; bring a
			// fresh instance up (running pending migrations) first, like setup admin.
			// The restore then reaches the handler, so an incompatible archive (e.g.
			// a pre-v0.17.1 backup) is rejected with a clear message instead of an
			// opaque app-state 503.
			if err := cliauth.Preflight(out, c, url); err != nil {
				return err
			}
			if hasFile {
				f, err := os.Open(filePath) //nolint:gosec // operator-supplied restore archive path
				if err != nil {
					return fmt.Errorf("open file: %w", err)
				}
				defer func() { _ = f.Close() }()
				filename := filePath
				if idx := strings.LastIndexByte(filePath, '/'); idx >= 0 {
					filename = filePath[idx+1:]
				}
				if err := c.SetupRestoreUpload(filename, f); err != nil {
					return fmt.Errorf("setup restore upload: %w", err)
				}
				fmt.Fprintln(out, "Backup restored. Log in with your restored credentials.")
				return nil
			}
			if err := c.SetupRestoreFromDisk(args[0]); err != nil {
				return fmt.Errorf("setup restore: %w", err)
			}
			fmt.Fprintln(out, "Backup restored. Log in with your restored credentials.")
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to a backup archive to upload and restore from")
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
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
