package clientops

import (
	"context"
	"net/url"
	"regexp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// LooksLikeUUID reports whether s is a canonical UUID (an id ref, not a title).
func LooksLikeUUID(s string) bool { return uuidRe.MatchString(s) }

// FindUserGamesByRef returns every library game matching ref: a UUID is fetched
// directly (one element); a title is a search query (zero or more). No cmd, no
// picker — callers own disambiguation.
func FindUserGamesByRef(ctx context.Context, c *cliclient.Client, key, ref string) ([]cliclient.UserGame, error) {
	if LooksLikeUUID(ref) {
		u, err := c.GetUserGame(ctx, key, ref)
		if err != nil {
			return nil, err
		}
		return []cliclient.UserGame{*u}, nil
	}
	res, err := c.ListUserGames(ctx, key, urlValues("q", ref))
	if err != nil {
		return nil, err
	}
	return res.UserGames, nil
}

// urlValues builds a url.Values from alternating key,value pairs, skipping
// pairs with an empty value.
func urlValues(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return v
}

// SetIf sets key in v only when val is non-empty.
func SetIf(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}
