package storefront

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storelink"
)

func TestStoreURL(t *testing.T) {
	cases := []struct {
		name       string
		storefront string
		link       string
		wantURL    string
		wantOK     bool
	}{
		{"steam", "steam", "440", "https://store.steampowered.com/app/440/", true},
		{"gog", "gog", "the-witcher-3-wild-hunt", "https://www.gog.com/game/the-witcher-3-wild-hunt", true},
		{"epic", "epic-games-store", "fortnite", "https://store.epicgames.com/en-US/p/fortnite", true},
		{"psn", "playstation-store", "10002694", "https://store.playstation.com/en-us/concept/10002694", true},
		{"humble null", "humble-bundle", "", "", false},
		{"empty link", "steam", "", "", false},
		{"unknown storefront", "itch", "abc", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotOK := StoreURL(c.storefront, c.link)
			if gotURL != c.wantURL || gotOK != c.wantOK {
				t.Fatalf("StoreURL(%q,%q) = (%q,%v), want (%q,%v)",
					c.storefront, c.link, gotURL, gotOK, c.wantURL, c.wantOK)
			}
		})
	}
}

// The storefronts with store links, which the enrichment worker selects on.
func TestStoreLinkStorefronts(t *testing.T) {
	want := []string{"steam", "playstation-store", "epic-games-store", "gog"}
	if got := StoreLinkStorefronts(); !slices.Equal(got, want) {
		t.Errorf("StoreLinkStorefronts() = %v, want %v", got, want)
	}
}

type fakePSNConceptClient struct{ npsso string }

func (f *fakePSNConceptClient) Authenticate(_ context.Context, npsso string) (string, error) {
	f.npsso = npsso
	return "access", nil
}

func (f *fakePSNConceptClient) ResolveConceptID(_ context.Context, _, titleID string) (string, error) {
	return "concept-" + titleID, nil
}

func TestStoreLinkResolverFactory(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	t.Run("credential-free storefronts resolve without a connection", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		resolverFor := NewConnections(testDB, testEncrypter(t), Clients{}).StoreLinkResolverFactory()
		for _, sf := range []string{"steam", "gog", "epic-games-store"} {
			if r, err := resolverFor(ctx, sf, userID); err != nil || r == nil {
				t.Errorf("%s: resolver = %v, %v; want a resolver", sf, r, err)
			}
		}
	})

	t.Run("storefronts without store links", func(t *testing.T) {
		resolverFor := NewConnections(testDB, testEncrypter(t), Clients{}).StoreLinkResolverFactory()
		for _, sf := range []string{"humble-bundle", "amazon-games", "itch"} {
			if _, err := resolverFor(ctx, sf, "u"); err == nil {
				t.Errorf("%s: want an error", sf)
			}
		}
	})

	t.Run("playstation store", func(t *testing.T) {
		enc := testEncrypter(t)
		notJSON, err := enc.Encrypt([]byte("not json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name    string
			arrange func(t *testing.T, conns *Connections, userID string)
			want    error
		}{
			{"no connection", func(*testing.T, *Connections, string) {}, ErrPlayStationStoreNotConnected},
			{"disconnected", func(t *testing.T, _ *Connections, userID string) {
				if _, err := testDB.ExecContext(ctx,
					`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (gen_random_uuid(), ?, 'playstation-store', 'daily')`, userID); err != nil {
					t.Fatal(err)
				}
			}, ErrPlayStationStoreNotConnected},
			{"does not decrypt", func(t *testing.T, _ *Connections, userID string) {
				seedStoredSignIn(t, userID, "playstation-store", "not-ciphertext")
			}, ErrPlayStationStoreSignInUnreadable},
			{"does not decode", func(t *testing.T, _ *Connections, userID string) {
				seedStoredSignIn(t, userID, "playstation-store", notJSON)
			}, ErrPlayStationStoreSignInUnreadable},
		} {
			t.Run(tc.name, func(t *testing.T) {
				truncateAllTables(t)
				userID := seedUser(t)
				conns := NewConnections(testDB, enc, Clients{})
				tc.arrange(t, conns, userID)
				if _, err := conns.StoreLinkResolverFactory()(ctx, "playstation-store", userID); !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
			})
		}

		t.Run("signs in with the stored token", func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			fake := &fakePSNConceptClient{}
			prev := newPSNConceptClient
			newPSNConceptClient = func() storelink.PSNConceptClient { return fake }
			t.Cleanup(func() { newPSNConceptClient = prev })
			conns := NewConnections(testDB, enc, Clients{})
			if err := conns.Store(ctx, userID, "playstation-store", PlayStationStoreCredentials{NPSSOToken: "npsso-token", OnlineID: "ana"}); err != nil {
				t.Fatal(err)
			}
			r, err := conns.StoreLinkResolverFactory()(ctx, "playstation-store", userID)
			if err != nil {
				t.Fatal(err)
			}
			link, err := r.Resolve(ctx, "CUSA0001", nil)
			if err != nil {
				t.Fatal(err)
			}
			if link != "concept-CUSA0001" || fake.npsso != "npsso-token" {
				t.Errorf("resolved %q signing in with %q; want concept-CUSA0001 with npsso-token", link, fake.npsso)
			}
		})
	})
}
