package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Settings ---

func TestGetSettings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "us"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	s, err := c.GetSettings("k")
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.DealRegion != "us" {
		t.Errorf("DealRegion = %q, want us", s.DealRegion)
	}
}

func TestUpdateSettings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["deal_region"] != "eu" {
			t.Errorf("deal_region = %v, want eu", body["deal_region"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "eu"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	s, err := c.UpdateSettings("k", "eu")
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if s.DealRegion != "eu" {
		t.Errorf("DealRegion = %q, want eu", s.DealRegion)
	}
}

// --- Channels ---

func TestListChannels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "ch-1", "name": "my-webhook", "created_at": "2026-01-01T00:00:00Z"},
			{"id": "ch-2", "name": "slack-alerts", "created_at": "2026-02-01T00:00:00Z"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	channels, err := c.ListChannels("k")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("len(channels) = %d, want 2", len(channels))
	}
	if channels[0].ID != "ch-1" || channels[0].Name != "my-webhook" {
		t.Errorf("channels[0] = %+v", channels[0])
	}
	if channels[1].ID != "ch-2" || channels[1].Name != "slack-alerts" {
		t.Errorf("channels[1] = %+v", channels[1])
	}
}

func TestCreateChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "my-hook" {
			t.Errorf("name = %v, want my-hook", body["name"])
		}
		if body["url"] != "https://example.com/hook" {
			t.Errorf("url = %v, want https://example.com/hook", body["url"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ch-new", "name": "my-hook", "created_at": "2026-06-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ch, err := c.CreateChannel("k", "my-hook", "https://example.com/hook")
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if ch.ID != "ch-new" || ch.Name != "my-hook" {
		t.Errorf("channel = %+v", ch)
	}
}

func TestUpdateChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// Only name should be present in this partial update — url must not be sent.
		if _, ok := body["name"]; !ok {
			t.Error("name missing from body")
		}
		if body["name"] != "renamed" {
			t.Errorf("name = %v, want renamed", body["name"])
		}
		if _, ok := body["url"]; ok {
			t.Error("unexpected url key in partial update body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ch-7", "name": "renamed", "created_at": "2026-06-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ch, err := c.UpdateChannel("k", "ch-7", map[string]any{"name": "renamed"})
	if err != nil {
		t.Fatalf("UpdateChannel: %v", err)
	}
	if ch.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", ch.Name)
	}
}

func TestDeleteChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-del", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer k")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.DeleteChannel("k", "ch-del"); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
}

func TestTestChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-9/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.TestChannel("k", "ch-9"); err != nil {
		t.Fatalf("TestChannel: %v", err)
	}
}

func TestTestURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["url"] != "https://example.com/test-hook" {
			t.Errorf("url = %v, want https://example.com/test-hook", body["url"])
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.TestURL("k", "https://example.com/test-hook"); err != nil {
		t.Fatalf("TestURL: %v", err)
	}
}

// --- Event types ---

func TestListEventTypes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/event-types", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"type": "job.completed", "scope": "user", "category": "jobs",
				"label": "Job completed", "default_on": true,
			},
			{
				"type": "backup.failed", "scope": "admin", "category": "backup",
				"label": "Backup failed", "default_on": false,
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	types, err := c.ListEventTypes("k")
	if err != nil {
		t.Fatalf("ListEventTypes: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("len(types) = %d, want 2", len(types))
	}
	if types[0].Type != "job.completed" || types[0].Category != "jobs" {
		t.Errorf("types[0] = %+v", types[0])
	}
	if !types[0].DefaultOn {
		t.Errorf("types[0].DefaultOn = false, want true")
	}
	if types[1].Type != "backup.failed" || types[1].Scope != "admin" {
		t.Errorf("types[1] = %+v", types[1])
	}
	if types[1].DefaultOn {
		t.Errorf("types[1].DefaultOn = true, want false")
	}
}

// --- Subscriptions ---

func TestListSubscriptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"job.completed", "sync.failed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	types, err := c.ListSubscriptions("k")
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("len(types) = %d, want 2", len(types))
	}
	if types[0] != "job.completed" || types[1] != "sync.failed" {
		t.Errorf("types = %v", types)
	}
}

func TestPutSubscriptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		raw, ok := body["event_types"]
		if !ok {
			t.Fatal("event_types missing from body")
		}
		arr, ok := raw.([]any)
		if !ok || len(arr) != 1 || arr[0] != "backup.failed" {
			t.Errorf("event_types = %v, want [backup.failed]", raw)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"backup.failed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	types, err := c.PutSubscriptions("k", []string{"backup.failed"})
	if err != nil {
		t.Fatalf("PutSubscriptions: %v", err)
	}
	if len(types) != 1 || types[0] != "backup.failed" {
		t.Errorf("types = %v, want [backup.failed]", types)
	}
}

func TestResetSubscriptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"job.completed", "sync.failed", "backup.failed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	types, err := c.ResetSubscriptions("k")
	if err != nil {
		t.Fatalf("ResetSubscriptions: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("len(types) = %d, want 3", len(types))
	}
	if types[0] != "job.completed" {
		t.Errorf("types[0] = %q, want job.completed", types[0])
	}
}

// --- Error surfacing ---

func TestDeleteChannel_ErrorSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-missing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "channel not found"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	err := c.DeleteChannel("k", "ch-missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "channel not found") {
		t.Errorf("error = %v, want it to surface the server message", err)
	}
}
