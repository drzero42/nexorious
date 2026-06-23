package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newGameAddCmd() *cobra.Command {
	var (
		status, platform, storefront string
		notes                        string
		wishlist, loved              bool
		rating                       int
	)
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a game to your collection (IGDB lookup)",
		Long: "Add a game to your collection via IGDB lookup.\n\n" +
			"The query supports backend ID inference: \"igdb:NNNN\" does an exact\n" +
			"ID lookup, a bare \"NNNN\" does an ID lookup plus a name search, and\n" +
			"anything else is a name search.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if wishlist && platform != "" {
				return fmt.Errorf("--wishlist and --platform are mutually exclusive")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			cand, err := resolveIGDBCandidate(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			if _, err := c.ImportIGDBGame(p.Key, cand.IgdbID); err != nil {
				return fmt.Errorf("import IGDB game failed: %w", err)
			}

			in := cliclient.CreateUserGameInput{
				GameID:       cand.IgdbID,
				PlayStatus:   status,
				IsLoved:      loved,
				IsWishlisted: wishlist,
			}
			if cmd.Flags().Changed("rating") {
				in.PersonalRating = &rating
			}
			if cmd.Flags().Changed("notes") {
				in.PersonalNotes = &notes
			}
			if platform != "" {
				pl, sf := splitPlatform(platform)
				if storefront != "" {
					sf = storefront
				}
				if err := validatePlatform(c, p.Key, pl); err != nil {
					return err
				}
				in.Platforms = []cliclient.PlatformInput{{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}}
			}

			ug, err := c.CreateUserGame(p.Key, in)
			if err != nil {
				return fmt.Errorf("add game failed: %w", err)
			}
			fmt.Fprintf(out, "Added %q (%s).\n", cand.Title, ug.ID)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "not_started", "Initial play status")
	f.StringVar(&platform, "platform", "", "Platform slug, optionally platform/storefront")
	f.StringVar(&storefront, "storefront", "", "Storefront slug (overrides platform/storefront)")
	f.StringVar(&notes, "notes", "", "Personal notes")
	f.BoolVar(&wishlist, "wishlist", false, "Add to the wishlist instead of the library")
	f.BoolVar(&loved, "loved", false, "Mark as loved")
	f.IntVar(&rating, "rating", 0, "Personal rating 1–5")
	return cmd
}

// splitPlatform splits "platform/storefront" into its parts; a bare value yields
// an empty storefront.
func splitPlatform(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// findIGDBCandidates returns the IGDB candidates for a query. The backend
// performs ID inference ("igdb:NNNN" / bare number), so callers just pass the
// raw query. No cmd, no picker — callers own disambiguation.
func findIGDBCandidates(c *cliclient.Client, key, query string) ([]cliclient.IGDBCandidate, error) {
	res, err := c.SearchIGDB(key, query, 10)
	if err != nil {
		return nil, err
	}
	return res.Games, nil
}

// resolveIGDBCandidate finds the IGDB game to add by searching for the query
// (backend ID inference included), with a TTY picker / off-TTY candidate-list
// error when the search is ambiguous.
func resolveIGDBCandidate(cmd *cobra.Command, c *cliclient.Client, key, query string) (*cliclient.IGDBCandidate, error) {
	games, err := findIGDBCandidates(c, key, query)
	if err != nil {
		return nil, err
	}
	switch len(games) {
	case 0:
		return nil, fmt.Errorf("no IGDB results for %q", query)
	case 1:
		return &games[0], nil
	}
	if interactive(cmd) {
		return pickIGDB(cmd, games)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d IGDB games; re-run with igdb:<id>:", query, len(games))
	for i := range games {
		g := &games[i]
		marker := ""
		if g.UserGameID != nil {
			marker = " (in library)"
		}
		fmt.Fprintf(&b, "\n  %d  %s  %s%s", g.IgdbID, g.Title, g.ReleaseDate, marker)
	}
	return nil, fmt.Errorf("%s", b.String())
}

func pickIGDB(cmd *cobra.Command, games []cliclient.IGDBCandidate) (*cliclient.IGDBCandidate, error) {
	out := cmd.OutOrStdout()
	for i := range games {
		g := &games[i]
		marker := ""
		if g.UserGameID != nil {
			marker = " (in library)"
		}
		fmt.Fprintf(out, "%2d) %s  %s%s\n", i+1, g.Title, g.ReleaseDate, marker)
	}
	fmt.Fprint(out, "Select a game [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return &games[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(games) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return &games[n-1], nil
}
