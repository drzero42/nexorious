package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
)

type fakeSteamClient struct {
	summary         *steamsvc.SteamPlayerSummary
	err             error
	calls           int
	apiKey, steamID string
}

func (f *fakeSteamClient) GetPlayerSummaries(_ context.Context, apiKey, steamID string) (*steamsvc.SteamPlayerSummary, error) {
	f.calls++
	f.apiKey, f.steamID = apiKey, steamID
	return f.summary, f.err
}

const (
	validSteamID  = "76561198012345678"
	validSteamKey = "AABBCCDD00112233445566778899AABB"
)

func TestConnectSteamRejects(t *testing.T) {
	public := &steamsvc.SteamPlayerSummary{PersonaName: "Ana", CommunityVisibilityState: 3}
	for _, tc := range []struct {
		name      string
		steamID   string
		key       string
		client    fakeSteamClient
		want      error
		wantCalls int
	}{
		{name: "steam id is not a SteamID64", steamID: "12345", key: validSteamKey, client: fakeSteamClient{summary: public}, want: ErrInvalidSteamID},
		{name: "key is not 32 hex digits", steamID: validSteamID, key: "tooshort", client: fakeSteamClient{summary: public}, want: ErrInvalidSteamAPIKey},
		{name: "no such player", steamID: validSteamID, key: validSteamKey, client: fakeSteamClient{}, want: ErrInvalidSteamID, wantCalls: 1},
		{name: "profile is private", steamID: validSteamID, key: validSteamKey, client: fakeSteamClient{summary: &steamsvc.SteamPlayerSummary{PersonaName: "Ana", CommunityVisibilityState: 1}}, want: ErrSteamPrivateProfile, wantCalls: 1},
		{name: "rate limited", steamID: validSteamID, key: validSteamKey, client: fakeSteamClient{err: fmt.Errorf("get summaries: %w", steamsvc.ErrRateLimited)}, want: ErrSteamRateLimited, wantCalls: 1},
		{name: "steam unreachable", steamID: validSteamID, key: validSteamKey, client: fakeSteamClient{err: errors.New(`Get "https://api.steampowered.com/?key=` + validSteamKey + `": dial tcp: timeout`)}, want: ErrSteamUnavailable, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			client := tc.client
			conns := NewConnections(testDB, testEncrypter(t), Clients{Steam: &client})

			_, err := conns.ConnectSteam(context.Background(), userID, tc.steamID, tc.key)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), tc.key) {
				t.Errorf("err %q carries the Web API key", err)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("Steam called %d times, want %d", client.calls, tc.wantCalls)
			}
			if st, err := conns.Status(context.Background(), userID, "steam"); err != nil || st.Connected {
				t.Errorf("status = %+v, %v; want nothing stored", st, err)
			}
		})
	}
}

func TestConnectSteamStoresTheSignIn(t *testing.T) {
	truncateAllTables(t)
	userID := seedUser(t)
	client := &fakeSteamClient{summary: &steamsvc.SteamPlayerSummary{PersonaName: "Ana", CommunityVisibilityState: 3}}
	conns := NewConnections(testDB, testEncrypter(t), Clients{Steam: client})
	ctx := context.Background()

	name, err := conns.ConnectSteam(ctx, userID, validSteamID, validSteamKey)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if name != "Ana" {
		t.Errorf("name = %q, want Ana", name)
	}
	if client.apiKey != validSteamKey || client.steamID != validSteamID {
		t.Errorf("Steam asked with key %q id %q", client.apiKey, client.steamID)
	}
	loaded, err := conns.Load(ctx, userID, "steam")
	if err != nil {
		t.Fatal(err)
	}
	var got SteamCredentials
	if err := json.Unmarshal(loaded.Plaintext, &got); err != nil {
		t.Fatal(err)
	}
	if want := (SteamCredentials{WebAPIKey: validSteamKey, SteamID: validSteamID, DisplayName: "Ana"}); got != want {
		t.Errorf("stored %+v, want %+v", got, want)
	}
}
