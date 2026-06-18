package main

import (
	"bufio"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
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

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, gameFilter{
				use: useFilter, status: filterStatus, tag: filterTag, platform: filterPlatform, wishlist: filterWishlist,
				wishlistSet: cmd.Flags().Changed("filter-wishlist"),
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
				if err := c.DeleteUserGame(p.Key, games[i].ID); err != nil {
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

// gameFilter is the subset of list filters supported for bulk edit/rm selection.
type gameFilter struct {
	use                   bool
	status, tag, platform string
	wishlist, wishlistSet bool
}

// gamesForRefsOrFilter resolves explicit refs (args) or, when f.use is set, runs
// a filtered list. Exactly one mode must be used.
func gamesForRefsOrFilter(cmd *cobra.Command, c *cliclient.Client, key string, args []string, f gameFilter) ([]cliclient.UserGame, error) {
	if f.use {
		if len(args) > 0 {
			return nil, fmt.Errorf("pass refs or --filter, not both")
		}
		params := url.Values{}
		setIf(params, "play_status", f.status)
		setIf(params, "platform", f.platform)
		if f.wishlistSet {
			params.Set("wishlist", strconv.FormatBool(f.wishlist))
		}
		if f.tag != "" {
			id, err := resolveTagID(c, key, f.tag)
			if err != nil {
				return nil, err
			}
			params.Set("tag", id)
		}
		params.Set("per_page", "200")
		res, err := c.ListUserGames(key, params)
		if err != nil {
			return nil, err
		}
		if res.Total > len(res.UserGames) {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: filter matched %d games but only the first %d are affected; narrow the filter and re-run for the rest\n",
				res.Total, len(res.UserGames))
		}
		return res.UserGames, nil
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
