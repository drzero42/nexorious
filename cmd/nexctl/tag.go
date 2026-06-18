package main

import (
	"bufio"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newTagCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tag", Short: "Manage your tags"}
	cmd.AddCommand(newTagListCmd(), newTagCreateCmd(), newTagRenameCmd(), newTagRmCmd())
	return cmd
}

// resolveTagRef resolves a tag by id (UUID) or name (case-insensitive).
func resolveTagRef(c *cliclient.Client, key, ref string) (*cliclient.Tag, error) {
	if looksLikeUUID(ref) {
		// No single-tag GET endpoint; fetch the list and match by id.
		tags, err := c.ListTags(key)
		if err != nil {
			return nil, err
		}
		for i := range tags {
			if tags[i].ID == ref {
				return &tags[i], nil
			}
		}
		return nil, fmt.Errorf("no tag with id %s", ref)
	}
	tags, err := c.ListTags(key)
	if err != nil {
		return nil, err
	}
	for i := range tags {
		if strings.EqualFold(tags[i].Name, ref) {
			return &tags[i], nil
		}
	}
	return nil, fmt.Errorf("no tag named %q", ref)
}

func newTagListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your tags",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			tags, err := cliclient.New(p.URL).ListTags(p.Key)
			if err != nil {
				return fmt.Errorf("list tags failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, tags)
			}
			if flagBool(cmd, "quiet") {
				for i := range tags {
					fmt.Fprintln(out, tags[i].ID)
				}
				return nil
			}
			if len(tags) == 0 {
				fmt.Fprintln(out, "No tags.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tCOLOR\tGAMES")
			for i := range tags {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", tags[i].ID, tags[i].Name, deref(tags[i].Color), tags[i].GameCount)
			}
			return tw.Flush()
		},
	}
}

func newTagCreateCmd() *cobra.Command {
	var color string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			var colorPtr *string
			if cmd.Flags().Changed("color") {
				colorPtr = &color
			}
			tag, err := cliclient.New(p.URL).CreateTag(p.Key, args[0], colorPtr)
			if err != nil {
				return fmt.Errorf("create tag failed: %w", err)
			}
			fmt.Fprintf(out, "Created tag %q (%s).\n", tag.Name, tag.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "Tag color (e.g. #6B7280)")
	return cmd
}

func newTagRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <ref> <new-name>",
		Short: "Rename a tag",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			tag, err := resolveTagRef(c, p.Key, args[0])
			if err != nil {
				return err
			}
			newName := args[1]
			if _, err := c.UpdateTag(p.Key, tag.ID, &newName, nil); err != nil {
				return fmt.Errorf("rename tag failed: %w", err)
			}
			fmt.Fprintf(out, "Renamed %q to %q.\n", tag.Name, newName)
			return nil
		},
	}
}

func newTagRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <ref>",
		Short: "Delete a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			tag, err := resolveTagRef(c, p.Key, args[0])
			if err != nil {
				return err
			}
			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete tag %q?", tag.Name), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			if err := c.DeleteTag(p.Key, tag.ID); err != nil {
				return fmt.Errorf("delete tag failed: %w", err)
			}
			fmt.Fprintf(out, "Deleted tag %q.\n", tag.Name)
			return nil
		},
	}
}
