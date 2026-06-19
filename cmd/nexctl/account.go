package main

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Authenticate and inspect the active session",
	}
	cmd.AddCommand(newLoginCmd(), newLogoutCmd(), newWhoamiCmd(), newAPIKeyCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	var urlFlag, usernameFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a Nexorious server and store an API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, urlFlag, usernameFlag)
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Server URL (prompted if omitted)")
	cmd.Flags().StringVar(&usernameFlag, "username", "", "Username (prompted if omitted)")
	return cmd
}

func runLogin(cmd *cobra.Command, urlFlag, usernameFlag string) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	name := profileName(cmd, cfg)
	existing, _ := cfg.ProfileNamed(name)

	src := cmd.InOrStdin()
	in := bufio.NewReader(src)
	out := cmd.OutOrStdout()

	url := cliui.FirstNonEmpty(urlFlag, existing.URL)
	if url == "" {
		url, err = cliui.Prompt(in, out, fmt.Sprintf("Server URL [%s]: ", cliauth.DefaultServerURL))
		if err != nil {
			return err
		}
		url = cliui.FirstNonEmpty(url, cliauth.DefaultServerURL)
	}

	username := cliui.FirstNonEmpty(usernameFlag, existing.Username)
	if username == "" {
		username, err = cliui.Prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}

	password, err := cliui.ReadPassword(in, src, out, fmt.Sprintf("Password for %s@%s: ", username, url))
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	return cliauth.LoginAndStoreKey(out, cliclient.New(url), cfg, name, url, username, password)
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored API key and clear it from config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, cfg, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			if p.KeyID != "" {
				if err := cliclient.New(p.URL).RevokeAPIKeyWithBearer(p.Key, p.KeyID); err != nil {
					fmt.Fprintf(out, "warning: could not revoke key on server: %v\n", err)
				}
			}
			if err := clearStoredKey(cfg, profileName(cmd, cfg)); err != nil {
				return err
			}
			fmt.Fprintf(out, "Logged out of %s.\n", p.URL)
			return nil
		},
	}
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the user authenticated by the stored API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			username, err := cliclient.New(p.URL).Me(p.Key)
			if err != nil {
				return fmt.Errorf("could not verify stored key (it may be revoked or expired): %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s @ %s\n", username, p.URL)
			return nil
		},
	}
}

// clearStoredKey wipes the API key from the named profile and saves config.
func clearStoredKey(cfg *clicfg.Config, name string) error {
	p, _ := cfg.ProfileNamed(name)
	p.Key, p.KeyID, p.KeyName = "", "", ""
	cfg.SetProfile(name, p)
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
