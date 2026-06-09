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
