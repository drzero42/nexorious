package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoolCreateWithFilter(t *testing.T) {
	var gotFilter any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotFilter = b["filter"]
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "p-1", "name": "RPGs"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "create", "RPGs", "--filter", `{"filters":[{"genre":["RPG"]}]}`})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool create: %v\n%s", err, out.String())
	}
	if gotFilter == nil {
		t.Fatal("filter not forwarded")
	}
}

func TestPoolCreateInvalidFilterJSON(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"pool", "create", "X", "--filter", "{not json"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected invalid --filter JSON error before any network call")
	}
}
