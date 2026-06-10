package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// updateCheckTimeout bounds the GitHub call so `version` stays snappy even
// when offline. Applied as a context deadline; the client's own 30s HTTP
// timeout stays as is, the shorter deadline wins.
const updateCheckTimeout = 3 * time.Second

// newUpdateCheckClient builds the GitHub release client. Package-level so
// tests can point the command at an httptest server.
var newUpdateCheckClient = updatecheck.NewClient

// newVersionCmd returns the `version` subcommand. The values printed are
// injected at build time via -ldflags `-X main.version=... -X main.commit=...`.
// After printing them it reports whether a newer release is available, unless
// --no-check is set, UPDATE_CHECK_ENABLED=false, or the build is not a
// release (non-semver version such as "dev").
func newVersionCmd() *cobra.Command {
	var noCheck bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "nexorious %s (%s)\n", version, commit)

			if noCheck || !updateCheckEnabled() || !updatecheck.IsValidVersion(version) {
				return
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), updateCheckTimeout)
			defer cancel()

			line, failed := checkForUpdate(ctx, newUpdateCheckClient(), version)

			if failed {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
				return
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
		},
	}
	cmd.Flags().BoolVar(&noCheck, "no-check", false, "skip checking GitHub for a newer release")
	return cmd
}

// updateCheckEnabled mirrors the server's UPDATE_CHECK_ENABLED opt-out. The
// full internal/config struct cannot be parsed here (it requires DATABASE_URL
// etc.), so the variable is read directly. Unset or unparseable means enabled.
func updateCheckEnabled() bool {
	v, ok := os.LookupEnv("UPDATE_CHECK_ENABLED")
	if !ok {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}

// checkForUpdate fetches the latest release and renders the one-line result.
// failed=true means the line is a non-fatal error note destined for stderr.
func checkForUpdate(ctx context.Context, client *updatecheck.Client, running string) (line string, failed bool) {
	rel, err := client.FetchLatest(ctx)
	if err != nil {
		return fmt.Sprintf("update check failed: %v", err), true
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if updatecheck.UpdateAvailable(running, latest) {
		return fmt.Sprintf("Update available: %s — %s", latest, rel.HTMLURL), false
	}
	return "You are running the latest version.", false
}
