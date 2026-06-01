package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

// newRootCmd constructs the cobra root command. Exposed for tests.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nexorious",
		Short: "Nexorious — self-hosted game collection manager",
		Long: "Nexorious manages a self-hosted personal game collection with IGDB metadata,\n" +
			"Steam and PSN sync, and JSON/CSV import/export.\n\n" +
			"Running the binary with no subcommand starts the HTTP server (alias for `serve`).",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Default action (no subcommand) is `serve` for backwards compatibility
		// with the previous `./nexorious` invocation.
		RunE: runServe,
	}

	root.PersistentFlags().String("config", "", "Path to a .env file (default: .env in working directory)")

	root.AddCommand(newServeCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
