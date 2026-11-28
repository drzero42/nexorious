package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Each storefront's status names the account from its stored sign-in.
func TestStatusAccount(t *testing.T) {
	for _, tc := range []struct {
		name       string
		storefront string
		stored     any
		want       string
	}{
		{"steam", "steam", SteamCredentials{WebAPIKey: "k", SteamID: "s", DisplayName: "Ana"}, "Ana"},
		{"playstation store", "playstation-store", PlayStationStoreCredentials{NPSSOToken: "t", OnlineID: "ana_psn"}, "ana_psn"},
		{"gog", "gog", GOGCredentials{RefreshToken: "r", Username: "ana"}, "ana"},
		{"humble bundle stores no name", "humble-bundle", HumbleBundleCredentials{SessionCookie: "c"}, ""},
		{"epic", "epic-games-store", map[string]string{"user.json": `{"displayName":"Ana"}`}, "Ana"},
		{"epic snapshot without user.json", "epic-games-store", map[string]string{"other.json": "{}"}, ""},
		{"amazon", "amazon-games", json.RawMessage(`{"extensions":{"customer_info":{"given_name":"Ana"}}}`), "Ana"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			conns := NewConnections(testDB, testEncrypter(t), Clients{})
			if err := conns.Store(context.Background(), userID, tc.storefront, tc.stored); err != nil {
				t.Fatal(err)
			}
			st, err := conns.Status(context.Background(), userID, tc.storefront)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if want := (Status{Connected: true, Account: tc.want}); st != want {
				t.Errorf("status = %+v, want %+v", st, want)
			}
		})
	}
}

func TestStatusStates(t *testing.T) {
	ctx := context.Background()

	t.Run("nothing stored", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		st, err := NewConnections(testDB, testEncrypter(t), Clients{}).Status(ctx, userID, "steam")
		if err != nil || st != (Status{}) {
			t.Errorf("status = %+v, %v; want not connected", st, err)
		}
	})

	t.Run("a sign-in that does not decrypt is undecryptable", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		seedStoredSignIn(t, userID, "steam", "enc:v1:not-ciphertext")
		st, err := NewConnections(testDB, testEncrypter(t), Clients{}).Status(ctx, userID, "steam")
		if err != nil {
			t.Fatal(err)
		}
		if want := (Status{Connected: true, Undecryptable: true, CredentialsError: true}); st != want {
			t.Errorf("status = %+v, want %+v", st, want)
		}
	})

	t.Run("a flagged sign-in reports the credentials error", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		conns := NewConnections(testDB, testEncrypter(t), Clients{})
		if err := conns.Store(ctx, userID, "gog", GOGCredentials{Username: "ana"}); err != nil {
			t.Fatal(err)
		}
		if _, err := testDB.ExecContext(ctx, `UPDATE user_sync_configs SET credentials_error = true WHERE user_id = ?`, userID); err != nil {
			t.Fatal(err)
		}
		st, err := conns.Status(ctx, userID, "gog")
		if err != nil {
			t.Fatal(err)
		}
		if want := (Status{Connected: true, CredentialsError: true, Account: "ana"}); st != want {
			t.Errorf("status = %+v, want %+v", st, want)
		}
	})

	t.Run("a sign-in that does not decode is ErrUndecodable", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		enc := testEncrypter(t)
		ct, err := enc.Encrypt([]byte(`"not an object"`))
		if err != nil {
			t.Fatal(err)
		}
		seedStoredSignIn(t, userID, "steam", ct)
		if _, err := NewConnections(testDB, enc, Clients{}).Status(ctx, userID, "steam"); !errors.Is(err, ErrUndecodable) {
			t.Errorf("err = %v, want ErrUndecodable", err)
		}
	})
}

func TestDisconnect(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	conns := NewConnections(testDB, testEncrypter(t), Clients{})
	ctx := context.Background()
	if err := conns.Disconnect(ctx, userID, "steam"); err != nil {
		t.Fatalf("disconnect with nothing stored: %v", err)
	}
	if err := conns.Store(ctx, userID, "steam", SteamCredentials{DisplayName: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if err := conns.Disconnect(ctx, userID, "steam"); err != nil {
		t.Fatal(err)
	}
	if st, err := conns.Status(ctx, userID, "steam"); err != nil || st.Connected {
		t.Errorf("status after disconnect = %+v, %v; want not connected", st, err)
	}
	if err := conns.Disconnect(ctx, userID, "nope"); err == nil {
		t.Error("disconnecting an unknown storefront succeeded")
	}
}
