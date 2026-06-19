package main

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
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
			srv := buildMCPServer(p)
			return srv.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}

// buildMCPServer registers every mirror tool against a server whose handlers use
// the given profile's URL + key. Transport is bound by the caller.
func buildMCPServer(p clicfg.Profile) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nexorious", Version: version}, nil)
	c := cliclient.New(p.URL)
	registerGameTools(srv, c, p.Key)
	registerPoolTools(srv, c, p.Key)
	registerTagTools(srv, c, p.Key)
	registerSyncTools(srv, c, p.Key)
	return srv
}

// mcpToolError maps a cliclient error to an actionable tool error. A 403 from a
// read-scoped key on a write tool gets a specific, corrective message.
func mcpToolError(action string, err error) error { //nolint:unused // called by tool registrars in tasks 4–7
	if err == nil {
		return nil
	}
	if isForbidden(err) {
		return fmt.Errorf("%s: this profile's API key is read-only; mint a write-scoped key (`nexctl account api-key …`) to modify the collection", action)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// isForbidden reports whether err is an HTTP 403 from cliclient.
// cliclient.httpError produces plain fmt.Errorf strings ("server returned 403…");
// there is no typed error to unwrap, so we match on the message directly.
func isForbidden(err error) bool { //nolint:unused // called by mcpToolError, used in tasks 4–7
	return strings.Contains(err.Error(), "server returned 403")
}

func registerGameTools(_ *mcp.Server, _ *cliclient.Client, _ string) {}
func registerPoolTools(_ *mcp.Server, _ *cliclient.Client, _ string) {}
func registerTagTools(_ *mcp.Server, _ *cliclient.Client, _ string)  {}
func registerSyncTools(_ *mcp.Server, _ *cliclient.Client, _ string) {}
