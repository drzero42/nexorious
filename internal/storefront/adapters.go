package storefront

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
	epicgamesstoresvc "github.com/drzero42/nexorious/internal/services/epicgamesstore"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
	"github.com/drzero42/nexorious/internal/services/storelink"
)

// Clients are the storefront service clients Connections reaches the
// storefronts through. They are passed in so tests can stand fakes in for the
// third parties; Epic's also needs LEGENDARY_WORK_DIR.
type Clients struct {
	Steam            SteamClient
	PlayStationStore PlayStationStoreClient
	Epic             *epicgamesstoresvc.Client
	GOG              *gogsvc.Client
	HumbleBundle     HumbleBundleClient
	Amazon           AmazonGamesClient
}

// AdapterFactory is the sync worker's adapter factory: it builds the
// storefront's library adapter from the stored Connection in cfg.
type AdapterFactory func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (storefrontadapter.Adapter, error)

// definition is one storefront in the registry.
type definition struct {
	slug string
	// adapter builds the library adapter from a decrypted stored sign-in.
	// It returns ErrCredentials when the sign-in does not decode.
	adapter func(s *signIn) (storefrontadapter.Adapter, error)
	// account decodes the account's display name from a decrypted stored
	// sign-in; nil for a storefront that stores none.
	account func(plain []byte) (string, error)
	// storeURL builds the product-page URL from a non-empty store link; nil
	// for a storefront without store links (store_links.go).
	storeURL func(link string) string
	// resolver builds the resolver for a user's store links; nil for a
	// storefront without store links.
	resolver func(ctx context.Context, c *Connections, userID string) (storelink.Resolver, error)
}

// definitions is the registry: one definition per storefront, in the order
// the sync API lists them (GET /sync/config).
var definitions = []definition{
	{slug: "steam", adapter: steamAdapter, account: steamAccount, storeURL: steamStoreURL, resolver: steamResolver},
	{slug: "playstation-store", adapter: playStationStoreAdapter, account: playStationStoreAccount, storeURL: playStationStoreURL, resolver: playStationStoreResolver},
	{slug: "epic-games-store", adapter: epicGamesStoreAdapter, account: epicGamesStoreAccount, storeURL: epicGamesStoreURL, resolver: epicGamesStoreResolver},
	{slug: "gog", adapter: gogAdapter, account: gogAccount, storeURL: gogStoreURL, resolver: gogResolver},
	{slug: "humble-bundle", adapter: humbleBundleAdapter},
	{slug: "amazon-games", adapter: amazonGamesAdapter, account: amazonGamesAccount},
}

func lookup(storefront string) (definition, bool) {
	for _, d := range definitions {
		if d.slug == storefront {
			return d, true
		}
	}
	return definition{}, false
}

// Slugs returns the storefronts' slugs, in the order the sync API lists them.
func Slugs() []string {
	slugs := make([]string, len(definitions))
	for i, d := range definitions {
		slugs[i] = d.slug
	}
	return slugs
}

// Known reports whether slug is a storefront in the registry.
func Known(slug string) bool {
	_, ok := lookup(slug)
	return ok
}

// signIn is a stored Connection as a sync loaded it.
type signIn struct {
	ctx       context.Context
	userID    string
	plaintext []byte
	clients   Clients
	stored    *storedCredentialsWriter
}

// save encrypts a refreshed sign-in and stores it while the sync's sign-in is
// still the stored one. It runs detached from the sync's cancellation: a
// refresh the storefront already accepted must not be lost.
func (s *signIn) save(plain []byte) error {
	return s.stored.save(context.WithoutCancel(s.ctx), plain)
}

// AdapterFactory returns the sync worker's adapter factory.
func (c *Connections) AdapterFactory() AdapterFactory {
	return func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (storefrontadapter.Adapter, error) {
		def, ok := lookup(storefront)
		if !ok {
			return nil, fmt.Errorf("unknown storefront: %s", storefront)
		}
		if cfg.StorefrontCredentials == nil {
			return nil, storefrontadapter.ErrCredentials
		}
		loaded := *cfg.StorefrontCredentials
		plain, err := c.encrypter.Decrypt(loaded)
		if err != nil {
			slog.WarnContext(logging.WithUserID(ctx, cfg.UserID), "adapter factory: decrypt failed", logging.KeyErr, err, logging.KeySource, storefront, logging.Cat(logging.CategoryAuth))
			return nil, storefrontadapter.ErrCredentials
		}
		stored := newStoredCredentialsWriter(c, cfg.UserID, storefront, loaded)
		inner, err := def.adapter(&signIn{ctx: ctx, userID: cfg.UserID, plaintext: plain, clients: c.clients, stored: stored})
		if err != nil {
			return nil, err
		}
		return stored.bind(inner), nil
	}
}

// decode unmarshals a typed stored sign-in; one that does not decode is a
// credentials failure.
func decode[T any](plain []byte) (T, error) {
	var creds T
	if err := json.Unmarshal(plain, &creds); err != nil {
		return creds, storefrontadapter.ErrCredentials
	}
	return creds, nil
}

func steamAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	creds, err := decode[SteamCredentials](s.plaintext)
	if err != nil {
		return nil, err
	}
	return steamsvc.NewAdapter(steamsvc.NewClient(), creds.WebAPIKey, creds.SteamID), nil
}

func playStationStoreAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	creds, err := decode[PlayStationStoreCredentials](s.plaintext)
	if err != nil {
		return nil, err
	}
	return playstationstoresvc.NewAdapter(playstationstoresvc.NewClient(), creds.NPSSOToken), nil
}

func gogAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	creds, err := decode[GOGCredentials](s.plaintext)
	if err != nil {
		return nil, err
	}
	onNewTokens := func(refreshToken string) error {
		creds.RefreshToken = refreshToken
		plain, err := json.Marshal(creds) //nolint:gosec // marshaled only to be encrypted before storage; never logged or returned
		if err != nil {
			return err
		}
		return s.save(plain)
	}
	return gogsvc.NewAdapter(s.clients.GOG, creds.RefreshToken, onNewTokens), nil
}

func humbleBundleAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	creds, err := decode[HumbleBundleCredentials](s.plaintext)
	if err != nil {
		return nil, err
	}
	return humblesvc.NewAdapter(humblesvc.NewClient(), creds.SessionCookie), nil
}

func epicGamesStoreAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	snapshot, err := decode[map[string]string](s.plaintext)
	if err != nil {
		return nil, err
	}
	onSnapshot := func(snapshot map[string]string) error {
		plain, _ := json.Marshal(snapshot) //nolint:errcheck // marshaling a map[string]string cannot fail
		return s.save(plain)
	}
	return epicgamesstoresvc.NewAdapter(s.clients.Epic, s.userID, snapshot, onSnapshot), nil
}

func amazonGamesAdapter(s *signIn) (storefrontadapter.Adapter, error) {
	if !json.Valid(s.plaintext) {
		return nil, storefrontadapter.ErrCredentials
	}
	return amazongamessvc.NewAdapter(s.clients.Amazon, s.plaintext, func(userData json.RawMessage) error {
		return s.save(userData)
	}), nil
}
