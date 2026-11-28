package storefront

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// stubAdapter returns err from GetLibrary.
type stubAdapter struct{ err error }

func (s stubAdapter) GetLibrary(context.Context, int, func([]storefrontadapter.ExternalGameEntry) error) error {
	return s.err
}

// A credentials failure from a bound adapter names the sign-in the sync
// loaded; every other result passes through untouched.
func TestBind(t *testing.T) {
	loaded := "enc:v1:loaded"
	bind := func(err error) storefrontadapter.Adapter {
		return newStoredCredentialsWriter(nil, "u1", "steam", loaded).bind(stubAdapter{err: err})
	}
	err := bind(storefrontadapter.ErrCredentials).GetLibrary(context.Background(), 10, nil)
	var bound *storefrontadapter.CredentialsErrorFor
	if !errors.Is(err, storefrontadapter.ErrCredentials) || !errors.As(err, &bound) {
		t.Fatalf("credentials failure: err = %v, want a CredentialsErrorFor wrapping ErrCredentials", err)
	}
	if bound.Credentials == nil || *bound.Credentials != loaded {
		t.Errorf("credentials failure bound to %v, want %q", bound.Credentials, loaded)
	}

	other := errors.New("network down")
	if err := bind(other).GetLibrary(context.Background(), 10, nil); err != other {
		t.Errorf("other failure: err = %v, want it returned unchanged", err)
	}
	if err := bind(nil).GetLibrary(context.Background(), 10, nil); err != nil {
		t.Errorf("success: err = %v, want nil", err)
	}
}

// The factory initially binds every store to the stored sign-in it decrypted.
func TestAdapterFactory_BindsStoresToLoadedSignIn(t *testing.T) {
	enc := testEncrypter(t)
	factory := NewConnections(nil, enc).AdapterFactory(Clients{})
	for _, tc := range []struct {
		storefront string
		creds      string
	}{
		{"steam", `{"web_api_key":"k","steam_id":"1"}`},
		{"playstation-store", `{"npsso_token":"t"}`},
		{"gog", `{"refresh_token":"r","username":"u"}`},
		{"humble-bundle", `{"session_cookie":"c"}`},
		{"epic-games-store", `{"k":"v"}`},
		{"amazon-games", amazonOldUserData},
	} {
		t.Run(tc.storefront, func(t *testing.T) {
			ct, err := enc.Encrypt([]byte(tc.creds))
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := factory(context.Background(), tc.storefront, models.UserSyncConfig{UserID: "u1", StorefrontCredentials: &ct})
			if err != nil {
				t.Fatalf("factory: %v", err)
			}
			got, isBound := adapter.(credentialsBoundAdapter)
			if !isBound {
				t.Fatal("factory returned an unbound adapter")
			}
			if got.w.ciphertext() != ct {
				t.Error("adapter is not bound to the loaded ciphertext")
			}
		})
	}
}

// A missing, undecryptable or undecodable sign-in is a credentials failure
// for every storefront; an unknown storefront is not.
func TestAdapterFactory_UnusableSignIn(t *testing.T) {
	enc := testEncrypter(t)
	factory := NewConnections(nil, enc).AdapterFactory(Clients{})
	notJSON, err := enc.Encrypt([]byte("not json"))
	if err != nil {
		t.Fatal(err)
	}
	garbage := "enc:v1:not-a-ciphertext"
	for _, d := range definitions {
		for name, creds := range map[string]*string{"none": nil, "undecryptable": &garbage, "undecodable": &notJSON} {
			t.Run(d.slug+"/"+name, func(t *testing.T) {
				_, err := factory(context.Background(), d.slug, models.UserSyncConfig{UserID: "u1", StorefrontCredentials: creds})
				if !errors.Is(err, storefrontadapter.ErrCredentials) {
					t.Errorf("err = %v, want ErrCredentials", err)
				}
			})
		}
	}
	if _, err := factory(context.Background(), "nope", models.UserSyncConfig{StorefrontCredentials: &notJSON}); err == nil || errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("unknown storefront: err = %v, want a non-credentials error", err)
	}
}

// A sync's refreshed sign-in replaces the stored one while it is still the one
// the sync read or last wrote, so a sync that refreshes twice keeps both.
func TestStoredCredentialsWriter_SavesWhileStillStored(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	seedStoredSignIn(t, userID, "gog", "loaded-ct")
	w := newStoredCredentialsWriter(NewConnections(testDB, testEncrypter(t)), userID, "gog", "loaded-ct")
	ctx := context.Background()
	if err := w.write(ctx, "refreshed-1"); err != nil {
		t.Fatal(err)
	}
	if got := storedSignIn(t, userID, "gog"); got != "refreshed-1" {
		t.Fatalf("after one refresh stored %q, want refreshed-1", got)
	}
	if err := w.write(ctx, "refreshed-2"); err != nil {
		t.Fatal(err)
	}
	if got := storedSignIn(t, userID, "gog"); got != "refreshed-2" {
		t.Errorf("after a second refresh stored %q, want refreshed-2", got)
	}
}

// A reconnect committed while the sync ran is kept: the older sign-in's
// refreshed value is dropped without an error that would fail the sync.
func TestStoredCredentialsWriter_KeepsReconnect(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	seedStoredSignIn(t, userID, "epic-games-store", "loaded-ct")
	w := newStoredCredentialsWriter(NewConnections(testDB, testEncrypter(t)), userID, "epic-games-store", "loaded-ct")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_sync_configs SET storefront_credentials = 'reconnected-ct' WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if err := w.write(context.Background(), "refreshed-old"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := storedSignIn(t, userID, "epic-games-store"); got != "reconnected-ct" {
		t.Errorf("stored %q, want the reconnected sign-in kept", got)
	}
}

// A disconnect committed while the sync ran stays a disconnect: the sync's
// refreshed sign-in must not bring the credentials back (#1335).
func TestStoredCredentialsWriter_KeepsDisconnect(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	seedStoredSignIn(t, userID, "epic-games-store", "loaded-ct")
	w := newStoredCredentialsWriter(NewConnections(testDB, testEncrypter(t)), userID, "epic-games-store", "loaded-ct")
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE user_sync_configs SET storefront_credentials = NULL WHERE user_id = ?`, userID); err != nil {
		t.Fatal(err)
	}
	if err := w.write(context.Background(), "refreshed-old"); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stored *string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ?`, userID).
		Scan(context.Background(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("stored %q after disconnect, want NULL", *stored)
	}
}
