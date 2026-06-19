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

// gameEditInput is the input schema for game_edit.
type gameEditInput struct {
	Refs          []string        `json:"refs,omitempty" jsonschema:"one or more game titles or ids"`
	Filter        *gameEditFilter `json:"filter,omitempty" jsonschema:"select games by filter instead of refs"`
	PlayStatus    string          `json:"play_status,omitempty"`
	Rating        *int            `json:"rating,omitempty"`
	Loved         *bool           `json:"loved,omitempty"`
	Notes         *string         `json:"notes,omitempty"`
	AddPlatform   string          `json:"add_platform,omitempty" jsonschema:"platform[/storefront] to add"`
	RmPlatform    string          `json:"rm_platform,omitempty"`
	Hours         *float64        `json:"hours,omitempty"`
	HoursPlatform string          `json:"hours_platform,omitempty"`
	AddTags       []string        `json:"add_tags,omitempty"`
	RmTags        []string        `json:"rm_tags,omitempty"`
}

// gameEditFilter selects games by server-side filter criteria.
type gameEditFilter struct {
	PlayStatus string `json:"play_status,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Wishlist   *bool  `json:"wishlist,omitempty"`
}

// gameWriteOutput is the standard output for write tools (edit, rm, acquire).
type gameWriteOutput struct {
	Updated    []gameBrief `json:"updated,omitempty"`
	Removed    []gameBrief `json:"removed,omitempty"`
	Candidates []gameBrief `json:"candidates,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// gameAddInput is the input schema for game_add.
type gameAddInput struct {
	Title      string  `json:"title,omitempty" jsonschema:"IGDB title to search (mutually exclusive with igdb_id)"`
	IgdbID     int     `json:"igdb_id,omitempty" jsonschema:"exact IGDB game id (skips title search)"`
	PlayStatus string  `json:"play_status,omitempty"`
	Platform   string  `json:"platform,omitempty" jsonschema:"platform[/storefront] slug"`
	Storefront string  `json:"storefront,omitempty" jsonschema:"storefront slug (overrides platform/storefront)"`
	Notes      *string `json:"notes,omitempty"`
	Wishlist   bool    `json:"wishlist,omitempty"`
	Loved      bool    `json:"loved,omitempty"`
	Rating     *int    `json:"rating,omitempty"`
}

// igdbCandidateBrief is the concise IGDB projection returned when a title search
// is ambiguous (multiple results). It uses igdb_id (int) and release_date to
// help MCP consumers distinguish candidates without confusing play-status fields.
type igdbCandidateBrief struct {
	IgdbID      int    `json:"igdb_id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date,omitempty"`
}

// gameAddOutput is the output for game_add.
type gameAddOutput struct {
	Game       *gameBrief           `json:"game,omitempty"`
	Candidates []igdbCandidateBrief `json:"candidates,omitempty"`
	Message    string               `json:"message,omitempty"`
}

// gameAcquireInput is the input schema for game_acquire.
type gameAcquireInput struct {
	Ref             string `json:"ref" jsonschema:"game id (UUID) or title substring"`
	Platform        string `json:"platform" jsonschema:"platform[/storefront] slug (required)"`
	Storefront      string `json:"storefront,omitempty" jsonschema:"storefront slug (overrides platform/storefront)"`
	OwnershipStatus string `json:"ownership_status,omitempty"`
}

// gameRmInput is the input schema for game_rm.
type gameRmInput struct {
	Refs   []string        `json:"refs,omitempty" jsonschema:"one or more game titles or ids"`
	Filter *gameEditFilter `json:"filter,omitempty" jsonschema:"select games by filter instead of refs"`
}

// mcpResolveEditTargets resolves game targets from refs or filter for edit/rm tools.
// Returns candidates (no mutation) if any ref is ambiguous or zero.
func mcpResolveEditTargets(c *cliclient.Client, key string, refs []string, filter *gameEditFilter) (games []cliclient.UserGame, candidates []gameBrief, msg string, err error) {
	if filter != nil {
		f := gameFilter{
			use:      true,
			status:   filter.PlayStatus,
			tag:      filter.Tag,
			platform: filter.Platform,
		}
		if filter.Wishlist != nil {
			f.wishlist = *filter.Wishlist
			f.wishlistSet = true
		}
		gs, _, err := gamesByFilter(c, key, f)
		if err != nil {
			return nil, nil, "", err
		}
		return gs, nil, "", nil
	}
	// resolve each ref individually; collect ambiguous matches as candidates
	var all []cliclient.UserGame
	for _, ref := range refs {
		matches, err := findUserGamesByRef(c, key, ref)
		if err != nil {
			return nil, nil, "", err
		}
		switch len(matches) {
		case 0:
			return nil, nil, fmt.Sprintf("no game matching %q in your library", ref), nil
		case 1:
			all = append(all, matches[0])
		default:
			cands := make([]gameBrief, len(matches))
			for i := range matches {
				cands[i] = briefOf(&matches[i])
			}
			return nil, cands, fmt.Sprintf("%q matches %d games; call again with one of these ids", ref, len(matches)), nil
		}
	}
	return all, nil, "", nil
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

	mcp.AddTool(s, &mcp.Tool{Name: "game_add", Description: "Add a game to the collection via IGDB lookup. Provide title or igdb_id. If title matches multiple IGDB games, returns candidates with igdb_id — call again with igdb_id to proceed."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameAddInput) (*mcp.CallToolResult, gameAddOutput, error) {
			if in.IgdbID == 0 && in.Title == "" {
				return nil, gameAddOutput{}, fmt.Errorf("game_add: provide title or igdb_id")
			}
			cands, err := findIGDBCandidates(c, key, in.IgdbID, in.Title)
			if err != nil {
				return nil, gameAddOutput{}, mcpToolError("game_add", err)
			}
			if len(cands) == 0 {
				return nil, gameAddOutput{}, fmt.Errorf("game_add: no IGDB results for %q", in.Title)
			}
			if len(cands) > 1 {
				briefs := make([]igdbCandidateBrief, len(cands))
				for i, g := range cands {
					briefs[i] = igdbCandidateBrief{IgdbID: g.IgdbID, Title: g.Title, ReleaseDate: g.ReleaseDate}
				}
				return nil, gameAddOutput{
					Candidates: briefs,
					Message:    fmt.Sprintf("%q matches %d IGDB games; call again with igdb_id", in.Title, len(cands)),
				}, nil
			}
			chosen := &cands[0]
			if _, err := c.ImportIGDBGame(key, chosen.IgdbID); err != nil {
				return nil, gameAddOutput{}, mcpToolError("game_add (import)", err)
			}
			status := in.PlayStatus
			if status == "" {
				status = "not_started"
			}
			input := cliclient.CreateUserGameInput{
				GameID:         chosen.IgdbID,
				PlayStatus:     status,
				IsLoved:        in.Loved,
				IsWishlisted:   in.Wishlist,
				PersonalRating: in.Rating,
				PersonalNotes:  in.Notes,
			}
			if in.Platform != "" {
				pl, sf := splitPlatform(in.Platform)
				if in.Storefront != "" {
					sf = in.Storefront
				}
				input.Platforms = []cliclient.PlatformInput{{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}}
			}
			ug, err := c.CreateUserGame(key, input)
			if err != nil {
				return nil, gameAddOutput{}, mcpToolError("game_add (create)", err)
			}
			b := briefOf(ug)
			return nil, gameAddOutput{Game: &b}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_edit", Description: "Edit one or more games in the collection (status, rating, notes, platforms, tags). Use refs (ids/title substrings) or filter. Ambiguous refs return candidates — call again with the id."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameEditInput) (*mcp.CallToolResult, gameWriteOutput, error) {
			if len(in.Refs) == 0 && in.Filter == nil {
				return nil, gameWriteOutput{}, fmt.Errorf("game_edit: provide refs or filter")
			}
			games, cands, msg, err := mcpResolveEditTargets(c, key, in.Refs, in.Filter)
			if err != nil {
				return nil, gameWriteOutput{}, mcpToolError("game_edit", err)
			}
			if cands != nil || msg != "" {
				return nil, gameWriteOutput{Candidates: cands, Message: msg}, nil
			}
			opts := editOpts{
				status:        in.PlayStatus,
				statusSet:     in.PlayStatus != "",
				addPlatform:   in.AddPlatform,
				rmPlatform:    in.RmPlatform,
				hoursPlatform: in.HoursPlatform,
				addTags:       in.AddTags,
				rmTags:        in.RmTags,
			}
			if in.Rating != nil {
				opts.rating = *in.Rating
				opts.ratingSet = true
			}
			if in.Notes != nil {
				opts.notes = *in.Notes
				opts.notesSet = true
			}
			if in.Hours != nil {
				opts.hours = *in.Hours
				opts.hoursSet = true
			}
			if in.Loved != nil {
				if *in.Loved {
					opts.loved = true
				} else {
					opts.noLoved = true
				}
			}
			updated := make([]gameBrief, 0, len(games))
			for i := range games {
				u := &games[i]
				if err := editOne(c, key, u, opts); err != nil {
					return nil, gameWriteOutput{}, mcpToolError(fmt.Sprintf("game_edit %q", u.Title()), err)
				}
				updated = append(updated, briefOf(u))
			}
			return nil, gameWriteOutput{Updated: updated}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_acquire", Description: "Promote a wishlisted game to the library with a platform. Ref is a game id or title substring."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameAcquireInput) (*mcp.CallToolResult, gameWriteOutput, error) {
			if in.Platform == "" {
				return nil, gameWriteOutput{}, fmt.Errorf("game_acquire: platform is required")
			}
			matches, err := findUserGamesByRef(c, key, in.Ref)
			if err != nil {
				return nil, gameWriteOutput{}, mcpToolError("game_acquire", err)
			}
			if len(matches) == 0 {
				return nil, gameWriteOutput{}, fmt.Errorf("game_acquire: no game matching %q", in.Ref)
			}
			if len(matches) > 1 {
				cands := make([]gameBrief, len(matches))
				for i := range matches {
					cands[i] = briefOf(&matches[i])
				}
				return nil, gameWriteOutput{
					Candidates: cands,
					Message:    fmt.Sprintf("%q matches %d games; call again with one of these ids", in.Ref, len(matches)),
				}, nil
			}
			u := &matches[0]
			pl, sf := splitPlatform(in.Platform)
			if in.Storefront != "" {
				sf = in.Storefront
			}
			ownership := in.OwnershipStatus
			if ownership == "" {
				ownership = "owned"
			}
			ug, err := c.MoveToLibrary(key, u.ID, []cliclient.PlatformInput{
				{Platform: pl, Storefront: sf, OwnershipStatus: ownership},
			})
			if err != nil {
				return nil, gameWriteOutput{}, mcpToolError("game_acquire", err)
			}
			b := briefOf(ug)
			return nil, gameWriteOutput{Updated: []gameBrief{b}}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "game_rm", Description: "Remove games from the collection. Use refs (ids/title substrings) or filter. No confirmation is required — the call is the intent."},
		func(_ context.Context, _ *mcp.CallToolRequest, in gameRmInput) (*mcp.CallToolResult, gameWriteOutput, error) {
			if len(in.Refs) == 0 && in.Filter == nil {
				return nil, gameWriteOutput{}, fmt.Errorf("game_rm: provide refs or filter")
			}
			games, cands, msg, err := mcpResolveEditTargets(c, key, in.Refs, in.Filter)
			if err != nil {
				return nil, gameWriteOutput{}, mcpToolError("game_rm", err)
			}
			if cands != nil || msg != "" {
				return nil, gameWriteOutput{Candidates: cands, Message: msg}, nil
			}
			removed := make([]gameBrief, 0, len(games))
			for i := range games {
				u := &games[i]
				if err := c.DeleteUserGame(key, u.ID); err != nil {
					return nil, gameWriteOutput{}, mcpToolError(fmt.Sprintf("game_rm %q", u.Title()), err)
				}
				removed = append(removed, briefOf(u))
			}
			return nil, gameWriteOutput{Removed: removed}, nil
		})
}
