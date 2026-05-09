package igdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		// Trademark
		{"FIFA®", 2, "fifa"},
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

	results, err := client.SearchGames(context.Background(), "The Witcher 3", 10)
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
