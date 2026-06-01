package main

import (
	"fmt"
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

// --- Temporary stubs, replaced in Tasks 4 and 5. Keep the package compiling. ---

func newAPIKeyListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error { return runListKeys(cmd, false) }}
}

func runListKeys(_ *cobra.Command, _ bool) error {
	return fmt.Errorf("not implemented")
}

func newAPIKeyRevokeCmd() *cobra.Command {
	return &cobra.Command{Use: "revoke"}
}
