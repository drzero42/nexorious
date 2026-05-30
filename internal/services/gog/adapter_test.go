package gog

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeGOGClient satisfies clientInterface without making real HTTP calls.
type fakeGOGClient struct {
	refreshResult  *TokenResponse
	refreshErr     error
	libraryErr     error
	libraryEntries []ExternalGameEntry

	refreshCalled         bool
	getLibraryCalled      bool
	refreshInput          string
	getLibraryAccessToken string
}

func (f *fakeGOGClient) RefreshToken(_ context.Context, refreshToken string) (*TokenResponse, error) {
	f.refreshCalled = true
	f.refreshInput = refreshToken
	return f.refreshResult, f.refreshErr
}

func (f *fakeGOGClient) GetLibrary(_ context.Context, accessToken string, _ int, onBatch func([]ExternalGameEntry) error) error {
	f.getLibraryCalled = true
	f.getLibraryAccessToken = accessToken
	if f.libraryErr != nil {
		return f.libraryErr
	}
	if len(f.libraryEntries) > 0 {
		return onBatch(f.libraryEntries)
	}
	return nil
}

func TestGOGAdapter_RefreshesTokenBeforeFetch(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	a := NewAdapter(fake, "old-refresh", nil)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.refreshCalled {
		t.Error("expected RefreshToken to be called")
	}
	if fake.refreshInput != "old-refresh" {
		t.Errorf("expected refresh input 'old-refresh', got %q", fake.refreshInput)
	}
	if fake.getLibraryAccessToken != "new-access" {
		t.Errorf("expected GetLibrary called with 'new-access', got %q", fake.getLibraryAccessToken)
	}
}

func TestGOGAdapter_CallsOnNewTokensAfterRefresh(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	var gotAccess, gotRefresh string
	onNewTokens := func(access, refresh string) error {
		gotAccess = access
		gotRefresh = refresh
		return nil
	}
	a := NewAdapter(fake, "old-refresh", onNewTokens)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAccess != "new-access" || gotRefresh != "new-refresh" {
		t.Errorf("onNewTokens got (%q, %q), want ('new-access', 'new-refresh')", gotAccess, gotRefresh)
	}
}

func TestGOGAdapter_AuthExpiredReturnsCredentialsError(t *testing.T) {
	fake := &fakeGOGClient{refreshErr: ErrGOGAuthExpired}
	a := NewAdapter(fake, "expired-refresh", nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected ErrCredentials, got %v", err)
	}
	if fake.getLibraryCalled {
		t.Error("expected GetLibrary not to be called on auth failure")
	}
}

func TestGOGAdapter_TransientRefreshErrorPropagates(t *testing.T) {
	transientErr := errors.New("connection refused")
	fake := &fakeGOGClient{refreshErr: transientErr}
	a := NewAdapter(fake, "old-refresh", nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, transientErr) {
		t.Errorf("expected transient error to propagate, got %v", err)
	}
	if errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Error("expected non-credentials error for transient failure")
	}
}

func TestGOGAdapter_OnNewTokensFailureDoesNotAbortFetch(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	onNewTokens := func(_, _ string) error {
		return errors.New("db write failed")
	}
	a := NewAdapter(fake, "old-refresh", onNewTokens)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("expected no error when persist fails, got %v", err)
	}
	if !fake.getLibraryCalled {
		t.Error("expected GetLibrary to still be called after persist failure")
	}
}
