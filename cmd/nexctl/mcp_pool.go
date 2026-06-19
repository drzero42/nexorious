package main

import (
	"context"
	"encoding/json"
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

// poolWriteOutput is the standard output for pool write tools.
type poolWriteOutput struct {
	Pool       *poolBrief  `json:"pool,omitempty"`
	Pools      []poolBrief `json:"pools,omitempty"`
	Candidates []poolBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// poolGamesOutput is the output for pool membership tools.
type poolGamesOutput struct {
	Pool       *poolBrief  `json:"pool,omitempty"`
	Candidates []poolBrief `json:"candidates,omitempty"`
	GameCands  []gameBrief `json:"game_candidates,omitempty"`
	Count      int64       `json:"count,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// poolCreateInput is the input schema for pool_create.
type poolCreateInput struct {
	Name   string          `json:"name" jsonschema:"pool name"`
	Color  *string         `json:"color,omitempty" jsonschema:"hex color e.g. #6B7280"`
	Filter json.RawMessage `json:"filter,omitempty" jsonschema:"saved filter as JSON e.g. {\"filters\":[{\"genre\":[\"RPG\"]}]}"`
}

// poolEditInput is the input schema for pool_edit.
type poolEditInput struct {
	Ref         string          `json:"ref" jsonschema:"pool id (UUID) or name"`
	Name        *string         `json:"name,omitempty" jsonschema:"new pool name"`
	Color       *string         `json:"color,omitempty" jsonschema:"new hex color"`
	Filter      json.RawMessage `json:"filter,omitempty" jsonschema:"new saved filter as JSON"`
	ClearFilter bool            `json:"clear_filter,omitempty" jsonschema:"set true to remove the saved filter"`
}

// poolRmInput is the input schema for pool_rm.
type poolRmInput struct {
	Ref string `json:"ref" jsonschema:"pool id (UUID) or name"`
}

// poolAddInput is the input schema for pool_add.
type poolAddInput struct {
	Pool  string   `json:"pool" jsonschema:"pool id (UUID) or name"`
	Games []string `json:"games" jsonschema:"one or more game ids (UUID) or title substrings"`
}

// poolRemoveInput is the input schema for pool_remove.
type poolRemoveInput struct {
	Pool  string   `json:"pool" jsonschema:"pool id (UUID) or name"`
	Games []string `json:"games" jsonschema:"one or more game ids (UUID) or title substrings"`
}

// poolQueueInput is the input schema for pool_queue.
type poolQueueInput struct {
	Pool  string   `json:"pool" jsonschema:"pool id (UUID) or name"`
	Games []string `json:"games" jsonschema:"ordered list of game ids (UUID) or title substrings for the queue"`
}

// poolReorderInput is the input schema for pool_reorder.
type poolReorderInput struct {
	Pools []string `json:"pools" jsonschema:"pool ids (UUID) or names in desired order"`
}

// resolvePoolForWrite resolves a single pool from ref; returns candidates output if ambiguous.
func resolvePoolForWrite(c *cliclient.Client, key, ref, tool string) (*cliclient.PoolListItem, *poolWriteOutput, error) {
	pools, err := findPoolsByRef(c, key, ref)
	if err != nil {
		return nil, nil, mcpToolError(tool, err)
	}
	switch len(pools) {
	case 0:
		return nil, nil, fmt.Errorf("%s: no pool matching %q", tool, ref)
	case 1:
		return &pools[0], nil, nil
	default:
		cands := make([]poolBrief, len(pools))
		for i, p := range pools {
			cands[i] = poolBriefOf(p)
		}
		out := &poolWriteOutput{
			Candidates: cands,
			Message:    fmt.Sprintf("%q matches %d pools; call again with one of these ids", ref, len(pools)),
		}
		return nil, out, nil
	}
}

// resolveGamesForPool resolves each game ref to a user-game; returns game candidates if any are ambiguous.
func resolveGamesForPool(c *cliclient.Client, key string, refs []string, tool string) (ids []string, cands []gameBrief, msg string, err error) {
	var resolved []string
	for _, ref := range refs {
		matches, err := findUserGamesByRef(c, key, ref)
		if err != nil {
			return nil, nil, "", mcpToolError(tool, err)
		}
		switch len(matches) {
		case 0:
			return nil, nil, fmt.Sprintf("no game matching %q in your library", ref), nil
		case 1:
			resolved = append(resolved, matches[0].ID)
		default:
			briefs := make([]gameBrief, len(matches))
			for i := range matches {
				briefs[i] = briefOf(&matches[i])
			}
			return nil, briefs, fmt.Sprintf("%q matches %d games; call again with one of these ids", ref, len(matches)), nil
		}
	}
	return resolved, nil, "", nil
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

	mcp.AddTool(s, &mcp.Tool{Name: "pool_create", Description: "Create a new play-planning pool."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolCreateInput) (*mcp.CallToolResult, poolWriteOutput, error) {
			pool, err := c.CreatePool(key, in.Name, in.Color, in.Filter)
			if err != nil {
				return nil, poolWriteOutput{}, mcpToolError("pool_create", err)
			}
			brief := poolBrief{ID: pool.ID, Name: pool.Name, Color: pool.Color}
			return nil, poolWriteOutput{Pool: &brief, Message: fmt.Sprintf("Created pool %q (%s).", pool.Name, pool.ID)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_edit", Description: "Edit a pool's name, color, or saved filter. Ambiguous name matches return candidates — call again with the id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolEditInput) (*mcp.CallToolResult, poolWriteOutput, error) {
			ref, cands, err := resolvePoolForWrite(c, key, in.Ref, "pool_edit")
			if err != nil {
				return nil, poolWriteOutput{}, err
			}
			if cands != nil {
				return nil, *cands, nil
			}
			fields := map[string]any{}
			if in.Name != nil {
				fields["name"] = *in.Name
			}
			if in.Color != nil {
				fields["color"] = *in.Color
			}
			if in.ClearFilter {
				fields["filter"] = nil
			} else if len(in.Filter) > 0 {
				fields["filter"] = in.Filter
			}
			if len(fields) == 0 {
				return nil, poolWriteOutput{}, fmt.Errorf("pool_edit: nothing to change; provide name, color, filter, or clear_filter")
			}
			updated, err := c.UpdatePool(key, ref.ID, fields)
			if err != nil {
				return nil, poolWriteOutput{}, mcpToolError("pool_edit", err)
			}
			brief := poolBrief{ID: updated.ID, Name: updated.Name, Color: updated.Color}
			return nil, poolWriteOutput{Pool: &brief, Message: fmt.Sprintf("Updated pool %q.", updated.Name)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_rm", Description: "Delete a pool. Ambiguous name matches return candidates — call again with the id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolRmInput) (*mcp.CallToolResult, poolWriteOutput, error) {
			ref, cands, err := resolvePoolForWrite(c, key, in.Ref, "pool_rm")
			if err != nil {
				return nil, poolWriteOutput{}, err
			}
			if cands != nil {
				return nil, *cands, nil
			}
			if err := c.DeletePool(key, ref.ID); err != nil {
				return nil, poolWriteOutput{}, mcpToolError("pool_rm", err)
			}
			return nil, poolWriteOutput{Message: fmt.Sprintf("Deleted pool %q (%s).", ref.Name, ref.ID)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_add", Description: "Add games to a pool as candidates. Ambiguous pool/game refs return candidates — call again with ids."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolAddInput) (*mcp.CallToolResult, poolGamesOutput, error) {
			if len(in.Games) == 0 {
				return nil, poolGamesOutput{}, fmt.Errorf("pool_add: provide at least one game")
			}
			pool, cands, err := resolvePoolForWrite(c, key, in.Pool, "pool_add")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if cands != nil {
				poolCands := cands.Candidates
				return nil, poolGamesOutput{Candidates: poolCands, Message: cands.Message}, nil
			}
			ids, gameCands, msg, err := resolveGamesForPool(c, key, in.Games, "pool_add")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if gameCands != nil || msg != "" {
				brief := poolBriefOf(*pool)
				return nil, poolGamesOutput{Pool: &brief, GameCands: gameCands, Message: msg}, nil
			}
			pb := poolBriefOf(*pool)
			if len(ids) == 1 {
				if err := c.AddPoolGame(key, pool.ID, ids[0]); err != nil {
					return nil, poolGamesOutput{}, mcpToolError("pool_add", err)
				}
				return nil, poolGamesOutput{Pool: &pb, Count: 1, Message: fmt.Sprintf("Added 1 game to %q.", pool.Name)}, nil
			}
			n, err := c.BulkAddPoolGames(key, pool.ID, ids)
			if err != nil {
				return nil, poolGamesOutput{}, mcpToolError("pool_add", err)
			}
			return nil, poolGamesOutput{Pool: &pb, Count: n, Message: fmt.Sprintf("Added %d game(s) to %q.", n, pool.Name)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_remove", Description: "Remove games from a pool. Ambiguous pool/game refs return candidates — call again with ids."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolRemoveInput) (*mcp.CallToolResult, poolGamesOutput, error) {
			if len(in.Games) == 0 {
				return nil, poolGamesOutput{}, fmt.Errorf("pool_remove: provide at least one game")
			}
			pool, cands, err := resolvePoolForWrite(c, key, in.Pool, "pool_remove")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if cands != nil {
				return nil, poolGamesOutput{Candidates: cands.Candidates, Message: cands.Message}, nil
			}
			ids, gameCands, msg, err := resolveGamesForPool(c, key, in.Games, "pool_remove")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if gameCands != nil || msg != "" {
				brief := poolBriefOf(*pool)
				return nil, poolGamesOutput{Pool: &brief, GameCands: gameCands, Message: msg}, nil
			}
			pb := poolBriefOf(*pool)
			for _, id := range ids {
				if err := c.RemovePoolGame(key, pool.ID, id); err != nil {
					return nil, poolGamesOutput{}, mcpToolError(fmt.Sprintf("pool_remove %s", id), err)
				}
			}
			return nil, poolGamesOutput{Pool: &pb, Count: int64(len(ids)), Message: fmt.Sprintf("Removed %d game(s) from %q.", len(ids), pool.Name)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_queue", Description: "Set a pool's ordered queue (declarative — BulkAdd then SetQueue). Ambiguous pool/game refs return candidates — call again with ids."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolQueueInput) (*mcp.CallToolResult, poolGamesOutput, error) {
			pool, cands, err := resolvePoolForWrite(c, key, in.Pool, "pool_queue")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if cands != nil {
				return nil, poolGamesOutput{Candidates: cands.Candidates, Message: cands.Message}, nil
			}
			ids, gameCands, msg, err := resolveGamesForPool(c, key, in.Games, "pool_queue")
			if err != nil {
				return nil, poolGamesOutput{}, err
			}
			if gameCands != nil || msg != "" {
				brief := poolBriefOf(*pool)
				return nil, poolGamesOutput{Pool: &brief, GameCands: gameCands, Message: msg}, nil
			}
			pb := poolBriefOf(*pool)
			if len(ids) > 0 {
				if _, err := c.BulkAddPoolGames(key, pool.ID, ids); err != nil {
					return nil, poolGamesOutput{}, mcpToolError("pool_queue (bulk-add)", err)
				}
			}
			if err := c.SetQueue(key, pool.ID, ids); err != nil {
				return nil, poolGamesOutput{}, mcpToolError("pool_queue (set-queue)", err)
			}
			if len(ids) == 0 {
				return nil, poolGamesOutput{Pool: &pb, Message: fmt.Sprintf("Cleared the queue for %q.", pool.Name)}, nil
			}
			return nil, poolGamesOutput{Pool: &pb, Count: int64(len(ids)), Message: fmt.Sprintf("Queued %d game(s) in %q.", len(ids), pool.Name)}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "pool_reorder", Description: "Reorder pools — positions follow argument order. Ambiguous pool refs return candidates — call again with ids."},
		func(_ context.Context, _ *mcp.CallToolRequest, in poolReorderInput) (*mcp.CallToolResult, poolWriteOutput, error) {
			if len(in.Pools) == 0 {
				return nil, poolWriteOutput{}, fmt.Errorf("pool_reorder: provide at least one pool")
			}
			var ids []string
			var briefs []poolBrief
			for _, ref := range in.Pools {
				pool, cands, err := resolvePoolForWrite(c, key, ref, "pool_reorder")
				if err != nil {
					return nil, poolWriteOutput{}, err
				}
				if cands != nil {
					return nil, *cands, nil
				}
				ids = append(ids, pool.ID)
				briefs = append(briefs, poolBriefOf(*pool))
			}
			if err := c.ReorderPools(key, ids); err != nil {
				return nil, poolWriteOutput{}, mcpToolError("pool_reorder", err)
			}
			return nil, poolWriteOutput{Pools: briefs, Message: fmt.Sprintf("Reordered %d pool(s).", len(ids))}, nil
		})
}
