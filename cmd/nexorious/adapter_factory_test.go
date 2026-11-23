package main

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// stubAdapter returns err from GetLibrary.
type stubAdapter struct{ err error }

func (s stubAdapter) GetLibrary(context.Context, int, func([]tasks.ExternalGameEntry) error) error {
	return s.err
}

// A credentials failure from a bound adapter names the sign-in the sync
// loaded; every other result passes through untouched.
func TestBoundToCredentials(t *testing.T) {
	loaded := "enc:v1:loaded"
	err := boundToCredentials(stubAdapter{err: tasks.ErrCredentials}, loaded).GetLibrary(context.Background(), 10, nil)
	var bound *tasks.CredentialsErrorFor
	if !errors.Is(err, tasks.ErrCredentials) || !errors.As(err, &bound) {
		t.Fatalf("credentials failure: err = %v, want a CredentialsErrorFor wrapping ErrCredentials", err)
	}
	if bound.Credentials == nil || *bound.Credentials != loaded {
		t.Errorf("credentials failure bound to %v, want %q", bound.Credentials, loaded)
	}

	other := errors.New("network down")
	err = boundToCredentials(stubAdapter{err: other}, loaded).GetLibrary(context.Background(), 10, nil)
	if err != other {
		t.Errorf("other failure: err = %v, want it returned unchanged", err)
	}
	if err := boundToCredentials(stubAdapter{}, loaded).GetLibrary(context.Background(), 10, nil); err != nil {
		t.Errorf("success: err = %v, want nil", err)
	}
}

// The factory initially binds every store to the stored sign-in it decrypted.
func TestAdapterFactory_BindsStoresToLoadedSignIn(t *testing.T) {
	enc, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	factory := buildAdapterFactory(nil, enc, nil)
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
			if got.ciphertext() != ct {
				t.Error("adapter is not bound to the loaded ciphertext")
			}
		})
	}
}
