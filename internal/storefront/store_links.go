package storefront

import (
	"context"
	"errors"
	"fmt"

	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
	"github.com/drzero42/nexorious/internal/services/storelink"
)

// A store link is the storefront's own id for a game's product page,
// external_games.store_link. A storefront has store links when its definition
// says how to build the page's URL from one (storeURL) and how to resolve one
// (resolver); Humble Bundle and Amazon Games have neither.

// The ways StoreLinkResolverFactory fails for the PlayStation Store, whose
// resolver signs in with the user's stored token.
var (
	// ErrStoreLinkNotConnected: the user has no stored PlayStation Store
	// sign-in.
	ErrStoreLinkNotConnected = errors.New("playstation store: not connected")
	// ErrStoreLinkSignInUnreadable: the stored sign-in does not decrypt or
	// does not decode.
	ErrStoreLinkSignInUnreadable = errors.New("playstation store: stored sign-in is unreadable")
)

// StoreURL turns a store link into the storefront's product-page URL. It
// returns false for an empty link and for a storefront without store links
// or not in the registry. The URL formats live here so a store changing its
// scheme is a code fix, not a re-sync.
func StoreURL(storefront, storeLink string) (string, bool) {
	def, ok := lookup(storefront)
	if !ok || def.storeURL == nil || storeLink == "" {
		return "", false
	}
	return def.storeURL(storeLink), true
}

// StoreLinkStorefronts returns the slugs of the storefronts whose store links
// can be resolved, in registry order.
func StoreLinkStorefronts() []string {
	var slugs []string
	for _, d := range definitions {
		if d.resolver != nil {
			slugs = append(slugs, d.slug)
		}
	}
	return slugs
}

// StoreLinkResolverFactory returns the store-link enrichment worker's
// resolver factory. Steam, GOG and Epic resolve without a sign-in; the
// PlayStation Store resolver signs in with the user's stored token.
func (c *Connections) StoreLinkResolverFactory() func(ctx context.Context, storefront, userID string) (storelink.Resolver, error) {
	return func(ctx context.Context, storefront, userID string) (storelink.Resolver, error) {
		def, ok := lookup(storefront)
		if !ok || def.resolver == nil {
			return nil, fmt.Errorf("no store links for storefront: %s", storefront)
		}
		return def.resolver(ctx, c, userID)
	}
}

func steamStoreURL(link string) string {
	return fmt.Sprintf("https://store.steampowered.com/app/%s/", link)
}

func playStationStoreURL(link string) string {
	return fmt.Sprintf("https://store.playstation.com/en-us/concept/%s", link)
}

func epicGamesStoreURL(link string) string {
	return fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", link)
}

func gogStoreURL(link string) string {
	return fmt.Sprintf("https://www.gog.com/game/%s", link)
}

func steamResolver(context.Context, *Connections, string) (storelink.Resolver, error) {
	return storelink.NewSteamResolver(), nil
}

func epicGamesStoreResolver(context.Context, *Connections, string) (storelink.Resolver, error) {
	return storelink.NewEpicResolver(nil, ""), nil
}

func gogResolver(context.Context, *Connections, string) (storelink.Resolver, error) {
	return storelink.NewGOGResolver(nil, ""), nil
}

// newPSNConceptClient builds the client a PlayStation Store resolver signs in
// and resolves through; one per resolver, as each item had before. Tests
// replace it.
var newPSNConceptClient = func() storelink.PSNConceptClient { return playstationstoresvc.NewClient() }

func playStationStoreResolver(ctx context.Context, c *Connections, userID string) (storelink.Resolver, error) {
	loaded, err := c.Load(ctx, userID, "playstation-store")
	if err != nil {
		return nil, err
	}
	if !loaded.Connected {
		return nil, ErrStoreLinkNotConnected
	}
	if loaded.Plaintext == nil {
		return nil, ErrStoreLinkSignInUnreadable
	}
	creds, err := decode[PlayStationStoreCredentials](loaded.Plaintext)
	if err != nil {
		return nil, ErrStoreLinkSignInUnreadable
	}
	return storelink.NewPSNResolver(newPSNConceptClient(), creds.NPSSOToken), nil
}
