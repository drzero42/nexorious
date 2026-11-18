package main

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameRmCmd() *cobra.Command {
	var filterStatus, filterTag, filterPlatform string
	var filterWishlist, useFilter bool
	cmd := &cobra.Command{
		Use:   "rm <ref…>",
		Short: "Remove games from your collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, useFilter, clientops.GameFilter{
				Status: filterStatus, Tag: filterTag, Platform: filterPlatform, Wishlist: filterWishlist,
				WishlistSet: cmd.Flags().Changed("filter-wishlist"),
			})
			if err != nil {
				return err
			}
			if len(games) == 0 {
				fmt.Fprintln(out, "No games matched.")
				return nil
			}

			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete %d game(s)?", len(games)), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			for i := range games {
				if err := c.DeleteUserGame(cmd.Context(), p.Key, games[i].ID); err != nil {
					return fmt.Errorf("delete %s failed: %w", games[i].ID, err)
				}
				fmt.Fprintf(out, "Removed %q (%s).\n", games[i].Title(), games[i].ID)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&useFilter, "filter", false, "Select games by filter instead of refs")
	f.StringVar(&filterStatus, "status", "", "Filter: play status")
	f.StringVar(&filterTag, "tag", "", "Filter: tag name")
	f.StringVar(&filterPlatform, "platform", "", "Filter: platform slug")
	f.BoolVar(&filterWishlist, "filter-wishlist", false, "Filter: only wishlisted")
	return cmd
}

// gamesForRefsOrFilter resolves explicit refs (args) or, when useFilter is set,
// runs a filtered list. Exactly one mode must be used.
func gamesForRefsOrFilter(cmd *cobra.Command, c *cliclient.Client, key string, args []string, useFilter bool, f clientops.GameFilter) ([]cliclient.UserGame, error) {
	if useFilter {
		if len(args) > 0 {
			return nil, fmt.Errorf("pass refs or --filter, not both")
		}
		games, total, err := clientops.GamesByFilter(cmd.Context(), c, key, f)
		if err != nil {
			return nil, err
		}
		if total > len(games) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: filter matched %d games but only the first %d are affected; narrow the filter and re-run for the rest\n",
				total, len(games))
		}
		return games, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("provide one or more refs, or --filter")
	}
	games := make([]cliclient.UserGame, 0, len(args))
	for _, ref := range args {
		u, err := resolveUserGameRef(cmd, c, key, ref)
		if err != nil {
			return nil, err
		}
		games = append(games, *u)
	}
	return games, nil
}
