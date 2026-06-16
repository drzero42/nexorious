package igdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestAuthManager_GetAccessToken_Concurrent exercises the token cache under
// concurrent readers and writers. The original lock-free fast path in
// GetAccessToken read am.accessToken / am.expiresAt without holding the lock
// while fetchToken and InvalidateToken wrote them under the lock — a genuine
// data race that this test deterministically surfaces under `-race`.
//
// The production hot path triggers exactly this: igdb.(*Client).SearchGames
// runs its fuzzy + exact queries concurrently and both call GetAccessToken.
func TestAuthManager_GetAccessToken_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(twitchTokenResponse{
			AccessToken: "race-token",
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
		accessToken:  "seed-token",
		expiresAt:    time.Now().Add(time.Hour),
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	// Many concurrent readers (the fast path).
	for range 50 {
		wg.Go(func() {
			for range 200 {
				if _, err := am.GetAccessToken(ctx); err != nil {
					t.Errorf("GetAccessToken: %v", err)
					return
				}
			}
		})
	}

	// Concurrent writers, forcing the cache to be invalidated and refetched
	// while readers are in flight.
	for range 10 {
		wg.Go(func() {
			for range 200 {
				am.InvalidateToken()
			}
		})
	}

	wg.Wait()
}
