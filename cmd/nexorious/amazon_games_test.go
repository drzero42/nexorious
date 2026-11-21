package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
	epicgamesstoresvc "github.com/drzero42/nexorious/internal/services/epicgamesstore"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// fakeAmazonLibrary stands in for nile; during runs while the sync is in
// flight, to simulate the user reconnecting.
type fakeAmazonLibrary struct {
	refreshed json.RawMessage
	err       error
	during    func()
}

func (f *fakeAmazonLibrary) Configured() bool { return true }

func (f *fakeAmazonLibrary) Library(context.Context, json.RawMessage) ([]amazongamessvc.Entitlement, json.RawMessage, error) {
	if f.during != nil {
		f.during()
	}
	return []amazongamessvc.Entitlement{{ProductID: "p1", Title: "Game"}}, f.refreshed, f.err
}

const (
	amazonOldUserData       = `{"extensions":{"customer_info":{"user_id":"amzn1.account.X","given_name":"Ama"}},"tokens":{"bearer":{"access_token":"old"}}}`
	amazonRefreshedUserData = `{"extensions":{"customer_info":{"user_id":"amzn1.account.X","given_name":"Ama"}},"tokens":{"bearer":{"access_token":"refreshed"}}}`
	amazonReconnectUserData = `{"extensions":{"customer_info":{"user_id":"amzn1.account.Y","given_name":"New"}},"tokens":{"bearer":{"access_token":"reconnected"}}}`
)

func amazonTestEncrypter(t *testing.T) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

// seedAmazonSync stores userData as the user's Amazon connection and returns
// the row as the sync worker loads it.
func seedAmazonSync(t *testing.T, enc *crypto.Encrypter, userData string) models.UserSyncConfig {
	t.Helper()
	truncateAllTables(t)
	userID := seedResetUser(t, "amazon-factory", "pw", false)
	ct, err := enc.Encrypt([]byte(userData))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES ('cfg-amz', ?, 'amazon-games', 'manual', ?, now(), now())`, userID, ct,
	).Exec(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var cfg models.UserSyncConfig
	if err := testDB.NewSelect().Model(&cfg).Where("id = 'cfg-amz'").Scan(ctx); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func storedAmazonUserData(t *testing.T, enc *crypto.Encrypter, userID string) string {
	t.Helper()
	var ct string
	if err := testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'amazon-games'`, userID).
		Scan(context.Background(), &ct); err != nil {
		t.Fatal(err)
	}
	plain, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	return string(plain)
}

func runAmazonFactory(t *testing.T, enc *crypto.Encrypter, cfg models.UserSyncConfig, lib amazongamessvc.LibraryClient) error {
	t.Helper()
	factory := buildAdapterFactory(testDB, enc, epicgamesstoresvc.NewClient(""), lib)
	adapter, err := factory(context.Background(), "amazon-games", cfg)
	if err != nil {
		return err
	}
	return adapter.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
}

func TestAdapterFactory_AmazonGames_RefreshPersistedAfterFailedImport(t *testing.T) {
	enc := amazonTestEncrypter(t)
	cfg := seedAmazonSync(t, enc, amazonOldUserData)
	lib := &fakeAmazonLibrary{refreshed: json.RawMessage(amazonRefreshedUserData), err: errors.New("amazon: nile library sync failed")}
	if err := runAmazonFactory(t, enc, cfg, lib); err == nil {
		t.Fatal("expected the sync to fail")
	}
	if got := storedAmazonUserData(t, enc, cfg.UserID); got != amazonRefreshedUserData {
		t.Errorf("stored %s, want the refreshed user data", got)
	}
}

func TestAdapterFactory_AmazonGames_ReconnectDuringSyncWins(t *testing.T) {
	enc := amazonTestEncrypter(t)
	cfg := seedAmazonSync(t, enc, amazonOldUserData)
	lib := &fakeAmazonLibrary{refreshed: json.RawMessage(amazonRefreshedUserData)}
	lib.during = func() {
		ct, err := enc.Encrypt([]byte(amazonReconnectUserData))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testDB.NewRaw(`UPDATE user_sync_configs SET storefront_credentials = ? WHERE id = 'cfg-amz'`, ct).
			Exec(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if err := runAmazonFactory(t, enc, cfg, lib); err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
	if got := storedAmazonUserData(t, enc, cfg.UserID); got != amazonReconnectUserData {
		t.Errorf("stored %s, want the reconnect's user data", got)
	}
}

func TestAdapterFactory_AmazonGames_UnusableCredentials(t *testing.T) {
	enc := amazonTestEncrypter(t)
	cfg := seedAmazonSync(t, enc, amazonOldUserData)

	noCreds := cfg
	noCreds.StorefrontCredentials = nil
	if err := runAmazonFactory(t, enc, noCreds, &fakeAmazonLibrary{}); !errors.Is(err, tasks.ErrCredentials) {
		t.Errorf("no credentials: err = %v, want ErrCredentials", err)
	}

	garbage := "enc:v1:not-a-ciphertext"
	bad := cfg
	bad.StorefrontCredentials = &garbage
	if err := runAmazonFactory(t, enc, bad, &fakeAmazonLibrary{}); !errors.Is(err, tasks.ErrCredentials) {
		t.Errorf("undecryptable credentials: err = %v, want ErrCredentials", err)
	}
}

// A refused sign-in is a credentials error bound to the stored sign-in the
// sync loaded, so the worker can leave a reconnected account unflagged.
func TestAdapterFactory_AmazonGames_RefusalIsBoundToLoadedSignIn(t *testing.T) {
	enc := amazonTestEncrypter(t)
	cfg := seedAmazonSync(t, enc, amazonOldUserData)
	err := runAmazonFactory(t, enc, cfg, &fakeAmazonLibrary{err: amazongamessvc.ErrAuthFailed})
	if !errors.Is(err, tasks.ErrCredentials) {
		t.Fatalf("err = %v, want ErrCredentials", err)
	}
	var bound *tasks.CredentialsErrorFor
	if !errors.As(err, &bound) || bound.Credentials == nil || *bound.Credentials != *cfg.StorefrontCredentials {
		t.Errorf("err = %v, want it bound to the loaded ciphertext", err)
	}
}
