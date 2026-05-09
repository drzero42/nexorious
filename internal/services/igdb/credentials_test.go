package igdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
)

func TestIsAuthError_TrueForAuthFailure(t *testing.T) {
	err := fmt.Errorf("%w: Twitch returned status 403", igdb.ErrTwitchAuth)
	if !igdb.IsAuthError(err) {
		t.Error("IsAuthError should return true for ErrTwitchAuth wrapping an HTTP status")
	}
}

func TestIsAuthError_FalseForNetworkError(t *testing.T) {
	err := fmt.Errorf("connection refused")
	if igdb.IsAuthError(err) {
		t.Error("IsAuthError should return false for non-auth errors")
	}
}

func TestIsAuthError_FalseForNil(t *testing.T) {
	if igdb.IsAuthError(nil) {
		t.Error("IsAuthError should return false for nil")
	}
}

func TestValidateCredentials_NotConfigured(t *testing.T) {
	client := igdb.NewClient(&config.Config{}) // no IGDB creds
	err := client.ValidateCredentials(context.Background())
	if !errors.Is(err, igdb.ErrIGDBNotConfigured) {
		t.Errorf("expected ErrIGDBNotConfigured, got %v", err)
	}
}

func TestValidateCredentials_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer ts.Close()

	cfg := &config.Config{
		IGDBClientID:          "test-id",
		IGDBClientSecret:      "test-secret",
		IGDBRequestsPerSecond: 4.0,
		IGDBBurstCapacity:     8,
	}
	client := igdb.NewClientWithTokenURL(cfg, ts.URL)
	err := client.ValidateCredentials(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateCredentials_AuthError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	cfg := &config.Config{
		IGDBClientID:          "bad-id",
		IGDBClientSecret:      "bad-secret",
		IGDBRequestsPerSecond: 4.0,
		IGDBBurstCapacity:     8,
	}
	client := igdb.NewClientWithTokenURL(cfg, ts.URL)
	err := client.ValidateCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
	if !igdb.IsAuthError(err) {
		t.Errorf("expected auth error, got %v", err)
	}
}
