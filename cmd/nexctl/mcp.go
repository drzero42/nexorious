package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
	"github.com/drzero42/nexorious/internal/mcptools"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Run or configure the local MCP server"}
	cmd.AddCommand(newMCPConfigCmd(), newMCPServeCmd())
	return cmd
}

func newMCPConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the agent-config stanza for the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args := []string{"mcp", "serve"}
			if name, _ := cmd.Flags().GetString("profile"); name != "" { //nolint:errcheck // absent flag yields ""
				args = append(args, "--profile", name)
			}
			stanza := map[string]any{
				"mcpServers": map[string]any{
					"nexorious": map[string]any{"command": "nexctl", "args": args},
				},
			}
			return cliui.EncodeJSON(cmd.OutOrStdout(), stanza)
		},
	}
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run a local stdio MCP server over the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			key := func(context.Context) string { return p.Key }
			// The profile's key scope is not known locally: serve every tool
			// and let the REST scope check 403 a write sub-call (#1262 replaces
			// this with a bridge to the scope-aware /api/mcp).
			srv := mcptools.NewServer(cliclient.New(p.URL), key, version, mcptools.ScopeWrite)
			return srv.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
