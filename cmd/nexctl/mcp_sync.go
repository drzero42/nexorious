package main

import (
	"context"

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
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	ExternalID string   `json:"external_id"`
	Platforms  []string `json:"platforms,omitempty"`
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
			for _, eg := range all {
				if eg.SyncStatus == "needs_review" {
					pending = append(pending, externalGameBrief{
						ID:         eg.ID,
						Title:      eg.Title,
						ExternalID: eg.ExternalID,
						Platforms:  eg.Platforms,
					})
				}
			}
			return nil, syncReviewOutput{PendingReview: pending, Total: len(pending)}, nil
		})
}
