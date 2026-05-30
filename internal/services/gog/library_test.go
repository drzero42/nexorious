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

func TestGetLibrary_SinglePage(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(1001, "Game A", true, false, false)},
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
	if entries[0].ExternalID != "1001" {
		t.Errorf("ExternalID: got %q", entries[0].ExternalID)
	}
	if entries[0].Title != "Game A" {
		t.Errorf("Title: got %q", entries[0].Title)
	}
	if len(entries[0].Platforms) == 0 || entries[0].Platforms[0] != "pc-windows" {
		t.Errorf("Platforms: got %v", entries[0].Platforms)
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

func TestGetLibrary_DualPlatform_EmitsOneEntryWithAllPlatforms(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(2001, "Linux Game", true, false, true)},
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
		t.Fatalf("want 1 entry for dual-platform game, got %d", len(entries))
	}
	if entries[0].ExternalID != "2001" {
		t.Errorf("unexpected ExternalID %q", entries[0].ExternalID)
	}
	platforms := map[string]bool{}
	for _, p := range entries[0].Platforms {
		platforms[p] = true
	}
	if !platforms["pc-windows"] {
		t.Error("expected pc-windows in Platforms")
	}
	if !platforms["pc-linux"] {
		t.Error("expected pc-linux in Platforms")
	}
}

func TestGetLibrary_WindowsOnlyEmitsOneEntry(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(3001, "Windows Only", true, false, false)},
	})
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	var entries []gog.ExternalGameEntry
	_ = c.GetLibrary(context.Background(), "token", 50, func(batch []gog.ExternalGameEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry for Windows-only game, got %d", len(entries))
	}
	if len(entries[0].Platforms) == 0 || entries[0].Platforms[0] != "pc-windows" {
		t.Errorf("Platforms: got %v", entries[0].Platforms)
	}
}

func TestGetLibrary_PlaytimeAlwaysZero(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(4001, "Some Game", true, false, false)},
	})
	defer srv.Close()

	c := gog.NewClientWithURLs(srv.URL, srv.URL)
	var entries []gog.ExternalGameEntry
	_ = c.GetLibrary(context.Background(), "token", 50, func(batch []gog.ExternalGameEntry) error {
		entries = append(entries, batch...)
		return nil
	})
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	if entries[0].PlaytimeHours != 0 {
		t.Errorf("PlaytimeHours should be 0, got %v", entries[0].PlaytimeHours)
	}
}

func TestGetLibrary_MacGameEmitsMacEntry(t *testing.T) {
	srv := makeProductsServer(t, [][]map[string]any{
		{product(5001, "Mac Game", false, true, false)},
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
		t.Fatalf("want 1 entry for Mac-only game, got %d", len(entries))
	}
	if entries[0].ExternalID != "5001" {
		t.Errorf("ExternalID: got %q", entries[0].ExternalID)
	}
	if entries[0].Title != "Mac Game" {
		t.Errorf("Title: got %q", entries[0].Title)
	}
	if len(entries[0].Platforms) == 0 || entries[0].Platforms[0] != "pc-mac" {
		t.Errorf("Platforms: got %v", entries[0].Platforms)
	}
}
