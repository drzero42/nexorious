package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveJobItem(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/job-items/ji-42/resolve", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.ResolveJobItem("k", "ji-42", 9999); err != nil {
		t.Fatalf("ResolveJobItem: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/job-items/ji-42/resolve" {
		t.Errorf("path = %q, want /api/job-items/ji-42/resolve", gotPath)
	}
	if igdbID, ok := gotBody["igdb_id"].(float64); !ok || int(igdbID) != 9999 {
		t.Errorf("igdb_id = %v, want 9999", gotBody["igdb_id"])
	}
}

func TestResolveJobItemPathEscaping(t *testing.T) {
	var gotRequestURI string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/job-items/", func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.ResolveJobItem("k", "id with spaces", 1); err != nil {
		t.Fatalf("ResolveJobItem path escape: %v", err)
	}
	if gotRequestURI != "/api/job-items/id%20with%20spaces/resolve" {
		t.Errorf("request URI = %q, want /api/job-items/id%%20with%%20spaces/resolve", gotRequestURI)
	}
}

func TestSkipJobItem(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/job-items/ji-7/skip", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.SkipJobItem("k", "ji-7"); err != nil {
		t.Fatalf("SkipJobItem: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/job-items/ji-7/skip" {
		t.Errorf("path = %q, want /api/job-items/ji-7/skip", gotPath)
	}
}
