package clientops

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// SplitPlatform splits "platform/storefront" into its parts; a bare value yields
// an empty storefront.
func SplitPlatform(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// FindIGDBCandidates returns the IGDB candidates for a query. The backend
// performs ID inference ("igdb:NNNN" / bare number), so callers just pass the
// raw query. No cmd, no picker — callers own disambiguation.
func FindIGDBCandidates(ctx context.Context, c *cliclient.Client, key, query string) ([]cliclient.IGDBCandidate, error) {
	res, err := c.SearchIGDB(ctx, key, query, 10)
	if err != nil {
		return nil, err
	}
	return res.Games, nil
}

// GameFilter is the subset of list filters supported for bulk edit/rm selection.
// WishlistSet distinguishes "filter on Wishlist" from "no wishlist filter".
type GameFilter struct {
	Status, Tag, Platform string
	Wishlist, WishlistSet bool
}

// GamesByFilter runs a filtered library query and returns the matching games and
// the server total (for truncation warnings). No cmd, no picker — callers own
// the warning and the per_page=200 cap is applied here.
func GamesByFilter(ctx context.Context, c *cliclient.Client, key string, f GameFilter) (games []cliclient.UserGame, total int, err error) {
	params := url.Values{}
	SetIf(params, "play_status", f.Status)
	SetIf(params, "platform", f.Platform)
	if f.WishlistSet {
		params.Set("wishlist", strconv.FormatBool(f.Wishlist))
	}
	if f.Tag != "" {
		id, err := ResolveTagID(ctx, c, key, f.Tag)
		if err != nil {
			return nil, 0, err
		}
		params.Set("tag", id)
	}
	params.Set("per_page", "200")
	res, err := c.ListUserGames(ctx, key, params)
	if err != nil {
		return nil, 0, err
	}
	return res.UserGames, res.Total, nil
}
