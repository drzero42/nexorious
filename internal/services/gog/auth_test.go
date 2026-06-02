package gog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/services/gog"
)

func TestBuildAuthURL(t *testing.T) {
	c := gog.NewClient()
	u := c.BuildAuthURL()
	if u == "" {
		t.Fatal("expected non-empty auth URL")
	}
	const wantPrefix = "https://login.gog.com/auth"
	if len(u) < len(wantPrefix) || u[:len(wantPrefix)] != wantPrefix {
		t.Errorf("auth URL should start with %s, got %s", wantPrefix, u)
	}
}

func TestExchangeCode_Success(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "authorization_code" {
			http.Error(w, "wrong grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-abc",
			"refresh_token": "refresh-xyz",
			"expires_in":    3600,
			"user_id":       "12345",
		})
	}))
	defer tokenSrv.Close()

	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userData.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":   "12345",
			"username": "TestUser",
		})
	}))
	defer embedSrv.Close()

	c := gog.NewClientWithURLs(tokenSrv.URL, embedSrv.URL)
	tok, err := c.ExchangeCode(context.Background(), "code-abc")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok.AccessToken != "access-abc" {
		t.Errorf("AccessToken: got %q", tok.AccessToken)
	}
	if tok.RefreshToken != "refresh-xyz" {
		t.Errorf("RefreshToken: got %q", tok.RefreshToken)
	}
	if tok.UserID != "12345" {
		t.Errorf("UserID: got %q", tok.UserID)
	}
	if tok.Username != "TestUser" {
		t.Errorf("Username: got %q", tok.Username)
	}
}

func TestExchangeCode_TokenEndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad code", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	_, err := c.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error for non-200 token response")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "wrong grant_type", http.StatusBadRequest)
			return
		}
		if r.FormValue("refresh_token") != "old-refresh" {
			http.Error(w, "wrong token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"user_id":       "12345",
		})
	}))
	defer tokenSrv.Close()

	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"userId": "12345", "username": "TestUser"})
	}))
	defer embedSrv.Close()

	c := gog.NewClientWithURLs(tokenSrv.URL, embedSrv.URL)
	tok, err := c.RefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken: got %q", tok.AccessToken)
	}
	if tok.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken: got %q", tok.RefreshToken)
	}
}

func TestRefreshToken_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "expired", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	_, err := c.RefreshToken(context.Background(), "expired-token")
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}
}

func TestParseAuthCode(t *testing.T) {
	const fullURL = "https://embed.gog.com/on_login_success?origin=client&code=XXX"

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"bare code", "XXX", "XXX", false},
		{"full url", fullURL, "XXX", false},
		{"reordered params", "https://embed.gog.com/on_login_success?code=XXX&origin=client", "XXX", false},
		{"extra params", "https://embed.gog.com/on_login_success?origin=client&code=XXX&foo=bar", "XXX", false},
		{"uppercase host", "https://EMBED.GOG.COM/on_login_success?code=XXX", "XXX", false},
		{"whitespace around bare code", "  XXX  ", "XXX", false},
		{"whitespace around url", "  " + fullURL + "  ", "XXX", false},
		{"wrong host", "https://evil.example.com/on_login_success?code=XXX", "", true},
		{"missing code", "https://embed.gog.com/on_login_success?origin=client", "", true},
		{"trailing slash path", "https://embed.gog.com/on_login_success/?code=XXX", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gog.ParseAuthCode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got code %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
