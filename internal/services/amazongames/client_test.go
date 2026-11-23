package amazongames

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testdata/nile/ was written by nile v1.2.0 itself (its config migration
// encrypting testdata/user-data.json, with NILE_CONFIG_PATH=testdata), so
// decrypting it here proves the format matches nile's, not just this
// package's own round trip.

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	return reflect.DeepEqual(va, vb)
}

func TestReadNileConfig_DecryptsFileWrittenByNile(t *testing.T) {
	got, err := readNileConfig("testdata")
	if err != nil {
		t.Fatalf("readNileConfig: %v", err)
	}
	if !sameJSON(t, got, readFixture(t, "user-data.json")) {
		t.Errorf("decrypted user data differs from the fixture's plaintext:\n%s", got)
	}
}

func TestWriteNileConfig_RoundTripsInNileLayout(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	dir := t.TempDir()
	if err := writeNileConfig(dir, userData); err != nil {
		t.Fatalf("writeNileConfig: %v", err)
	}
	// nile names the file md5(user_id).enc — the same name nile itself wrote.
	if _, err := os.Stat(filepath.Join(dir, "nile", "095bcd5f2ec20d88c189d52912352950.enc")); err != nil {
		t.Errorf("user data file not at nile's name: %v", err)
	}
	var cu map[string]string
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "nile", "current_user.json")), &cu); err != nil {
		t.Fatal(err)
	}
	if cu["user_id"] != "amzn1.account.FIXTUREUSERID" || cu["name"] != "Test" {
		t.Errorf("current_user.json = %v", cu)
	}
	got, err := readNileConfig(dir)
	if err != nil {
		t.Fatalf("readNileConfig: %v", err)
	}
	if !sameJSON(t, got, userData) {
		t.Errorf("round trip changed the user data:\n%s", got)
	}
	if strings.Contains(string(readFile(t, filepath.Join(dir, "nile", "095bcd5f2ec20d88c189d52912352950.enc"))), "FAKE-REFRESH-TOKEN") {
		t.Error("user data written in the clear")
	}
}

func TestDecryptUserData_WrongKeyOrCorrupt(t *testing.T) {
	enc := readFixture(t, "nile/095bcd5f2ec20d88c189d52912352950.enc")
	if _, err := decryptUserData("amzn1.account.SOMEONEELSE", enc); err == nil {
		t.Error("expected an error decrypting under another user's key")
	}
	if _, err := decryptUserData("amzn1.account.FIXTUREUSERID", enc[:20]); err == nil {
		t.Error("expected an error for a truncated file")
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeNile records each call and lets a test act as nile inside the config dir.
type fakeNile struct {
	args  []string
	dir   string
	do    func(dir string) (stdout, stderr string, err error)
	doCtx func(ctx context.Context, dir string) (stdout, stderr string, err error) // when set, used instead of do
}

func (f *fakeNile) run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	f.args = args
	f.dir = dir
	if f.doCtx != nil {
		stdout, stderr, err := f.doCtx(ctx, dir)
		return []byte(stdout), []byte(stderr), err
	}
	if f.do == nil {
		return nil, nil, nil
	}
	stdout, stderr, err := f.do(dir)
	return []byte(stdout), []byte(stderr), err
}

func newTestClient(f *fakeNile) *Client {
	return &Client{run: f.run, lookPath: func(string) (string, error) { return "/usr/bin/nile", nil }}
}

var errExit1 = errors.New("exit status 1")

func TestLoginURL_ParsesNileOutput(t *testing.T) {
	f := &fakeNile{do: func(string) (string, string, error) {
		return `{"client_id": "abc123", "code_verifier": "verif", "serial": "SERIAL1", "url": "https://amazon.com/ap/signin?x=1"}` + "\n", "", nil
	}}
	login, err := newTestClient(f).LoginURL(context.Background())
	if err != nil {
		t.Fatalf("LoginURL: %v", err)
	}
	want := Login{URL: "https://amazon.com/ap/signin?x=1", ClientID: "abc123", CodeVerifier: "verif", Serial: "SERIAL1"}
	if *login != want {
		t.Errorf("login = %+v, want %+v", *login, want)
	}
	if strings.Join(f.args, " ") != "auth --login --non-interactive" {
		t.Errorf("args = %v", f.args)
	}
	if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
		t.Errorf("config dir %s left behind", f.dir)
	}
}

func TestLoginURL_Errors(t *testing.T) {
	cases := map[string]func(string) (string, string, error){
		"non-zero exit": func(string) (string, string, error) { return "", "boom", errExit1 },
		"unparseable":   func(string) (string, string, error) { return "Login URL: https://…", "", nil },
		"incomplete":    func(string) (string, string, error) { return `{"url": "https://amazon.com"}`, "", nil },
		// nile prints nothing and exits 0 when the dir already holds a sign-in.
		"empty": func(string) (string, string, error) { return "", "ERROR [CLI]: You are already logged in", nil },
	}
	for name, do := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := newTestClient(&fakeNile{do: do}).LoginURL(context.Background()); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestRegister_ReturnsAccountAndUserData(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(dir string) (string, string, error) {
		if err := writeNileConfig(dir, userData); err != nil {
			t.Fatal(err)
		}
		return "", "INFO [AUTH_MANAGER]: Succesfully registered a device", nil
	}}
	login := Login{ClientID: "cid", CodeVerifier: "ver", Serial: "SER"}
	account, got, err := newTestClient(f).Register(context.Background(), "CODE", login)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if account.UserID != "amzn1.account.FIXTUREUSERID" || account.Name != "Test" {
		t.Errorf("account = %+v", account)
	}
	if !sameJSON(t, got, userData) {
		t.Errorf("user data = %s", got)
	}
	wantArgs := "register --code CODE --client-id cid --code-verifier ver --serial SER"
	if strings.Join(f.args, " ") != wantArgs {
		t.Errorf("args = %q, want %q", strings.Join(f.args, " "), wantArgs)
	}
	if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
		t.Errorf("config dir %s left behind", f.dir)
	}
}

// nile syncs the library right after registering; that sync failing must not
// throw away a registration that succeeded.
func TestRegister_SucceedsWhenTrailingSyncFails(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(dir string) (string, string, error) {
		if err := writeNileConfig(dir, userData); err != nil {
			t.Fatal(err)
		}
		return "", "Traceback …\nKeyError: 'entitlements'", errExit1
	}}
	if _, _, err := newTestClient(f).Register(context.Background(), "CODE", Login{ClientID: "c", CodeVerifier: "v", Serial: "s"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegister_FailureIsSanitized(t *testing.T) {
	login := Login{ClientID: "434c49454e544944234132554d56484f58375550345637", CodeVerifier: "VERIFIER-abc_def", Serial: "SERIAL-XYZ"}
	code := "ANAUTHCODE-123"
	f := &fakeNile{do: func(string) (string, string, error) {
		// nile logs the client id, and a traceback can echo its arguments.
		return "", "INFO [AUTH_MANAGER]: Registerring a device. ID: " + login.ClientID +
			"\nERROR [AUTH_MANAGER]: Failed to register a device\nargs: --code " + code +
			" --code-verifier " + login.CodeVerifier + " --serial " + login.Serial +
			"\nAttributeError: 'NoneType' object has no attribute 'encode'", errExit1
	}}
	_, _, err := newTestClient(f).Register(context.Background(), code, login)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, secret := range []string{code, login.ClientID, login.CodeVerifier, login.Serial} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q leaks %q", err, secret)
		}
	}
}

func writeLibrary(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "nile", "library.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLibrary_RebuildsConfigAndReadsEntitlements(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{}
	f.do = func(dir string) (string, string, error) {
		// nile finds its sign-in where it expects it…
		seen, err := readNileConfig(dir)
		if err != nil {
			t.Fatalf("nile config not rebuilt: %v", err)
		}
		if !sameJSON(t, seen, userData) {
			t.Errorf("nile saw different user data: %s", seen)
		}
		// …and its SDK download is skipped.
		if _, err := os.Stat(filepath.Join(dir, "nile", "SDK")); err != nil {
			t.Errorf("SDK dir missing: %v", err)
		}
		// nile refreshes the access token, rewriting the user data.
		refreshed := strings.Replace(string(userData), "Atna|FAKE-ACCESS-TOKEN", "Atna|NEW-ACCESS-TOKEN", 1)
		if err := writeNileConfig(dir, json.RawMessage(refreshed)); err != nil {
			t.Fatal(err)
		}
		writeLibrary(t, dir, `[
			{"id": "e1", "product": {"id": "amzn1.adg.product.1", "title": "Game One", "productLine": "Sonic:Game"}},
			{"id": "e2", "product": {"id": "amzn1.adg.product.2", "title": "Game Two"}}
		]`)
		return "", "INFO [LIBRARY]: Successfully synced the library", nil
	}
	ents, refreshed, err := newTestClient(f).Library(context.Background(), userData)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	want := []Entitlement{{"amzn1.adg.product.1", "Game One"}, {"amzn1.adg.product.2", "Game Two"}}
	if !reflect.DeepEqual(ents, want) {
		t.Errorf("entitlements = %+v, want %+v", ents, want)
	}
	if !strings.Contains(string(refreshed), "Atna|NEW-ACCESS-TOKEN") {
		t.Errorf("refreshed user data not returned: %s", refreshed)
	}
	if strings.Join(f.args, " ") != "library sync --force" {
		t.Errorf("args = %v", f.args)
	}
	if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
		t.Errorf("config dir %s left behind", f.dir)
	}
}

func TestLibrary_EmptyLibrary(t *testing.T) {
	f := &fakeNile{do: func(dir string) (string, string, error) {
		writeLibrary(t, dir, `[]`)
		return "", "", nil
	}}
	ents, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json"))
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if len(ents) != 0 {
		t.Errorf("entitlements = %v, want none", ents)
	}
}

// The stderr fixture is nile v1.2.0's real output for a refresh token Amazon
// refused (a 400), captured on 2026-11-20.
func TestLibrary_RejectedRefreshIsAuthFailure(t *testing.T) {
	stderr := string(readFixture(t, "stderr-refresh-rejected.txt"))
	f := &fakeNile{do: func(string) (string, string, error) { return "", stderr, errExit1 }}
	_, refreshed, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json"))
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
	if refreshed == nil {
		t.Error("user data nile left behind should still be returned")
	}
}

func TestLibrary_NotLoggedInIsAuthFailure(t *testing.T) {
	f := &fakeNile{do: func(string) (string, string, error) {
		return "", "ERROR [CLI]: User not logged in", errExit1
	}}
	if _, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json")); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestLibrary_Errors(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string) (string, string, error){
		// A connection failure on refresh is not a credentials problem.
		"refresh connection error": func(*testing.T, string) (string, string, error) {
			return "", "ERROR [AUTH_MANAGER]: Failed to refresh the token HTTPSConnectionPool(host='api.amazon.com')\nKeyError: 'entitlements'", errExit1
		},
		"no library written": func(*testing.T, string) (string, string, error) { return "", "", nil },
		"unparseable library": func(t *testing.T, dir string) (string, string, error) {
			writeLibrary(t, dir, `{"not": "a list"`)
			return "", "", nil
		},
		// A later page failing: nile crashes before writing library.json.
		"late page failure": func(*testing.T, string) (string, string, error) {
			return "", "INFO [LIBRARY]: Got next token in response, making next request\nKeyError: 'entitlements'", errExit1
		},
	}
	for name, do := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeNile{do: func(dir string) (string, string, error) { return do(t, dir) }}
			_, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json"))
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, ErrAuthFailed) {
				t.Errorf("err = %v, must not be a credentials failure", err)
			}
		})
	}
}

func TestLibrary_ErrorIsSanitized(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(string) (string, string, error) {
		return "", "DEBUG token=Atna|FAKE-ACCESS-TOKEN refresh=Atnr|FAKE-REFRESH-TOKEN\n" +
			"key FAKE-DEVICE-PRIVATE-KEY adp FAKE-ADP-TOKEN serial 0123456789ABCDEF0123456789ABCDEF\n" +
			"KeyError: 'entitlements'", errExit1
	}}
	_, _, err := newTestClient(f).Library(context.Background(), userData)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, secret := range []string{"FAKE-ACCESS-TOKEN", "FAKE-REFRESH-TOKEN", "FAKE-DEVICE-PRIVATE-KEY", "FAKE-ADP-TOKEN", "0123456789ABCDEF0123456789ABCDEF"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q leaks %q", err, secret)
		}
	}
	if !strings.Contains(err.Error(), "KeyError") {
		t.Errorf("error %q lost nile's own failure line", err)
	}
}

func TestLibrary_RejectsUserDataWithoutUserID(t *testing.T) {
	f := &fakeNile{}
	if _, _, err := newTestClient(f).Library(context.Background(), json.RawMessage(`{"tokens": {}}`)); err == nil {
		t.Fatal("expected an error")
	}
	if f.args != nil {
		t.Error("nile ran without a usable sign-in")
	}
}

// The worker marks every game a sync does not return as removed, so a library
// that is not a list, or that loses an entry's id, must fail rather than read
// as fewer games.
func TestLibrary_RefusesLibraryThatWouldDropGames(t *testing.T) {
	for name, body := range map[string]string{
		"null":            `null`,
		"object":          `{}`,
		"entry no id":     `[{"product": {"id": "p1", "title": "A"}}, {"product": {"title": "B"}}]`,
		"entry blank id":  `[{"product": {"id": "  ", "title": "B"}}]`,
		"entry padded id": `[{"product": {"id": "p1 ", "title": "A"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeNile{do: func(dir string) (string, string, error) {
				writeLibrary(t, dir, body)
				return "", "", nil
			}}
			if _, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json")); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// nile can log a refused refresh and still exit 0 without writing a library.
func TestLibrary_RejectedRefreshWithCleanExitIsAuthFailure(t *testing.T) {
	f := &fakeNile{do: func(string) (string, string, error) {
		return "", "ERROR [AUTH_MANAGER]:\t Failed to refresh the token <Response [400]>\nINFO [LIBRARY]:\t Got next token in response, making next request", nil
	}}
	if _, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json")); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

// After a refused refresh nile 1.2.0 still writes `[]` when the failed
// response carries an empty entitlement list; that must not read as a library
// with no games.
func TestLibrary_RejectedRefreshWithEmptyLibraryIsAuthFailure(t *testing.T) {
	f := &fakeNile{do: func(dir string) (string, string, error) {
		writeLibrary(t, dir, `[]`)
		return "", "ERROR [AUTH_MANAGER]:\t Failed to refresh the token <Response [400]>", nil
	}}
	if _, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json")); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

// A refresh that failed without a refusal (a 5xx, a dropped connection) also
// leaves nile reading with an expired token, so its library is not trusted;
// it is an ordinary error, not a credentials one.
func TestLibrary_FailedRefreshDiscardsLibrary(t *testing.T) {
	for name, line := range map[string]string{
		"server error":     "ERROR [AUTH_MANAGER]:\t Failed to refresh the token <Response [503]>",
		"connection error": "ERROR [AUTH_MANAGER]:\t Failed to refresh the token HTTPSConnectionPool(host='api.amazon.com', port=443)",
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeNile{do: func(dir string) (string, string, error) {
				writeLibrary(t, dir, `[]`)
				return "", line, nil
			}}
			_, _, err := newTestClient(f).Library(context.Background(), readFixture(t, "user-data.json"))
			if err == nil {
				t.Fatal("expected an error, not an empty library")
			}
			if errors.Is(err, ErrAuthFailed) {
				t.Errorf("err = %v, must not be a credentials failure", err)
			}
		})
	}
}

func TestLibrary_RefreshedUserDataWithUnreadableExpiryIsNotReturned(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	for _, expiry := range []string{`"invalid"`, `null`, `"36.5"`, `0`, `-5`} {
		f := &fakeNile{do: func(dir string) (string, string, error) {
			bad := strings.Replace(string(userData), `"expires_in": "3600"`, `"expires_in": `+expiry, 1)
			if err := writeNileConfig(dir, json.RawMessage(bad)); err != nil {
				t.Fatal(err)
			}
			writeLibrary(t, dir, `[]`)
			return "", "", nil
		}}
		_, refreshed, err := newTestClient(f).Library(context.Background(), userData)
		if err != nil {
			t.Fatalf("expires_in %s: %v", expiry, err)
		}
		if refreshed != nil {
			t.Errorf("expires_in %s: user data returned for storing", expiry)
		}
	}
	for _, expiry := range []string{`"3600"`, `3600`, `"0"`} {
		f := &fakeNile{do: func(dir string) (string, string, error) {
			good := strings.Replace(string(userData), `"expires_in": "3600"`, `"expires_in": `+expiry, 1)
			good = strings.Replace(good, "Atna|FAKE-ACCESS-TOKEN", "Atna|NEWER", 1)
			if err := writeNileConfig(dir, json.RawMessage(good)); err != nil {
				t.Fatal(err)
			}
			writeLibrary(t, dir, `[]`)
			return "", "", nil
		}}
		if _, refreshed, _ := newTestClient(f).Library(context.Background(), userData); refreshed == nil {
			t.Errorf("expires_in %s: usable user data was not returned", expiry)
		}
	}
}

// Refreshed user data is handed back for storing only if nile could sign in
// with it again.
func TestLibrary_UnusableRefreshedUserDataIsNotReturned(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(dir string) (string, string, error) {
		blank := strings.Replace(string(userData), "Atna|FAKE-ACCESS-TOKEN", "", 1)
		if err := writeNileConfig(dir, json.RawMessage(blank)); err != nil {
			t.Fatal(err)
		}
		return "", "KeyError: 'entitlements'", errExit1
	}}
	_, refreshed, err := newTestClient(f).Library(context.Background(), userData)
	if err == nil {
		t.Fatal("expected an error")
	}
	if refreshed != nil {
		t.Errorf("user data with a blank access token was returned for storing: %s", refreshed)
	}
}

func TestRegister_RefusesUserDataWithoutTokens(t *testing.T) {
	f := &fakeNile{do: func(dir string) (string, string, error) {
		noTokens := `{"extensions": {"customer_info": {"user_id": "amzn1.account.X", "given_name": "A"}}, "tokens": {"bearer": {}}}`
		if err := writeNileConfig(dir, json.RawMessage(noTokens)); err != nil {
			t.Fatal(err)
		}
		return "", "", nil
	}}
	if _, _, err := newTestClient(f).Register(context.Background(), "CODE", Login{ClientID: "c", CodeVerifier: "v", Serial: "s"}); err == nil {
		t.Fatal("expected an error")
	}
}

// signOut acts as nile 1.2.0's logout after Amazon accepted it: the signed-in
// user and its user data file are removed.
func signOut(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"current_user.json", "095bcd5f2ec20d88c189d52912352950.enc"} {
		if err := os.Remove(filepath.Join(dir, "nile", name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLogout_DeregistersWithTheGivenSignIn(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{}
	f.do = func(dir string) (string, string, error) {
		seen, err := readNileConfig(dir)
		if err != nil {
			t.Fatalf("nile config not rebuilt: %v", err)
		}
		if !sameJSON(t, seen, userData) {
			t.Errorf("nile saw different user data: %s", seen)
		}
		signOut(t, dir)
		return "", "INFO [AUTH_MANAGER]: Successfully deregistered a device", nil
	}
	if err := newTestClient(f).Logout(context.Background(), userData); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if strings.Join(f.args, " ") != "auth --logout" {
		t.Errorf("args = %v", f.args)
	}
	if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
		t.Errorf("config dir %s left behind", f.dir)
	}
}

// Amazon refusing the deregistration leaves nile signed in and exits 0.
func TestLogout_RefusedDeregistrationIsAnError(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(string) (string, string, error) { return "", "", nil }}
	err := newTestClient(f).Logout(context.Background(), userData)
	if err == nil || !strings.Contains(err.Error(), "could not deregister") {
		t.Fatalf("err = %v, want a deregistration failure", err)
	}
	if _, statErr := os.Stat(f.dir); !os.IsNotExist(statErr) {
		t.Errorf("config dir %s left behind", f.dir)
	}
}

func TestLogout_FailureIsSanitized(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{do: func(string) (string, string, error) {
		return "", "DEBUG token=Atna|FAKE-ACCESS-TOKEN refresh=Atnr|FAKE-REFRESH-TOKEN\n" +
			"key FAKE-DEVICE-PRIVATE-KEY adp FAKE-ADP-TOKEN serial 0123456789ABCDEF0123456789ABCDEF\n" +
			"ConnectionError: deregister", errExit1
	}}
	err := newTestClient(f).Logout(context.Background(), userData)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, secret := range []string{"FAKE-ACCESS-TOKEN", "FAKE-REFRESH-TOKEN", "FAKE-DEVICE-PRIVATE-KEY", "FAKE-ADP-TOKEN", "0123456789ABCDEF0123456789ABCDEF"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q leaks %q", err, secret)
		}
	}
	if !strings.Contains(err.Error(), "ConnectionError") {
		t.Errorf("error %q lost nile's own failure line", err)
	}
}

func TestLogout_RefusesUserDataWithoutUserID(t *testing.T) {
	f := &fakeNile{}
	if err := newTestClient(f).Logout(context.Background(), json.RawMessage(`{"tokens":{}}`)); err == nil {
		t.Fatal("expected an error")
	}
	if f.args != nil {
		t.Errorf("nile ran: %v", f.args)
	}
}

// nile reads the device serial to fetch entitlements, so user data without one
// is not a usable sign-in: a registration that wrote none is refused, and
// refreshed data that lost it is not handed back for storing.
func TestUserData_WithoutDeviceSerialIsRefused(t *testing.T) {
	full := readFixture(t, "user-data.json")
	for _, serial := range []string{`""`, `null`} {
		noSerial := strings.Replace(string(full), `"0123456789ABCDEF0123456789ABCDEF"`, serial, 1)
		if noSerial == string(full) {
			t.Fatal("fixture has no serial to remove")
		}
		reg := &fakeNile{do: func(dir string) (string, string, error) {
			if err := writeNileConfig(dir, json.RawMessage(noSerial)); err != nil {
				t.Fatal(err)
			}
			return "", "", nil
		}}
		if _, _, err := newTestClient(reg).Register(context.Background(), "CODE", Login{ClientID: "c", CodeVerifier: "v", Serial: "s"}); err == nil {
			t.Errorf("serial %s: registration accepted", serial)
		}
		lib := &fakeNile{do: func(dir string) (string, string, error) {
			if err := writeNileConfig(dir, json.RawMessage(noSerial)); err != nil {
				t.Fatal(err)
			}
			writeLibrary(t, dir, `[]`)
			return "", "", nil
		}}
		if _, refreshed, err := newTestClient(lib).Library(context.Background(), full); err != nil || refreshed != nil {
			t.Errorf("serial %s: Library returned refreshed=%s err=%v, want no refreshed data and no error", serial, refreshed, err)
		}
	}
}

// A nile that leaves a child holding its output pipes (a PyInstaller
// launcher's Python process does) is cut off at the deadline with its whole
// process group, instead of the call waiting for the child.
func TestRunNile_DeadlineKillsTheProcessGroup(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-alive")
	script := filepath.Join(dir, "fake-nile")
	// The child sleeps holding stdout/stderr, then would write a marker file.
	body := "#!/bin/sh\n(sleep 5; touch '" + marker + "') &\nsleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil { //nolint:gosec // test script
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := runNile(ctx, script, dir)
	if err == nil {
		t.Fatal("expected an error from a killed nile")
	}
	if took := time.Since(start); took > nileWaitDelay+2*time.Second {
		t.Errorf("runNile returned after %v, want within the deadline plus %v", took, nileWaitDelay)
	}
	time.Sleep(6 * time.Second)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("nile's child outlived the deadline")
	}
}

// nile's register syncs the whole library after writing the registration; a
// slow sync must not hold the sign-in up, so Register stops nile as soon as
// usable user data is on disk.
func TestRegister_ReturnsOnceTheRegistrationIsWritten(t *testing.T) {
	userData := readFixture(t, "user-data.json")
	f := &fakeNile{doCtx: func(ctx context.Context, dir string) (string, string, error) {
		if err := writeNileConfig(dir, userData); err != nil {
			t.Fatal(err)
		}
		select { // the trailing library sync, which would take 30 s
		case <-ctx.Done():
			return "", "", errors.New("signal: killed")
		case <-time.After(30 * time.Second):
			return "", "", nil
		}
	}}
	start := time.Now()
	account, got, err := newTestClient(f).Register(context.Background(), "CODE", Login{ClientID: "c", CodeVerifier: "v", Serial: "s"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Register took %v, want it to stop nile once the registration was written", took)
	}
	if account == nil || !sameJSON(t, got, userData) {
		t.Errorf("Register returned account=%v data=%s", account, got)
	}
}

// nile's temporary files (a PyInstaller nile extracts its runtime there) go
// in the per-call folder, which is deleted after every call, even when nile
// was killed before it could clean up.
func TestRunNile_TempFilesStayInTheCallFolder(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "fake-nile")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf %s \"$TMPDIR\"\n"), 0o700); err != nil { //nolint:gosec // test script
		t.Fatal(err)
	}
	stdout, _, err := runNile(context.Background(), script, dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != dir {
		t.Errorf("nile's TMPDIR = %q, want the call folder %q", stdout, dir)
	}
}

// With TMPDIR unset, a host that points TEMP (or TMP) at an executable folder
// because /tmp is noexec gets the call folder there, as PyInstaller would.
func TestNewConfigDir_FollowsPyInstallerTempOrder(t *testing.T) {
	temp, tmp := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", "")
	t.Setenv("TEMP", temp)
	t.Setenv("TMP", tmp)
	dir, err := newConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	defer removeConfigDir(dir)
	if filepath.Dir(dir) != temp {
		t.Errorf("call folder %s, want it under TEMP %s", dir, temp)
	}
	t.Setenv("TEMP", "")
	dir2, err := newConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	defer removeConfigDir(dir2)
	if filepath.Dir(dir2) != tmp {
		t.Errorf("call folder %s, want it under TMP %s", dir2, tmp)
	}
}

// A TEMP that does not exist is passed over for the next candidate, TMP.
func TestNewConfigDir_SkipsAMissingTempFolder(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", "")
	t.Setenv("TEMP", filepath.Join(t.TempDir(), "gone"))
	t.Setenv("TMP", tmp)
	dir, err := newConfigDir()
	if err != nil {
		t.Fatalf("newConfigDir: %v", err)
	}
	defer removeConfigDir(dir)
	if filepath.Dir(dir) != tmp {
		t.Errorf("call folder %s, want it under TMP %s", dir, tmp)
	}
}

// A stale TMPDIR with TEMP and TMP unset falls back to /tmp itself.
func TestNewConfigDir_StaleTMPDIRFallsBackToTmp(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "gone"))
	t.Setenv("TEMP", "")
	t.Setenv("TMP", "")
	dir, err := newConfigDir()
	if err != nil {
		t.Fatalf("newConfigDir: %v", err)
	}
	defer removeConfigDir(dir)
	if filepath.Dir(dir) != "/tmp" {
		t.Errorf("call folder %s, want it under /tmp", dir)
	}
}
