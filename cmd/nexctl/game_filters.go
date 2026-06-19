package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
	"github.com/drzero42/nexorious/internal/enum"
)

// gameFilters aggregates every facet value the `game list` filters accept, for
// discovery. Statuses are stable enums read from the client; storefronts and
// the IGDB-derived facets come from the server so the client stays decoupled
// from the backend version.
type gameFilters struct {
	PlayStatuses       []string               `json:"play_statuses"`
	OwnershipStatuses  []string               `json:"ownership_statuses"`
	Storefronts        []cliclient.Storefront `json:"storefronts"`
	Genres             []string               `json:"genres"`
	GameModes          []string               `json:"game_modes"`
	Themes             []string               `json:"themes"`
	PlayerPerspectives []string               `json:"player_perspectives"`
}

func newGameFiltersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "filters",
		Short: "List valid values for the game-list filters",
		Long: "List the valid values for each game-list filter facet: play and " +
			"ownership statuses, storefronts, and the genre/game-mode/theme/" +
			"perspective values present in your library. Tags and platforms have " +
			"their own commands (`tag list`, `platform list`).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			opts, err := c.GetFilterOptions(p.Key)
			if err != nil {
				return fmt.Errorf("get filter options failed: %w", err)
			}
			storefronts, err := c.ListStorefronts(p.Key)
			if err != nil {
				return fmt.Errorf("list storefronts failed: %w", err)
			}

			f := gameFilters{
				PlayStatuses:       enum.AllPlayStatuses(),
				OwnershipStatuses:  enum.AllOwnershipStatuses(),
				Storefronts:        storefronts,
				Genres:             opts.Genres,
				GameModes:          opts.GameModes,
				Themes:             opts.Themes,
				PlayerPerspectives: opts.PlayerPerspectives,
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, f)
			}
			if flagBool(cmd, "quiet") {
				printFilterValuesQuiet(out, f)
				return nil
			}
			printFilterSection(out, "Play statuses", f.PlayStatuses)
			printFilterSection(out, "Ownership statuses", f.OwnershipStatuses)
			printStorefrontSection(out, f.Storefronts)
			printFilterSection(out, "Genres", f.Genres)
			printFilterSection(out, "Game modes", f.GameModes)
			printFilterSection(out, "Themes", f.Themes)
			printFilterSection(out, "Player perspectives", f.PlayerPerspectives)
			return nil
		},
	}
}

// printFilterSection renders a named facet as an indented list. Empty facets
// are skipped.
func printFilterSection(out io.Writer, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", title)
	for _, v := range values {
		fmt.Fprintf(out, "  %s\n", v)
	}
}

// printStorefrontSection renders storefronts as slug + display-name pairs.
func printStorefrontSection(out io.Writer, storefronts []cliclient.Storefront) {
	if len(storefronts) == 0 {
		return
	}
	fmt.Fprintf(out, "\nStorefronts\n")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, s := range storefronts {
		fmt.Fprintf(tw, "  %s\t%s\n", s.Name, s.DisplayName)
	}
	_ = tw.Flush() //nolint:errcheck // best-effort render to an in-memory/stdout writer
}

// printFilterValuesQuiet emits every facet value as a bare token, one per line,
// for piping. Storefronts use their slug (not the display name).
func printFilterValuesQuiet(out io.Writer, f gameFilters) {
	for _, group := range [][]string{
		f.PlayStatuses, f.OwnershipStatuses,
		f.Genres, f.GameModes, f.Themes, f.PlayerPerspectives,
	} {
		for _, v := range group {
			fmt.Fprintln(out, v)
		}
	}
	for _, s := range f.Storefronts {
		fmt.Fprintln(out, s.Name)
	}
}
