package main

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameEditCmd() *cobra.Command {
	var (
		status, notes, addPlatform, rmPlatform, hoursPlatform string
		rating                                                int
		hours                                                 float64
		loved, noLoved                                        bool
		addTags, rmTags                                       []string
		useFilter                                             bool
		filterStatus, filterTag, filterPlatform               string
		filterWishlist                                        bool
	)
	cmd := &cobra.Command{
		Use:   "edit <ref…>",
		Short: "Edit one or more games (status, rating, notes, platforms, tags)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if loved && noLoved {
				return fmt.Errorf("--loved and --no-loved are mutually exclusive")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, useFilter, clientops.GameFilter{
				Status: filterStatus, Tag: filterTag, Platform: filterPlatform,
				Wishlist: filterWishlist, WishlistSet: cmd.Flags().Changed("filter-wishlist"),
			})
			if err != nil {
				return err
			}
			if len(games) == 0 {
				fmt.Fprintln(out, "No games matched.")
				return nil
			}
			if useFilter || len(games) > 1 {
				ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
					fmt.Sprintf("Edit %d game(s)?", len(games)), flagBool(cmd, "yes"))
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("aborted")
				}
			}

			ch := cmd.Flags().Changed
			for i := range games {
				u := &games[i]
				if err := clientops.EditOne(cmd.Context(), c, p.Key, u, clientops.EditOpts{
					Status: status, StatusSet: ch("status"),
					Rating: rating, RatingSet: ch("rating"),
					Loved: loved, NoLoved: noLoved,
					Notes: notes, NotesSet: ch("notes"),
					AddPlatform: addPlatform, RmPlatform: rmPlatform,
					Hours: hours, HoursSet: ch("hours"), HoursPlatform: hoursPlatform,
					AddTags: addTags, RmTags: rmTags,
				}); err != nil {
					return fmt.Errorf("edit %q (%s): %w", u.Title(), u.ID, err)
				}
				fmt.Fprintf(out, "Updated %q (%s).\n", u.Title(), u.ID)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Set play status")
	f.IntVar(&rating, "rating", 0, "Set personal rating 1–5")
	f.BoolVar(&loved, "loved", false, "Mark as loved")
	f.BoolVar(&noLoved, "no-loved", false, "Unmark loved")
	f.StringVar(&notes, "notes", "", "Set personal notes")
	f.StringVar(&addPlatform, "add-platform", "", "Add a platform slug (platform[/storefront])")
	f.StringVar(&rmPlatform, "rm-platform", "", "Remove a platform by slug")
	f.Float64Var(&hours, "hours", 0, "Set hours played on a platform")
	f.StringVar(&hoursPlatform, "platform", "", "Platform slug for --hours (when the game has several)")
	f.StringArrayVar(&addTags, "tag", nil, "Add a tag (repeatable)")
	f.StringArrayVar(&rmTags, "untag", nil, "Remove a tag (repeatable)")
	f.BoolVar(&useFilter, "filter", false, "Select games by filter instead of refs")
	f.StringVar(&filterStatus, "filter-status", "", "Filter: play status")
	f.StringVar(&filterTag, "filter-tag", "", "Filter: tag name")
	f.StringVar(&filterPlatform, "filter-platform", "", "Filter: platform slug")
	f.BoolVar(&filterWishlist, "filter-wishlist", false, "Filter: only wishlisted")
	return cmd
}
