package cliclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetChangelog_QueryModes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":true,"current":"0.90.0","markdown":"## 0.90.0\n","entries":[{"version":"0.90.0","date":"2026-06-20","groups":[{"title":"Features","items":["x"]}]}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL)

	res, err := c.GetChangelog("k", false, "")
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if !res.Available || res.Current != "0.90.0" || len(res.Entries) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if gotQuery != "" {
		t.Fatalf("default mode should send no query, got %q", gotQuery)
	}

	if _, err := c.GetChangelog("k", true, ""); err != nil || gotQuery != "range=all" {
		t.Fatalf("--all query = %q err=%v", gotQuery, err)
	}
	if _, err := c.GetChangelog("k", false, "0.17.1"); err != nil || gotQuery != "since=0.17.1" {
		t.Fatalf("--since query = %q err=%v", gotQuery, err)
	}
}
