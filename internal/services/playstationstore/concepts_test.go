package playstationstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResolveConceptID covers the catalog concepts endpoint, whose response is a
// top-level JSON array of concept summaries (NOT an object).
func TestResolveConceptID(t *testing.T) {
	t.Run("array with id returns first concept id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/catalog/v2/titles/CUSA12345_00/concepts" {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			_, _ = w.Write([]byte(`[{"id":"10002694","name":"Some Game"}]`))
		}))
		defer srv.Close()
		c := NewClient()
		c.SetHTTPClient(srv.Client())
		c.SetGamelistURL(srv.URL)

		got, err := c.ResolveConceptID(context.Background(), "tok", "CUSA12345_00")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "10002694" {
			t.Fatalf("concept id = %q, want 10002694", got)
		}
	})

	t.Run("numeric id is accepted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"id":10002694}]`))
		}))
		defer srv.Close()
		c := NewClient()
		c.SetHTTPClient(srv.Client())
		c.SetGamelistURL(srv.URL)

		got, err := c.ResolveConceptID(context.Background(), "tok", "CUSA1_00")
		if err != nil || got != "10002694" {
			t.Fatalf("got (%q,%v), want (10002694,nil)", got, err)
		}
	})

	t.Run("empty array yields no concept", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()
		c := NewClient()
		c.SetHTTPClient(srv.Client())
		c.SetGamelistURL(srv.URL)

		got, err := c.ResolveConceptID(context.Background(), "tok", "CUSA2_00")
		if err != nil || got != "" {
			t.Fatalf("got (%q,%v), want (\"\",nil)", got, err)
		}
	})

	t.Run("404 yields no concept, no error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		c := NewClient()
		c.SetHTTPClient(srv.Client())
		c.SetGamelistURL(srv.URL)

		got, err := c.ResolveConceptID(context.Background(), "tok", "CUSA3_00")
		if err != nil || got != "" {
			t.Fatalf("got (%q,%v), want (\"\",nil)", got, err)
		}
	})
}
