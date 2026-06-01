package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newLogoutCmd returns the `logout` subcommand.
func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored API key and clear it from config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogout(cmd)
		},
	}
}

func runLogout(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return fmt.Errorf("not logged in (no stored API key)")
	}

	// Best-effort server-side revocation; a failure still clears local config.
	if p.KeyID != "" {
		if err := cliclient.New(p.URL).RevokeAPIKeyWithBearer(p.Key, p.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke key on server: %v\n", err)
		}
	}

	p.Key = ""
	p.KeyID = ""
	p.KeyName = ""
	cfg.SetProfile(cfg.CurrentName(), p)
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(out, "Logged out of %s.\n", p.URL)
	return nil
}
