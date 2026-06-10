package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// newVersionCmd returns the `version` subcommand. The values printed are
// injected at build time via -ldflags `-X main.version=... -X main.commit=...`.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "nexorious %s (%s)\n", version, commit)
		},
	}
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
