package steam_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/services/steam"
)

func TestGetOwnedGames_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"games": []map[string]any{
					{"appid": 730, "name": "Counter-Strike 2", "playtime_forever": 120}, // 120 min → 2.0h
					{"appid": 440, "name": "Team Fortress 2", "playtime_forever": 0},    // 0 min → 0.0h
					{"appid": 570, "name": "Dota 2", "playtime_forever": 90},            // 90 min → 1.5h (sub-hour)
					{"appid": 620, "name": "Portal 2", "playtime_forever": 45},          // 45 min → 0.75h
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if err != nil {
		t.Fatalf("GetOwnedGames: %v", err)
	}
	if len(games) != 4 {
		t.Fatalf("want 4 games, got %d", len(games))
	}
	if games[0].AppID != 730 {
		t.Errorf("AppID: got %d, want 730", games[0].AppID)
	}
	if games[0].Title != "Counter-Strike 2" {
		t.Errorf("Title: got %q", games[0].Title)
	}
	if games[0].PlaytimeHours != 2.0 {
		t.Errorf("PlaytimeHours: got %v, want 2.0 (120 min / 60)", games[0].PlaytimeHours)
	}
	if games[1].PlaytimeHours != 0.0 {
		t.Errorf("PlaytimeHours for 0-minute game: got %v, want 0.0", games[1].PlaytimeHours)
	}
	if games[2].PlaytimeHours != 1.5 {
		t.Errorf("PlaytimeHours for 90-min game: got %v, want 1.5 (sub-hour precision)", games[2].PlaytimeHours)
	}
	if games[3].PlaytimeHours != 0.75 {
		t.Errorf("PlaytimeHours for 45-min game: got %v, want 0.75", games[3].PlaytimeHours)
	}
}

func TestGetAppDetailsPlatforms_HappyPath_MixedPlatforms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"730": map[string]any{
				"success": true,
				"data": map[string]any{
					"platforms": map[string]any{
						"windows": true,
						"mac":     false,
						"linux":   true,
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	pl, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err != nil {
		t.Fatalf("GetAppDetailsPlatforms: %v", err)
	}
	if !pl.Windows {
		t.Error("expected Windows=true")
	}
	if pl.Mac {
		t.Error("expected Mac=false")
	}
	if !pl.Linux {
		t.Error("expected Linux=true")
	}
}

func TestGetAppDetailsPlatforms_ReturnsZeroValueNoError(t *testing.T) {
	tests := []struct {
		name string
		// body is the JSON the appdetails endpoint returns for appid 730.
		body map[string]any
	}{
		{
			// success=false means Steam has no current store data for this appid
			// (e.g. removed or delisted games). The caller treats Platforms{} the
			// same as all-false platforms and falls back to a default, so we must
			// not error here.
			name: "success false",
			body: map[string]any{"730": map[string]any{"success": false}},
		},
		{
			name: "all-false platforms",
			body: map[string]any{
				"730": map[string]any{
					"success": true,
					"data": map[string]any{
						"platforms": map[string]any{
							"windows": false,
							"mac":     false,
							"linux":   false,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.body)
			}))
			defer srv.Close()

			c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
			pl, err := c.GetAppDetailsPlatforms(context.Background(), 730)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if pl.Windows || pl.Mac || pl.Linux {
				t.Errorf("expected all-false Platforms{}, got %+v", pl)
			}
		})
	}
}

func TestGetAppDetailsPlatforms_HTTP429_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for HTTP 429, got nil")
	}
}

func TestGetAppDetailsPlatforms_HTTP500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestGetAppDetailsPlatforms_UsesFiltersPlatformsInURL(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"730": map[string]any{
				"success": true,
				"data": map[string]any{
					"platforms": map[string]any{"windows": true, "mac": false, "linux": false},
				},
			},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, _ = c.GetAppDetailsPlatforms(context.Background(), 730)
	if q := capturedURL; q != "/api/appdetails?appids=730&filters=platforms" {
		t.Errorf("wrong URL: got %q", q)
	}
}

func TestGetAppDetailsPlatforms_MissingAppIDKey_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"999": map[string]any{"success": true},
		})
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetAppDetailsPlatforms(context.Background(), 730)
	if err == nil {
		t.Fatal("expected error for missing appid key in response, got nil")
	}
}

func TestGetOwnedGames_AuthFailure_ReturnsErrAPIKeyRejected(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
			_, err := c.GetOwnedGames(context.Background(), "badkey", "steamid")
			if !errors.Is(err, steam.ErrAPIKeyRejected) {
				t.Errorf("expected ErrAPIKeyRejected on %d, got %v", status, err)
			}
		})
	}
}

func TestGetPlayerAchievements(t *testing.T) {
	cases := []struct {
		name         string
		statusCode   int
		body         string
		wantUnlocked int
		wantTotal    int
		wantOK       bool
	}{
		{
			name:         "mixed",
			statusCode:   http.StatusOK,
			body:         `{"playerstats":{"success":true,"achievements":[{"apiname":"a","achieved":1},{"apiname":"b","achieved":0},{"apiname":"c","achieved":1}]}}`,
			wantUnlocked: 2, wantTotal: 3, wantOK: true,
		},
		{
			name:         "all locked",
			statusCode:   http.StatusOK,
			body:         `{"playerstats":{"success":true,"achievements":[{"apiname":"a","achieved":0},{"apiname":"b","achieved":0}]}}`,
			wantUnlocked: 0, wantTotal: 2, wantOK: true,
		},
		{
			name:       "private profile",
			statusCode: http.StatusOK,
			body:       `{"playerstats":{"error":"Profile is not public","success":false}}`,
			wantOK:     false,
		},
		{
			name:       "no stats",
			statusCode: http.StatusOK,
			body:       `{"playerstats":{"error":"Requested app has no stats","success":false}}`,
			wantOK:     false,
		},
		{
			name:       "http 403 forbidden",
			statusCode: http.StatusForbidden,
			wantOK:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.statusCode != http.StatusOK {
					w.WriteHeader(tc.statusCode)
					return
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)

			unlocked, total, ok, err := c.GetPlayerAchievements(context.Background(), "key", "steam123", 440)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (unlocked != tc.wantUnlocked || total != tc.wantTotal) {
				t.Errorf("got %d/%d, want %d/%d", unlocked, total, tc.wantUnlocked, tc.wantTotal)
			}
		})
	}
}

// ownedGamesClient returns a client whose owned-games endpoint answers body
// with HTTP 200.
func ownedGamesClient(t *testing.T, body string) *steam.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
}

// A reply with neither games nor game_count is what Steam sends when the
// account's game details are private. It is not an empty library (#1310).
func TestGetOwnedGames_NoGamesNoCount_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if !errors.Is(err, steam.ErrOwnedGamesUnavailable) {
		t.Fatalf("want ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
	}
}

func TestGetOwnedGames_NoResponseObject_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	c := ownedGamesClient(t, `{}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if !errors.Is(err, steam.ErrOwnedGamesUnavailable) {
		t.Fatalf("want ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
	}
}

// A count above zero with no list contradicts itself; reading it as an empty
// library would mark every game removed, so it is an error too.
func TestGetOwnedGames_CountWithoutGames_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{"game_count":1}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if !errors.Is(err, steam.ErrOwnedGamesUnavailable) {
		t.Fatalf("want ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
	}
}

func TestGetOwnedGames_NullGames_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{"games":null}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if !errors.Is(err, steam.ErrOwnedGamesUnavailable) {
		t.Fatalf("want ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
	}
}

// An account that owns nothing gets game_count 0 and no games key.
func TestGetOwnedGames_ZeroGameCount_ReturnsEmptyLibrary(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{"game_count":0}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if err != nil {
		t.Fatalf("GetOwnedGames: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("want 0 games, got %d", len(games))
	}
}

func TestGetOwnedGames_IncompleteReply_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	for name, reply := range map[string]string{
		"empty list without count": `{"response":{"games":[]}}`,
		"partial list":             `{"response":{"game_count":2,"games":[{"appid":730}]}}`,
		"zero count with a game":   `{"response":{"game_count":0,"games":[{"appid":730}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := ownedGamesClient(t, reply)
			games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
			if !errors.Is(err, steam.ErrOwnedGamesUnavailable) || len(games) != 0 {
				t.Fatalf("want no games and ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
			}
		})
	}
}

// An empty list beside a nonzero count contradicts itself, the same as a count
// with no list at all.
func TestGetOwnedGames_EmptyGamesWithNonzeroCount_ReturnsErrOwnedGamesUnavailable(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{"game_count":1,"games":[]}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if !errors.Is(err, steam.ErrOwnedGamesUnavailable) {
		t.Fatalf("want ErrOwnedGamesUnavailable, got games=%v err=%v", games, err)
	}
}

func TestGetOwnedGames_EmptyGamesWithZeroCount_ReturnsEmptyLibrary(t *testing.T) {
	c := ownedGamesClient(t, `{"response":{"game_count":0,"games":[]}}`)
	games, err := c.GetOwnedGames(context.Background(), "key", "steamid")
	if err != nil {
		t.Fatalf("GetOwnedGames: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("want 0 games, got %d", len(games))
	}
}

// A 429 on the player summary is ErrRateLimited, so connecting can tell it
// from an unreachable Steam (#1485).
func TestGetPlayerSummaries_429IsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	if _, err := c.GetPlayerSummaries(context.Background(), "key", "76561198000000000"); !errors.Is(err, steam.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
