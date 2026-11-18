// Package mcptools is the MCP tool surface over the Nexorious REST API: a pure
// mirror of the nexctl game/pool/tag/sync/doctor command tree. Handlers call
// the REST API through cliclient and the shared operations in
// internal/clientops, so the CLI and MCP front-ends cannot drift.
//
// The package is transport-agnostic: callers bind the returned server to stdio
// (nexctl mcp serve) or an HTTP handler.
package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// KeyFunc returns the API key used for the REST sub-calls of one tool call.
// Each handler calls it once, with the tool call's ctx, and uses that key for
// every sub-call. A fixed-key caller (nexctl) returns its profile key; a
// multi-user caller (the server) reads the caller's key from ctx.
type KeyFunc func(ctx context.Context) string

// NewServer builds a server whose handlers call the REST API through c with
// the key that apiKey resolves per call. It registers the tools scope allows
// (see toolScopes) and refuses calls to any other tool, so a read-scoped
// server neither lists nor runs a write tool. version is reported as the MCP
// implementation version. The transport is bound by the caller.
func NewServer(c *cliclient.Client, apiKey KeyFunc, version string, scope Scope) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nexorious", Version: version}, nil)
	ts := &toolSet{srv: srv, scope: scope}
	registerGameTools(ts, c, apiKey)
	registerPoolTools(ts, c, apiKey)
	registerTagTools(ts, c, apiKey)
	registerSyncTools(ts, c, apiKey)
	registerDoctorTools(ts, c, apiKey)
	srv.AddReceivingMiddleware(enforceToolScope(scope))
	return srv
}

// toolError maps a REST error to an actionable tool error prefixed with action.
//
//   - 403 anywhere in the chain: the key is read-only; tell the agent to mint a
//     write-scoped key.
//   - A bare 404/422 (the error is the *cliclient.HTTPError itself): "not
//     found" / the server's validation message, without the "server returned
//     NNN" noise.
//   - Anything else, including a 404/422 wrapped with step context (e.g. "add
//     platform: …"): wrapped unchanged, so that context is kept.
func toolError(action string, err error) error {
	if err == nil {
		return nil
	}
	var he *cliclient.HTTPError
	if errors.As(err, &he) && he.Status == http.StatusForbidden {
		return fmt.Errorf("%s: this API key is read-only; mint a write-scoped key to modify the collection", action)
	}
	// A type assertion, not errors.As: only an unwrapped error is rewritten, so
	// step context on a wrapped one is never dropped.
	if he, ok := err.(*cliclient.HTTPError); ok {
		switch {
		case he.Status == http.StatusNotFound && he.Message != "":
			return fmt.Errorf("%s: not found: %s", action, he.Message)
		case he.Status == http.StatusNotFound:
			return fmt.Errorf("%s: not found", action)
		case he.Status == http.StatusUnprocessableEntity && he.Message != "":
			return fmt.Errorf("%s: %s", action, he.Message)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
