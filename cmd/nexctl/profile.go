package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configured server profiles",
	}
	cmd.AddCommand(newProfileListCmd(), newProfileUseCmd(), newProfileAddCmd(), newProfileRmCmd())
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			names := cfg.Names()
			if flagBool(cmd, "json") {
				// Emit a redacted view — never the stored API key (cfg.Profiles
				// includes the live bearer token in Profile.Key).
				type profileView struct {
					Name     string `json:"name"`
					URL      string `json:"url"`
					Username string `json:"username"`
					Current  bool   `json:"current"`
				}
				views := make([]profileView, 0, len(names))
				for _, n := range names {
					p, _ := cfg.ProfileNamed(n)
					views = append(views, profileView{
						Name:     n,
						URL:      p.URL,
						Username: p.Username,
						Current:  n == cfg.CurrentName(),
					})
				}
				return cliui.EncodeJSON(out, views)
			}
			if flagBool(cmd, "quiet") {
				for _, n := range names {
					fmt.Fprintln(out, n)
				}
				return nil
			}
			if len(names) == 0 {
				fmt.Fprintln(out, "No profiles. Run `nexctl account login` to create one.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "CURRENT\tNAME\tURL\tUSER")
			for _, n := range names {
				p, _ := cfg.ProfileNamed(n)
				marker := ""
				if n == cfg.CurrentName() {
					marker = "*"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", marker, n, p.URL, p.Username)
			}
			return tw.Flush()
		},
	}
}

func newProfileUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if err := cfg.SetCurrent(args[0]); err != nil {
				return err
			}
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Now using profile %q.\n", args[0])
			return nil
		},
	}
}

func newProfileAddCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a profile and switch to it (run `account login` to authenticate)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if _, ok := cfg.ProfileNamed(args[0]); ok {
				return fmt.Errorf("profile %q already exists", args[0])
			}
			cfg.SetProfile(args[0], clicfg.Profile{URL: url})
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created profile %q (now current). Run `nexctl account login` to authenticate.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "Server URL for the new profile")
	return cmd
}

func newProfileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, err := clicfg.Load()
			if err != nil {
				return err
			}
			if _, ok := cfg.ProfileNamed(args[0]); !ok {
				return fmt.Errorf("no profile named %q", args[0])
			}
			var ok bool
			ok, err = cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete profile %q?", args[0]), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			cfg.RemoveProfile(args[0])
			if err := clicfg.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(out, "Deleted profile %q.\n", args[0])
			return nil
		},
	}
}
