package main

import (
	"bufio"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newImportReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <job-id>",
		Short: "Interactively resolve a job's items that need review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !interactive(cmd) {
				return fmt.Errorf("review is interactive; use 'import resolve'/'import skip' off a terminal")
			}
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			params := url.Values{}
			params.Set("status", "pending_review")
			page, err := c.GetJobItems(p.Key, args[0], params)
			if err != nil {
				return fmt.Errorf("get job items failed: %w", err)
			}
			if len(page.Items) == 0 {
				fmt.Fprintln(out, "Nothing to review.")
				return nil
			}
			in := bufio.NewReader(cmd.InOrStdin())
			for i := range page.Items {
				item := &page.Items[i]
				fmt.Fprintf(out, "\n%s\n", item.SourceTitle)
				fmt.Fprint(out, "[s]earch IGDB / s[k]ip / [n]ext / [q]uit: ")
				line, err := in.ReadString('\n')
				if err != nil {
					// EOF or closed stdin: stop reviewing
					break
				}
				switch strings.TrimSpace(line) {
				case "s":
					results, err := c.SearchIGDB(p.Key, item.SourceTitle, 10)
					if err != nil {
						fmt.Fprintf(out, "search failed: %v\n", err)
						continue
					}
					if len(results.Games) == 0 {
						fmt.Fprintln(out, "No results found.")
						continue
					}
					for j, cand := range results.Games {
						fmt.Fprintf(out, "  %2d) %s (%s)\n", j+1, cand.Title, cand.ReleaseDate)
					}
					fmt.Fprint(out, "Select [1]: ")
					selLine, err := in.ReadString('\n')
					if err != nil {
						// EOF/closed stdin mid-selection: stop reviewing, same as
						// the outer-loop EOF (an unlabeled break here would only
						// exit the switch and silently advance to the next item).
						return nil
					}
					selStr := strings.TrimSpace(selLine)
					sel := 1
					if selStr != "" {
						n, parseErr := strconv.Atoi(selStr)
						if parseErr != nil || n < 1 || n > len(results.Games) {
							fmt.Fprintf(out, "invalid selection %q\n", selStr)
							continue
						}
						sel = n
					}
					chosen := results.Games[sel-1]
					if resolveErr := c.ResolveJobItem(p.Key, item.ID, chosen.IgdbID); resolveErr != nil {
						fmt.Fprintf(out, "resolve error: %v\n", resolveErr)
						continue
					}
					fmt.Fprintf(out, "resolved %s -> igdb %d\n", item.SourceTitle, chosen.IgdbID)
				case "k":
					if skipErr := c.SkipJobItem(p.Key, item.ID); skipErr != nil {
						fmt.Fprintf(out, "skip error: %v\n", skipErr)
						continue
					}
					fmt.Fprintf(out, "skipped %s\n", item.SourceTitle)
				case "n":
					continue
				case "q":
					return nil
				default:
					fmt.Fprintln(out, "Unknown choice; skipping to next.")
				}
			}
			return nil
		},
	}
}

func newImportResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <item-id>",
		Short: "Resolve a job item by setting its IGDB game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			igdbID, err := cmd.Flags().GetInt("igdb-id")
			if err != nil {
				return err
			}
			if igdbID <= 0 {
				return fmt.Errorf("--igdb-id is required")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if err := cliclient.New(p.URL).ResolveJobItem(p.Key, args[0], igdbID); err != nil {
				return fmt.Errorf("resolve failed: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resolved %s -> igdb %d\n", args[0], igdbID)
			return nil
		},
	}
	cmd.Flags().Int("igdb-id", 0, "IGDB game ID to assign")
	return cmd
}

func newImportSkipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skip <item-id>",
		Short: "Mark a job item as skipped",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Skip item %s?", args[0]), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if err := cliclient.New(p.URL).SkipJobItem(p.Key, args[0]); err != nil {
				return fmt.Errorf("skip failed: %w", err)
			}
			fmt.Fprintf(out, "skipped %s\n", args[0])
			return nil
		},
	}
}
