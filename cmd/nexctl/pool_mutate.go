package main

import (
	"bufio"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// parseFilterFlag validates a --filter JSON string client-side (fail fast before
// any network call) and returns it as raw JSON.
func parseFilterFlag(s string) (json.RawMessage, error) {
	if s == "" {
		return nil, nil
	}
	if !json.Valid([]byte(s)) {
		return nil, fmt.Errorf("--filter is not valid JSON")
	}
	return json.RawMessage(s), nil
}

func newPoolCreateCmd() *cobra.Command {
	var color, filter string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			raw, err := parseFilterFlag(filter)
			if err != nil {
				return err
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			var colorPtr *string
			if cmd.Flags().Changed("color") {
				colorPtr = &color
			}
			pool, err := cliclient.New(p.URL).CreatePool(p.Key, args[0], colorPtr, raw)
			if err != nil {
				return fmt.Errorf("create pool failed: %w", err)
			}
			fmt.Fprintf(out, "Created pool %q (%s).\n", pool.Name, pool.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "Pool color")
	cmd.Flags().StringVar(&filter, "filter", "", `Saved filter as JSON, e.g. {"filters":[{"genre":["RPG"]}]}`)
	return cmd
}

func newPoolEditCmd() *cobra.Command {
	var name, color, filter string
	var clearFilter bool
	cmd := &cobra.Command{
		Use:   "edit <ref>",
		Short: "Edit a pool (name/color/filter)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			ch := cmd.Flags().Changed
			fields := map[string]any{}
			if ch("name") {
				fields["name"] = name
			}
			if ch("color") {
				fields["color"] = color
			}
			if clearFilter {
				fields["filter"] = nil
			} else if ch("filter") {
				raw, err := parseFilterFlag(filter)
				if err != nil {
					return err
				}
				fields["filter"] = raw
			}
			if len(fields) == 0 {
				return fmt.Errorf("nothing to change; pass --name/--color/--filter/--clear-filter")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			ref, err := resolvePoolRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			if _, err := c.UpdatePool(p.Key, ref.ID, fields); err != nil {
				return fmt.Errorf("edit pool failed: %w", err)
			}
			fmt.Fprintf(out, "Updated pool %q.\n", ref.Name)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "New name")
	f.StringVar(&color, "color", "", "New color")
	f.StringVar(&filter, "filter", "", "New saved filter as JSON")
	f.BoolVar(&clearFilter, "clear-filter", false, "Remove the saved filter")
	return cmd
}

func newPoolRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <ref>",
		Short: "Delete a pool",
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
			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete pool %q?", ref.Name), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			if err := c.DeletePool(p.Key, ref.ID); err != nil {
				return fmt.Errorf("delete pool failed: %w", err)
			}
			fmt.Fprintf(out, "Deleted pool %q.\n", ref.Name)
			return nil
		},
	}
}
