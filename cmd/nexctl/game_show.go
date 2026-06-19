package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show details for a game (by id or title)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			u, err := resolveUserGameRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, u)
			}
			fmt.Fprintf(out, "%s\n", u.Title())
			fmt.Fprintf(out, "  id:      %s\n", u.ID)
			fmt.Fprintf(out, "  status:  %s\n", statusOf(u))
			fmt.Fprintf(out, "  rating:  %s\n", ratingOf(u))
			fmt.Fprintf(out, "  loved:   %t\n", u.IsLoved)
			fmt.Fprintf(out, "  hours:   %s\n", strconv.FormatFloat(u.HoursPlayed, 'f', -1, 64))
			fmt.Fprintf(out, "  tags:    %s\n", tagsOf(u))
			if u.PersonalNotes != nil && *u.PersonalNotes != "" {
				fmt.Fprintf(out, "  notes:   %s\n", *u.PersonalNotes)
			}
			if len(u.Platforms) > 0 {
				fmt.Fprintln(out, "  platforms:")
				for i := range u.Platforms {
					pl := &u.Platforms[i]
					hours := "-"
					if pl.HoursPlayed != nil {
						hours = strconv.FormatFloat(*pl.HoursPlayed, 'f', -1, 64)
					}
					fmt.Fprintf(out, "    - %s [%s] (%s h, id %s)\n", deref(pl.Platform), deref(pl.Storefront), hours, pl.ID)
				}
			}
			return nil
		},
	}
}

func deref(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
