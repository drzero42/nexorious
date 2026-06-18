package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// resolveGameIDs resolves each game ref to a user-game id (preserving order).
func resolveGameIDs(cmd *cobra.Command, c *cliclient.Client, key string, refs []string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		u, err := resolveUserGameRef(cmd, c, key, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, u.ID)
	}
	return ids, nil
}

func newPoolAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <pool-ref> <game-ref…>",
		Short: "Add games to a pool (as candidates)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			pool, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ids, err := resolveGameIDs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			if len(ids) == 1 {
				if err := c.AddPoolGame(p.Key, pool.ID, ids[0]); err != nil {
					return fmt.Errorf("add to pool failed: %w", err)
				}
				fmt.Fprintf(out, "Added 1 game to %q.\n", pool.Name)
				return nil
			}
			n, err := c.BulkAddPoolGames(p.Key, pool.ID, ids)
			if err != nil {
				return fmt.Errorf("add to pool failed: %w", err)
			}
			fmt.Fprintf(out, "Added %d game(s) to %q.\n", n, pool.Name)
			return nil
		},
	}
}

func newPoolRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <pool-ref> <game-ref…>",
		Short: "Remove games from a pool",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			pool, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			ids, err := resolveGameIDs(cmd, c, p.Key, args[1:])
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := c.RemovePoolGame(p.Key, pool.ID, id); err != nil {
					return fmt.Errorf("remove %s failed: %w", id, err)
				}
			}
			fmt.Fprintf(out, "Removed %d game(s) from %q.\n", len(ids), pool.Name)
			return nil
		},
	}
}
