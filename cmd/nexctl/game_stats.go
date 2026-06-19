package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show collection statistics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			s, err := c.GetCollectionStats(p.Key)
			if err != nil {
				return fmt.Errorf("get collection stats failed: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, s)
			}
			if flagBool(cmd, "quiet") {
				fmt.Fprintln(out, s.TotalGames)
				return nil
			}

			avg := "n/a"
			if s.AverageRating != nil {
				avg = strconv.FormatFloat(*s.AverageRating, 'f', -1, 64)
			}
			fmt.Fprintf(out, "Total games:     %d\n", s.TotalGames)
			fmt.Fprintf(out, "Hours played:    %s\n", strconv.FormatFloat(s.TotalHoursPlayed, 'f', -1, 64))
			fmt.Fprintf(out, "Completion rate: %s%%\n", strconv.FormatFloat(s.CompletionRate, 'f', -1, 64))
			fmt.Fprintf(out, "Pile of shame:   %d\n", s.PileOfShame)
			fmt.Fprintf(out, "Average rating:  %s\n", avg)

			printStatSection(out, "Completion", s.CompletionStats)
			printStatSection(out, "Ownership", s.OwnershipStats)
			printStatSection(out, "Platforms", s.PlatformStats)
			printStatSection(out, "Genres", s.GenreStats)
			return nil
		},
	}
}

// printStatSection renders a named map of counts as an indented sub-table,
// ordered by count desc then name asc. Empty maps are skipped.
func printStatSection(out io.Writer, title string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	fmt.Fprintf(out, "\n%s\n", title)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, k := range keys {
		fmt.Fprintf(tw, "  %s\t%d\n", k, counts[k])
	}
	_ = tw.Flush() //nolint:errcheck // best-effort render to an in-memory/stdout writer
}
