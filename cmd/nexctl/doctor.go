package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newDoctorCmd() *cobra.Command {
	var check string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect and fix library health issues (\"smells\")",
		Long: "doctor scans your collection for data-quality issues. With no flags it\n" +
			"prints a summary of every check; --check <id> lists the flagged games for\n" +
			"one check. Use the apply/ignore/restore subcommands to act on findings.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if check != "" {
				return runDoctorDetail(cmd, check)
			}
			return runDoctorSummary(cmd)
		},
	}
	cmd.Flags().StringVar(&check, "check", "", "List the flagged games for one check id")
	cmd.AddCommand(newDoctorApplyCmd(), newDoctorIgnoreCmd(), newDoctorRestoreCmd(), newDoctorIgnoredCmd())
	return cmd
}

func runDoctorSummary(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	p, _, err := resolveProfile(cmd)
	if err != nil {
		return err
	}
	checks, err := cliclient.New(p.URL).ListSmells(p.Key)
	if err != nil {
		return fmt.Errorf("list smells failed: %w", err)
	}
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, checks)
	}
	if flagBool(cmd, "quiet") {
		for i := range checks {
			if checks[i].Count > 0 {
				fmt.Fprintln(out, checks[i].ID)
			}
		}
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCHECK\tTIER\tFIXABLE\tCOUNT")
	for i := range checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n",
			checks[i].ID, checks[i].Title, checks[i].Tier, yesNo(checks[i].AutoFixable), checks[i].Count)
	}
	return tw.Flush()
}

func runDoctorDetail(cmd *cobra.Command, checkID string) error {
	out := cmd.OutOrStdout()
	p, _, err := resolveProfile(cmd)
	if err != nil {
		return err
	}
	res, err := cliclient.New(p.URL).ListSmellItems(p.Key, checkID, 1, 200)
	if err != nil {
		return fmt.Errorf("list smell items failed: %w", err)
	}
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, res)
	}
	if flagBool(cmd, "quiet") {
		for i := range res.Items {
			fmt.Fprintln(out, res.Items[i].UserGameID)
		}
		return nil
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(out, "No games flagged by this check.")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "USER_GAME_ID\tTITLE\tSUGGESTION")
	for i := range res.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", res.Items[i].UserGameID, res.Items[i].Title, suggestionOf(res.Items[i]))
	}
	return tw.Flush()
}

// suggestionOf renders the per-item hint: the suggested status (auto-fix
// checks) or the human-readable detail (e.g. impossible-acquired-date), else "-".
func suggestionOf(it cliclient.FlaggedItem) string {
	if it.SuggestedStatus != nil && *it.SuggestedStatus != "" {
		return "→ " + *it.SuggestedStatus
	}
	if it.Detail != nil && *it.Detail != "" {
		return *it.Detail
	}
	return "-"
}

// collectFlaggedIDs returns every flagged user_game_id for a check, across pages.
//
//nolint:unused // used by Tasks 3 & 4
func collectFlaggedIDs(c *cliclient.Client, key, checkID string) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		res, err := c.ListSmellItems(key, checkID, page, 200)
		if err != nil {
			return nil, err
		}
		for i := range res.Items {
			ids = append(ids, res.Items[i].UserGameID)
		}
		if res.Pages <= page || res.Pages == 0 {
			break
		}
	}
	return ids, nil
}

func newDoctorApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <check> [refs...]",
		Short: "Apply an auto-fix check's suggestion (all flagged games, or specific refs)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			checkID := args[0]
			refs := args[1:]

			var ids []string
			if len(refs) == 0 {
				ids, err = collectFlaggedIDs(c, p.Key, checkID)
				if err != nil {
					return fmt.Errorf("list smell items failed: %w", err)
				}
			} else {
				for _, ref := range refs {
					u, rErr := resolveUserGameRef(cmd, c, p.Key, ref)
					if rErr != nil {
						return rErr
					}
					ids = append(ids, u.ID)
				}
			}
			if len(ids) == 0 {
				fmt.Fprintln(out, "Nothing to apply — no games flagged by this check.")
				return nil
			}

			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Apply %q suggestion to %d game(s)?", checkID, len(ids)), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}

			res, err := c.ApplySmell(p.Key, checkID, ids)
			if err != nil {
				return fmt.Errorf("apply failed: %w", err)
			}
			fmt.Fprintf(out, "Applied %d, skipped %d.\n", res.Applied, res.Skipped)
			return nil
		},
	}
}
func newDoctorIgnoreCmd() *cobra.Command {
	return &cobra.Command{Use: "ignore", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
func newDoctorRestoreCmd() *cobra.Command {
	return &cobra.Command{Use: "restore", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
func newDoctorIgnoredCmd() *cobra.Command {
	return &cobra.Command{Use: "ignored", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
