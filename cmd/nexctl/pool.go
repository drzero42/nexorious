package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newPoolCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pool", Short: "Manage play-planning pools"}
	cmd.AddCommand(newPoolListCmd(), newPoolShowCmd(), newPoolCreateCmd(), newPoolEditCmd(), newPoolRmCmd(),
		newPoolAddCmd(), newPoolRemoveCmd(), newPoolQueueCmd(), newPoolReorderCmd())
	return cmd
}

// findPoolsByRef returns every pool matching ref: a UUID is matched exactly; a
// name is matched case-insensitively (zero or more results). No cmd, no picker —
// callers own disambiguation.
func findPoolsByRef(c *cliclient.Client, key, ref string) ([]cliclient.PoolListItem, error) {
	pools, err := c.ListPools(key)
	if err != nil {
		return nil, err
	}
	if looksLikeUUID(ref) {
		for i := range pools {
			if pools[i].ID == ref {
				return []cliclient.PoolListItem{pools[i]}, nil
			}
		}
		return nil, fmt.Errorf("no pool with id %s", ref)
	}
	var matches []cliclient.PoolListItem
	for i := range pools {
		if strings.EqualFold(pools[i].Name, ref) {
			matches = append(matches, pools[i])
		}
	}
	return matches, nil
}

// resolvePoolRef resolves a pool by id (UUID) or name (case-insensitive) via the
// pool list. Many matches prompt a TTY picker or, off-TTY, error with candidates.
func resolvePoolRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.PoolListItem, error) {
	pools, err := findPoolsByRef(c, key, ref)
	if err != nil {
		return nil, err
	}
	switch len(pools) {
	case 0:
		return nil, fmt.Errorf("no pool named %q", ref)
	case 1:
		return &pools[0], nil
	}
	// Convert to []*cliclient.PoolListItem for pickPool
	ptrs := make([]*cliclient.PoolListItem, len(pools))
	for i := range pools {
		ptrs[i] = &pools[i]
	}
	if interactive(cmd) {
		return pickPool(cmd, ptrs)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d pools; re-run with an id:", ref, len(pools))
	for i := range pools {
		fmt.Fprintf(&b, "\n  %s  %s", pools[i].ID, pools[i].Name)
	}
	return nil, fmt.Errorf("%s", b.String())
}

func pickPool(cmd *cobra.Command, pools []*cliclient.PoolListItem) (*cliclient.PoolListItem, error) {
	out := cmd.OutOrStdout()
	for i, p := range pools {
		fmt.Fprintf(out, "%2d) %s\n", i+1, p.Name)
	}
	fmt.Fprint(out, "Select a pool [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return pools[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(pools) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return pools[n-1], nil
}

func newPoolListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your pools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			pools, err := cliclient.New(p.URL).ListPools(p.Key)
			if err != nil {
				return fmt.Errorf("list pools failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, pools)
			}
			if flagBool(cmd, "quiet") {
				for i := range pools {
					fmt.Fprintln(out, pools[i].ID)
				}
				return nil
			}
			if len(pools) == 0 {
				fmt.Fprintln(out, "No pools.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tPOS\tQUEUE\tCANDIDATES\tFILTER")
			for i := range pools {
				pl := &pools[i]
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%t\n", pl.ID, pl.Name, pl.Position, pl.QueueCount, pl.CandidateCount, pl.HasFilter)
			}
			return tw.Flush()
		},
	}
}

func newPoolShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show a pool's queue and candidates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ref, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			detail, err := c.GetPool(p.Key, ref.ID)
			if err != nil {
				return fmt.Errorf("get pool failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, detail)
			}
			fmt.Fprintf(out, "%s\n  id:     %s\n  filter: %t\n\n", detail.Name, detail.ID, detail.HasFilter)
			fmt.Fprintln(out, "QUEUE:")
			if len(detail.Queue) == 0 {
				fmt.Fprintln(out, "  (empty)")
			}
			for i := range detail.Queue {
				u := &detail.Queue[i]
				fmt.Fprintf(out, "  %2d. %s  [%s]\n", i+1, u.Title(), statusOf(u))
			}
			fmt.Fprintln(out, "\nCANDIDATES:")
			if len(detail.Candidates) == 0 {
				fmt.Fprintln(out, "  (none)")
			}
			for i := range detail.Candidates {
				u := &detail.Candidates[i]
				fmt.Fprintf(out, "  - %s  [%s]\n", u.Title(), statusOf(u))
			}
			return nil
		},
	}
}
