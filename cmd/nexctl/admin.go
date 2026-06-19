package main

import (
	"bufio"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Operator commands (admin only)",
	}
	cmd.AddCommand(
		newAdminUserCmd(),
		newAdminResetCmd(),
	)
	return cmd
}

func newAdminUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage server users",
	}
	cmd.AddCommand(
		newAdminUserListCmd(),
		newAdminUserShowCmd(),
		newAdminUserCreateCmd(),
		newAdminUserEnableCmd(),
		newAdminUserDisableCmd(),
		newAdminUserSetAdminCmd(),
		newAdminUserPasswdCmd(),
		newAdminUserRmCmd(),
	)
	return cmd
}

func newAdminUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			users, err := cliclient.New(p.URL).ListUsers(p.Key)
			if err != nil {
				return fmt.Errorf("list users: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, users)
			}
			if flagBool(cmd, "quiet") {
				for i := range users {
					fmt.Fprintln(out, users[i].ID)
				}
				return nil
			}
			if len(users) == 0 {
				fmt.Fprintln(out, "No users.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tUSERNAME\tACTIVE\tADMIN\tCREATED")
			for i := range users {
				u := &users[i]
				fmt.Fprintf(tw, "%s\t%s\t%v\t%v\t%s\n",
					u.ID, u.Username, u.IsActive, u.IsAdmin, u.CreatedAt)
			}
			return tw.Flush()
		},
	}
}

func newAdminUserShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			u, err := cliclient.New(p.URL).GetUser(p.Key, args[0])
			if err != nil {
				return fmt.Errorf("get user: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, u)
			}
			fmt.Fprintf(out, "id:         %s\n", u.ID)
			fmt.Fprintf(out, "username:   %s\n", u.Username)
			fmt.Fprintf(out, "active:     %v\n", u.IsActive)
			fmt.Fprintf(out, "admin:      %v\n", u.IsAdmin)
			fmt.Fprintf(out, "created_at: %s\n", u.CreatedAt)
			fmt.Fprintf(out, "updated_at: %s\n", u.UpdatedAt)
			return nil
		},
	}
}

func newAdminUserCreateCmd() *cobra.Command {
	var adminFlag bool
	var passwordFlag string
	cmd := &cobra.Command{
		Use:   "create <username>",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			username := args[0]

			password := passwordFlag
			if password == "" {
				src := cmd.InOrStdin()
				in := bufio.NewReader(src)
				var err error
				password, err = cliui.ReadPassword(in, src, out, fmt.Sprintf("Password for %s: ", username))
				if err != nil {
					return err
				}
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			u, err := cliclient.New(p.URL).CreateUser(p.Key, username, password, adminFlag)
			if err != nil {
				return fmt.Errorf("create user: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, u)
			}
			if flagBool(cmd, "quiet") {
				fmt.Fprintln(out, u.ID)
				return nil
			}
			fmt.Fprintf(out, "%s\t%s\t%v\t%v\t%s\n",
				u.ID, u.Username, u.IsActive, u.IsAdmin, u.CreatedAt)
			return nil
		},
	}
	cmd.Flags().BoolVar(&adminFlag, "admin", false, "Grant the new user admin privileges")
	cmd.Flags().StringVar(&passwordFlag, "password", "", "Password (prompted if omitted)")
	return cmd
}

func newAdminUserEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a user account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if _, err := cliclient.New(p.URL).UpdateUser(p.Key, args[0], map[string]any{"is_active": true}); err != nil {
				return fmt.Errorf("enable user: %w", err)
			}
			fmt.Fprintf(out, "enabled user %s\n", args[0])
			return nil
		},
	}
}

func newAdminUserDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a user account (drops sessions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			ok, err := cliui.Confirm(in, out,
				fmt.Sprintf("Disable user %s? This will drop all their active sessions.", args[0]),
				flagBool(cmd, "yes"))
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
			if _, err := cliclient.New(p.URL).UpdateUser(p.Key, args[0], map[string]any{"is_active": false}); err != nil {
				return fmt.Errorf("disable user: %w", err)
			}
			fmt.Fprintf(out, "disabled user %s\n", args[0])
			return nil
		},
	}
}

func newAdminUserSetAdminCmd() *cobra.Command {
	var revokeFlag bool
	cmd := &cobra.Command{
		Use:   "set-admin <id>",
		Short: "Grant or revoke admin privileges",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			isAdmin := !revokeFlag
			if _, err := cliclient.New(p.URL).UpdateUser(p.Key, args[0], map[string]any{"is_admin": isAdmin}); err != nil {
				return fmt.Errorf("set-admin: %w", err)
			}
			if isAdmin {
				fmt.Fprintf(out, "granted admin to user %s\n", args[0])
			} else {
				fmt.Fprintf(out, "revoked admin from user %s\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&revokeFlag, "revoke", false, "Revoke admin privileges instead of granting them")
	return cmd
}

func newAdminUserPasswdCmd() *cobra.Command {
	var passwordFlag string
	cmd := &cobra.Command{
		Use:   "passwd <id>",
		Short: "Reset a user's password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			password := passwordFlag
			if password == "" {
				src := cmd.InOrStdin()
				in := bufio.NewReader(src)
				var err error
				password, err = cliui.ReadPassword(in, src, out, fmt.Sprintf("New password for %s: ", args[0]))
				if err != nil {
					return err
				}
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if err := cliclient.New(p.URL).ResetUserPassword(p.Key, args[0], password); err != nil {
				return fmt.Errorf("passwd: %w", err)
			}
			fmt.Fprintf(out, "password updated for user %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&passwordFlag, "password", "", "New password (prompted if omitted)")
	return cmd
}

func newAdminUserRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a user and all their data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			impact, err := c.GetDeletionImpact(p.Key, args[0])
			if err != nil {
				return fmt.Errorf("get deletion impact: %w", err)
			}
			fmt.Fprintf(out, "User:         %s (%s)\n", impact.Username, impact.UserID)
			fmt.Fprintf(out, "Games:        %d\n", impact.TotalGames)
			fmt.Fprintf(out, "Tags:         %d\n", impact.TotalTags)
			fmt.Fprintf(out, "Import jobs:  %d\n", impact.TotalImportJobs)
			fmt.Fprintf(out, "Export jobs:  %d\n", impact.TotalExportJobs)
			fmt.Fprintf(out, "Sync jobs:    %d\n", impact.TotalSyncJobs)
			fmt.Fprintf(out, "Sync configs: %d\n", impact.TotalSyncConfigs)
			fmt.Fprintf(out, "Sessions:     %d\n", impact.TotalSessions)
			if impact.Warning != "" {
				fmt.Fprintf(out, "Warning:      %s\n", impact.Warning)
			}

			ok, err := cliui.Confirm(in, out,
				fmt.Sprintf("Permanently delete user %s and all their data?", args[0]),
				flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}
			if err := c.DeleteUser(p.Key, args[0]); err != nil {
				return fmt.Errorf("delete user: %w", err)
			}
			fmt.Fprintf(out, "removed user %s\n", args[0])
			return nil
		},
	}
}

func newAdminResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Obliterate ALL user game data (preserves admin accounts and game catalog)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			ok, err := cliui.Confirm(in, out,
				"WARNING: This will PERMANENTLY obliterate ALL user game data for every user. "+
					"Admin accounts and the game catalog are preserved, but all games, tags, "+
					"jobs, sync configs, and sessions will be deleted. This cannot be undone. Proceed?",
				flagBool(cmd, "yes"))
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
			n, err := cliclient.New(p.URL).AdminReset(p.Key)
			if err != nil {
				return fmt.Errorf("admin reset: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, map[string]any{"deleted": n})
			}
			if flagBool(cmd, "quiet") {
				fmt.Fprintln(out, n)
				return nil
			}
			fmt.Fprintf(out, "deleted %d games\n", n)
			return nil
		},
	}
}
