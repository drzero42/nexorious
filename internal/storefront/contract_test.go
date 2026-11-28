package storefront

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// What connect stores, the module loads: for each storefront, a sign-in stored
// through Connections.Store builds a library adapter bound to the stored
// ciphertext, and Load decodes back to the value that was stored.
func TestConnectionContract(t *testing.T) {
	for _, tc := range []struct {
		storefront string
		stored     any // what the connect handler stores
		decoded    any // pointer to a zero value Load's plaintext decodes into
	}{
		{"steam", SteamCredentials{WebAPIKey: "k", SteamID: "76561198000000000", DisplayName: "Ana"}, &SteamCredentials{}},
		{"playstation-store", PlayStationStoreCredentials{NPSSOToken: "t", OnlineID: "ana_psn"}, &PlayStationStoreCredentials{}},
		{"gog", GOGCredentials{RefreshToken: "r", Username: "ana"}, &GOGCredentials{}},
		{"humble-bundle", HumbleBundleCredentials{SessionCookie: "c"}, &HumbleBundleCredentials{}},
		{"epic-games-store", map[string]string{"user.json": `{"displayName":"Ana"}`}, &map[string]string{}},
		{"amazon-games", json.RawMessage(amazonOldUserData), &json.RawMessage{}},
	} {
		t.Run(tc.storefront, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			conns := NewConnections(testDB, testEncrypter(t), Clients{})
			ctx := context.Background()
			if err := conns.Store(ctx, userID, tc.storefront, tc.stored); err != nil {
				t.Fatalf("store: %v", err)
			}

			cfg := syncConfig(t, userID, tc.storefront)
			adapter, err := conns.AdapterFactory()(ctx, tc.storefront, cfg)
			if err != nil {
				t.Fatalf("factory: %v", err)
			}
			if got := adapter.(credentialsBoundAdapter).w.ciphertext(); got != *cfg.StorefrontCredentials {
				t.Error("adapter is not bound to the stored sign-in")
			}

			loaded, err := conns.Load(ctx, userID, tc.storefront)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !loaded.Connected || loaded.CredentialsError {
				t.Fatalf("loaded = %+v, want connected without a credentials error", loaded)
			}
			if err := json.Unmarshal(loaded.Plaintext, tc.decoded); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got := reflect.ValueOf(tc.decoded).Elem().Interface(); !reflect.DeepEqual(got, tc.stored) {
				t.Errorf("loaded %#v, want %#v", got, tc.stored)
			}
		})
	}
}

// Storing a sign-in clears a credentials error a sync flagged on the old one.
func TestStore_ClearsCredentialsError(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	conns := NewConnections(testDB, testEncrypter(t), Clients{})
	ctx := context.Background()
	if err := conns.Store(ctx, userID, "humble-bundle", HumbleBundleCredentials{SessionCookie: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE user_sync_configs SET credentials_error = true WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if err := conns.Store(ctx, userID, "humble-bundle", HumbleBundleCredentials{SessionCookie: "new"}); err != nil {
		t.Fatal(err)
	}
	if loaded, err := conns.Load(ctx, userID, "humble-bundle"); err != nil || loaded.CredentialsError {
		t.Errorf("loaded = %+v, err = %v; want the credentials error cleared", loaded, err)
	}
}

// take returns the sign-in it removed, once.
func TestTake(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	conns := NewConnections(testDB, testEncrypter(t), Clients{})
	ctx := context.Background()
	if removed, err := conns.take(ctx, userID, "amazon-games"); err != nil || removed != nil {
		t.Fatalf("take with nothing stored = %v, %v; want nil, nil", removed, err)
	}
	if err := conns.Store(ctx, userID, "amazon-games", json.RawMessage(amazonOldUserData)); err != nil {
		t.Fatal(err)
	}
	stored := storedSignIn(t, userID, "amazon-games")
	removed, err := conns.take(ctx, userID, "amazon-games")
	if err != nil || removed == nil || *removed != stored {
		t.Fatalf("take = %v, %v; want the stored sign-in", removed, err)
	}
	if loaded, err := conns.Load(ctx, userID, "amazon-games"); err != nil || loaded.Connected {
		t.Errorf("after take loaded = %+v, err = %v; want not connected", loaded, err)
	}
	if removed, err := conns.take(ctx, userID, "amazon-games"); err != nil || removed != nil {
		t.Errorf("second take = %v, %v; want nil, nil", removed, err)
	}
}

// A GOG sync saves the refresh token GOG returns and keeps every other stored
// field, so a field connect stores is not dropped by the first refresh.
func TestGOGRefreshKeepsStoredFields(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	ctx := context.Background()

	var usedToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			usedToken = r.URL.Query().Get("refresh_token")
			fmt.Fprint(w, `{"access_token":"a","refresh_token":"new","expires_in":3600,"user_id":"1"}`)
		case "/account/getFilteredProducts":
			fmt.Fprint(w, `{"totalProducts":0,"numPages":1,"page":1,"products":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	conns := NewConnections(testDB, testEncrypter(t), Clients{GOG: gogsvc.NewClientWithURLs(srv.URL, srv.URL)})
	if err := conns.Store(ctx, userID, "gog", GOGCredentials{RefreshToken: "old", Username: "ana"}); err != nil {
		t.Fatal(err)
	}
	adapter, err := conns.AdapterFactory()(ctx, "gog", syncConfig(t, userID, "gog"))
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if err := adapter.GetLibrary(ctx, 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
	if usedToken != "old" {
		t.Errorf("refreshed with %q, want the stored refresh token", usedToken)
	}

	loaded, err := conns.Load(ctx, userID, "gog")
	if err != nil {
		t.Fatal(err)
	}
	var got GOGCredentials
	if err := json.Unmarshal(loaded.Plaintext, &got); err != nil {
		t.Fatal(err)
	}
	if want := (GOGCredentials{RefreshToken: "new", Username: "ana"}); got != want {
		t.Errorf("stored %+v after the refresh, want %+v", got, want)
	}
}
