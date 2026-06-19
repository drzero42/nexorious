package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndDeleteTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "RPG" {
			t.Errorf("name = %v", body["name"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-1", "name": "RPG"})
	})
	mux.HandleFunc("/api/tags/t-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	tag, err := c.CreateTag("k", "RPG", nil)
	if err != nil || tag.ID != "t-1" {
		t.Fatalf("CreateTag: %v %+v", err, tag)
	}
	if err := c.DeleteTag("k", "t-1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
}
