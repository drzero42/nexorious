package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage your user configuration",
	}
	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
	)
	// N4 will append notify subcommands here.
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Show user settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			s, err := cliclient.New(p.URL).GetSettings(p.Key)
			if err != nil {
				return fmt.Errorf("get config: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, s)
			}
			fmt.Fprintf(out, "deal_region: %s\n", s.DealRegion)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	var dealRegion string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update user settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			s, err := cliclient.New(p.URL).UpdateSettings(p.Key, dealRegion)
			if err != nil {
				return fmt.Errorf("set config: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, s)
			}
			fmt.Fprintf(out, "deal_region: %s\n", s.DealRegion)
			return nil
		},
	}
	cmd.Flags().StringVar(&dealRegion, "deal-region", "", "Deal region code (e.g. us, eu)")
	if err := cmd.MarkFlagRequired("deal-region"); err != nil {
		panic(err)
	}
	return cmd
}
