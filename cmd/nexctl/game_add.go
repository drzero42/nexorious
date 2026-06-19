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
		igdbID                       int
		status, platform, storefront string
		notes                        string
		wishlist, loved              bool
		rating                       int
	)
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a game to your collection (IGDB lookup)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if wishlist && platform != "" {
				return fmt.Errorf("--wishlist and --platform are mutually exclusive")
			}
			if igdbID == 0 && len(args) == 0 {
				return fmt.Errorf("provide a title or --igdb-id")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			cand, err := resolveIGDBCandidate(cmd, c, p.Key, igdbID, firstArg(args))
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
	f.IntVar(&igdbID, "igdb-id", 0, "Add by IGDB id (skips title search)")
	f.StringVar(&status, "status", "not_started", "Initial play status")
	f.StringVar(&platform, "platform", "", "Platform slug, optionally platform/storefront")
	f.StringVar(&storefront, "storefront", "", "Storefront slug (overrides platform/storefront)")
	f.StringVar(&notes, "notes", "", "Personal notes")
	f.BoolVar(&wishlist, "wishlist", false, "Add to the wishlist instead of the library")
	f.BoolVar(&loved, "loved", false, "Mark as loved")
	f.IntVar(&rating, "rating", 0, "Personal rating 1–5")
	return cmd
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// splitPlatform splits "platform/storefront" into its parts; a bare value yields
// an empty storefront.
func splitPlatform(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// findIGDBCandidates returns the IGDB candidates for an id or title query. No
// cmd, no picker — callers own disambiguation.
func findIGDBCandidates(c *cliclient.Client, key string, igdbID int, title string) ([]cliclient.IGDBCandidate, error) {
	if igdbID != 0 {
		res, err := c.GetIGDBGame(key, igdbID)
		if err != nil {
			return nil, err
		}
		return res.Games, nil
	}
	res, err := c.SearchIGDB(key, title, 10)
	if err != nil {
		return nil, err
	}
	return res.Games, nil
}

// resolveIGDBCandidate finds the IGDB game to add: by id, or by title search with
// a TTY picker / off-TTY candidate-list error.
func resolveIGDBCandidate(cmd *cobra.Command, c *cliclient.Client, key string, igdbID int, title string) (*cliclient.IGDBCandidate, error) {
	games, err := findIGDBCandidates(c, key, igdbID, title)
	if err != nil {
		return nil, err
	}
	if igdbID != 0 {
		if len(games) == 0 {
			return nil, fmt.Errorf("no IGDB game with id %d", igdbID)
		}
		return &games[0], nil
	}
	switch len(games) {
	case 0:
		return nil, fmt.Errorf("no IGDB results for %q", title)
	case 1:
		return &games[0], nil
	}
	if interactive(cmd) {
		return pickIGDB(cmd, games)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d IGDB games; re-run with --igdb-id:", title, len(games))
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
