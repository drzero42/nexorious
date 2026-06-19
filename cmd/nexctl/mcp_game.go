package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/enum"
)

// gameBrief is the concise list projection (human fields + id for chaining).
type gameBrief struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	PlayStatus string   `json:"play_status,omitempty"`
	Rating     *int     `json:"rating,omitempty"`
	Hours      float64  `json:"hours_played"`
	Wishlist   bool     `json:"wishlist"`
	Platforms  []string `json:"platforms,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

func briefOf(u *cliclient.UserGame) gameBrief {
	b := gameBrief{ID: u.ID, Title: u.Title(), Rating: u.PersonalRating,
		Hours: u.HoursPlayed, Wishlist: u.IsWishlisted}
	if u.PlayStatus != nil {
		b.PlayStatus = *u.PlayStatus
	}
	for i := range u.Platforms {
		if u.Platforms[i].Platform != nil {
			b.Platforms = append(b.Platforms, *u.Platforms[i].Platform)
		}
	}
	for _, t := range u.Tags {
		b.Tags = append(b.Tags, t.Name)
	}
	return b
}

// gamePlatformDetail is one platform row in a full game detail.
type gamePlatformDetail struct {
	ID              string   `json:"id"`
	Platform        *string  `json:"platform,omitempty"`
	Storefront      *string  `json:"storefront,omitempty"`
	HoursPlayed     *float64 `json:"hours_played,omitempty"`
	OwnershipStatus *string  `json:"ownership_status,omitempty"`
}

// gameDetail is the full projection for game_show.
type gameDetail struct {
	gameBrief
	Notes    *string              `json:"notes,omitempty"`
	IsLoved  bool                 `json:"is_loved"`
	PlatRows []gamePlatformDetail `json:"platform_rows,omitempty"`
}

func detailOf(u *cliclient.UserGame) gameDetail {
	d := gameDetail{gameBrief: briefOf(u), Notes: u.PersonalNotes, IsLoved: u.IsLoved}
	for _, p := range u.Platforms {
		row := gamePlatformDetail{
			ID:              p.ID,
			Platform:        p.Platform,
			Storefront:      p.Storefront,
			HoursPlayed:     p.HoursPlayed,
			OwnershipStatus: p.OwnershipStatus,
		}
		d.PlatRows = append(d.PlatRows, row)
	}
	return d
}

type gameListInput struct {
	Q               string `json:"q,omitempty" jsonschema:"free-text title search"`
	PlayStatus      string `json:"play_status,omitempty" jsonschema:"filter by play status: not_started, in_progress, completed, mastered, dominated, shelved, dropped, replay"`
	OwnershipStatus string `json:"ownership_status,omitempty" jsonschema:"filter by ownership status"`
	Wishlist        *bool  `json:"wishlist,omitempty" jsonschema:"true = only wishlisted, false = only library"`
	Genre           string `json:"genre,omitempty"`
	Platform        string `json:"platform,omitempty"`
	Storefront      string `json:"storefront,omitempty"`
	SortBy          string `json:"sort_by,omitempty"`
	SortOrder       string `json:"sort_order,omitempty"`
	Page            int    `json:"page,omitempty"`
	PerPage         int    `json:"per_page,omitempty" jsonschema:"page size, max 200"`
}

type gameListOutput struct {
	Games []gameBrief `json:"games"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Pages int         `json:"pages"`
}

type gameShowInput struct {
	Ref string `json:"ref" jsonschema:"game id (UUID) or title substring"`
}

type gameShowOutput struct {
	Game       *gameDetail `json:"game,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

type gameFiltersOutput struct {
	PlayStatuses       []string `json:"play_statuses"`
	OwnershipStatuses  []string `json:"ownership_statuses"`
	Storefronts        []string `json:"storefronts"`
	Genres             []string `json:"genres"`
	GameModes          []string `json:"game_modes"`
	Themes             []string `json:"themes"`
	PlayerPerspectives []string `json:"player_perspectives"`
}

func registerGameTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "game_list", Description: "List/search the collection (concise projection)."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameListInput) (*mcp.CallToolResult, gameListOutput, error) {
			params := url.Values{}
			setIf(params, "q", in.Q)
			setIf(params, "play_status", in.PlayStatus)
			setIf(params, "ownership_status", in.OwnershipStatus)
			setIf(params, "genre", in.Genre)
			setIf(params, "platform", in.Platform)
			setIf(params, "storefront", in.Storefront)
			setIf(params, "sort_by", in.SortBy)
			setIf(params, "sort_order", in.SortOrder)
			if in.Wishlist != nil {
				params.Set("wishlist", strconv.FormatBool(*in.Wishlist))
			}
			if in.Page > 0 {
				params.Set("page", strconv.Itoa(in.Page))
			}
			if in.PerPage > 0 {
				params.Set("per_page", strconv.Itoa(in.PerPage))
			}
			res, err := c.ListUserGames(key, params)
			if err != nil {
				return nil, gameListOutput{}, mcpToolError("game_list", err)
			}
			out := gameListOutput{Total: res.Total, Page: res.Page, Pages: res.Pages}
			for i := range res.UserGames {
				out.Games = append(out.Games, briefOf(&res.UserGames[i]))
			}
			return nil, out, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_show", Description: "Show full detail for one game (by id or title). If the title matches multiple games, returns candidates to disambiguate."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameShowInput) (*mcp.CallToolResult, gameShowOutput, error) {
			games, err := findUserGamesByRef(c, key, in.Ref)
			if err != nil {
				return nil, gameShowOutput{}, mcpToolError("game_show", err)
			}
			switch len(games) {
			case 0:
				return nil, gameShowOutput{}, fmt.Errorf("game_show: no game matching %q in your library", in.Ref)
			case 1:
				d := detailOf(&games[0])
				return nil, gameShowOutput{Game: &d}, nil
			default:
				candidates := make([]gameBrief, len(games))
				for i := range games {
					candidates[i] = briefOf(&games[i])
				}
				return nil, gameShowOutput{
					Candidates: candidates,
					Message:    fmt.Sprintf("%q matches %d games; call again with one of these ids", in.Ref, len(games)),
				}, nil
			}
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_stats", Description: "Return aggregate collection statistics (total games, completion rate, hours, rating, breakdown by status/platform/genre)."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, *cliclient.CollectionStats, error) {
			stats, err := c.GetCollectionStats(key)
			if err != nil {
				return nil, nil, mcpToolError("game_stats", err)
			}
			return nil, stats, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_filters", Description: "Return valid values for every game_list filter: play/ownership statuses, storefronts, and library-derived genre/game-mode/theme/perspective facets."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, gameFiltersOutput, error) {
			opts, err := c.GetFilterOptions(key)
			if err != nil {
				return nil, gameFiltersOutput{}, mcpToolError("game_filters (filter-options)", err)
			}
			storefronts, err := c.ListStorefronts(key)
			if err != nil {
				return nil, gameFiltersOutput{}, mcpToolError("game_filters (storefronts)", err)
			}
			slugs := make([]string, len(storefronts))
			for i, sf := range storefronts {
				slugs[i] = sf.Name
			}
			out := gameFiltersOutput{
				PlayStatuses:       enum.AllPlayStatuses(),
				OwnershipStatuses:  enum.AllOwnershipStatuses(),
				Storefronts:        slugs,
				Genres:             opts.Genres,
				GameModes:          opts.GameModes,
				Themes:             opts.Themes,
				PlayerPerspectives: opts.PlayerPerspectives,
			}
			return nil, out, nil
		})
}
