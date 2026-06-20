package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations on a running server",
		Long: "Apply pending database migrations by driving POST /api/migrate/run and\n" +
			"polling until the server reports ready — the CLI equivalent of the web\n" +
			"migration UI's \"Run migrations\" button. Prints \"No pending migrations.\"\n" +
			"when the server is already up to date.",
		RunE: runNexctlMigrate,
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	cmd.AddCommand(newMigrateStatusCmd())
	return cmd
}

func runNexctlMigrate(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	url := resolveServerURL(cmd)
	client := cliclient.New(url)

	state, _, err := client.MigrationStatus()
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	switch state {
	case "ready":
		fmt.Fprintln(out, "No pending migrations.")
		return nil
	case "db_unavailable":
		return fmt.Errorf("database is unavailable")
	}
	return cliauth.RunMigrateAndWait(out, client)
}

func newMigrateStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the server's migration state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			state, detail, err := cliclient.New(resolveServerURL(cmd)).MigrationStatus()
			if err != nil {
				return fmt.Errorf("migration status: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, map[string]any{"state": state, "detail": detail})
			}
			fmt.Fprintf(out, "state: %s\n", state)
			if detail != "" {
				fmt.Fprintf(out, "detail: %s\n", detail)
			}
			return nil
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	return cmd
}
