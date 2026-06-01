package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newAPIKeyCmd returns the `api-key` parent command (aliased `keys`).
func newAPIKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "api-key",
		Aliases: []string{"keys"},
		Short:   "Manage your API keys on a Nexorious server",
	}
	cmd.AddCommand(newAPIKeyGenerateCmd())
	cmd.AddCommand(newAPIKeyListCmd())
	cmd.AddCommand(newAPIKeyRevokeCmd())
	return cmd
}

// currentProfile loads the CLI config and returns the active profile, erroring
// with a login hint if there is no stored API key.
func currentProfile() (clicfg.Profile, *clicfg.Config, error) {
	cfg, err := clicfg.Load()
	if err != nil {
		return clicfg.Profile{}, nil, err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return clicfg.Profile{}, nil, fmt.Errorf("not logged in (run `nexorious login` first)")
	}
	return p, cfg, nil
}

// formatNullableTime renders a *time.Time in local time, or zero when nil.
func formatNullableTime(t *time.Time, zero string) string {
	if t == nil {
		return zero
	}
	return t.Local().Format("2006-01-02 15:04")
}

func newAPIKeyGenerateCmd() *cobra.Command {
	var name, scopes, expiresAt string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Create a new API key and print it once",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerate(cmd, name, scopes, expiresAt)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Label for the key (required)")
	cmd.Flags().StringVar(&scopes, "scopes", "write", "Key scopes: read or write")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "Optional expiry as an RFC3339 timestamp")
	return cmd
}

func runGenerate(cmd *cobra.Command, name, scopes, expiresAt string) error {
	out := cmd.OutOrStdout()
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if scopes != "read" && scopes != "write" {
		return fmt.Errorf("--scopes must be 'read' or 'write'")
	}

	p, _, err := currentProfile()
	if err != nil {
		return err
	}
	client := cliclient.New(p.URL)

	// Non-fatal warning if an active key already uses this name (names aren't unique).
	if existing, err := client.ListAPIKeys(p.Key); err == nil {
		for _, k := range existing {
			if k.Name == name {
				fmt.Fprintf(out, "warning: an API key named %q already exists\n", name)
				break
			}
		}
	}

	var expPtr *string
	if expiresAt != "" {
		expPtr = &expiresAt
	}
	key, err := client.CreateAPIKeyWithBearer(p.Key, name, scopes, expPtr)
	if err != nil {
		return fmt.Errorf("create API key failed: %w", err)
	}

	fmt.Fprintf(out, "API key created. Copy it now — it will not be shown again:\n\n  %s\n\n", key.Key)
	fmt.Fprintf(out, "id:      %s\nname:    %s\nscopes:  %s\nexpires: %s\n",
		key.ID, key.Name, key.Scopes, formatNullableTime(key.ExpiresAt, "never"))
	return nil
}

func newAPIKeyListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your API keys",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListKeys(cmd, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit raw JSON instead of a table")
	return cmd
}

func runListKeys(cmd *cobra.Command, asJSON bool) error {
	out := cmd.OutOrStdout()
	p, _, err := currentProfile()
	if err != nil {
		return err
	}
	keys, err := cliclient.New(p.URL).ListAPIKeys(p.Key)
	if err != nil {
		return fmt.Errorf("list API keys failed: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(keys); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
		return nil
	}

	if len(keys) == 0 {
		fmt.Fprintln(out, "No API keys.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSCOPES\tCREATED\tLAST USED\tEXPIRES")
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			k.ID, k.Name, k.Scopes,
			k.CreatedAt.Local().Format("2006-01-02 15:04"),
			formatNullableTime(k.LastUsedAt, "never"),
			formatNullableTime(k.ExpiresAt, "–"),
		)
	}
	return tw.Flush()
}

func newAPIKeyRevokeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <id-or-name>",
		Short: "Revoke an API key by id or name (from `api-key list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRevoke(cmd, args[0], yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false,
		"Skip the confirmation prompt when revoking the key this CLI is using")
	return cmd
}

func runRevoke(cmd *cobra.Command, idOrName string, yes bool) error {
	out := cmd.OutOrStdout()
	p, cfg, err := currentProfile()
	if err != nil {
		return err
	}
	client := cliclient.New(p.URL)

	keys, err := client.ListAPIKeys(p.Key)
	if err != nil {
		return fmt.Errorf("list API keys failed: %w", err)
	}
	targetID, err := resolveKeyID(keys, idOrName)
	if err != nil {
		return err
	}

	self := targetID == p.KeyID
	if self && !yes {
		fmt.Fprint(out, "Revoke the key this CLI is currently using? This will log you out. [y/N] ")
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // EOF/partial line still yields the typed answer
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
	}

	if err := client.RevokeAPIKeyWithBearer(p.Key, targetID); err != nil {
		return fmt.Errorf("revoke failed: %w", err)
	}

	if self {
		url := p.URL
		if err := clearStoredKey(cfg); err != nil {
			return err
		}
		fmt.Fprintf(out, "Revoked API key %s.\nThat was the key this CLI was using — you have been logged out of %s.\n", targetID, url)
		return nil
	}

	fmt.Fprintf(out, "Revoked API key %s.\n", targetID)
	return nil
}

// resolveKeyID maps an id-or-name argument to a single key id. An exact id match
// wins; otherwise it matches active keys by name, requiring exactly one match.
func resolveKeyID(keys []cliclient.APIKey, idOrName string) (string, error) {
	for _, k := range keys {
		if k.ID == idOrName {
			return k.ID, nil
		}
	}
	var matches []string
	for _, k := range keys {
		if k.Name == idOrName {
			matches = append(matches, k.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no API key with id or name %q", idOrName)
	default:
		return "", fmt.Errorf("multiple active keys named %q; revoke by id instead (see `api-key list`)", idOrName)
	}
}
