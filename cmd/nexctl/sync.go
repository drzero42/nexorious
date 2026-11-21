package main

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
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
	// amazon-games is two steps: without --attempt-id, connect first starts a
	// sign-in (startAmazonGamesLogin) and fills it in.
	"amazon-games": {
		{bodyKey: "attempt_id", flagName: "attempt-id", label: "Sign-in attempt id", secret: false},
		{bodyKey: "redirect_url", flagName: "redirect-url", label: "Address Amazon took you to", secret: false},
	},
}

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "sync", Short: "Manage storefront sync"}
	cmd.AddCommand(newSyncStatusCmd(), newSyncConnectCmd(), newSyncDisconnectCmd(),
		newSyncConfigCmd(), newSyncRunCmd(), newSyncResetCmd(),
		newSyncReviewCmd(), newSyncResolveCmd(), newSyncSkipCmd(), newSyncRetryCmd())
	return cmd
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
  humble-bundle      --session-cookie
  amazon-games       --redirect-url (and --attempt-id from an earlier run);
                     without --attempt-id it prints the Amazon sign-in address first`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}

			fields, ok := storefrontCreds[sf]
			if !ok {
				return fmt.Errorf("no credential fields defined for storefront %q", sf)
			}

			if sf == "amazon-games" {
				done, err := startAmazonGamesLogin(cmd, c, p.Key)
				if err != nil || done {
					return err
				}
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

			resp, err := c.ConnectStorefront(cmd.Context(), p.Key, sf, body)
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
	f.String("attempt-id", "", "Sign-in attempt id from an earlier run (amazon-games only)")
	f.String("redirect-url", "", "Address Amazon took you to after signing in (amazon-games only)")
	return cmd
}

// startAmazonGamesLogin starts an Amazon sign-in unless --attempt-id names one,
// prints the address to open, and sets --attempt-id for the field loop, which
// then prompts for the address. Off a TTY there is nothing to read the address
// from, so it prints the command that finishes the sign-in and reports done.
func startAmazonGamesLogin(cmd *cobra.Command, c *cliclient.Client, key string) (done bool, err error) {
	out := cmd.OutOrStdout()
	attempt, _ := cmd.Flags().GetString("attempt-id") //nolint:errcheck // absent flag yields ""
	if attempt != "" {
		return false, nil
	}
	if redirect, _ := cmd.Flags().GetString("redirect-url"); redirect != "" { //nolint:errcheck // absent flag yields ""
		// An address only finishes the attempt it was signed in for.
		return false, errors.New("--redirect-url needs the --attempt-id printed with its sign-in address; run without --redirect-url to start one")
	}
	login, err := c.StartAmazonGamesLogin(cmd.Context(), key)
	if err != nil {
		return false, fmt.Errorf("start Amazon sign-in failed: %w", err)
	}
	fmt.Fprintf(out, "Open this address, sign in to Amazon, then copy the address of the page you land on:\n\n  %s\n\n", login.LoginURL)
	if !interactive(cmd) {
		fmt.Fprintf(out, "Then, within 15 minutes, run:\n  nexctl sync connect amazon-games --attempt-id %s --redirect-url '<address>'\n", login.AttemptID)
		return true, nil
	}
	return false, cmd.Flags().Set("attempt-id", login.AttemptID)
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
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

			if err := c.DisconnectStorefront(cmd.Context(), p.Key, sf); err != nil {
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
				configs, err := c.ListSyncConfigs(cmd.Context(), p.Key)
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
			storefront, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}
			status, err := c.GetSyncStatus(cmd.Context(), p.Key, storefront)
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("frequency") {
				freq, err := cmd.Flags().GetString("frequency")
				if err != nil {
					return err
				}
				cfg, err := c.UpdateSyncConfig(cmd.Context(), p.Key, sf, freq)
				if err != nil {
					return fmt.Errorf("update sync config failed: %w", err)
				}
				if flagBool(cmd, "json") {
					return cliui.EncodeJSON(out, cfg)
				}
				fmt.Fprintf(out, "%s sync frequency: %s\n", cfg.Storefront, cfg.Frequency)
				return nil
			}

			cfg, err := c.GetSyncConfig(cmd.Context(), p.Key, sf)
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}
			result, err := c.TriggerSync(cmd.Context(), p.Key, sf)
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
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

			if err := c.ResetSyncData(cmd.Context(), p.Key, sf); err != nil {
				return fmt.Errorf("reset sync data failed: %w", err)
			}
			fmt.Fprintf(out, "reset %s sync data\n", sf)
			return nil
		},
	}
}

// resolveExternalRef resolves an external game by UUID or title from the given
// storefront's external-game list. UUID matching is exact; title matching is
// case-insensitive. When multiple titles match on a non-interactive session it
// returns an error listing candidates; on an interactive session it presents a
// numbered picker.
func resolveExternalRef(cmd *cobra.Command, c *cliclient.Client, key, sf, ref string) (*cliclient.ExternalGame, error) {
	games, err := clientops.FindExternalGamesByRef(cmd.Context(), c, key, sf, ref)
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
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
			if err := c.RematchExternalGame(cmd.Context(), p.Key, eg.ID, igdbID, orphanAction); err != nil {
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
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
			if err := c.SkipExternalGame(cmd.Context(), p.Key, eg.ID); err != nil {
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}
			if err := c.RetryFailedExternalGames(cmd.Context(), p.Key, sf); err != nil {
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
			sf, err := clientops.ResolveStorefront(cmd.Context(), c, p.Key, args[0])
			if err != nil {
				return err
			}
			all, err := c.ListExternalGames(cmd.Context(), p.Key, sf)
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
				if eg.StoreURL != nil {
					fmt.Fprintf(out, "  %s\n", *eg.StoreURL)
				}
				fmt.Fprint(out, "[s]earch IGDB / s[k]ip / [n]ext / [q]uit: ")
				line, err := in.ReadString('\n')
				if err != nil {
					// EOF or closed stdin: stop reviewing
					break
				}
				switch strings.TrimSpace(line) {
				case "s":
					results, err := c.SearchIGDB(cmd.Context(), p.Key, eg.Title, 10)
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
					if rematchErr := c.RematchExternalGame(cmd.Context(), p.Key, eg.ID, chosen.IgdbID, ""); rematchErr != nil {
						fmt.Fprintf(out, "rematch error: %v (use 'sync resolve --orphan-action' to handle this)\n", rematchErr)
						continue
					}
					fmt.Fprintf(out, "resolved %s -> igdb %d\n", eg.Title, chosen.IgdbID)
				case "k":
					if skipErr := c.SkipExternalGame(cmd.Context(), p.Key, eg.ID); skipErr != nil {
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
