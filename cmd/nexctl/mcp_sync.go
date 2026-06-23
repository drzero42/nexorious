package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

type syncStatusInput struct {
	Storefront string `json:"storefront,omitempty" jsonschema:"storefront slug (e.g. steam, psn, gog, epic); omit to list all configured storefronts"`
}

type syncStatusOutput struct {
	// Single storefront status (when storefront was specified).
	Status *cliclient.SyncStatus `json:"status,omitempty"`
	// All storefront configs (when storefront was omitted).
	Configs []cliclient.SyncConfig `json:"configs,omitempty"`
}

type syncReviewInput struct {
	Storefront string `json:"storefront" jsonschema:"storefront slug to list pending-review external games for"`
}

type externalGameBrief struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	ExternalID     string   `json:"external_id"`
	Platforms      []string `json:"platforms,omitempty"`
	IgdbTitle      *string  `json:"igdb_title,omitempty"`
	ResolvedIgdbID *int     `json:"resolved_igdb_id,omitempty"`
	StoreURL       *string  `json:"store_url,omitempty"`
}

func externalBriefOf(eg *cliclient.ExternalGame) externalGameBrief {
	return externalGameBrief{
		ID:             eg.ID,
		Title:          eg.Title,
		ExternalID:     eg.ExternalID,
		Platforms:      eg.Platforms,
		IgdbTitle:      eg.IgdbTitle,
		ResolvedIgdbID: eg.ResolvedIgdbID,
		StoreURL:       eg.StoreURL,
	}
}

// mcpResolveExternalRef resolves a single external-game ref within a storefront.
// On a unique match it returns the game; on ambiguity (>1) or no match it returns
// candidates/message for the agent to disambiguate, without mutating anything.
func mcpResolveExternalRef(c *cliclient.Client, key, sf, ref string) (*cliclient.ExternalGame, []externalGameBrief, string, error) {
	matches, err := findExternalGamesByRef(c, key, sf, ref)
	if err != nil {
		return nil, nil, "", err
	}
	switch len(matches) {
	case 0:
		return nil, nil, fmt.Sprintf("no external game matching %q in %s", ref, sf), nil
	case 1:
		return &matches[0], nil, "", nil
	default:
		cands := make([]externalGameBrief, len(matches))
		for i := range matches {
			cands[i] = externalBriefOf(&matches[i])
		}
		return nil, cands, fmt.Sprintf("%q matches %d external games; call again with one of these ids", ref, len(matches)), nil
	}
}

type syncRunInput struct {
	Storefront string `json:"storefront" jsonschema:"storefront slug to sync (e.g. steam, psn, gog, epic)"`
}

type syncJobOutput struct {
	JobID      string `json:"job_id,omitempty"`
	Storefront string `json:"storefront,omitempty"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
}

type syncStorefrontInput struct {
	Storefront string `json:"storefront" jsonschema:"storefront slug"`
}

type syncAckOutput struct {
	Storefront string `json:"storefront,omitempty"`
	Message    string `json:"message"`
}

type syncResolveInput struct {
	Storefront   string `json:"storefront" jsonschema:"storefront slug"`
	Ref          string `json:"ref" jsonschema:"external-game id or title"`
	IgdbID       int    `json:"igdb_id" jsonschema:"IGDB id to match this external game to"`
	OrphanAction string `json:"orphan_action,omitempty" jsonschema:"how to handle a user-game left orphaned by the rematch (e.g. remove)"`
}

type syncSkipInput struct {
	Storefront string `json:"storefront" jsonschema:"storefront slug"`
	Ref        string `json:"ref" jsonschema:"external-game id or title"`
}

type syncMatchOutput struct {
	Candidates []externalGameBrief `json:"candidates,omitempty"`
	Message    string              `json:"message,omitempty"`
}

type syncReviewOutput struct {
	PendingReview []externalGameBrief `json:"pending_review"`
	Total         int                 `json:"total"`
}

func registerSyncTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "sync_status", Description: "Return sync status. Without a storefront, lists all configured storefronts. With a storefront slug, returns real-time status for that storefront."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncStatusInput) (*mcp.CallToolResult, syncStatusOutput, error) {
			if in.Storefront == "" {
				configs, err := c.ListSyncConfigs(key)
				if err != nil {
					return nil, syncStatusOutput{}, mcpToolError("sync_status", err)
				}
				return nil, syncStatusOutput{Configs: configs}, nil
			}
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncStatusOutput{}, mcpToolError("sync_status", err)
			}
			status, err := c.GetSyncStatus(key, sf)
			if err != nil {
				return nil, syncStatusOutput{}, mcpToolError("sync_status", err)
			}
			return nil, syncStatusOutput{Status: status}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_review", Description: "List external games with sync_status 'needs_review' for a storefront, so an agent can help the user match them to IGDB entries."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncReviewInput) (*mcp.CallToolResult, syncReviewOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncReviewOutput{}, mcpToolError("sync_review", err)
			}
			all, err := c.ListExternalGames(key, sf)
			if err != nil {
				return nil, syncReviewOutput{}, mcpToolError("sync_review", err)
			}
			var pending []externalGameBrief
			for i := range all {
				if all[i].SyncStatus == "needs_review" {
					pending = append(pending, externalBriefOf(&all[i]))
				}
			}
			return nil, syncReviewOutput{PendingReview: pending, Total: len(pending)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_run", Description: "Trigger a sync for a storefront. Enqueues an async job and returns its id; does not block."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncRunInput) (*mcp.CallToolResult, syncJobOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncJobOutput{}, mcpToolError("sync_run", err)
			}
			res, err := c.TriggerSync(key, sf)
			if err != nil {
				return nil, syncJobOutput{}, mcpToolError("sync_run", err)
			}
			return nil, syncJobOutput{JobID: res.JobID, Storefront: res.Storefront, Status: res.Status, Message: res.Message}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_resolve", Description: "Resolve a pending-review external game by matching it to an IGDB id. Ambiguous refs return candidates — call again with the id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncResolveInput) (*mcp.CallToolResult, syncMatchOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_resolve", err)
			}
			eg, cands, msg, err := mcpResolveExternalRef(c, key, sf, in.Ref)
			if err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_resolve", err)
			}
			if cands != nil || msg != "" {
				return nil, syncMatchOutput{Candidates: cands, Message: msg}, nil
			}
			if err := c.RematchExternalGame(key, eg.ID, in.IgdbID, in.OrphanAction); err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_resolve", err)
			}
			return nil, syncMatchOutput{Message: fmt.Sprintf("resolved %q to IGDB id %d", eg.Title, in.IgdbID)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_skip", Description: "Skip a pending-review external game (leave it unmatched). Ambiguous refs return candidates — call again with the id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncSkipInput) (*mcp.CallToolResult, syncMatchOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_skip", err)
			}
			eg, cands, msg, err := mcpResolveExternalRef(c, key, sf, in.Ref)
			if err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_skip", err)
			}
			if cands != nil || msg != "" {
				return nil, syncMatchOutput{Candidates: cands, Message: msg}, nil
			}
			if err := c.SkipExternalGame(key, eg.ID); err != nil {
				return nil, syncMatchOutput{}, mcpToolError("sync_skip", err)
			}
			return nil, syncMatchOutput{Message: fmt.Sprintf("skipped %q", eg.Title)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_retry", Description: "Retry failed external-game matches for a storefront."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_retry", err)
			}
			if err := c.RetryFailedExternalGames(key, sf); err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_retry", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "retrying failed external-game matches"}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_reset", Description: "Reset (clear) sync data for a storefront. Destructive — call explicitly."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_reset", err)
			}
			if err := c.ResetSyncData(key, sf); err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_reset", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "sync data reset"}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sync_disconnect", Description: "Disconnect a storefront (remove its stored credentials)."},
		func(_ context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			sf, err := resolveStorefront(c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_disconnect", err)
			}
			if err := c.DisconnectStorefront(key, sf); err != nil {
				return nil, syncAckOutput{}, mcpToolError("sync_disconnect", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "storefront disconnected"}, nil
		})
}
