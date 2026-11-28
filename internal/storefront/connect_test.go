package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
	epicgamesstoresvc "github.com/drzero42/nexorious/internal/services/epicgamesstore"
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
		{name: "cookie is blank", cookie: "  \n", want: ErrHumbleBundleCookieMissing},
		{name: "humble rejects the cookie", cookie: "c", client: fakeHumbleBundleClient{err: humblesvc.ErrCredentials}, want: ErrInvalidHumbleBundleCookie, wantCalls: 1},
		{name: "humble unreachable", cookie: "c", client: fakeHumbleBundleClient{err: errors.New("dial tcp: timeout")}, want: ErrHumbleBundleUnavailable, wantCalls: 1},
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
		wantReason string // the start of the message
	}{
		{name: "no code", code: "", status: http.StatusOK, want: ErrGOGAuthCodeMissing, wantReason: "gog: auth code is required"},
		{name: "url is not gog's login redirect", code: "https://example.com/?code=x", status: http.StatusOK, want: ErrInvalidGOGAuthCode, wantReason: "that doesn't look like a GOG login URL"},
		{name: "login redirect without a code", code: "https://embed.gog.com/on_login_success?origin=client", status: http.StatusOK, want: ErrInvalidGOGAuthCode, wantReason: "couldn't find an authorization code"},
		{name: "gog rejects the code", code: "bad", status: http.StatusBadRequest, want: ErrGOGSignInFailed, wantReason: "gog: token HTTP 400"},
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

// fakeLegendary puts a stand-in legendary on PATH: `auth --code good` signs
// in as Player, any other code fails.
func fakeLegendary(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake legendary is a shell script")
	}
	bin := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = auth ] && [ "$3" = good ]; then
  mkdir -p "$XDG_CONFIG_HOME/legendary"
  echo '{"displayName":"Player","account_id":"acct"}' > "$XDG_CONFIG_HOME/legendary/user.json"
  exit 0
fi
echo "auth failed" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(bin, "legendary"), []byte(script), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestConnectEpicGamesStore(t *testing.T) {
	ctx := context.Background()
	fakeLegendary(t)

	for _, tc := range []struct {
		name       string
		workDir    string
		code       string
		want       error
		wantReason string // the start of the message
	}{
		{name: "legendary not configured", workDir: "", code: "good", want: ErrEpicGamesStoreNotConfigured, wantReason: "epic games store: legendary is not configured"},
		{name: "no code", workDir: t.TempDir(), code: "", want: ErrEpicGamesStoreAuthCodeMissing, wantReason: "epic games store: auth code is required"},
		{name: "legendary rejects the code", workDir: t.TempDir(), code: "bad", want: ErrEpicGamesStoreSignInFailed, wantReason: "epic: legendary auth --code bad: auth failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			conns := NewConnections(testDB, testEncrypter(t), Clients{Epic: epicgamesstoresvc.NewClient(tc.workDir)})
			_, err := conns.ConnectEpicGamesStore(ctx, userID, tc.code)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason) {
				t.Errorf("message %q, want it to start with %q", err, tc.wantReason)
			}
			if connected(t, conns, userID, "epic-games-store") {
				t.Error("a rejected sign-in was stored")
			}
		})
	}

	t.Run("stores legendary's snapshot", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		conns := NewConnections(testDB, testEncrypter(t), Clients{Epic: epicgamesstoresvc.NewClient(t.TempDir())})
		account, err := conns.ConnectEpicGamesStore(ctx, userID, "good")
		if err != nil {
			t.Fatal(err)
		}
		if want := (EpicGamesStoreAccount{DisplayName: "Player", AccountID: "acct"}); account != want {
			t.Errorf("account = %+v, want %+v", account, want)
		}
		snapshot := loadedAs[map[string]string](t, conns, userID, "epic-games-store")
		if !strings.Contains(snapshot["user.json"], `"displayName":"Player"`) {
			t.Errorf("stored snapshot %v, want legendary's user.json", snapshot)
		}
	})
}

func TestDisconnectEpicGamesStore(t *testing.T) {
	ctx := context.Background()

	t.Run("removes the sign-in and the legacy legendary directory", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		workDir := t.TempDir()
		conns := NewConnections(testDB, testEncrypter(t), Clients{Epic: epicgamesstoresvc.NewClient(workDir)})
		if err := conns.Store(ctx, userID, "epic-games-store", map[string]string{"user.json": "{}"}); err != nil {
			t.Fatal(err)
		}
		legacyDir := filepath.Join(workDir, userID)
		if err := os.MkdirAll(legacyDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := conns.DisconnectEpicGamesStore(ctx, userID); err != nil {
			t.Fatal(err)
		}
		if connected(t, conns, userID, "epic-games-store") {
			t.Error("the sign-in is still stored")
		}
		if _, err := os.Stat(legacyDir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy dir still there (stat err %v)", err)
		}
	})

	t.Run("a cleanup failure is only logged", func(t *testing.T) {
		truncateAllTables(t)
		conns := NewConnections(testDB, testEncrypter(t), Clients{Epic: epicgamesstoresvc.NewClient(t.TempDir())})
		// Cleanup refuses a user id that is not a single safe path element.
		if err := conns.DisconnectEpicGamesStore(ctx, "../escape"); err != nil {
			t.Fatalf("err = %v, want the disconnect to succeed", err)
		}
	})
}

// fakeAmazonGames stands in for nile's sign-in, registration and sign-out.
type fakeAmazonGames struct {
	notInstalled bool
	loginErr     error
	registerErr  error
	noAccount    bool // Register succeeds without an account
	gotCode      string
	gotLogin     amazongamessvc.Login
	signedOut    chan string // receives the user data each Logout was given
}

func (f *fakeAmazonGames) Configured() bool { return !f.notInstalled }

func (f *fakeAmazonGames) Library(context.Context, json.RawMessage) ([]amazongamessvc.Entitlement, json.RawMessage, error) {
	return nil, nil, errors.New("not used by connecting")
}

func (f *fakeAmazonGames) LoginURL(context.Context) (*amazongamessvc.Login, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return &amazongamessvc.Login{URL: "https://amazon.com/ap/signin", ClientID: "client", CodeVerifier: "verifier", Serial: "serial"}, nil
}

func (f *fakeAmazonGames) Register(_ context.Context, code string, login amazongamessvc.Login) (*amazongamessvc.Account, json.RawMessage, error) {
	f.gotCode, f.gotLogin = code, login
	if f.registerErr != nil {
		return nil, nil, f.registerErr
	}
	if f.noAccount {
		return nil, json.RawMessage(amazonOldUserData), nil
	}
	return &amazongamessvc.Account{UserID: "amzn1.account.X", Name: "Ama"}, json.RawMessage(amazonOldUserData), nil
}

func (f *fakeAmazonGames) Logout(_ context.Context, userData json.RawMessage) error {
	if f.signedOut != nil {
		f.signedOut <- string(userData)
	}
	return nil
}

const amazonLanding = "https://www.amazon.com/?openid.oa2.authorization_code=ANcode&openid.mode=id_res"

func TestStartAmazonGamesSignIn(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		client fakeAmazonGames
		want   error
	}{
		{name: "nile not installed", client: fakeAmazonGames{notInstalled: true}, want: ErrAmazonGamesNotInstalled},
		{name: "nile cannot start a sign-in", client: fakeAmazonGames{loginErr: errors.New("nile: exit 1")}, want: ErrAmazonGamesStartFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client
			conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: &client})
			if _, err := conns.StartAmazonGamesSignIn(ctx, "u1"); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("keeps nile's values out of the response", func(t *testing.T) {
		conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: &fakeAmazonGames{}})
		signIn, err := conns.StartAmazonGamesSignIn(ctx, "u1")
		if err != nil {
			t.Fatal(err)
		}
		if signIn.LoginURL != "https://amazon.com/ap/signin" || signIn.AttemptID == "" {
			t.Errorf("sign-in = %+v", signIn)
		}
		for _, secret := range []string{"client", "verifier", "serial"} {
			if strings.Contains(signIn.AttemptID, secret) {
				t.Errorf("attempt id carries %q in the clear", secret)
			}
		}
	})
}

func TestConnectAmazonGames(t *testing.T) {
	ctx := context.Background()
	start := func(t *testing.T, conns *Connections, userID string) string {
		t.Helper()
		signIn, err := conns.StartAmazonGamesSignIn(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		return signIn.AttemptID
	}

	for _, tc := range []struct {
		name       string
		client     fakeAmazonGames
		attempt    func(t *testing.T, conns *Connections, userID string) string
		redirect   string
		later      time.Duration // how long after the start the finish runs
		want       error
		wantReason string // the start of the message
	}{
		{name: "nile not installed", client: fakeAmazonGames{notInstalled: true}, attempt: func(*testing.T, *Connections, string) string { return "x" }, redirect: amazonLanding, want: ErrAmazonGamesNotInstalled},
		{name: "no attempt id", attempt: func(*testing.T, *Connections, string) string { return "" }, redirect: amazonLanding, want: ErrAmazonGamesAttemptIDMissing},
		{name: "landing address without a code", attempt: start, redirect: "https://www.amazon.com/?openid.mode=cancel", want: ErrInvalidAmazonGamesRedirect, wantReason: "that address has no sign-in code"},
		{name: "no landing address", attempt: start, redirect: "", want: ErrInvalidAmazonGamesRedirect, wantReason: "paste the address Amazon took you to"},
		{name: "attempt id is not ours", attempt: func(*testing.T, *Connections, string) string { return "enc:v1:garbage" }, redirect: amazonLanding, want: ErrAmazonGamesAttemptUnusable},
		{name: "a stored sign-in pasted as the attempt id", attempt: func(t *testing.T, _ *Connections, _ string) string {
			ct, err := testEncrypter(t).Encrypt([]byte(amazonOldUserData))
			if err != nil {
				t.Fatal(err)
			}
			return ct
		}, redirect: amazonLanding, want: ErrAmazonGamesAttemptUnusable},
		{name: "attempt id is too long", attempt: func(*testing.T, *Connections, string) string { return strings.Repeat("a", maxAmazonAttemptIDLen+1) }, redirect: amazonLanding, want: ErrAmazonGamesAttemptUnusable},
		{name: "another user's attempt", attempt: func(t *testing.T, conns *Connections, _ string) string { return start(t, conns, "someone-else") }, redirect: amazonLanding, want: ErrAmazonGamesAttemptUnusable},
		{name: "attempt older than fifteen minutes", attempt: start, redirect: amazonLanding, later: 16 * time.Minute, want: ErrAmazonGamesAttemptUnusable},
		{name: "amazon rejects the sign-in", client: fakeAmazonGames{registerErr: errors.New("nile: register failed")}, attempt: start, redirect: amazonLanding, want: ErrAmazonGamesSignInRejected},
		{name: "nile registers without an account", client: fakeAmazonGames{noAccount: true}, attempt: start, redirect: amazonLanding, want: ErrAmazonGamesNoAccount},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			client := tc.client
			conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: &client})
			attempt := tc.attempt(t, conns, userID)
			conns.SetAmazonLoginClockForTest(func() time.Time { return time.Now().Add(tc.later) })
			_, err := conns.ConnectAmazonGames(ctx, userID, attempt, tc.redirect)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), tc.wantReason) {
				t.Errorf("message %q, want it to start with %q", err, tc.wantReason)
			}
			if connected(t, conns, userID, "amazon-games") {
				t.Error("a rejected sign-in was stored")
			}
		})
	}

	t.Run("registers with the started sign-in and stores nile's user data", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		client := &fakeAmazonGames{}
		conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: client})
		attempt := start(t, conns, userID)
		conns.SetAmazonLoginClockForTest(func() time.Time { return time.Now().Add(14 * time.Minute) })
		name, err := conns.ConnectAmazonGames(ctx, userID, attempt, amazonLanding)
		if err != nil {
			t.Fatal(err)
		}
		if name != "Ama" {
			t.Errorf("name = %q, want Ama", name)
		}
		if want := (amazongamessvc.Login{ClientID: "client", CodeVerifier: "verifier", Serial: "serial"}); client.gotCode != "ANcode" || client.gotLogin != want {
			t.Errorf("registered code %q with %+v; want ANcode with %+v", client.gotCode, client.gotLogin, want)
		}
		loaded, err := conns.Load(ctx, userID, "amazon-games")
		if err != nil {
			t.Fatal(err)
		}
		if string(loaded.Plaintext) != amazonOldUserData {
			t.Errorf("stored %s, want nile's user data", loaded.Plaintext)
		}
	})
}

func TestDisconnectAmazonGames(t *testing.T) {
	ctx := context.Background()

	t.Run("signs out the device the removed sign-in belongs to", func(t *testing.T) {
		truncateAllTables(t)
		userID := seedUser(t)
		client := &fakeAmazonGames{signedOut: make(chan string, 1)}
		conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: client})
		if err := conns.Store(ctx, userID, "amazon-games", json.RawMessage(amazonOldUserData)); err != nil {
			t.Fatal(err)
		}
		if err := conns.DisconnectAmazonGames(ctx, userID); err != nil {
			t.Fatal(err)
		}
		conns.WaitForAmazonSignOutsForTest()
		if connected(t, conns, userID, "amazon-games") {
			t.Error("the sign-in is still stored")
		}
		select {
		case got := <-client.signedOut:
			if got != amazonOldUserData {
				t.Errorf("signed out %s, want the removed sign-in", got)
			}
		default:
			t.Error("the device was not signed out")
		}
	})

	for _, tc := range []struct {
		name   string
		client *fakeAmazonGames
		stored bool
	}{
		{name: "nothing stored", client: &fakeAmazonGames{signedOut: make(chan string, 1)}},
		{name: "nile not installed", client: &fakeAmazonGames{notInstalled: true, signedOut: make(chan string, 1)}, stored: true},
	} {
		t.Run(tc.name+": no sign-out", func(t *testing.T) {
			truncateAllTables(t)
			userID := seedUser(t)
			conns := NewConnections(testDB, testEncrypter(t), Clients{Amazon: tc.client})
			if tc.stored {
				if err := conns.Store(ctx, userID, "amazon-games", json.RawMessage(amazonOldUserData)); err != nil {
					t.Fatal(err)
				}
			}
			if err := conns.DisconnectAmazonGames(ctx, userID); err != nil {
				t.Fatal(err)
			}
			conns.WaitForAmazonSignOutsForTest()
			if connected(t, conns, userID, "amazon-games") {
				t.Error("the sign-in is still stored")
			}
			if len(tc.client.signedOut) != 0 {
				t.Error("a device was signed out")
			}
		})
	}
}
