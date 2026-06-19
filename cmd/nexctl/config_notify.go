package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newConfigNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Manage notification channels and subscriptions",
	}
	cmd.AddCommand(
		newNotifyChannelCmd(),
		newNotifySubCmd(),
		newNotifyTestURLCmd(),
		newNotifyEventsCmd(),
	)
	return cmd
}

// newNotifyChannelCmd returns the "notify channel" subgroup.
func newNotifyChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Manage notification channels",
	}
	cmd.AddCommand(
		newNotifyChannelListCmd(),
		newNotifyChannelCreateCmd(),
		newNotifyChannelEditCmd(),
		newNotifyChannelRmCmd(),
		newNotifyChannelTestCmd(),
	)
	return cmd
}

// newNotifySubCmd returns the "notify sub" subgroup.
func newNotifySubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sub",
		Short: "Manage notification subscriptions",
	}
	cmd.AddCommand(
		newNotifySubListCmd(),
		newNotifySubSetCmd(),
		newNotifySubResetCmd(),
	)
	return cmd
}

// resolveChannelURL returns the URL from the --url flag when it is set to a
// non-empty value, otherwise it prompts for it no-echo. This is the Shoutrrr
// URL which may embed tokens, so it is never accepted as a positional arg.
func resolveChannelURL(cmd *cobra.Command, urlFlag string) (string, error) {
	if cmd.Flags().Changed("url") && urlFlag != "" {
		return urlFlag, nil
	}
	// Flag absent, or present-but-empty: prompt no-echo either way.
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	return cliui.ReadPassword(in, out, "Channel URL: ")
}

// printEventTypeList renders a slice of event-type strings honouring --json and
// -q. With neither flag it prints a one-column table; an empty list prints
// emptyMsg (when non-empty) instead of a bare header.
func printEventTypeList(cmd *cobra.Command, types []string, emptyMsg string) error {
	out := cmd.OutOrStdout()
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, types)
	}
	if flagBool(cmd, "quiet") {
		for _, t := range types {
			fmt.Fprintln(out, t)
		}
		return nil
	}
	if len(types) == 0 && emptyMsg != "" {
		fmt.Fprintln(out, emptyMsg)
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "EVENT TYPE")
	for _, t := range types {
		fmt.Fprintln(tw, t)
	}
	return tw.Flush()
}

// printChannel prints a single NotifyChannel as a one-line table row.
func printChannel(cmd *cobra.Command, ch *cliclient.NotifyChannel) error {
	out := cmd.OutOrStdout()
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, ch)
	}
	if flagBool(cmd, "quiet") {
		fmt.Fprintln(out, ch.ID)
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCREATED")
	fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.ID, ch.Name, ch.CreatedAt)
	return tw.Flush()
}

func newNotifyChannelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List notification channels",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			channels, err := cliclient.New(p.URL).ListChannels(p.Key)
			if err != nil {
				return fmt.Errorf("list channels: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, channels)
			}
			if flagBool(cmd, "quiet") {
				for i := range channels {
					fmt.Fprintln(out, channels[i].ID)
				}
				return nil
			}
			if len(channels) == 0 {
				fmt.Fprintln(out, "No channels.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tCREATED")
			for i := range channels {
				ch := &channels[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.ID, ch.Name, ch.CreatedAt)
			}
			return tw.Flush()
		},
	}
}

func newNotifyChannelCreateCmd() *cobra.Command {
	var urlFlag string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a notification channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			channelURL, err := resolveChannelURL(cmd, urlFlag)
			if err != nil {
				return err
			}
			if channelURL == "" {
				return fmt.Errorf("channel URL is required")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			ch, err := cliclient.New(p.URL).CreateChannel(p.Key, name, channelURL)
			if err != nil {
				return fmt.Errorf("create channel: %w", err)
			}
			return printChannel(cmd, ch)
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Shoutrrr channel URL (prompted no-echo if omitted)")
	return cmd
}

func newNotifyChannelEditCmd() *cobra.Command {
	var nameFlag, urlFlag string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Update a notification channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			fields := make(map[string]any)

			if cmd.Flags().Changed("name") {
				fields["name"] = nameFlag
			}
			if cmd.Flags().Changed("url") {
				if urlFlag != "" {
					fields["url"] = urlFlag
				} else {
					// --url was passed but empty — prompt no-echo.
					out := cmd.OutOrStdout()
					in := bufio.NewReader(cmd.InOrStdin())
					u, err := cliui.ReadPassword(in, out, "Channel URL: ")
					if err != nil {
						return err
					}
					fields["url"] = u
				}
			}

			if len(fields) == 0 {
				return fmt.Errorf("nothing to update")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			ch, err := cliclient.New(p.URL).UpdateChannel(p.Key, id, fields)
			if err != nil {
				return fmt.Errorf("edit channel: %w", err)
			}
			return printChannel(cmd, ch)
		},
	}
	cmd.Flags().StringVar(&nameFlag, "name", "", "New channel name")
	cmd.Flags().StringVar(&urlFlag, "url", "", "New Shoutrrr channel URL (prompted no-echo if flag present but empty)")
	return cmd
}

func newNotifyChannelRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a notification channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Delete channel %s?", args[0]), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}
			if err := cliclient.New(p.URL).DeleteChannel(p.Key, args[0]); err != nil {
				return fmt.Errorf("delete channel: %w", err)
			}
			fmt.Fprintf(out, "removed channel %s\n", args[0])
			return nil
		},
	}
}

func newNotifyChannelTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "Send a test notification via a channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if err := cliclient.New(p.URL).TestChannel(p.Key, args[0]); err != nil {
				return fmt.Errorf("test channel: %w", err)
			}
			fmt.Fprintln(out, "test notification sent")
			return nil
		},
	}
}

func newNotifyTestURLCmd() *cobra.Command {
	var urlFlag string
	cmd := &cobra.Command{
		Use:   "test-url",
		Short: "Test an arbitrary Shoutrrr URL without saving it",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			var rawURL string
			if cmd.Flags().Changed("url") {
				rawURL = urlFlag
			}
			if rawURL == "" {
				in := bufio.NewReader(cmd.InOrStdin())
				var err error
				rawURL, err = cliui.ReadPassword(in, out, "Channel URL: ")
				if err != nil {
					return err
				}
			}
			if rawURL == "" {
				return fmt.Errorf("channel URL is required")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if err := cliclient.New(p.URL).TestURL(p.Key, rawURL); err != nil {
				return fmt.Errorf("test URL: %w", err)
			}
			fmt.Fprintln(out, "test notification sent")
			return nil
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Shoutrrr URL to test (prompted no-echo if omitted)")
	return cmd
}

func newNotifySubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List subscribed event types",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			types, err := cliclient.New(p.URL).ListSubscriptions(p.Key)
			if err != nil {
				return fmt.Errorf("list subscriptions: %w", err)
			}
			return printEventTypeList(cmd, types, "No subscriptions.")
		},
	}
}

func newNotifySubSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <event-type>...",
		Short: "Replace the subscription set",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			types, err := cliclient.New(p.URL).PutSubscriptions(p.Key, args)
			if err != nil {
				return fmt.Errorf("set subscriptions: %w", err)
			}
			return printEventTypeList(cmd, types, "")
		},
	}
}

func newNotifySubResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset subscriptions to defaults",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			types, err := cliclient.New(p.URL).ResetSubscriptions(p.Key)
			if err != nil {
				return fmt.Errorf("reset subscriptions: %w", err)
			}
			return printEventTypeList(cmd, types, "")
		},
	}
}

func newNotifyEventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "List available notification event types",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			events, err := cliclient.New(p.URL).ListEventTypes(p.Key)
			if err != nil {
				return fmt.Errorf("list event types: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, events)
			}
			if flagBool(cmd, "quiet") {
				for i := range events {
					fmt.Fprintln(out, events[i].Type)
				}
				return nil
			}
			if len(events) == 0 {
				fmt.Fprintln(out, "No event types.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "TYPE\tSCOPE\tCATEGORY\tLABEL\tDEFAULT")
			for i := range events {
				e := &events[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\n", e.Type, e.Scope, e.Category, e.Label, e.DefaultOn)
			}
			return tw.Flush()
		},
	}
}
