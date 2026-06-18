package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

const (
	migratePollInterval = 1 * time.Second
	migrateTimeout      = 5 * time.Minute
)

type setupOpts struct {
	url           string
	username      string
	passwordStdin bool
	login         bool
}

// newSetupCmd returns the `setup` subcommand. It drives the server's existing
// POST /api/auth/setup/admin endpoint over HTTP to create the first admin user.
func newSetupCmd() *cobra.Command {
	var opts setupOpts
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create the first admin user on a running server",
		Long: "Create the first admin user by driving the server's setup endpoint over\n" +
			"HTTP. The server must already be running and reachable. Pending database\n" +
			"migrations are applied automatically first, bringing a fresh instance up in\n" +
			"one command. Pass --login to also log in with the same credentials and store\n" +
			"an API key, so subsequent CLI commands are ready to use. Intended to be run\n" +
			"via `docker exec` / `kubectl exec` into the running container.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.url, "url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	cmd.Flags().StringVar(&opts.username, "username", "", "Admin username (prompted if omitted; required with --password-stdin)")
	cmd.Flags().BoolVar(&opts.passwordStdin, "password-stdin", false, "Read the password from stdin instead of prompting")
	cmd.Flags().BoolVar(&opts.login, "login", false, "After creating the admin, log in with the same credentials and store an API key")
	return cmd
}

func runSetup(cmd *cobra.Command, opts setupOpts) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))

	// Validate the input mode before touching the network.
	if !opts.passwordStdin && !stdinIsTTY {
		return fmt.Errorf("no password: pass --password-stdin to read it from stdin, or run interactively")
	}
	if opts.passwordStdin && opts.username == "" {
		return fmt.Errorf("--username is required with --password-stdin")
	}

	url := cliui.FirstNonEmpty(opts.url, cliauth.DefaultServerURL)
	client := cliclient.New(url)

	if err := preflight(out, client, url); err != nil {
		return err
	}

	username := opts.username
	if username == "" { // interactive (TTY) path
		var err error
		username, err = cliui.Prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username is required")
		}
	}

	password, err := resolveSetupPassword(in, out, opts.passwordStdin)
	if err != nil {
		return err
	}

	res, err := client.SetupAdmin(username, password)
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	if err := reportSetupResult(out, username, res); err != nil {
		return err
	}

	// --login: the admin now exists; reuse the credentials we already have to log
	// in and store an API key. The admin creation (above) has already succeeded
	// and printed its success line, so any failure here is scoped to the login
	// step — the operator must NOT re-run setup, only `nexctl account login`.
	if opts.login {
		cfg, err := clicfg.Load()
		if err != nil {
			return fmt.Errorf("admin created, but loading CLI config for --login failed (run \"nexctl account login\"): %w", err)
		}
		if err := cliauth.LoginAndStoreKey(out, client, cfg, cfg.CurrentName(), url, username, password); err != nil {
			return fmt.Errorf("admin created, but --login failed (run \"nexctl account login\"): %w", err)
		}
	}
	return nil
}

// preflight checks server health before credentials are read. Pending
// migrations (and a migration another process is already running) are applied
// and waited on automatically so a fresh instance comes up in one command. A
// migration that previously *failed* is the one state setup will not paper
// over — it surfaces the failure detail and aborts, because re-running a broken
// migration almost never fixes it; the operator must investigate first.
func preflight(out io.Writer, client *cliclient.Client, url string) error {
	status, err := client.Health()
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	switch status {
	case "ok":
		return nil
	case "db_unavailable":
		return fmt.Errorf("database is unavailable")
	case "needs_migration", "migrating":
		// needs_migration: run them. migrating: another process is already
		// applying them; runMigrateAndWait's POST returns 409 (tolerated) and
		// we fall through to polling until ready.
		return runMigrateAndWait(out, client)
	case "migration_failed":
		return migrationFailedErr(client)
	default:
		return fmt.Errorf("server is not ready (status: %s)", status)
	}
}

// migrationFailedErr builds the abort error for a server stuck in
// migration_failed, surfacing the failure detail from the status endpoint when
// available so the operator has something to investigate.
func migrationFailedErr(client *cliclient.Client) error {
	_, detail, err := client.MigrationStatus()
	if err == nil && detail != "" {
		return fmt.Errorf("migrations previously failed: %s — resolve the underlying problem (check the server logs) before retrying", detail)
	}
	return fmt.Errorf("migrations previously failed — resolve the underlying problem (check the server logs) before retrying")
}

// runMigrateAndWait triggers migrations on the server and polls until the
// server reports ready, or fails / times out. Driven over HTTP so the running
// server's own migrator applies them (a DB-direct migration would leave the
// running server's cached state stale; see the design doc).
func runMigrateAndWait(out io.Writer, client *cliclient.Client) error {
	fmt.Fprintln(out, "Running pending migrations...")
	// RunMigrations tolerates 409 ("already in progress"), so this is also the
	// correct path when health was already "migrating" — we simply fall through
	// to polling the in-progress migration.
	if err := client.RunMigrations(); err != nil {
		return fmt.Errorf("start migrations: %w", err)
	}
	deadline := time.Now().Add(migrateTimeout)
	for {
		state, detail, err := client.MigrationStatus()
		if err != nil {
			return fmt.Errorf("poll migration status: %w", err)
		}
		switch state {
		case "ready":
			fmt.Fprintln(out, "Migrations complete.")
			return nil
		case "migration_failed":
			if detail != "" {
				return fmt.Errorf("migrations failed: %s", detail)
			}
			return fmt.Errorf("migrations failed — check the server logs")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for migrations (last state: %s)", migrateTimeout, state)
		}
		time.Sleep(migratePollInterval)
	}
}

// resolveSetupPassword returns the admin password. With passwordStdin it reads
// a single line from in (no confirmation, docker-login style). Otherwise it
// prompts twice with no echo on the TTY and requires the entries to match.
func resolveSetupPassword(in *bufio.Reader, out io.Writer, passwordStdin bool) (string, error) {
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
	// Reuse promptSecret (from reset_password.go): hidden entry on a TTY, line
	// read otherwise. The non-TTY case never reaches here — runSetup rejects a
	// non-TTY stdin without --password-stdin before calling this.
	read := func(label string) (string, error) {
		return promptSecret(in, out, label)
	}
	return confirmInteractivePassword(read)
}

// confirmInteractivePassword prompts for the password twice via read and
// returns it only if both entries match. Factored out so the match/mismatch
// logic is unit-testable without a TTY.
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

// reportSetupResult maps a SetupResult to user-facing output and an error
// (non-nil => non-zero exit). A nil error means success.
func reportSetupResult(out io.Writer, username string, res *cliclient.SetupResult) error {
	switch res.StatusCode {
	case http.StatusCreated:
		fmt.Fprintf(out, "Admin user %q created.\n", username)
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("setup already complete; an admin user already exists")
	case http.StatusBadRequest:
		if res.Message != "" {
			return errors.New(res.Message)
		}
		return errors.New("invalid request")
	case http.StatusFound, http.StatusMovedPermanently, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		switch {
		case strings.HasPrefix(res.Location, "/migrate"):
			return fmt.Errorf("migrations are pending — run \"nexorious migrate\" first")
		case strings.HasPrefix(res.Location, "/db-error"):
			return fmt.Errorf("database is unavailable")
		default:
			return fmt.Errorf("server redirected to %q; setup is not currently available", res.Location)
		}
	default:
		return fmt.Errorf("unexpected response from server: %d", res.StatusCode)
	}
}
