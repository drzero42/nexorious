package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
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

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, gameFilter{
				use: useFilter, status: filterStatus, tag: filterTag, platform: filterPlatform,
				wishlist: filterWishlist, wishlistSet: cmd.Flags().Changed("filter-wishlist"),
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
				if err := editOne(c, p.Key, u, editOpts{
					status: status, statusSet: ch("status"),
					rating: rating, ratingSet: ch("rating"),
					loved: loved, noLoved: noLoved,
					notes: notes, notesSet: ch("notes"),
					addPlatform: addPlatform, rmPlatform: rmPlatform,
					hours: hours, hoursSet: ch("hours"), hoursPlatform: hoursPlatform,
					addTags: addTags, rmTags: rmTags,
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

type editOpts struct {
	status                  string
	statusSet               bool
	rating                  int
	ratingSet               bool
	loved, noLoved          bool
	notes                   string
	notesSet                bool
	addPlatform, rmPlatform string
	hours                   float64
	hoursSet                bool
	hoursPlatform           string
	addTags, rmTags         []string
}

func editOne(c *cliclient.Client, key string, u *cliclient.UserGame, o editOpts) error {
	if o.addPlatform != "" {
		pl, sf := splitPlatform(o.addPlatform)
		if err := c.AddPlatform(key, u.ID, cliclient.PlatformInput{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}); err != nil {
			return fmt.Errorf("add platform: %w", err)
		}
	}
	if o.rmPlatform != "" {
		pid, err := platformIDBySlug(c, key, u, o.rmPlatform)
		if err != nil {
			return err
		}
		if err := c.DeletePlatform(key, u.ID, pid); err != nil {
			return fmt.Errorf("remove platform: %w", err)
		}
	}
	if o.hoursSet {
		pid, err := targetPlatformID(c, key, u, o.hoursPlatform)
		if err != nil {
			return err
		}
		if err := c.UpdatePlatform(key, u.ID, pid, map[string]any{"hours_played": o.hours}); err != nil {
			return fmt.Errorf("set hours: %w", err)
		}
	}
	if o.statusSet {
		if _, err := c.UpdateProgress(key, u.ID, o.status); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
	}
	fields := map[string]any{}
	if o.ratingSet {
		fields["personal_rating"] = o.rating
	}
	if o.loved {
		fields["is_loved"] = true
	}
	if o.noLoved {
		fields["is_loved"] = false
	}
	if o.notesSet {
		fields["personal_notes"] = o.notes
	}
	if len(fields) > 0 {
		if _, err := c.UpdateUserGame(key, u.ID, fields); err != nil {
			return fmt.Errorf("update fields: %w", err)
		}
	}
	if len(o.addTags) > 0 || len(o.rmTags) > 0 {
		if err := applyTagEdits(c, key, u, o.addTags, o.rmTags); err != nil {
			return err
		}
	}
	return nil
}

// platformIDBySlug finds the platform row id whose platform slug matches.
func platformIDBySlug(c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return "", err
	}
	for i := range cur.Platforms {
		if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
			return cur.Platforms[i].ID, nil
		}
	}
	return "", fmt.Errorf("no platform %q on this game", slug)
}

// targetPlatformID picks the platform row for --hours: the named slug, else the
// sole platform, else an error asking which.
func targetPlatformID(c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return "", err
	}
	if slug != "" {
		for i := range cur.Platforms {
			if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
				return cur.Platforms[i].ID, nil
			}
		}
		return "", fmt.Errorf("no platform %q on this game", slug)
	}
	if len(cur.Platforms) == 1 {
		return cur.Platforms[0].ID, nil
	}
	return "", fmt.Errorf("game has %d platforms; specify which with --platform", len(cur.Platforms))
}

// applyTagEdits computes current ∪ add \ remove and replaces the tag set.
func applyTagEdits(c *cliclient.Client, key string, u *cliclient.UserGame, add, remove []string) error {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return err
	}
	set := map[string]string{} // lower -> display
	for _, t := range cur.Tags {
		set[strings.ToLower(t.Name)] = t.Name
	}
	for _, t := range add {
		set[strings.ToLower(t)] = t
	}
	for _, t := range remove {
		delete(set, strings.ToLower(t))
	}
	names := make([]string, 0, len(set))
	for _, disp := range set {
		names = append(names, disp)
	}
	if _, err := c.ReplaceTags(key, u.ID, names); err != nil {
		return fmt.Errorf("update tags: %w", err)
	}
	return nil
}
