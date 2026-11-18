package clientops

import (
	"context"
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// ResolveStorefront validates arg against the configured storefronts by fetching
// the sync config list. Matching is case-insensitive. On success the canonical
// Storefront string is returned; on failure a descriptive error listing valid
// options is returned.
func ResolveStorefront(ctx context.Context, c *cliclient.Client, key, arg string) (string, error) {
	configs, err := c.ListSyncConfigs(ctx, key)
	if err != nil {
		return "", fmt.Errorf("list sync configs failed: %w", err)
	}
	if len(configs) == 0 {
		return "", fmt.Errorf("no storefronts available on this server")
	}
	for _, cfg := range configs {
		if strings.EqualFold(cfg.Storefront, arg) {
			return cfg.Storefront, nil
		}
	}
	slugs := make([]string, len(configs))
	for i, cfg := range configs {
		slugs[i] = cfg.Storefront
	}
	return "", fmt.Errorf("unknown storefront %q; valid: %s", arg, strings.Join(slugs, ", "))
}

// FindExternalGamesByRef returns every external game matching ref from the given
// storefront: a UUID is matched exactly (one element or error); a title is
// matched case-insensitively (zero or more). No cmd, no picker — callers own
// disambiguation.
func FindExternalGamesByRef(ctx context.Context, c *cliclient.Client, key, sf, ref string) ([]cliclient.ExternalGame, error) {
	games, err := c.ListExternalGames(ctx, key, sf)
	if err != nil {
		return nil, err
	}
	if LooksLikeUUID(ref) {
		for i := range games {
			if games[i].ID == ref {
				return []cliclient.ExternalGame{games[i]}, nil
			}
		}
		return nil, fmt.Errorf("no external game with id %s", ref)
	}
	var matches []cliclient.ExternalGame
	for i := range games {
		if strings.EqualFold(games[i].Title, ref) {
			matches = append(matches, games[i])
		}
	}
	return matches, nil
}
