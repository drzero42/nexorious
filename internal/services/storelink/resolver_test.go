package storelink

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSteamResolver(t *testing.T) {
	got, err := NewSteamResolver().Resolve(context.Background(), "440", nil)
	if err != nil || got != "440" {
		t.Fatalf("steam resolve = (%q,%v), want (440,nil)", got, err)
	}
}

func TestGOGResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/12345" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":12345,"slug":"the-witcher-3-wild-hunt"}`))
	}))
	defer srv.Close()
	r := NewGOGResolver(srv.Client(), srv.URL)
	got, err := r.Resolve(context.Background(), "12345", nil)
	if err != nil || got != "the-witcher-3-wild-hunt" {
		t.Fatalf("gog resolve = (%q,%v)", got, err)
	}
}

func TestEpicResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ns-xyz":"fortnite","ns-abc":"other"}`))
	}))
	defer srv.Close()
	r := NewEpicResolver(srv.Client(), srv.URL)
	got, err := r.Resolve(context.Background(), "app123", map[string]string{"namespace": "ns-xyz"})
	if err != nil || got != "fortnite" {
		t.Fatalf("epic resolve = (%q,%v)", got, err)
	}
	got, err = r.Resolve(context.Background(), "app456", map[string]string{"namespace": "missing"})
	if err != nil || got != "" {
		t.Fatalf("epic resolve missing = (%q,%v), want empty", got, err)
	}
}
