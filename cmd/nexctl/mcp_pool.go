package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

type poolBrief struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Color          *string `json:"color,omitempty"`
	Position       int     `json:"position"`
	HasFilter      bool    `json:"has_filter"`
	QueueCount     int64   `json:"queue_count"`
	CandidateCount int64   `json:"candidate_count"`
}

func poolBriefOf(p cliclient.PoolListItem) poolBrief {
	return poolBrief{
		ID:             p.ID,
		Name:           p.Name,
		Color:          p.Color,
		Position:       p.Position,
		HasFilter:      p.HasFilter,
		QueueCount:     p.QueueCount,
		CandidateCount: p.CandidateCount,
	}
}

type poolListOutput struct {
	Pools []poolBrief `json:"pools"`
}

type poolShowInput struct {
	Ref string `json:"ref" jsonschema:"pool id (UUID) or name"`
}

type poolShowOutput struct {
	Pool       *cliclient.PoolDetail `json:"pool,omitempty"`
	Candidates []poolBrief           `json:"candidates,omitempty"`
	Message    string                `json:"message,omitempty"`
}

func registerPoolTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "pool_list", Description: "List all play-planning pools with queue and candidate counts."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, poolListOutput, error) {
			pools, err := c.ListPools(key)
			if err != nil {
				return nil, poolListOutput{}, mcpToolError("pool_list", err)
			}
			out := poolListOutput{Pools: make([]poolBrief, len(pools))}
			for i, p := range pools {
				out.Pools[i] = poolBriefOf(p)
			}
			return nil, out, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_show", Description: "Show a pool's full detail including its ordered queue and candidates. If the name matches multiple pools, returns candidates to disambiguate."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolShowInput) (*mcp.CallToolResult, poolShowOutput, error) {
			pools, err := findPoolsByRef(c, key, in.Ref)
			if err != nil {
				return nil, poolShowOutput{}, mcpToolError("pool_show", err)
			}
			switch len(pools) {
			case 0:
				return nil, poolShowOutput{}, fmt.Errorf("pool_show: no pool matching %q", in.Ref)
			case 1:
				detail, err := c.GetPool(key, pools[0].ID)
				if err != nil {
					return nil, poolShowOutput{}, mcpToolError("pool_show", err)
				}
				return nil, poolShowOutput{Pool: detail}, nil
			default:
				candidates := make([]poolBrief, len(pools))
				for i, p := range pools {
					candidates[i] = poolBriefOf(p)
				}
				return nil, poolShowOutput{
					Candidates: candidates,
					Message:    fmt.Sprintf("%q matches %d pools; call again with one of these ids", in.Ref, len(pools)),
				}, nil
			}
		})
}
