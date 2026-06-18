package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Manage storefront sync"}
	cmd.AddCommand(newSyncStatusCmd())
	return cmd
}

// resolveStorefront validates arg against the configured storefronts by fetching
// the sync config list. Matching is case-insensitive. On success the canonical
// Storefront string is returned; on failure a descriptive error listing valid
// options is returned.
func resolveStorefront(c *cliclient.Client, key, arg string) (string, error) {
	configs, err := c.ListSyncConfigs(key)
	if err != nil {
		return "", fmt.Errorf("list sync configs failed: %w", err)
	}
	for _, cfg := range configs {
		if strings.EqualFold(cfg.Storefront, arg) {
			return cfg.Storefront, nil
		}
	}
	slugs := make([]string, len(configs))
	for i, cfg := range configs {
		slugs[i] = cfg.Storefront
	}
	return "", fmt.Errorf("unknown storefront %q; valid: %s", arg, strings.Join(slugs, ", "))
}

func newSyncStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [storefront]",
		Short: "Show sync status (all storefronts, or one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			if len(args) == 0 {
				configs, err := c.ListSyncConfigs(p.Key)
				if err != nil {
					return fmt.Errorf("list sync configs failed: %w", err)
				}
				if flagBool(cmd, "json") {
					return cliui.EncodeJSON(out, configs)
				}
				if flagBool(cmd, "quiet") {
					for _, cfg := range configs {
						fmt.Fprintln(out, cfg.Storefront)
					}
					return nil
				}
				if len(configs) == 0 {
					fmt.Fprintln(out, "No sync configs.")
					return nil
				}
				tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "STOREFRONT\tCONFIGURED\tFREQUENCY\tLAST-SYNCED")
				for _, cfg := range configs {
					lastSynced := "never"
					if cfg.LastSyncedAt != nil {
						lastSynced = *cfg.LastSyncedAt
					}
					configured := "no"
					if cfg.IsConfigured {
						configured = "yes"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", cfg.Storefront, configured, cfg.Frequency, lastSynced)
				}
				return tw.Flush()
			}

			// One arg: resolve storefront then show its status.
			storefront, err := resolveStorefront(c, p.Key, args[0])
			if err != nil {
				return err
			}
			status, err := c.GetSyncStatus(p.Key, storefront)
			if err != nil {
				return fmt.Errorf("get sync status failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, status)
			}
			lastSynced := "never"
			if status.LastSyncedAt != nil {
				lastSynced = *status.LastSyncedAt
			}
			activeJobID := "-"
			if status.ActiveJobID != nil {
				activeJobID = *status.ActiveJobID
			}
			syncing := "no"
			if status.IsSyncing {
				syncing = "yes"
			}
			fmt.Fprintf(out, "storefront:      %s\n", status.Storefront)
			fmt.Fprintf(out, "syncing:         %s\n", syncing)
			fmt.Fprintf(out, "active job id:   %s\n", activeJobID)
			fmt.Fprintf(out, "last synced:     %s\n", lastSynced)
			fmt.Fprintf(out, "external games:  %d\n", status.ExternalGameCount)
			return nil
		},
	}
}
