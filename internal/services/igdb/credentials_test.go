package igdb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"auth failure", fmt.Errorf("%w: Twitch returned status 403", igdb.ErrTwitchAuth), true},
		{"network error", fmt.Errorf("connection refused"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := igdb.IsAuthError(tt.err); got != tt.want {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestValidateCredentials_NotConfigured(t *testing.T) {
	client := igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)) // no IGDB creds
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
	client := igdb.NewClientWithTokenURL(cfg, ts.URL, ratelimit.NewLocal(100, 100))
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
	client := igdb.NewClientWithTokenURL(cfg, ts.URL, ratelimit.NewLocal(100, 100))
	err := client.ValidateCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid credentials")
	}
	if !igdb.IsAuthError(err) {
		t.Errorf("expected auth error, got %v", err)
	}
}
