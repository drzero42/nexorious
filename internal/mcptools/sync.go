package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
)

type syncStatusInput struct {
	Storefront string `json:"storefront,omitempty" jsonschema:"storefront slug; omit to list all configured storefronts and their slugs"`
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

// externalGameBrief is one entry of a storefront's External game list, or one
// candidate of an ambiguous ref: an entry, or (with only_this_edition) one
// edition, carrying its game's title and IGDB title so same-title games can be
// told apart.
type externalGameBrief struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	ExternalID     string   `json:"external_id"`
	Platforms      []string `json:"platforms"`
	IgdbTitle      *string  `json:"igdb_title,omitempty"`
	ResolvedIgdbID *int     `json:"resolved_igdb_id,omitempty"`
	StoreURL       *string  `json:"store_url,omitempty"`
	// Editions are the entry's editions, in sync_review only: the ids a
	// decision on one edition alone (only_this_edition) is sent to.
	Editions []externalEditionBrief `json:"editions,omitempty"`
}

// externalEditionBrief is one edition of an entry.
type externalEditionBrief struct {
	ID         string   `json:"id"`
	ExternalID string   `json:"external_id"`
	Platforms  []string `json:"platforms"`
	StoreURL   *string  `json:"store_url,omitempty"`
}

// platformsOrEmpty keeps platforms in the output as an array, also when a
// game has none: the field is always present, and never null.
func platformsOrEmpty(p []string) []string {
	if p == nil {
		return []string{}
	}
	return p
}

// externalBriefOf is the brief of an entry, without its editions.
func externalBriefOf(eg *cliclient.ExternalGame) externalGameBrief {
	return externalGameBrief{
		ID:             eg.ID,
		Title:          eg.Title,
		ExternalID:     eg.ExternalID,
		Platforms:      platformsOrEmpty(eg.Platforms),
		IgdbTitle:      eg.IgdbTitle,
		ResolvedIgdbID: eg.ResolvedIgdbID,
		StoreURL:       eg.StoreURL,
	}
}

// reviewBriefOf is the brief of an entry with its editions, for sync_review.
func reviewBriefOf(eg *cliclient.ExternalGame) externalGameBrief {
	b := externalBriefOf(eg)
	b.Editions = make([]externalEditionBrief, len(eg.Editions))
	for i, e := range eg.Editions {
		b.Editions[i] = externalEditionBrief{ID: e.ID, ExternalID: e.ExternalID, Platforms: platformsOrEmpty(e.Platforms), StoreURL: e.StoreURL}
	}
	return b
}

// editionCandidateOf is the candidate of one edition of an entry: the
// edition's id, storefront id, platforms and store link, with the entry's
// title and IGDB title.
func editionCandidateOf(eg *cliclient.ExternalGame, e cliclient.ExternalGameEdition) externalGameBrief {
	b := externalBriefOf(eg)
	b.ID, b.ExternalID, b.Platforms, b.StoreURL = e.ID, e.ExternalID, platformsOrEmpty(e.Platforms), e.StoreURL
	return b
}

// mcpResolveExternalRef resolves an external-game ref within a storefront to
// the id a decision is sent to (clientops.ExternalDecisionTarget), and the
// entry's title. On ambiguity or no match it returns candidates/message for
// the agent to disambiguate, without mutating anything: one candidate per
// matching entry, or with onlyThisEdition one per edition of the matching
// entries.
func mcpResolveExternalRef(ctx context.Context, c *cliclient.Client, key, sf, ref string, onlyThisEdition bool) (id, title string, cands []externalGameBrief, msg string, err error) {
	matches, err := clientops.FindExternalGamesByRef(ctx, c, key, sf, ref)
	if err != nil {
		return "", "", nil, "", err
	}
	if len(matches) == 0 {
		return "", "", nil, fmt.Sprintf("no external game matching %q in %s", ref, sf), nil
	}
	id, ambiguous, err := clientops.ExternalDecisionTarget(matches, onlyThisEdition)
	if err != nil {
		return "", "", nil, "", err
	}
	if !ambiguous {
		return id, matches[0].Game.Title, nil, "", nil
	}
	if onlyThisEdition {
		for i := range matches {
			for _, e := range matches[i].Game.Editions {
				cands = append(cands, editionCandidateOf(&matches[i].Game, e))
			}
		}
		return "", "", cands, fmt.Sprintf("%q matches %d editions; call again with one of these ids", ref, len(cands)), nil
	}
	cands = make([]externalGameBrief, len(matches))
	for i := range matches {
		cands[i] = externalBriefOf(&matches[i].Game)
	}
	return "", "", cands, fmt.Sprintf("%q matches %d external games; call again with one of these ids", ref, len(matches)), nil
}

type syncRunInput struct {
	Storefront string `json:"storefront" jsonschema:"storefront slug to sync (call sync_status with no storefront to list valid slugs)"`
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
	OrphanAction string `json:"orphan_action,omitempty" jsonschema:"keep or remove: what to do with a library game the rematch would leave with no storefront link (required when it would)"`
	// OnlyThisEdition rematches the one edition the ref names (#1452).
	OnlyThisEdition bool `json:"only_this_edition,omitempty" jsonschema:"rematch only the edition the ref names, not its other editions; ref must then be an edition's id (from an entry's editions) or a title with one edition"`
}

type syncSkipInput struct {
	Storefront      string `json:"storefront" jsonschema:"storefront slug"`
	Ref             string `json:"ref" jsonschema:"external-game id or title"`
	OnlyThisEdition bool   `json:"only_this_edition,omitempty" jsonschema:"skip only the edition the ref names, not its other editions; ref must then be an edition's id (from an entry's editions) or a title with one edition"`
}

type syncMatchOutput struct {
	Candidates []externalGameBrief `json:"candidates,omitempty"`
	Message    string              `json:"message,omitempty"`
}

type syncReviewOutput struct {
	PendingReview []externalGameBrief `json:"pending_review"`
	Total         int                 `json:"total"`
}

func registerSyncTools(s *toolSet, c *cliclient.Client, apiKey KeyFunc) {
	addTool(s, &mcp.Tool{Name: "sync_status", Description: "Return sync status. Without a storefront, lists all configured storefronts. With a storefront slug, returns real-time status for that storefront."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncStatusInput) (*mcp.CallToolResult, syncStatusOutput, error) {
			key := apiKey(ctx)
			if in.Storefront == "" {
				configs, err := c.ListSyncConfigs(ctx, key)
				if err != nil {
					return nil, syncStatusOutput{}, toolError("sync_status", err)
				}
				return nil, syncStatusOutput{Configs: configs}, nil
			}
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncStatusOutput{}, toolError("sync_status", err)
			}
			status, err := c.GetSyncStatus(ctx, key, sf)
			if err != nil {
				return nil, syncStatusOutput{}, toolError("sync_status", err)
			}
			return nil, syncStatusOutput{Status: status}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_review", Description: "List external games with sync_status 'needs_review' for a storefront, so an agent can help the user match them to IGDB entries."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncReviewInput) (*mcp.CallToolResult, syncReviewOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncReviewOutput{}, toolError("sync_review", err)
			}
			all, err := c.ListExternalGames(ctx, key, sf)
			if err != nil {
				return nil, syncReviewOutput{}, toolError("sync_review", err)
			}
			var pending []externalGameBrief
			for i := range all {
				if all[i].SyncStatus == "needs_review" {
					pending = append(pending, reviewBriefOf(&all[i]))
				}
			}
			return nil, syncReviewOutput{PendingReview: pending, Total: len(pending)}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_run", Description: "Trigger a sync for a storefront. Enqueues an async job and returns its id; does not block."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncRunInput) (*mcp.CallToolResult, syncJobOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncJobOutput{}, toolError("sync_run", err)
			}
			res, err := c.TriggerSync(ctx, key, sf)
			if err != nil {
				return nil, syncJobOutput{}, toolError("sync_run", err)
			}
			return nil, syncJobOutput{JobID: res.JobID, Storefront: res.Storefront, Status: res.Status, Message: res.Message}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_resolve", Description: "Match an external game to an IGDB id: one in review, an unmatched one (e.g. after an unskip), or a matched one to correct it. Writes it and its other editions to the library at once; with only_this_edition, only the edition the ref names, which takes two same-title games apart (rematching it back onto the others' match puts them together again). Refused while a sync is still working on the game; retry once the sync is done. Ambiguous refs return the matching external games (with only_this_edition: their editions, each with its game's title) in `candidates` — call again with an id."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncResolveInput) (*mcp.CallToolResult, syncMatchOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncMatchOutput{}, toolError("sync_resolve", err)
			}
			id, title, cands, msg, err := mcpResolveExternalRef(ctx, c, key, sf, in.Ref, in.OnlyThisEdition)
			if err != nil {
				return nil, syncMatchOutput{}, toolError("sync_resolve", err)
			}
			if cands != nil || msg != "" {
				return nil, syncMatchOutput{Candidates: cands, Message: msg}, nil
			}
			if err := c.RematchExternalGame(ctx, key, id, in.IgdbID, in.OrphanAction, in.OnlyThisEdition); err != nil {
				return nil, syncMatchOutput{}, toolError("sync_resolve", err)
			}
			return nil, syncMatchOutput{Message: fmt.Sprintf("resolved %q to IGDB id %d", title, in.IgdbID)}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_skip", Description: "Skip an external game and its other editions: the sync leaves them as they are and their open review items end; the library is not changed. With only_this_edition, skip only the edition the ref names (e.g. one platform of a package). Ambiguous refs return the matching external games (with only_this_edition: their editions, each with its game's title) in `candidates` — call again with an id."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncSkipInput) (*mcp.CallToolResult, syncMatchOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncMatchOutput{}, toolError("sync_skip", err)
			}
			id, title, cands, msg, err := mcpResolveExternalRef(ctx, c, key, sf, in.Ref, in.OnlyThisEdition)
			if err != nil {
				return nil, syncMatchOutput{}, toolError("sync_skip", err)
			}
			if cands != nil || msg != "" {
				return nil, syncMatchOutput{Candidates: cands, Message: msg}, nil
			}
			if err := c.SkipExternalGame(ctx, key, id, in.OnlyThisEdition); err != nil {
				return nil, syncMatchOutput{}, toolError("sync_skip", err)
			}
			return nil, syncMatchOutput{Message: fmt.Sprintf("skipped %q", title)}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_retry", Description: "Retry failed external-game matches for a storefront."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, toolError("sync_retry", err)
			}
			if err := c.RetryFailedExternalGames(ctx, key, sf); err != nil {
				return nil, syncAckOutput{}, toolError("sync_retry", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "retrying failed external-game matches"}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_reset", Description: "Reset (clear) sync data for a storefront. Destructive — call explicitly."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, toolError("sync_reset", err)
			}
			if err := c.ResetSyncData(ctx, key, sf); err != nil {
				return nil, syncAckOutput{}, toolError("sync_reset", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "sync data reset"}, nil
		})

	addTool(s, &mcp.Tool{Name: "sync_disconnect", Description: "Disconnect a storefront (remove its stored credentials)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in syncStorefrontInput) (*mcp.CallToolResult, syncAckOutput, error) {
			key := apiKey(ctx)
			sf, err := clientops.ResolveStorefront(ctx, c, key, in.Storefront)
			if err != nil {
				return nil, syncAckOutput{}, toolError("sync_disconnect", err)
			}
			if err := c.DisconnectStorefront(ctx, key, sf); err != nil {
				return nil, syncAckOutput{}, toolError("sync_disconnect", err)
			}
			return nil, syncAckOutput{Storefront: sf, Message: "storefront disconnected"}, nil
		})
}
