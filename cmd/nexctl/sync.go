package main

import (
	"bufio"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// storefrontCredField describes one credential field for a storefront.
type storefrontCredField struct {
	bodyKey  string // JSON key sent in the request body
	flagName string // cobra flag name (without --)
	label    string // prompt label shown to the user
	secret   bool   // whether to hide input (ReadPassword vs Prompt)
}

// storefrontCreds maps each storefront slug to its required credential fields.
var storefrontCreds = map[string][]storefrontCredField{
	"steam": {
		{bodyKey: "steam_id", flagName: "steam-id", label: "Steam ID", secret: false},
		{bodyKey: "web_api_key", flagName: "api-key", label: "Steam Web API key", secret: true},
	},
	"playstation-store": {
		{bodyKey: "npsso_token", flagName: "npsso", label: "PSN npsso token", secret: true},
	},
	"epic-games-store": {
		{bodyKey: "auth_code", flagName: "auth-code", label: "Epic auth code", secret: true},
	},
	"gog": {
		{bodyKey: "auth_code", flagName: "auth-code", label: "GOG auth code", secret: true},
	},
	"humble-bundle": {
		{bodyKey: "session_cookie", flagName: "session-cookie", label: "Humble session cookie", secret: true},
	},
}

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Manage storefront sync"}
	cmd.AddCommand(newSyncStatusCmd(), newSyncConnectCmd(), newSyncDisconnectCmd())
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
	if len(configs) == 0 {
		return "", fmt.Errorf("no storefronts available on this server")
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

func newSyncConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <storefront>",
		Short: "Configure credentials for a storefront",
		Long: `Configure credentials for a storefront.

Flag reference by storefront:
  steam              --steam-id, --api-key
  playstation-store  --npsso
  epic-games-store   --auth-code
  gog                --auth-code
  humble-bundle      --session-cookie`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			sf, err := resolveStorefront(c, p.Key, args[0])
			if err != nil {
				return err
			}

			fields, ok := storefrontCreds[sf]
			if !ok {
				return fmt.Errorf("no credential fields defined for storefront %q", sf)
			}

			in := bufio.NewReader(cmd.InOrStdin())
			body := make(map[string]string, len(fields))
			for _, f := range fields {
				val, _ := cmd.Flags().GetString(f.flagName) //nolint:errcheck // absent flag yields ""
				if val == "" && interactive(cmd) {
					if f.secret {
						val, err = cliui.ReadPassword(in, out, f.label+": ")
					} else {
						val, err = cliui.Prompt(in, out, f.label+": ")
					}
					if err != nil {
						return err
					}
				}
				if val == "" {
					return fmt.Errorf("missing --%s for %s", f.flagName, sf)
				}
				body[f.bodyKey] = val
			}

			resp, err := c.ConnectStorefront(p.Key, sf, body)
			if err != nil {
				return fmt.Errorf("connect failed: %w", err)
			}

			for _, key := range []string{"steam_username", "online_id", "display_name", "username", "message"} {
				if v, ok := resp[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						fmt.Fprintf(out, "connected %s as %s\n", sf, s)
						return nil
					}
				}
			}
			fmt.Fprintf(out, "connected %s\n", sf)
			return nil
		},
	}
	f := cmd.Flags()
	f.String("steam-id", "", "Steam ID (steam only)")
	f.String("api-key", "", "Steam Web API key (steam only)")
	f.String("npsso", "", "PSN npsso token (playstation-store only)")
	f.String("auth-code", "", "Auth code (epic-games-store, gog)")
	f.String("session-cookie", "", "Session cookie (humble-bundle only)")
	return cmd
}

func newSyncDisconnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disconnect <storefront>",
		Short: "Remove stored credentials for a storefront",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			sf, err := resolveStorefront(c, p.Key, args[0])
			if err != nil {
				return err
			}

			in := bufio.NewReader(cmd.InOrStdin())
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Disconnect %s?", sf), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			if err := c.DisconnectStorefront(p.Key, sf); err != nil {
				return fmt.Errorf("disconnect failed: %w", err)
			}
			fmt.Fprintf(out, "disconnected %s\n", sf)
			return nil
		},
	}
	return cmd
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
