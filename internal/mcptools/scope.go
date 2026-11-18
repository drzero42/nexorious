package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scope is the API-key scope a server is built for. Its values match the
// stored key scopes ("read"/"write").
type Scope string

const (
	// ScopeRead serves only the read tools.
	ScopeRead Scope = "read"
	// ScopeWrite serves every tool.
	ScopeWrite Scope = "write"
)

// allows reports whether a key of scope s may run a tool that needs scope
// need. Anything but ScopeWrite is treated as read, so a malformed scope fails
// safe.
func (s Scope) allows(need Scope) bool {
	return s == ScopeWrite || need == ScopeRead
}

// toolScopes is the one place each tool declares the scope it needs. A tool
// that mutates anything — or calls a REST endpoint that does — is ScopeWrite;
// when in doubt, choose write. addTool panics for a tool missing from this
// table, so a new tool cannot be registered without a classification.
var toolScopes = map[string]Scope{
	// game
	"game_list":    ScopeRead,
	"game_show":    ScopeRead,
	"game_stats":   ScopeRead,
	"game_filters": ScopeRead,
	"game_add":     ScopeWrite,
	"game_edit":    ScopeWrite,
	"game_acquire": ScopeWrite,
	"game_rm":      ScopeWrite,
	// pool
	"pool_list":    ScopeRead,
	"pool_show":    ScopeRead,
	"pool_create":  ScopeWrite,
	"pool_edit":    ScopeWrite,
	"pool_rm":      ScopeWrite,
	"pool_add":     ScopeWrite,
	"pool_remove":  ScopeWrite,
	"pool_queue":   ScopeWrite,
	"pool_reorder": ScopeWrite,
	// tag
	"tag_list":   ScopeRead,
	"tag_create": ScopeWrite,
	"tag_rename": ScopeWrite,
	"tag_rm":     ScopeWrite,
	// sync
	"sync_status":     ScopeRead,
	"sync_review":     ScopeRead, // lists needs_review items only
	"sync_run":        ScopeWrite,
	"sync_resolve":    ScopeWrite,
	"sync_skip":       ScopeWrite,
	"sync_retry":      ScopeWrite,
	"sync_reset":      ScopeWrite,
	"sync_disconnect": ScopeWrite,
	// doctor
	"library_smells_list":    ScopeRead,
	"library_smells_detail":  ScopeRead,
	"library_smells_ignored": ScopeRead,
	"library_smells_apply":   ScopeWrite,
	"library_smells_ignore":  ScopeWrite,
	"library_smells_restore": ScopeWrite,
}

// toolSet is the server under construction plus the scope it serves.
type toolSet struct {
	srv   *mcp.Server
	scope Scope
}

// addTool registers t on ts if ts's scope allows the scope t declares in
// toolScopes, so a read server never lists a write tool.
func addTool[In, Out any](ts *toolSet, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	need, ok := toolScopes[t.Name]
	if !ok {
		panic(fmt.Sprintf("mcptools: tool %q has no entry in toolScopes", t.Name))
	}
	if ts.scope.allows(need) {
		mcp.AddTool(ts.srv, t, h)
	}
}

// enforceToolScope refuses a tools/call for a tool the scope does not allow,
// with a tool error naming the fix. The read server does not register write
// tools at all, so this is defence in depth — and it turns the SDK's bare
// "unknown tool" into an actionable message. Unknown names pass through to the
// SDK's own handling.
func enforceToolScope(scope Scope) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			p, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok || p == nil {
				return next(ctx, method, req)
			}
			if need, known := toolScopes[p.Name]; known && !scope.allows(need) {
				res := &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(
						"%s: this API key is read-only; mint a write-scoped key to modify the collection", p.Name)}},
				}
				res.IsError = true
				return res, nil
			}
			return next(ctx, method, req)
		}
	}
}
