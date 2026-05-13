package main

import (
	"fmt"

	"github.com/spf13/cobra"
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
