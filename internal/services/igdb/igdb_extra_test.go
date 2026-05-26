package igdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// InvalidateToken
// ---------------------------------------------------------------------------

func TestAuthManager_InvalidateToken(t *testing.T) {
	am := &AuthManager{
		accessToken: "some-token",
		expiresAt:   time.Now().Add(1 * time.Hour),
	}
	am.InvalidateToken()
	if am.accessToken != "" {
		t.Errorf("expected empty accessToken after Invalidate, got %q", am.accessToken)
	}
	if !am.expiresAt.IsZero() {
		t.Error("expected zero expiresAt after Invalidate")
	}
}

// ---------------------------------------------------------------------------
// sortByScore
// ---------------------------------------------------------------------------

func TestSortByScore_SortsDescending(t *testing.T) {
	items := []scoredCandidate{
		{metadata: GameMetadata{Title: "Low"}, score: 0.5},
		{metadata: GameMetadata{Title: "High"}, score: 0.9},
		{metadata: GameMetadata{Title: "Mid"}, score: 0.7},
	}
	sortByScore(items)
	if items[0].metadata.Title != "High" || items[1].metadata.Title != "Mid" || items[2].metadata.Title != "Low" {
		t.Errorf("unexpected sort order: %v", items)
	}
}

// ---------------------------------------------------------------------------
// convertToGameMetadata — various field combinations
// ---------------------------------------------------------------------------

func TestConvertToGameMetadata_AllFieldsPopulated(t *testing.T) {
	ts := int64(1704067200) // 2024-01-01
	rating := 85.5
	ratingCount := int32(1000)
	summary := "A great game"
	coverID := "abc123"
	g := igdbGameResponse{
		ID:               1942,
		Name:             "Test Game",
		Slug:             "test-game",
		Summary:          &summary,
		FirstReleaseDate: &ts,
		TotalRating:      &rating,
		TotalRatingCount: &ratingCount,
		Cover:            &igdbCover{ImageID: coverID},
		Genres:           []igdbNamedItem{{Name: "RPG"}, {Name: "Action"}},
		InvolvedCompanies: []igdbCompany{
			{Company: igdbNamedItem{ID: 5, Name: "CD Projekt"}, Developer: true, Publisher: false},
			{Company: igdbNamedItem{ID: 6, Name: "GOG"}, Developer: false, Publisher: true},
		},
		Platforms:          []igdbPlatform{{ID: 6, Name: "PC (Microsoft Windows)"}},
		GameModes:          []igdbNamedItem{{Name: "Single-player"}},
		Themes:             []igdbNamedItem{{Name: "Fantasy"}},
		PlayerPerspectives: []igdbNamedItem{{Name: "Third person"}},
	}

	md := convertToGameMetadata(g)

	if md.IgdbID != 1942 {
		t.Errorf("IgdbID: expected 1942, got %d", md.IgdbID)
	}
	if md.Description == nil || *md.Description != "A great game" {
		t.Errorf("Description mismatch: %v", md.Description)
	}
	if md.ReleaseDate == nil || *md.ReleaseDate != "2024-01-01" {
		t.Errorf("ReleaseDate mismatch: %v", md.ReleaseDate)
	}
	if md.CoverImageID != "abc123" {
		t.Errorf("CoverImageID: expected abc123, got %q", md.CoverImageID)
	}
	if md.CoverArtURL == nil {
		t.Error("expected CoverArtURL to be set")
	}
	if md.Genre == nil || *md.Genre != "RPG" {
		t.Errorf("Genre mismatch: %v", md.Genre)
	}
	if md.Developer == nil || *md.Developer != "CD Projekt" {
		t.Errorf("Developer mismatch: %v", md.Developer)
	}
	if md.Publisher == nil || *md.Publisher != "GOG" {
		t.Errorf("Publisher mismatch: %v", md.Publisher)
	}
	if md.GameModes == nil || *md.GameModes != "Single-player" {
		t.Errorf("GameModes mismatch: %v", md.GameModes)
	}
	if md.Themes == nil || *md.Themes != "Fantasy" {
		t.Errorf("Themes mismatch: %v", md.Themes)
	}
	if md.PlayerPerspectives == nil || *md.PlayerPerspectives != "Third person" {
		t.Errorf("PlayerPerspectives mismatch: %v", md.PlayerPerspectives)
	}
}

func TestConvertToGameMetadata_MinimalFields(t *testing.T) {
	g := igdbGameResponse{
		ID:   42,
		Name: "Minimal Game",
		Slug: "minimal-game",
	}
	md := convertToGameMetadata(g)
	if md.IgdbID != 42 || md.Title != "Minimal Game" {
		t.Errorf("unexpected minimal metadata: %+v", md)
	}
	if md.CoverImageID != "" {
		t.Error("expected empty CoverImageID for game without cover")
	}
	if md.Genre != nil {
		t.Error("expected nil Genre for game without genres")
	}
}

func TestConvertToGameMetadata_CoverWithEmptyImageID(t *testing.T) {
	// Cover struct present but ImageID is empty — should not set CoverImageID.
	g := igdbGameResponse{
		ID:    99,
		Name:  "No Cover",
		Cover: &igdbCover{ImageID: ""},
	}
	md := convertToGameMetadata(g)
	if md.CoverImageID != "" {
		t.Errorf("expected empty CoverImageID, got %q", md.CoverImageID)
	}
}

// ---------------------------------------------------------------------------
// FetchFullMetadata
// ---------------------------------------------------------------------------

func TestClient_FetchFullMetadata_NotConfigured(t *testing.T) {
	c := &Client{configured: false}
	_, err := c.FetchFullMetadata(context.Background(), 42)
	if err != ErrIGDBNotConfigured {
		t.Errorf("expected ErrIGDBNotConfigured, got %v", err)
	}
}

func TestClient_FetchFullMetadata_Success(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		path := r.URL.Path
		if path == "/game_time_to_beats" {
			hastily := 7200   // 2h
			normally := 14400 // 4h
			completely := 36000
			_ = json.NewEncoder(w).Encode([]igdbTimeToBeatResponse{
				{Hastily: &hastily, Normally: &normally, Completely: &completely},
			})
			return
		}
		// /games endpoint
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1942, Name: "The Witcher 3", Slug: "the-witcher-3"},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "tok",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "cid",
			clientSecret: "cs",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}

	md, err := client.FetchFullMetadata(context.Background(), 1942)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md.IgdbID != 1942 {
		t.Errorf("expected IgdbID=1942, got %d", md.IgdbID)
	}
	if md.HowlongtobeatMain == nil {
		t.Error("expected HowlongtobeatMain to be set")
	} else if *md.HowlongtobeatMain != 2.0 {
		t.Errorf("expected HowlongtobeatMain=2.0, got %v", *md.HowlongtobeatMain)
	}
}

func TestClient_FetchFullMetadata_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter: rate.NewLimiter(rate.Inf, 1), apiURL: srv.URL, configured: true,
	}

	_, err := client.FetchFullMetadata(context.Background(), 99999)
	if err != ErrGameNotFound {
		t.Errorf("expected ErrGameNotFound, got %v", err)
	}
}

func TestClient_FetchFullMetadata_TimeToBeatEmpty(t *testing.T) {
	// fetchTimeToBeat returns empty array → no time-to-beat set, but no error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/game_time_to_beats" {
			_ = json.NewEncoder(w).Encode([]igdbTimeToBeatResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "Game", Slug: "game"},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter: rate.NewLimiter(rate.Inf, 1), apiURL: srv.URL, configured: true,
	}

	md, err := client.FetchFullMetadata(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md.HowlongtobeatMain != nil {
		t.Error("expected nil HowlongtobeatMain when TTB returns empty")
	}
}

// ---------------------------------------------------------------------------
// DownloadCoverArt
// ---------------------------------------------------------------------------

func TestClient_DownloadCoverArt_EmptyImageID(t *testing.T) {
	c := &Client{httpClient: &http.Client{}}
	url, err := c.DownloadCoverArt(context.Background(), "", t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL for empty imageID, got %q", url)
	}
}

func TestClient_DownloadCoverArt_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	coverDir := filepath.Join(dir, "cover_art")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	imageID := "existing123"
	localPath := filepath.Join(coverDir, imageID+".jpg")
	if err := os.WriteFile(localPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := &Client{httpClient: &http.Client{}}
	url, err := c.DownloadCoverArt(context.Background(), imageID, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "/static/cover_art/"+imageID+".jpg" {
		t.Errorf("unexpected url: %q", url)
	}
}

// ---------------------------------------------------------------------------
// searchIGDB — 401 retry path
// ---------------------------------------------------------------------------

func TestSearchIGDB_401_RetrySucceeds(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: return 401 to trigger retry.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/" { // token refresh
			_ = json.NewEncoder(w).Encode(twitchTokenResponse{
				AccessToken: "new-token",
				ExpiresIn:   3600,
			})
			return
		}
		// Retry: return games.
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "Retry Game", Slug: "retry-game"},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "expired-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "cid",
			clientSecret: "cs",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL + "/",
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}

	results, err := client.searchIGDB(context.Background(), `fields name; where id = 1;`)
	if err != nil {
		t.Fatalf("searchIGDB retry failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results after retry")
	}
}

func TestSearchIGDB_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter: rate.NewLimiter(rate.Inf, 1), apiURL: srv.URL, configured: true,
	}

	_, err := client.searchIGDB(context.Background(), "fields name;")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

// ---------------------------------------------------------------------------
// SearchGames — not configured
// ---------------------------------------------------------------------------

func TestClient_SearchGames_NotConfigured(t *testing.T) {
	c := &Client{configured: false}
	_, err := c.SearchGames(context.Background(), "anything", 10, nil)
	if err != ErrIGDBNotConfigured {
		t.Errorf("expected ErrIGDBNotConfigured, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetGameByID — not configured
// ---------------------------------------------------------------------------

func TestClient_GetGameByID_NotConfigured(t *testing.T) {
	c := &Client{configured: false}
	_, err := c.GetGameByID(context.Background(), 42)
	if err != ErrIGDBNotConfigured {
		t.Errorf("expected ErrIGDBNotConfigured, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// fetchTimeToBeat — non-200 status
// ---------------------------------------------------------------------------

func TestFetchTimeToBeat_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter: rate.NewLimiter(rate.Inf, 1), apiURL: srv.URL, configured: true,
	}

	_, err := client.fetchTimeToBeat(context.Background(), 42)
	if err == nil {
		t.Error("expected error for non-200 status from time-to-beat endpoint")
	}
}

// ---------------------------------------------------------------------------
// searchIGDB — 429 retry path
// ---------------------------------------------------------------------------

func TestSearchIGDB_429_RetryAfterHeader_RetriesAndSucceeds(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: 429 with Retry-After=0 so the test runs fast.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 7, Name: "Retried", Slug: "retried"},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter:       rate.NewLimiter(rate.Inf, 1),
		apiURL:        srv.URL,
		configured:    true,
		maxRetries:    3,
		backoffFactor: 0,
	}

	results, err := client.searchIGDB(context.Background(), `fields name; where id = 7;`)
	if err != nil {
		t.Fatalf("searchIGDB 429 retry failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != 7 {
		t.Fatalf("expected one result with ID 7, got %#v", results)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP calls (1 fail + 1 retry), got %d", got)
	}
}

func TestSearchIGDB_429_NoRetryAfter_UsesBackoffFactorAndSucceeds(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// 429 without Retry-After header — client must fall back to
			// IGDBBackoffFactor (set to 0 here to keep the test fast).
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 9, Name: "Backoff", Slug: "backoff"},
		})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter:       rate.NewLimiter(rate.Inf, 1),
		apiURL:        srv.URL,
		configured:    true,
		maxRetries:    3,
		backoffFactor: 0,
	}

	results, err := client.searchIGDB(context.Background(), `fields name; where id = 9;`)
	if err != nil {
		t.Fatalf("searchIGDB 429 backoff retry failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != 9 {
		t.Fatalf("expected one result with ID 9, got %#v", results)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP calls (1 fail + 1 retry), got %d", got)
	}
}

func TestSearchIGDB_429_ExhaustsRetriesReturnsError(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter:       rate.NewLimiter(rate.Inf, 1),
		apiURL:        srv.URL,
		configured:    true,
		maxRetries:    2,
		backoffFactor: 0,
	}

	_, err := client.searchIGDB(context.Background(), `fields name; where id = 1;`)
	if err == nil {
		t.Fatal("expected error after retries exhausted, got nil")
	}
	// 1 initial attempt + 2 retries = 3 total calls.
	if got := callCount.Load(); got != 3 {
		t.Fatalf("expected 3 HTTP calls (initial + maxRetries), got %d", got)
	}
}

func TestFetchTimeToBeat_429_RetriesAndSucceeds(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbTimeToBeatResponse{{}})
	}))
	defer srv.Close()

	client := &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken: "tok", expiresAt: time.Now().Add(1 * time.Hour),
			clientID: "cid", clientSecret: "cs",
			httpClient: srv.Client(), tokenURL: srv.URL,
		},
		limiter:       rate.NewLimiter(rate.Inf, 1),
		apiURL:        srv.URL,
		configured:    true,
		maxRetries:    3,
		backoffFactor: 0,
	}

	if _, err := client.fetchTimeToBeat(context.Background(), 42); err != nil {
		t.Fatalf("fetchTimeToBeat 429 retry failed: %v", err)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected 2 HTTP calls (1 fail + 1 retry), got %d", got)
	}
}
