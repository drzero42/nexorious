package steam_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

func TestSteamAdapter_APIKeyRejected_ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	a := steam.NewAdapter(c, "badkey", "76561198000000001")

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
}

// TestSteamAdapter_RateLimitExhausted_RetriesUntilSuccess verifies that an appid
// repeatedly rate-limited by Steam is retried until it succeeds — we never
// silently drop a game.
func TestSteamAdapter_RateLimitExhausted_RetriesUntilSuccess(t *testing.T) {
	ownedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"response":{"games":[{"appid":12345,"name":"Test Game","playtime_forever":60}]}}`)
	}))
	defer ownedSrv.Close()

	// appdetails returns 429 for the first 4 calls (the adapter sees 2 ErrRateLimited
	// because the client retries inline once before returning), then succeeds on
	// call 5. Retry-After: 0 makes the client's inline retry instant.
	var callCount atomic.Int32
	detailsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		if n <= 4 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"12345":{"success":true,"data":{"platforms":{"windows":true,"mac":false,"linux":false}}}}`)
	}))
	defer detailsSrv.Close()

	c := steam.NewClientForTests(ownedSrv.Client(), rate.NewLimiter(rate.Inf, 1), ownedSrv.URL, detailsSrv.URL)
	a := steam.NewAdapterForTests(c, "key", "steamid", 0)

	var got []storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		got = append(got, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 game in batch, got %d", len(got))
	}
	if len(got) > 0 && got[0].ExternalID != "12345" {
		t.Errorf("expected ExternalID 12345, got %s", got[0].ExternalID)
	}
}

// TestSteamAdapter_AppdetailsHardError_FailsSync verifies that a non-rate-limit
// error from appdetails (e.g. HTTP 500) causes GetLibrary to return an error
// rather than silently dropping the game.
func TestSteamAdapter_AppdetailsHardError_FailsSync(t *testing.T) {
	ownedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"response":{"games":[{"appid":12345,"name":"Test Game","playtime_forever":0}]}}`)
	}))
	defer ownedSrv.Close()

	detailsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer detailsSrv.Close()

	c := steam.NewClientForTests(ownedSrv.Client(), rate.NewLimiter(rate.Inf, 1), ownedSrv.URL, detailsSrv.URL)
	a := steam.NewAdapterForTests(c, "key", "steamid", 0)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil {
		t.Error("expected error for hard appdetails failure, got nil")
	}
}
