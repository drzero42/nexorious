package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameListCmd() *cobra.Command {
	var (
		status, ownership, tag, platform, storefront, genre string
		pool, sortBy, order                                 string
		wishlist, loved, hasNotes                           bool
		ratingMin, ratingMax, hoursMin, hoursMax            float64
		limit, page                                         int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List games in your collection",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			params := url.Values{}
			setIf(params, "play_status", status)
			setIf(params, "ownership_status", ownership)
			setIf(params, "platform", platform)
			setIf(params, "storefront", storefront)
			setIf(params, "genre", genre)
			setIf(params, "pool", pool)
			setIf(params, "sort_by", sortBy)
			setIf(params, "sort_order", order)
			if cmd.Flags().Changed("wishlist") {
				params.Set("wishlist", strconv.FormatBool(wishlist))
			}
			if cmd.Flags().Changed("loved") {
				params.Set("is_loved", strconv.FormatBool(loved))
			}
			if cmd.Flags().Changed("has-notes") {
				params.Set("has_notes", strconv.FormatBool(hasNotes))
			}
			if cmd.Flags().Changed("rating-min") {
				params.Set("rating_min", strconv.FormatFloat(ratingMin, 'f', -1, 64))
			}
			if cmd.Flags().Changed("rating-max") {
				params.Set("rating_max", strconv.FormatFloat(ratingMax, 'f', -1, 64))
			}
			if cmd.Flags().Changed("hours-min") {
				params.Set("time_to_beat_min", strconv.FormatFloat(hoursMin, 'f', -1, 64))
			}
			if cmd.Flags().Changed("hours-max") {
				params.Set("time_to_beat_max", strconv.FormatFloat(hoursMax, 'f', -1, 64))
			}
			if limit > 0 {
				params.Set("per_page", strconv.Itoa(limit))
			}
			if page > 0 {
				params.Set("page", strconv.Itoa(page))
			}
			if tag != "" {
				id, err := resolveTagID(c, p.Key, tag)
				if err != nil {
					return err
				}
				params.Set("tag", id)
			}

			res, err := c.ListUserGames(p.Key, params)
			if err != nil {
				return fmt.Errorf("list games failed: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, res)
			}
			if flagBool(cmd, "quiet") {
				for i := range res.UserGames {
					fmt.Fprintln(out, res.UserGames[i].ID)
				}
				return nil
			}
			if len(res.UserGames) == 0 {
				fmt.Fprintln(out, "No games.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTITLE\tSTATUS\tRATING\tHOURS\tPLATFORMS\tTAGS")
			for i := range res.UserGames {
				u := &res.UserGames[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					u.ID, u.Title(), statusOf(u), ratingOf(u),
					strconv.FormatFloat(u.HoursPlayed, 'f', -1, 64), platformsOf(u), tagsOf(u))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d of %d (page %d/%d)\n", len(res.UserGames), res.Total, max1(res.Page), max1(res.Pages))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Filter by play status")
	f.StringVar(&ownership, "ownership", "", "Filter by ownership status")
	f.StringVar(&tag, "tag", "", "Filter by tag name")
	f.StringVar(&platform, "platform", "", "Filter by platform slug")
	f.StringVar(&storefront, "storefront", "", "Filter by storefront slug")
	f.StringVar(&genre, "genre", "", "Filter by genre")
	f.StringVar(&pool, "pool", "", "Filter by pool id")
	f.StringVar(&sortBy, "sort", "", "Sort field (title, created_at, personal_rating, hours_played, …)")
	f.StringVar(&order, "order", "", "Sort order (asc|desc)")
	f.BoolVar(&wishlist, "wishlist", false, "Show only wishlisted games")
	f.BoolVar(&loved, "loved", false, "Filter by loved")
	f.BoolVar(&hasNotes, "has-notes", false, "Filter by has-notes")
	f.Float64Var(&ratingMin, "rating-min", 0, "Minimum personal rating")
	f.Float64Var(&ratingMax, "rating-max", 0, "Maximum personal rating")
	f.Float64Var(&hoursMin, "hours-min", 0, "Minimum time-to-beat hours")
	f.Float64Var(&hoursMax, "hours-max", 0, "Maximum time-to-beat hours")
	f.IntVar(&limit, "limit", 0, "Max results per page")
	f.IntVar(&page, "page", 0, "Page number")
	return cmd
}

func setIf(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// resolveTagID maps a tag name to its id (case-insensitive), erroring if unknown.
func resolveTagID(c *cliclient.Client, key, name string) (string, error) {
	tags, err := c.ListTags(key)
	if err != nil {
		return "", fmt.Errorf("resolve tag %q: %w", name, err)
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no tag named %q", name)
}
