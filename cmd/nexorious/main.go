package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

// errNoSubcommand is returned by the root command when invoked with no
// subcommand. It signals a non-zero exit after the help overview is printed,
// without main() emitting a redundant error line on top of the help text.
var errNoSubcommand = errors.New("no subcommand provided")

// newRootCmd constructs the cobra root command. Exposed for tests.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nexorious",
		Short: "Nexorious — self-hosted game collection manager",
		Long: "Nexorious manages a self-hosted personal game collection with IGDB metadata,\n" +
			"Steam and PSN sync, and JSON/CSV import/export.\n\n" +
			"Run `nexorious serve` to start the HTTP server. Invoking the binary with no\n" +
			"subcommand prints this help overview.",
		SilenceUsage:  true,
		SilenceErrors: true,
		// A bare `./nexorious` prints the help overview and exits non-zero,
		// rather than silently starting the server. The default legacyArgs
		// validator still rejects unknown subcommands (typos) before this
		// runs, so this fires only for a genuine no-subcommand invocation.
		// `serve` is the explicit way to start the HTTP server.
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errNoSubcommand
		},
	}

	root.PersistentFlags().String("config", "", "Path to a .env file (default: .env in working directory)")

	root.AddCommand(newServeCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newResetPasswordCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newWhoamiCmd())
	root.AddCommand(newAPIKeyCmd())

	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// errNoSubcommand means the help overview has already been printed;
		// just exit non-zero without a redundant error line.
		if !errors.Is(err, errNoSubcommand) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
