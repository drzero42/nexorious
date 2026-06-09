package gog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/services/gog"
)

func makeProductsServer(t *testing.T, pages [][]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/getFilteredProducts" {
			http.NotFound(w, r)
			return
		}
		pageStr := r.URL.Query().Get("page")
		page := 1
		if pageStr != "" {
			_, _ = fmt.Sscanf(pageStr, "%d", &page)
		}
		if page < 1 || page > len(pages) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalProducts":   len(pages) * 2,
				"numPages":        len(pages),
				"productsPerPage": 2,
				"page":            page,
				"products":        []any{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalProducts":   len(pages) * 2,
			"numPages":        len(pages),
			"productsPerPage": 2,
			"page":            page,
			"products":        pages[page-1],
		})
	}))
}

func product(id int64, title string, windows, mac, linux bool) map[string]any {
	return map[string]any{
		"id":    id,
		"title": title,
		"worksOn": map[string]any{
			"Windows": windows,
			"Mac":     mac,
			"Linux":   linux,
		},
	}
}

func TestGetLibrary_PlatformMapping(t *testing.T) {
	tests := []struct {
		name                string
		id                  int64
		title               string
		windows, mac, linux bool
		wantPlatforms       []string
	}{
		{
			name: "windows only", id: 1001, title: "Game A",
			windows:       true,
			wantPlatforms: []string{"pc-windows"},
		},
		{
			name: "mac only", id: 5001, title: "Mac Game",
			mac:           true,
			wantPlatforms: []string{"mac"},
		},
		{
			name: "windows and linux", id: 2001, title: "Linux Game",
			windows: true, linux: true,
			wantPlatforms: []string{"pc-windows", "pc-linux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := makeProductsServer(t, [][]map[string]any{
				{product(tt.id, tt.title, tt.windows, tt.mac, tt.linux)},
			})
			defer srv.Close()

			c := gog.NewClientWithURLs(srv.URL, srv.URL)
			var entries []gog.ExternalGameEntry
			err := c.GetLibrary(context.Background(), "token", 50, func(batch []gog.ExternalGameEntry) error {
				entries = append(entries, batch...)
				return nil
			})
			if err != nil {
				t.Fatalf("GetLibrary: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("want 1 entry, got %d", len(entries))
			}
			if entries[0].ExternalID != fmt.Sprintf("%d", tt.id) {
				t.Errorf("ExternalID: got %q, want %d", entries[0].ExternalID, tt.id)
			}
			if entries[0].Title != tt.title {
				t.Errorf("Title: got %q, want %q", entries[0].Title, tt.title)
			}
			// Playtime is always zero for GOG (no playtime data in the API).
			if entries[0].PlaytimeHours != 0 {
				t.Errorf("PlaytimeHours: got %v, want 0", entries[0].PlaytimeHours)
			}
			got := map[string]bool{}
			for _, p := range entries[0].Platforms {
				got[p] = true
			}
			if len(got) != len(tt.wantPlatforms) {
				t.Errorf("Platforms: got %v, want %v", entries[0].Platforms, tt.wantPlatforms)
			}
			for _, want := range tt.wantPlatforms {
				if !got[want] {
					t.Errorf("expected %q in Platforms, got %v", want, entries[0].Platforms)
				}
			}
		})
	}
}

func TestGetLibrary_MultiPage(t *testing.T) {
	pages := [][]map[string]any{
		{product(1001, "Game A", true, false, false), product(1002, "Game B", true, false, false)},
		{product(1003, "Game C", true, false, false)},
	}
	srv := makeProductsServer(t, pages)
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	var entries []gog.ExternalGameEntry
	err := c.GetLibrary(context.Background(), "token", 50, func(batch []gog.ExternalGameEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
}
