package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
)

type fakePlayStationStoreClient struct {
	info  *playstationstoresvc.PSNAccountInfo
	err   error
	calls int
}

func (f *fakePlayStationStoreClient) GetAccountInfo(context.Context, string) (*playstationstoresvc.PSNAccountInfo, error) {
	f.calls++
	return f.info, f.err
}

type fakeHumbleBundleClient struct {
	err    error
	calls  int
	cookie string
}

func (f *fakeHumbleBundleClient) Verify(_ context.Context, cookie string) error {
	f.calls++
	f.cookie = cookie
	return f.err
}

// fakeGOG serves GOG's token endpoint: it answers with status, or with tokens
// for username when status is 200, and records the code it was asked for.
func fakeGOG(t *testing.T, status int, username string) (*gogsvc.Client, *string) {
	t.Helper()
	var gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			gotCode = r.URL.Query().Get("code")
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		fmt.Fprintf(w, `{"access_token":"a","refresh_token":"r","expires_in":3600,"user_id":"1","username":%q}`, username)
	}))
	t.Cleanup(srv.Close)
	return gogsvc.NewClientWithURLs(srv.URL, srv.URL), &gotCode
}

// connected reports whether the user has a stored sign-in for storefront.
func connected(t *testing.T, conns *Connections, userID, storefront string) bool {
	t.Helper()
	st, err := conns.Status(context.Background(), userID, storefront)
	if err != nil {
		t.Fatal(err)
	}
	return st.Connected
}

func loadedAs[T any](t *testing.T, conns *Connections, userID, storefront string) T {
	t.Helper()
	loaded, err := conns.Load(context.Background(), userID, storefront)
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(loaded.Plaintext, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

const validNPSSO = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestConnectPlayStationStore(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		token     string
		client    fakePlayStationStoreClient
		want      error
		wantCalls int
	}{
		{name: "token is not 64 characters", token: "short", want: ErrNPSSOTokenLength},
		{name: "psn rejects the token", token: validNPSSO, client: fakePlayStationStoreClient{err: playstationstoresvc.ErrInvalidNPSSOToken}, want: ErrInvalidNPSSOToken, wantCalls: 1},
		{name: "psn fails otherwise", token: validNPSSO, client: fakePlayStationStoreClient{err: errors.New("psn: failed to create client")}, want: ErrInvalidNPSSOToken, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			client := tc.client
			conns := NewConnections(testDB, testEncrypter(t), Clients{PlayStationStore: &client})
			if _, err := conns.ConnectPlayStationStore(ctx, userID, tc.token); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("PSN called %d times, want %d", client.calls, tc.wantCalls)
			}
			if connected(t, conns, userID, "playstation-store") {
				t.Error("a rejected sign-in was stored")
			}
		})
	}

	t.Run("stores the token", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		client := &fakePlayStationStoreClient{info: &playstationstoresvc.PSNAccountInfo{OnlineID: "ana_psn", AccountID: "42"}}
		conns := NewConnections(testDB, testEncrypter(t), Clients{PlayStationStore: client})
		account, err := conns.ConnectPlayStationStore(ctx, userID, validNPSSO)
		if err != nil {
			t.Fatal(err)
		}
		if want := (PlayStationStoreAccount{OnlineID: "ana_psn", AccountID: "42"}); account != want {
			t.Errorf("account = %+v, want %+v", account, want)
		}
		if got, want := loadedAs[PlayStationStoreCredentials](t, conns, userID, "playstation-store"), (PlayStationStoreCredentials{NPSSOToken: validNPSSO, OnlineID: "ana_psn"}); got != want {
			t.Errorf("stored %+v, want %+v", got, want)
		}
	})
}

func TestConnectHumbleBundle(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		cookie    string
		client    fakeHumbleBundleClient
		want      error
		wantCalls int
	}{
		{name: "cookie is blank", cookie: "  \n", want: ErrHumbleCookieMissing},
		{name: "humble rejects the cookie", cookie: "c", client: fakeHumbleBundleClient{err: humblesvc.ErrCredentials}, want: ErrInvalidHumbleCookie, wantCalls: 1},
		{name: "humble unreachable", cookie: "c", client: fakeHumbleBundleClient{err: errors.New("dial tcp: timeout")}, want: ErrHumbleUnavailable, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			client := tc.client
			conns := NewConnections(testDB, testEncrypter(t), Clients{HumbleBundle: &client})
			if err := conns.ConnectHumbleBundle(ctx, userID, tc.cookie); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("Humble Bundle called %d times, want %d", client.calls, tc.wantCalls)
			}
			if connected(t, conns, userID, "humble-bundle") {
				t.Error("a rejected sign-in was stored")
			}
		})
	}

	t.Run("stores the trimmed cookie", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		client := &fakeHumbleBundleClient{}
		conns := NewConnections(testDB, testEncrypter(t), Clients{HumbleBundle: client})
		if err := conns.ConnectHumbleBundle(ctx, userID, "  sess  "); err != nil {
			t.Fatal(err)
		}
		if client.cookie != "sess" {
			t.Errorf("verified %q, want the trimmed cookie", client.cookie)
		}
		if got := loadedAs[HumbleBundleCredentials](t, conns, userID, "humble-bundle"); got.SessionCookie != "sess" {
			t.Errorf("stored %+v, want the trimmed cookie", got)
		}
	})
}

func TestConnectGOG(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		code       string
		status     int
		want       error
		wantReason string // the message, when it is the reason shown to the user
	}{
		{name: "no code", code: "", status: http.StatusOK, want: ErrGOGAuthCodeMissing, wantReason: "auth_code is required"},
		{name: "url is not gog's login redirect", code: "https://example.com/?code=x", status: http.StatusOK, want: ErrInvalidGOGAuthCode, wantReason: "that doesn't look like a GOG login URL"},
		{name: "login redirect without a code", code: "https://embed.gog.com/on_login_success?origin=client", status: http.StatusOK, want: ErrInvalidGOGAuthCode, wantReason: "couldn't find an authorization code"},
		{name: "gog rejects the code", code: "bad", status: http.StatusBadRequest, want: ErrGOGSignInFailed, wantReason: "gog auth failed: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			client, _ := fakeGOG(t, tc.status, "ana")
			conns := NewConnections(testDB, testEncrypter(t), Clients{GOG: client})
			_, err := conns.ConnectGOG(ctx, userID, tc.code)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason) {
				t.Errorf("message %q, want it to start with %q", err, tc.wantReason)
			}
			if connected(t, conns, userID, "gog") {
				t.Error("a rejected sign-in was stored")
			}
		})
	}

	t.Run("exchanges the code from the login redirect and stores the refresh token", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		client, gotCode := fakeGOG(t, http.StatusOK, "ana")
		conns := NewConnections(testDB, testEncrypter(t), Clients{GOG: client})
		name, err := conns.ConnectGOG(ctx, userID, "https://embed.gog.com/on_login_success?origin=client&code=XXX")
		if err != nil {
			t.Fatal(err)
		}
		if name != "ana" || *gotCode != "XXX" {
			t.Errorf("name = %q, exchanged code %q; want ana, XXX", name, *gotCode)
		}
		if got, want := loadedAs[GOGCredentials](t, conns, userID, "gog"), (GOGCredentials{RefreshToken: "r", Username: "ana"}); got != want {
			t.Errorf("stored %+v, want %+v", got, want)
		}
	})
}
