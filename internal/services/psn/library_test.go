package psn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestFetchPlayHistory_HappyPath(t *testing.T) {
	type responseBody struct {
		Titles []struct {
			TitleID      string `json:"titleId"`
			Name         string `json:"name"`
			Category     string `json:"category"`
			Service      string `json:"service"`
			PlayDuration string `json:"playDuration"`
		} `json:"titles"`
		NextOffset     int `json:"nextOffset"`
		TotalItemCount int `json:"totalItemCount"`
	}

	// 3 titles: PS5 owned, PS4 PS Plus, pspc_game (excluded)
	// totalItemCount=2 so only 1 HTTP call is made (offset 0 + limit 200 >= 2)
	body := responseBody{
		Titles: []struct {
			TitleID      string `json:"titleId"`
			Name         string `json:"name"`
			Category     string `json:"category"`
			Service      string `json:"service"`
			PlayDuration string `json:"playDuration"`
		}{
			{TitleID: "PPSA07950_00", Name: "Call of Duty®", Category: "ps5_native_game", Service: "none(purchased)", PlayDuration: "PT340H46M13S"},
			{TitleID: "CUSA12345_00", Name: "Some PS4 Game", Category: "ps4_game", Service: "ps_plus_premium", PlayDuration: "PT12H30M00S"},
			{TitleID: "PCPC99999_00", Name: "PC Game", Category: "pspc_game", Service: "none(purchased)", PlayDuration: "PT5H"},
		},
		NextOffset:     0,
		TotalItemCount: 2,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)

	result, err := c.fetchPlayHistory(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	ps5, ok := result["PPSA07950_00"]
	if !ok {
		t.Fatal("expected entry for PPSA07950_00")
	}
	if len(ps5.Platforms) == 0 || ps5.Platforms[0] != "playstation-5" {
		t.Errorf("expected Platforms[0]=playstation-5, got %v", ps5.Platforms)
	}
	if ps5.PlaytimeHours != 340 {
		t.Errorf("expected PlaytimeHours=340, got %v", ps5.PlaytimeHours)
	}
	if ps5.OwnershipStatus != "owned" {
		t.Errorf("expected OwnershipStatus=owned, got %q", ps5.OwnershipStatus)
	}
	if ps5.IsSubscription {
		t.Error("expected IsSubscription=false for PS5 entry")
	}

	ps4, ok := result["CUSA12345_00"]
	if !ok {
		t.Fatal("expected entry for CUSA12345_00")
	}
	if len(ps4.Platforms) == 0 || ps4.Platforms[0] != "playstation-4" {
		t.Errorf("expected Platforms[0]=playstation-4, got %v", ps4.Platforms)
	}
	if ps4.OwnershipStatus != "subscription" {
		t.Errorf("expected OwnershipStatus=subscription, got %q", ps4.OwnershipStatus)
	}
	if !ps4.IsSubscription {
		t.Error("expected IsSubscription=true for PS4 PS Plus entry")
	}

	// pspc_game entry must be excluded
	if _, ok := result["PCPC99999_00"]; ok {
		t.Error("expected pspc_game entry to be excluded")
	}
}

func TestFetchPlayHistory_Pagination(t *testing.T) {
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		offset := r.URL.Query().Get("offset")

		type titleEntry struct {
			TitleID      string `json:"titleId"`
			Name         string `json:"name"`
			Category     string `json:"category"`
			Service      string `json:"service"`
			PlayDuration string `json:"playDuration"`
		}
		type respBody struct {
			Titles         []titleEntry `json:"titles"`
			NextOffset     int          `json:"nextOffset"`
			TotalItemCount int          `json:"totalItemCount"`
		}

		var body respBody
		if offset == "0" || offset == "" {
			body = respBody{
				Titles: []titleEntry{
					{TitleID: "PPSA00001_00", Name: "Game One", Category: "ps5_native_game", Service: "none(purchased)", PlayDuration: "PT10H"},
				},
				NextOffset:     200,
				TotalItemCount: 201,
			}
		} else {
			body = respBody{
				Titles: []titleEntry{
					{TitleID: "PPSA00002_00", Name: "Game Two", Category: "ps4_game", Service: "none(purchased)", PlayDuration: "PT5H"},
				},
				NextOffset:     0,
				TotalItemCount: 201,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))

	result, err := c.fetchPlayHistory(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestFetchPlayHistory_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)

	_, err := c.fetchPlayHistory(context.Background(), "test-token")
	if err == nil {
		t.Fatal("expected non-nil error for HTTP 503")
	}
}

func TestFetchPurchasedGames_HappyPath(t *testing.T) {
	body := `{"data":{"purchasedTitlesRetrieve":{"games":[
		{"titleId":"CUSA10410_00","name":"CODE VEIN","platform":"PS4","subscriptionService":"PS_PLUS"},
		{"titleId":"PPSA01234_00","name":"Demon's Souls","platform":"PS5","subscriptionService":"NONE"},
		{"titleId":"PCPC00001_00","name":"PC Game","platform":"PC","subscriptionService":"NONE"}
	]}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	result, err := c.fetchPurchasedGames(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries (PC excluded), got %d", len(result))
	}

	ps4, ok := result["CUSA10410_00"]
	if !ok {
		t.Fatal("expected entry for CUSA10410_00")
	}
	if len(ps4.Platforms) == 0 || ps4.Platforms[0] != "playstation-4" {
		t.Errorf("expected Platforms[0]=playstation-4, got %v", ps4.Platforms)
	}
	if !ps4.IsSubscription {
		t.Error("expected IsSubscription=true for PS_PLUS game")
	}
	if ps4.OwnershipStatus != "subscription" {
		t.Errorf("expected OwnershipStatus=subscription, got %q", ps4.OwnershipStatus)
	}
	if ps4.PlaytimeHours != 0 {
		t.Errorf("expected PlaytimeHours=0, got %v", ps4.PlaytimeHours)
	}

	ps5, ok := result["PPSA01234_00"]
	if !ok {
		t.Fatal("expected entry for PPSA01234_00")
	}
	if len(ps5.Platforms) == 0 || ps5.Platforms[0] != "playstation-5" {
		t.Errorf("expected Platforms[0]=playstation-5, got %v", ps5.Platforms)
	}
	if ps5.IsSubscription {
		t.Error("expected IsSubscription=false for non-PS_PLUS game")
	}
	if ps5.OwnershipStatus != "owned" {
		t.Errorf("expected OwnershipStatus=owned, got %q", ps5.OwnershipStatus)
	}
}

func TestFetchPurchasedGames_GraphQLSchemaChanged(t *testing.T) {
	body := `{"data":{}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	_, err := c.fetchPurchasedGames(context.Background(), "test-token")
	if !errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Fatalf("expected ErrPSNGraphQLSchemaChanged, got %v", err)
	}
}

func TestFetchPurchasedGames_Pagination(t *testing.T) {
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body string
		if callCount == 1 {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00001_00","name":"Game A","platform":"PS4","subscriptionService":"NONE"},
				{"titleId":"CUSA00002_00","name":"Game B","platform":"PS4","subscriptionService":"NONE"}
			]}}}`
		} else {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00003_00","name":"Game C","platform":"PS5","subscriptionService":"NONE"}
			]}}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)
	c.SetGraphQLPageSize(2)
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))

	result, err := c.fetchPurchasedGames(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 total entries, got %d", len(result))
	}
}

func TestFetchPurchasedGames_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)

	_, err := c.fetchPurchasedGames(context.Background(), "test-token")
	if err == nil {
		t.Fatal("expected non-nil error for HTTP 500")
	}
	if errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Fatal("expected error to NOT be ErrPSNGraphQLSchemaChanged for HTTP 500")
	}
}

// ---------------------------------------------------------------------------
// mergePlayedPurchased
// ---------------------------------------------------------------------------

func TestMergePlayedPurchased_UnionBothSources(t *testing.T) {
	played := map[string]ExternalGameEntry{
		"DISC1": {ExternalID: "DISC1", Title: "Disc Game", Platforms: []string{"playstation-4"}, PlaytimeHours: 5, OwnershipStatus: "owned"},
	}
	purchased := map[string]ExternalGameEntry{
		"DL1": {ExternalID: "DL1", Title: "Digital", Platforms: []string{"playstation-5"}, PlaytimeHours: 0, OwnershipStatus: "owned"},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(result))
	}
}

func TestMergePlayedPurchased_PurchasedUpgradesSubscription(t *testing.T) {
	played := map[string]ExternalGameEntry{
		"GAME1": {ExternalID: "GAME1", Platforms: []string{"playstation-4"}, PlaytimeHours: 10, OwnershipStatus: "owned", IsSubscription: false},
	}
	purchased := map[string]ExternalGameEntry{
		"GAME1": {ExternalID: "GAME1", Platforms: []string{"playstation-4"}, PlaytimeHours: 0, OwnershipStatus: "subscription", IsSubscription: true},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	e := result[0]
	if !e.IsSubscription || e.OwnershipStatus != "subscription" {
		t.Errorf("expected subscription after merge, got IsSubscription=%v OwnershipStatus=%q", e.IsSubscription, e.OwnershipStatus)
	}
	if e.PlaytimeHours != 10 {
		t.Errorf("expected playtime preserved from play history (10), got %v", e.PlaytimeHours)
	}
}

func TestMergePlayedPurchased_PurchasedDoesNotDowngradeOwnership(t *testing.T) {
	played := map[string]ExternalGameEntry{
		"GAME1": {ExternalID: "GAME1", Platforms: []string{"playstation-4"}, PlaytimeHours: 5, OwnershipStatus: "owned", IsSubscription: false},
	}
	purchased := map[string]ExternalGameEntry{
		"GAME1": {ExternalID: "GAME1", Platforms: []string{"playstation-4"}, PlaytimeHours: 0, OwnershipStatus: "owned", IsSubscription: false},
	}
	result := mergePlayedPurchased(played, purchased)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].OwnershipStatus != "owned" || result[0].IsSubscription {
		t.Errorf("ownership should remain owned, got %q / %v", result[0].OwnershipStatus, result[0].IsSubscription)
	}
}

func TestMergePlayedPurchased_DiscGameNotInPurchased(t *testing.T) {
	played := map[string]ExternalGameEntry{
		"DISC1": {ExternalID: "DISC1", Platforms: []string{"playstation-4"}, PlaytimeHours: 3, OwnershipStatus: "owned"},
	}
	result := mergePlayedPurchased(played, map[string]ExternalGameEntry{})
	if len(result) != 1 || result[0].ExternalID != "DISC1" {
		t.Errorf("expected disc game in result, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// GetLibrary end-to-end (authFn injected)
// ---------------------------------------------------------------------------

func TestGetLibrary_MergesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/gamelist"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"titles": []map[string]any{
					{"titleId": "DISC1", "name": "Disc Game", "category": "ps4_game", "service": "none(purchased)", "playDuration": "PT5H"},
				},
				"totalItemCount": 1,
			})
		case strings.HasPrefix(r.URL.Path, "/api/graphql"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"purchasedTitlesRetrieve": map[string]any{
						"games": []map[string]any{
							{"titleId": "DL1", "name": "Digital Game", "platform": "PS5", "subscriptionService": "NONE"},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
	c.SetGraphQLURL(srv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "test-token", nil })
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))

	var total int
	err := c.GetLibrary(context.Background(), "fake-npsso", 10, func(batch []ExternalGameEntry) error {
		total += len(batch)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 merged entries (disc + digital), got %d", total)
	}
}

func TestGetLibrary_PlayHistoryError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
	c.SetGraphQLURL(srv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "tok", nil })

	err := c.GetLibrary(context.Background(), "npsso", 10, func([]ExternalGameEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when gamelist endpoint fails, got nil")
	}
}

func TestGetLibrary_GraphQLSchemaChanged_ReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/gamelist"):
			_ = json.NewEncoder(w).Encode(map[string]any{"titles": []any{}, "totalItemCount": 0})
		case strings.HasPrefix(r.URL.Path, "/api/graphql"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
	c.SetGraphQLURL(srv.URL)
	c.SetAuthFn(func(_ context.Context, _ string) (string, error) { return "tok", nil })
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))

	err := c.GetLibrary(context.Background(), "npsso", 10, func([]ExternalGameEntry) error { return nil })
	if !errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		t.Errorf("expected ErrPSNGraphQLSchemaChanged, got %v", err)
	}
}

// TestFetchPurchasedGames_RateLimiterWaitsBetweenPages covers the spec
// invariant from docs/sync.md § PSN: the adapter applies a conservative
// request delay between pages. With a 50ms-per-token limiter and 3 page
// fetches, the second and third calls each wait one token => total
// elapsed time must be >= 100ms.
func TestFetchPurchasedGames_RateLimiterWaitsBetweenPages(t *testing.T) {
	// Server emits 2 games per call for the first two calls and 1 game for
	// the third (so the < size condition breaks the loop after the third).
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		var body string
		if callCount < 3 {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00001_00","name":"Game A","platform":"PS5","subscriptionService":"NONE"},
				{"titleId":"CUSA00002_00","name":"Game B","platform":"PS5","subscriptionService":"NONE"}
			]}}}`
		} else {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00003_00","name":"Game C","platform":"PS5","subscriptionService":"NONE"}
			]}}}`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)
	c.SetGraphQLPageSize(2)
	c.SetLimiter(rate.NewLimiter(rate.Every(50*time.Millisecond), 1))

	start := time.Now()
	if _, err := c.fetchPurchasedGames(context.Background(), "test-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if callCount != 3 {
		t.Fatalf("expected 3 HTTP calls, got %d", callCount)
	}
	// First call passes through immediately (bucket starts full); the next
	// two each wait ~50ms => >= 100ms total.
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected elapsed >= 100ms, got %v (limiter not consulted between pages?)", elapsed)
	}
}
