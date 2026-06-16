package igdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestAuthManager_FetchesToken(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		_ = json.NewEncoder(w).Encode(twitchTokenResponse{
			AccessToken: "test-token-123",
			ExpiresIn:   3600,
			TokenType:   "bearer",
		})
	}))
	defer srv.Close()

	am := &AuthManager{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		httpClient:   srv.Client(),
		tokenURL:     srv.URL,
	}

	token, err := am.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token-123" {
		t.Fatalf("expected test-token-123, got %s", token)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", callCount.Load())
	}

	// Second call should use cache
	token2, err := am.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token2 != "test-token-123" {
		t.Fatalf("expected cached token, got %s", token2)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected still 1 call (cached), got %d", callCount.Load())
	}
}

func TestAuthManager_RefreshesExpiredToken(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		_ = json.NewEncoder(w).Encode(twitchTokenResponse{
			AccessToken: "refreshed-token",
			ExpiresIn:   3600,
			TokenType:   "bearer",
		})
	}))
	defer srv.Close()

	am := &AuthManager{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		httpClient:   srv.Client(),
		tokenURL:     srv.URL,
		accessToken:  "old-token",
		expiresAt:    time.Now().Add(-1 * time.Hour),
	}

	token, err := am.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "refreshed-token" {
		t.Fatalf("expected refreshed-token, got %s", token)
	}
}

func TestExpandQueries(t *testing.T) {
	tests := []struct {
		query    string
		wantLen  int    // at least this many queries returned
		contains string // at least one result should contain this (lowercased check)
	}{
		// No keywords → just the original
		{"The Witcher 3", 1, "the witcher 3"},
		// GOTY keyword
		{"Horizon Zero Dawn GOTY", 2, "game of the year"},
		// Colon
		{"Halo: Reach", 2, "halo reach"},
		// Year in parens
		{"Doom (2016)", 2, "doom"},
		// Trademark symbols are pre-sanitized before keyword expansion, so trademark-only
		// titles produce a single clean query rather than original + stripped variant.
		{"FIFA®", 1, "fifa"},
		{"Velocity®2X", 1, "velocity 2x"},
		// ™ + colon → two variants (pre-sanitized original + colon-stripped)
		{"Batman™: Arkham Knight", 2, "batman arkham knight"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			queries := expandQueries(tt.query)
			if len(queries) < tt.wantLen {
				t.Errorf("expandQueries(%q) returned %d queries, want at least %d", tt.query, len(queries), tt.wantLen)
			}
			found := false
			for _, q := range queries {
				if strings.Contains(strings.ToLower(q), tt.contains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expandQueries(%q) = %v, none contain %q", tt.query, queries, tt.contains)
			}
		})
	}
}

func TestAuthManager_UsesPreConfiguredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call Twitch when pre-configured token is valid")
	}))
	defer srv.Close()

	am := &AuthManager{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		httpClient:   srv.Client(),
		tokenURL:     srv.URL,
		accessToken:  "pre-configured-token",
		// No expiresAt set — zero value, treated as unknown expiry
	}

	token, err := am.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "pre-configured-token" {
		t.Fatalf("expected pre-configured-token, got %s", token)
	}
}

func TestClient_SearchGames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{
				ID:   1942,
				Name: "The Witcher 3: Wild Hunt",
				Slug: "the-witcher-3-wild-hunt",
				Platforms: []igdbPlatform{
					{ID: 6, Name: "PC (Microsoft Windows)"},
				},
			},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "test-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "test",
			clientSecret: "test",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}

	results, err := client.SearchGames(context.Background(), "The Witcher 3", 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Title != "The Witcher 3: Wild Hunt" {
		t.Fatalf("expected 'The Witcher 3: Wild Hunt', got %q", results[0].Title)
	}
}

func TestClient_GetGameByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{
				ID:   1942,
				Name: "The Witcher 3: Wild Hunt",
				Slug: "the-witcher-3-wild-hunt",
			},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "test-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "test",
			clientSecret: "test",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}

	result, err := client.GetGameByID(context.Background(), 1942)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IgdbID != 1942 {
		t.Fatalf("expected ID 1942, got %d", result.IgdbID)
	}
}

func TestClient_GetGameByID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "test-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "test",
			clientSecret: "test",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}

	_, err := client.GetGameByID(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestClient_SearchGames_EmptyPlatformIDs_NoClause(t *testing.T) {
	// SearchGames issues its fuzzy + exact queries concurrently, so the handler
	// runs on multiple goroutines — guard the shared slice.
	var (
		mu             sync.Mutex
		receivedBodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "X", Slug: "x"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	_, err := client.SearchGames(context.Background(), "x", 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The field list contains "platforms.name" (we request platforms in results).
	// What we want to assert is that no platform-FILTER clause is present.
	for _, b := range receivedBodies {
		if strings.Contains(b, "platforms = (") {
			t.Fatalf("nil platformIDs must not add a 'platforms = (...)' clause; got %q", b)
		}
	}
}

func TestClient_SearchGames_NonEmptyPlatformIDs_AppendsClause(t *testing.T) {
	var (
		mu             sync.Mutex
		receivedBodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "X", Slug: "x"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	_, err := client.SearchGames(context.Background(), "x", 10, []int{6, 14, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(receivedBodies) == 0 {
		t.Fatal("no IGDB requests captured")
	}
	for _, b := range receivedBodies {
		if !strings.Contains(b, "platforms = (6,14,3)") {
			t.Fatalf("body must contain 'platforms = (6,14,3)'; got %q", b)
		}
	}
}

func TestClient_SearchGames_EmptyFilteredResult_FallsBackUnfiltered(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		if strings.Contains(string(body), "platforms = (") {
			_ = json.NewEncoder(w).Encode([]igdbGameResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 42, Name: "Fallback Hit", Slug: "fallback-hit"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	results, err := client.SearchGames(context.Background(), "fallback hit", 10, []int{6})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback to return at least one result")
	}
	if results[0].IgdbID != 42 {
		t.Fatalf("expected fallback result IGDB ID 42, got %d", results[0].IgdbID)
	}

	var sawFiltered, sawUnfiltered bool
	for _, b := range bodies {
		if strings.Contains(b, "platforms = (") {
			sawFiltered = true
		} else {
			sawUnfiltered = true
		}
	}
	if !sawFiltered || !sawUnfiltered {
		t.Fatalf("expected both filtered and unfiltered request bodies; filtered=%v unfiltered=%v bodies=%v", sawFiltered, sawUnfiltered, bodies)
	}
}

// TestClient_SearchGames_PostFiltersWrongPlatformCandidates simulates IGDB
// honouring the `where platforms = (...)` clause on the exact-name path but
// ignoring it on the `search "..."` path (the actual observed IGDB behaviour
// per issue #615 — the Arcade-only "Grid" leaking into Steam-EG candidates).
// The post-filter in SearchGames must drop the wrong-platform candidate.
func TestClient_SearchGames_PostFiltersWrongPlatformCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Exact-name query: respect the where clause, return only the PC game.
		if strings.Contains(string(body), `where name = "GRID"`) {
			_ = json.NewEncoder(w).Encode([]igdbGameResponse{
				{ID: 118871, Name: "GRID", Slug: "grid-pc", Platforms: []igdbPlatform{{ID: 6, Name: "PC (Microsoft Windows)"}}},
			})
			return
		}
		// search "..." query: ignore the where clause, return both PC and Arcade.
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 118871, Name: "GRID", Slug: "grid-pc", Platforms: []igdbPlatform{{ID: 6, Name: "PC (Microsoft Windows)"}}},
			{ID: 327878, Name: "Grid", Slug: "grid-arcade", Platforms: []igdbPlatform{{ID: 52, Name: "Arcade"}}},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	results, err := client.SearchGames(context.Background(), "GRID", 10, []int{6})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range results {
		if r.IgdbID == 327878 {
			t.Fatalf("Arcade-only candidate 327878 must be post-filtered out; got %#v", r)
		}
	}
	// The PC GRID must survive.
	foundPC := false
	for _, r := range results {
		if r.IgdbID == 118871 {
			foundPC = true
			break
		}
	}
	if !foundPC {
		t.Fatalf("expected PC GRID (118871) to survive the post-filter; got results %v", results)
	}
}

// TestClient_SearchGames_KeepsCandidatesWithEmptyPlatforms verifies the
// recall-preserving rule: when IGDB returns a candidate with no platforms
// data, the post-filter keeps it rather than dropping (incomplete IGDB
// tagging shouldn't cost a legitimate match).
func TestClient_SearchGames_KeepsCandidatesWithEmptyPlatforms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 999, Name: "Title Without Platforms", Slug: "no-platforms"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	results, err := client.SearchGames(context.Background(), "Title Without Platforms", 10, []int{6})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected candidate with empty platforms to be kept")
	}
	if results[0].IgdbID != 999 {
		t.Fatalf("expected IGDB ID 999, got %d", results[0].IgdbID)
	}
}

func testIGDBClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "test-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "test",
			clientSecret: "test",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}
}
