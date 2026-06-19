package main

import (
	"bufio"
	"fmt"
	"strconv"
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
	cmd.AddCommand(newSyncStatusCmd(), newSyncConnectCmd(), newSyncDisconnectCmd(),
		newSyncConfigCmd(), newSyncRunCmd(), newSyncResetCmd(),
		newSyncReviewCmd(), newSyncResolveCmd(), newSyncSkipCmd(), newSyncRetryCmd())
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

// connectRejected reports whether a 200-status connect response actually signals
// a rejection. Steam returns {valid:false, error:"..."} on bad credentials with
// HTTP 200; some storefronts use a {success:false} shape. The returned reason
// prefers the server's error/message text.
func connectRejected(resp map[string]any) (string, bool) {
	flagFalse := func(key string) bool {
		v, ok := resp[key]
		if !ok {
			return false
		}
		b, ok := v.(bool)
		return ok && !b
	}
	if !flagFalse("valid") && !flagFalse("success") {
		return "", false
	}
	for _, key := range []string{"error", "message"} {
		if s, ok := resp[key].(string); ok && s != "" {
			return s, true
		}
	}
	return "server rejected credentials", true
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

			src := cmd.InOrStdin()
			in := bufio.NewReader(src)
			body := make(map[string]string, len(fields))
			for _, f := range fields {
				val, _ := cmd.Flags().GetString(f.flagName) //nolint:errcheck // absent flag yields ""
				if val == "" && interactive(cmd) {
					if f.secret {
						val, err = cliui.ReadPassword(in, src, out, f.label+": ")
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
			// Steam returns HTTP 200 with valid:false (and an error code) on bad
			// credentials rather than a 4xx, so a 2xx alone doesn't mean success.
			// Other storefronts only ever send a truthy flag on 200, so this guard
			// is safe across the board.
			if reason, rejected := connectRejected(resp); rejected {
				return fmt.Errorf("connect failed: %s", reason)
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

func newSyncConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <storefront>",
		Short: "Show or update sync configuration for a storefront",
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

			if cmd.Flags().Changed("frequency") {
				freq, err := cmd.Flags().GetString("frequency")
				if err != nil {
					return err
				}
				cfg, err := c.UpdateSyncConfig(p.Key, sf, freq)
				if err != nil {
					return fmt.Errorf("update sync config failed: %w", err)
				}
				if flagBool(cmd, "json") {
					return cliui.EncodeJSON(out, cfg)
				}
				fmt.Fprintf(out, "%s sync frequency: %s\n", cfg.Storefront, cfg.Frequency)
				return nil
			}

			cfg, err := c.GetSyncConfig(p.Key, sf)
			if err != nil {
				return fmt.Errorf("get sync config failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, cfg)
			}
			lastSynced := "never"
			if cfg.LastSyncedAt != nil {
				lastSynced = *cfg.LastSyncedAt
			}
			configured := "no"
			if cfg.IsConfigured {
				configured = "yes"
			}
			fmt.Fprintf(out, "storefront:   %s\n", cfg.Storefront)
			fmt.Fprintf(out, "frequency:    %s\n", cfg.Frequency)
			fmt.Fprintf(out, "configured:   %s\n", configured)
			fmt.Fprintf(out, "last synced:  %s\n", lastSynced)
			return nil
		},
	}
	cmd.Flags().String("frequency", "", "Sync frequency (e.g. daily, weekly)")
	return cmd
}

func newSyncRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <storefront>",
		Short: "Trigger a sync for a storefront",
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
			result, err := c.TriggerSync(p.Key, sf)
			if err != nil {
				return fmt.Errorf("trigger sync failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, result)
			}
			fmt.Fprintf(out, "started %s sync (job %s)\n", result.Storefront, result.JobID)
			return nil
		},
	}
}

func newSyncResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset <storefront>",
		Short: "Delete all synced data for a storefront",
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
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Delete ALL synced data for %s? This cannot be undone.", sf), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			if err := c.ResetSyncData(p.Key, sf); err != nil {
				return fmt.Errorf("reset sync data failed: %w", err)
			}
			fmt.Fprintf(out, "reset %s sync data\n", sf)
			return nil
		},
	}
}

// findExternalGamesByRef returns every external game matching ref from the given
// storefront: a UUID is matched exactly (one element or error); a title is
// matched case-insensitively (zero or more). No cmd, no picker — callers own
// disambiguation.
func findExternalGamesByRef(c *cliclient.Client, key, sf, ref string) ([]cliclient.ExternalGame, error) {
	games, err := c.ListExternalGames(key, sf)
	if err != nil {
		return nil, err
	}
	if looksLikeUUID(ref) {
		for i := range games {
			if games[i].ID == ref {
				return []cliclient.ExternalGame{games[i]}, nil
			}
		}
		return nil, fmt.Errorf("no external game with id %s", ref)
	}
	var matches []cliclient.ExternalGame
	for i := range games {
		if strings.EqualFold(games[i].Title, ref) {
			matches = append(matches, games[i])
		}
	}
	return matches, nil
}

// resolveExternalRef resolves an external game by UUID or title from the given
// storefront's external-game list. UUID matching is exact; title matching is
// case-insensitive. When multiple titles match on a non-interactive session it
// returns an error listing candidates; on an interactive session it presents a
// numbered picker.
func resolveExternalRef(cmd *cobra.Command, c *cliclient.Client, key, sf, ref string) (*cliclient.ExternalGame, error) {
	games, err := findExternalGamesByRef(c, key, sf, ref)
	if err != nil {
		return nil, err
	}
	switch len(games) {
	case 0:
		return nil, fmt.Errorf("no external game matching %q", ref)
	case 1:
		return &games[0], nil
	}
	// Convert to []*cliclient.ExternalGame for pickExternalGame
	ptrs := make([]*cliclient.ExternalGame, len(games))
	for i := range games {
		ptrs[i] = &games[i]
	}
	if interactive(cmd) {
		return pickExternalGame(cmd, ptrs)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d external games; re-run with an id:", ref, len(games))
	for i := range games {
		fmt.Fprintf(&b, "\n  %s  %s", games[i].ID, games[i].Title)
	}
	return nil, fmt.Errorf("%s", b.String())
}

func pickExternalGame(cmd *cobra.Command, games []*cliclient.ExternalGame) (*cliclient.ExternalGame, error) {
	out := cmd.OutOrStdout()
	for i, eg := range games {
		fmt.Fprintf(out, "%2d) %s\n", i+1, eg.Title)
	}
	fmt.Fprint(out, "Select a game [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return games[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(games) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return games[n-1], nil
}

func newSyncResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <storefront> <ref>",
		Short: "Manually match an external game to an IGDB id",
		Args:  cobra.ExactArgs(2),
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
			igdbID, err := cmd.Flags().GetInt("igdb-id")
			if err != nil {
				return err
			}
			if igdbID == 0 {
				return fmt.Errorf("--igdb-id is required")
			}
			orphanAction, err := cmd.Flags().GetString("orphan-action")
			if err != nil {
				return err
			}
			eg, err := resolveExternalRef(cmd, c, p.Key, sf, args[1])
			if err != nil {
				return err
			}
			if err := c.RematchExternalGame(p.Key, eg.ID, igdbID, orphanAction); err != nil {
				return fmt.Errorf("resolve failed: %w", err)
			}
			fmt.Fprintf(out, "resolved %s -> igdb %d\n", eg.Title, igdbID)
			return nil
		},
	}
	cmd.Flags().Int("igdb-id", 0, "IGDB game id to match to (required)")
	cmd.Flags().String("orphan-action", "", "How to handle a user-game left orphaned by the rematch (remove)")
	return cmd
}

func newSyncSkipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skip <storefront> <ref>",
		Short: "Mark an external game as skipped",
		Args:  cobra.ExactArgs(2),
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
			eg, err := resolveExternalRef(cmd, c, p.Key, sf, args[1])
			if err != nil {
				return err
			}
			in := bufio.NewReader(cmd.InOrStdin())
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Skip %s?", eg.Title), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}
			if err := c.SkipExternalGame(p.Key, eg.ID); err != nil {
				return fmt.Errorf("skip failed: %w", err)
			}
			fmt.Fprintf(out, "skipped %s\n", eg.Title)
			return nil
		},
	}
	return cmd
}

func newSyncRetryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry <storefront>",
		Short: "Re-queue failed external games for a storefront",
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
			if err := c.RetryFailedExternalGames(p.Key, sf); err != nil {
				return fmt.Errorf("retry failed: %w", err)
			}
			fmt.Fprintf(out, "re-queued failed %s games\n", sf)
			return nil
		},
	}
}

func newSyncReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <storefront>",
		Short: "Interactively review external games that need attention",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !interactive(cmd) {
				return fmt.Errorf("review is interactive; use 'sync resolve'/'sync skip' off a terminal")
			}
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
			all, err := c.ListExternalGames(p.Key, sf)
			if err != nil {
				return fmt.Errorf("list external games failed: %w", err)
			}
			var pending []cliclient.ExternalGame
			for _, eg := range all {
				if eg.SyncStatus == "needs_review" {
					pending = append(pending, eg)
				}
			}
			if len(pending) == 0 {
				fmt.Fprintln(out, "Nothing to review.")
				return nil
			}
			in := bufio.NewReader(cmd.InOrStdin())
			for i := range pending {
				eg := &pending[i]
				fmt.Fprintf(out, "\n%s\n", eg.Title)
				fmt.Fprint(out, "[s]earch IGDB / s[k]ip / [n]ext / [q]uit: ")
				line, err := in.ReadString('\n')
				if err != nil {
					// EOF or closed stdin: stop reviewing
					break
				}
				switch strings.TrimSpace(line) {
				case "s":
					results, err := c.SearchIGDB(p.Key, eg.Title, 10)
					if err != nil {
						fmt.Fprintf(out, "search failed: %v\n", err)
						continue
					}
					if len(results.Games) == 0 {
						fmt.Fprintln(out, "No results found.")
						continue
					}
					for j, cand := range results.Games {
						fmt.Fprintf(out, "  %2d) %s (%s)\n", j+1, cand.Title, cand.ReleaseDate)
					}
					fmt.Fprint(out, "Select [1]: ")
					selLine, err := in.ReadString('\n')
					if err != nil {
						// EOF/closed stdin mid-selection: stop reviewing, same as
						// the outer-loop EOF (an unlabeled break here would only
						// exit the switch and silently advance to the next item).
						return nil
					}
					selStr := strings.TrimSpace(selLine)
					sel := 1
					if selStr != "" {
						n, parseErr := strconv.Atoi(selStr)
						if parseErr != nil || n < 1 || n > len(results.Games) {
							fmt.Fprintf(out, "invalid selection %q\n", selStr)
							continue
						}
						sel = n
					}
					chosen := results.Games[sel-1]
					if rematchErr := c.RematchExternalGame(p.Key, eg.ID, chosen.IgdbID, ""); rematchErr != nil {
						fmt.Fprintf(out, "rematch error: %v (use 'sync resolve --orphan-action' to handle this)\n", rematchErr)
						continue
					}
					fmt.Fprintf(out, "resolved %s -> igdb %d\n", eg.Title, chosen.IgdbID)
				case "k":
					if skipErr := c.SkipExternalGame(p.Key, eg.ID); skipErr != nil {
						fmt.Fprintf(out, "skip error: %v\n", skipErr)
						continue
					}
					fmt.Fprintf(out, "skipped %s\n", eg.Title)
				case "n":
					continue
				case "q":
					return nil
				default:
					fmt.Fprintln(out, "Unknown choice; skipping to next.")
				}
			}
			return nil
		},
	}
}
