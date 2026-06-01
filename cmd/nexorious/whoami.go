package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newWhoamiCmd returns the `whoami` subcommand.
func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the user authenticated by the stored API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd)
		},
	}
}

func runWhoami(cmd *cobra.Command) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return fmt.Errorf("not logged in (run `nexorious login` first)")
	}

	username, err := cliclient.New(p.URL).Me(p.Key)
	if err != nil {
		return fmt.Errorf("could not verify stored key (it may be revoked or expired): %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s @ %s\n", username, p.URL)
	return nil
}
