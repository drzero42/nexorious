package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/drzero42/nexorious/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want application/vnd.github+json", got)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected a User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.10.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v0.10.0"}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL)
	rel, err := c.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.TagName != "v0.10.0" {
		t.Errorf("TagName = %q, want v0.10.0", rel.TagName)
	}
	if rel.HTMLURL != "https://github.com/drzero42/nexorious/releases/tag/v0.10.0" {
		t.Errorf("HTMLURL = %q", rel.HTMLURL)
	}
}

func TestFetchLatest_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestFetchLatest_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestFetchLatest_EmptyTagName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"","html_url":"x"}`))
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on empty tag_name, got nil")
	}
}
