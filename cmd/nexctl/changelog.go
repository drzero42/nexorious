package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newChangelogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Show release changelog entries",
		Long: "Show the changelog. With no flags, shows releases newer than the\n" +
			"version you last viewed (and marks them seen). --all shows the full\n" +
			"history (also marks seen). --since X.Y.Z shows entries newer than the\n" +
			"given version as a pure read (does not mark anything seen).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			all := flagBool(cmd, "all")
			since, _ := cmd.Flags().GetString("since") //nolint:errcheck // absent flag yields ""
			if all && since != "" {
				return fmt.Errorf("--all and --since are mutually exclusive")
			}

			c := cliclient.New(p.URL)
			res, err := c.GetChangelog(p.Key, all, since)
			if err != nil {
				return fmt.Errorf("get changelog failed: %w", err)
			}
			if !res.Available {
				fmt.Fprintln(out, "Changelog unavailable for this build.")
				return nil
			}
			if len(res.Entries) == 0 {
				fmt.Fprintln(out, "No new changelog entries.")
				return nil
			}
			for _, e := range res.Entries {
				if e.Date != "" {
					fmt.Fprintf(out, "## %s — %s\n\n", e.Version, e.Date)
				} else {
					fmt.Fprintf(out, "## %s\n\n", e.Version)
				}
				for _, g := range e.Groups {
					fmt.Fprintf(out, "### %s\n", g.Title)
					for _, it := range g.Items {
						fmt.Fprintf(out, "  - %s\n", it)
					}
					fmt.Fprintln(out)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Show the full changelog history (marks seen)")
	cmd.Flags().String("since", "", "Show entries newer than this version (pure read; e.g. 0.17.1)")
	return cmd
}
