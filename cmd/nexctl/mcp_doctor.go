package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// --- input schemas ---

type smellsDetailInput struct {
	CheckID string `json:"check_id" jsonschema:"check id (from library_smells_list)"`
	Page    int    `json:"page,omitempty" jsonschema:"page number (1-based)"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"page size, max 200"`
}

type smellsRefsInput struct {
	CheckID string   `json:"check_id" jsonschema:"check id (from library_smells_list)"`
	Refs    []string `json:"refs" jsonschema:"game ids (UUID) or title substrings; for apply, pass an empty array [] to fix every flagged game"`
}

// --- output projections ---

type smellsListOutput struct {
	Checks []cliclient.SmellSummaryItem `json:"checks"`
}

type smellsDetailOutput struct {
	Items []cliclient.FlaggedItem `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Pages int                     `json:"pages"`
}

type smellsApplyOutput struct {
	Applied    int         `json:"applied,omitempty"`
	Skipped    int         `json:"skipped,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type smellsMutateOutput struct {
	Ignored    int         `json:"ignored,omitempty"`
	Restored   int         `json:"restored,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type smellsIgnoredOutput struct {
	Items []cliclient.IgnoredItem `json:"items"`
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Pages int                     `json:"pages"`
}

// resolveSmellRefIDs resolves refs to user-game ids, returning candidates (no
// mutation) when any ref is ambiguous or unmatched.
func resolveSmellRefIDs(c *cliclient.Client, key string, refs []string) (ids []string, cands []gameBrief, msg string, err error) {
	games, cands, msg, err := mcpResolveEditTargets(c, key, refs, nil)
	if err != nil || cands != nil || msg != "" {
		return nil, cands, msg, err
	}
	ids = make([]string, len(games))
	for i := range games {
		ids[i] = games[i].ID
	}
	return ids, nil, "", nil
}

func registerDoctorTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_list", Description: "List every library-health check with its tier, whether it is auto-fixable, and how many games it currently flags."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, smellsListOutput, error) {
			checks, err := c.ListSmells(key)
			if err != nil {
				return nil, smellsListOutput{}, mcpToolError("library_smells_list", err)
			}
			return nil, smellsListOutput{Checks: checks}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_detail", Description: "List the flagged games for one check. suggested_status (auto-fix checks) or detail describes each finding."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsDetailInput) (*mcp.CallToolResult, smellsDetailOutput, error) {
			res, err := c.ListSmellItems(key, in.CheckID, in.Page, in.PerPage)
			if err != nil {
				return nil, smellsDetailOutput{}, mcpToolError("library_smells_detail", err)
			}
			return nil, smellsDetailOutput{Items: res.Items, Total: res.Total, Page: res.Page, Pages: res.Pages}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_apply", Description: "Apply an auto-fixable check's suggestion. With refs, fixes those games; with no refs, fixes every game the check flags. Ambiguous refs return candidates — call again with an id. Non-fixable checks return an error."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsApplyOutput, error) {
			var ids []string
			if len(in.Refs) == 0 {
				var err error
				ids, err = collectFlaggedIDs(c, key, in.CheckID)
				if err != nil {
					return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
				}
				if len(ids) == 0 {
					return nil, smellsApplyOutput{Message: "no games flagged by this check"}, nil
				}
			} else {
				resolved, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
				if err != nil {
					return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
				}
				if cands != nil || msg != "" {
					return nil, smellsApplyOutput{Candidates: cands, Message: msg}, nil
				}
				ids = resolved
			}
			res, err := c.ApplySmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsApplyOutput{}, mcpToolError("library_smells_apply", err)
			}
			return nil, smellsApplyOutput{Applied: res.Applied, Skipped: res.Skipped,
				Message: fmt.Sprintf("applied %d, skipped %d", res.Applied, res.Skipped)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_ignore", Description: "Dismiss flagged games for a check so they stop appearing. Ambiguous refs return candidates — call again with an id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsMutateOutput, error) {
			ids, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_ignore", err)
			}
			if cands != nil || msg != "" {
				return nil, smellsMutateOutput{Candidates: cands, Message: msg}, nil
			}
			n, err := c.IgnoreSmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_ignore", err)
			}
			return nil, smellsMutateOutput{Ignored: n, Message: fmt.Sprintf("dismissed %d game(s)", n)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_restore", Description: "Un-dismiss previously ignored games for a check. Ambiguous refs return candidates — call again with an id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsRefsInput) (*mcp.CallToolResult, smellsMutateOutput, error) {
			ids, cands, msg, err := resolveSmellRefIDs(c, key, in.Refs)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_restore", err)
			}
			if cands != nil || msg != "" {
				return nil, smellsMutateOutput{Candidates: cands, Message: msg}, nil
			}
			n, err := c.RestoreSmell(key, in.CheckID, ids)
			if err != nil {
				return nil, smellsMutateOutput{}, mcpToolError("library_smells_restore", err)
			}
			return nil, smellsMutateOutput{Restored: n, Message: fmt.Sprintf("restored %d game(s)", n)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "library_smells_ignored", Description: "List the games currently dismissed for one check."},
		func(_ context.Context, _ *mcp.CallToolRequest, in smellsDetailInput) (*mcp.CallToolResult, smellsIgnoredOutput, error) {
			res, err := c.ListIgnoredSmells(key, in.CheckID, in.Page, in.PerPage)
			if err != nil {
				return nil, smellsIgnoredOutput{}, mcpToolError("library_smells_ignored", err)
			}
			return nil, smellsIgnoredOutput{Items: res.Items, Total: res.Total, Page: res.Page, Pages: res.Pages}, nil
		})
}
