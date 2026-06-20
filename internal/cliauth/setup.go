package cliauth

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/drzero42/nexorious/internal/cliclient"
)

const (
	migratePollInterval = 1 * time.Second
	migrateTimeout      = 5 * time.Minute
)

// Preflight checks server health before credentials are read. Pending
// migrations (and a migration another process is already running) are applied
// and waited on automatically so a fresh instance comes up in one command. A
// migration that previously *failed* aborts with the failure detail rather than
// blindly retrying a broken migration.
func Preflight(out io.Writer, client *cliclient.Client, url string) error {
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
		return RunMigrateAndWait(out, client)
	case "migration_failed":
		return MigrationFailedErr(client)
	default:
		return fmt.Errorf("server is not ready (status: %s)", status)
	}
}

// MigrationFailedErr builds the abort error for a server stuck in
// migration_failed, surfacing the failure detail when available.
func MigrationFailedErr(client *cliclient.Client) error {
	_, detail, err := client.MigrationStatus()
	if err == nil && detail != "" {
		return fmt.Errorf("migrations previously failed: %s — resolve the underlying problem (check the server logs) before retrying", detail)
	}
	return fmt.Errorf("migrations previously failed — resolve the underlying problem (check the server logs) before retrying")
}

// RunMigrateAndWait triggers migrations on the server and polls until the
// server reports ready, or fails / times out. Driven over HTTP so the running
// server's own migrator applies them.
func RunMigrateAndWait(out io.Writer, client *cliclient.Client) error {
	fmt.Fprintln(out, "Running pending migrations...")
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

// ReportSetupResult maps a SetupResult to user-facing output and an error
// (non-nil => non-zero exit). A nil error means success.
func ReportSetupResult(out io.Writer, username string, res *cliclient.SetupResult) error {
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
			return fmt.Errorf("migrations are pending — run \"nexctl migrate\" first")
		case strings.HasPrefix(res.Location, "/db-error"):
			return fmt.Errorf("database is unavailable")
		default:
			return fmt.Errorf("server redirected to %q; setup is not currently available", res.Location)
		}
	default:
		return fmt.Errorf("unexpected response from server: %d", res.StatusCode)
	}
}
