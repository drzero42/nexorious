package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runConfigNotify drives newRootCmd with the given args pre-seeded against srvURL.
// Returns stdout+stderr combined and any execution error.
func runConfigNotify(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()
	seedProfile(t, srvURL)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// sampleChannels returns a reusable list of notification channels.
func sampleChannels() []map[string]any {
	return []map[string]any{
		{"id": "ch-1", "name": "Slack", "created_at": "2026-06-18T00:00:00Z"},
		{"id": "ch-2", "name": "Discord", "created_at": "2026-06-17T00:00:00Z"},
	}
}

// sampleEventTypes returns a reusable list of event types.
func sampleEventTypes() []map[string]any {
	return []map[string]any{
		{"type": "sync.completed", "scope": "user", "category": "sync", "label": "Sync completed", "default_on": true},
		{"type": "import.completed", "scope": "user", "category": "import", "label": "Import completed", "default_on": false},
	}
}

func TestNotifyChannelListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(sampleChannels())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "list")
	if err != nil {
		t.Fatalf("channel list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ch-1") || !strings.Contains(out, "ch-2") {
		t.Errorf("table missing ids: %q", out)
	}
	if !strings.Contains(out, "Slack") || !strings.Contains(out, "Discord") {
		t.Errorf("table missing names: %q", out)
	}
}

func TestNotifyChannelListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(sampleChannels())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "list", "-q")
	if err != nil {
		t.Fatalf("channel list -q: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "ch-1" || lines[1] != "ch-2" {
		t.Errorf("quiet ids = %q, want ch-1 and ch-2 on separate lines", out)
	}
}

func TestNotifyChannelCreateSendsNameAndURL(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ch-new", "name": "myhook", "created_at": "2026-06-19T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "create", "myhook", "--url", "https://x")
	if err != nil {
		t.Fatalf("channel create: %v\n%s", err, out)
	}
	if gotBody["name"] != "myhook" {
		t.Errorf("POST body name = %v, want myhook", gotBody["name"])
	}
	if gotBody["url"] != "https://x" {
		t.Errorf("POST body url = %v, want https://x", gotBody["url"])
	}
	if !strings.Contains(out, "ch-new") {
		t.Errorf("output missing channel id: %q", out)
	}
}

func TestNotifyChannelEditNameOnly(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ch-1", "name": "New", "created_at": "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "edit", "ch-1", "--name", "New")
	if err != nil {
		t.Fatalf("channel edit --name: %v\n%s", err, out)
	}
	if gotBody["name"] != "New" {
		t.Errorf("PATCH body name = %v, want New", gotBody["name"])
	}
	if _, present := gotBody["url"]; present {
		t.Errorf("PATCH body must NOT contain url key when only --name changed; got %v", gotBody)
	}
}

func TestNotifyChannelEditURLOnly(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ch-1", "name": "Slack", "created_at": "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "edit", "ch-1", "--url", "https://y")
	if err != nil {
		t.Fatalf("channel edit --url: %v\n%s", err, out)
	}
	if gotBody["url"] != "https://y" {
		t.Errorf("PATCH body url = %v, want https://y", gotBody["url"])
	}
	if _, present := gotBody["name"]; present {
		t.Errorf("PATCH body must NOT contain name key when only --url changed; got %v", gotBody)
	}
}

func TestNotifyChannelEditNoFlagsErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "edit", "ch-1")
	if err == nil {
		t.Fatal("expected error when no flags given to channel edit")
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("error = %v, want 'nothing to update'", err)
	}
}

func TestNotifyChannelRmConfirmed(t *testing.T) {
	var deleted bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "rm", "ch-1", "-y")
	if err != nil {
		t.Fatalf("channel rm -y: %v\n%s", err, out)
	}
	if !deleted {
		t.Fatal("DELETE not received")
	}
	if !strings.Contains(out, "removed channel ch-1") {
		t.Errorf("output = %q, want 'removed channel ch-1'", out)
	}
}

func TestNotifyChannelRmAborted(t *testing.T) {
	var deleteHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-1", func(w http.ResponseWriter, _ *http.Request) {
		deleteHit = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "rm", "ch-1")
	if err != nil {
		t.Fatalf("channel rm (no -y): %v\n%s", err, out)
	}
	if deleteHit {
		t.Fatal("DELETE must not be sent when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestNotifyChannelTest(t *testing.T) {
	var testHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/channels/ch-1/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		testHit = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "channel", "test", "ch-1")
	if err != nil {
		t.Fatalf("channel test: %v\n%s", err, out)
	}
	if !testHit {
		t.Fatal("POST /channels/ch-1/test not received")
	}
	if !strings.Contains(out, "test notification sent") {
		t.Errorf("output = %q, want 'test notification sent'", out)
	}
}

func TestNotifyTestURL(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "test-url", "--url", "https://z")
	if err != nil {
		t.Fatalf("test-url: %v\n%s", err, out)
	}
	if gotBody["url"] != "https://z" {
		t.Errorf("POST body url = %v, want https://z", gotBody["url"])
	}
	if !strings.Contains(out, "test notification sent") {
		t.Errorf("output = %q, want 'test notification sent'", out)
	}
}

func TestNotifySubListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"sync.completed", "import.completed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "sub", "list")
	if err != nil {
		t.Fatalf("sub list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sync.completed") || !strings.Contains(out, "import.completed") {
		t.Errorf("output missing event types: %q", out)
	}
}

func TestNotifySubListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"sync.completed", "import.completed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "sub", "list", "-q")
	if err != nil {
		t.Fatalf("sub list -q: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "sync.completed" || lines[1] != "import.completed" {
		t.Errorf("quiet output = %q, want two bare event type lines", out)
	}
}

func TestNotifySubSetSendsBody(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"a", "b"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "sub", "set", "a", "b")
	if err != nil {
		t.Fatalf("sub set: %v\n%s", err, out)
	}
	types, _ := gotBody["event_types"].([]any)
	if len(types) != 2 || types[0] != "a" || types[1] != "b" {
		t.Errorf("PUT body event_types = %v, want [a b]", gotBody["event_types"])
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("output missing result types: %q", out)
	}
}

func TestNotifySubReset(t *testing.T) {
	var resetHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		resetHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"event_types": []string{"sync.completed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "sub", "reset")
	if err != nil {
		t.Fatalf("sub reset: %v\n%s", err, out)
	}
	if !resetHit {
		t.Fatal("POST /subscriptions/reset not received")
	}
	if !strings.Contains(out, "sync.completed") {
		t.Errorf("output = %q, want default event types", out)
	}
}

func TestNotifyEventsTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/event-types", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(sampleEventTypes())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "events")
	if err != nil {
		t.Fatalf("events: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sync.completed") || !strings.Contains(out, "import.completed") {
		t.Errorf("table missing event types: %q", out)
	}
	if !strings.Contains(out, "Sync completed") {
		t.Errorf("table missing label: %q", out)
	}
}

func TestNotifyEventsQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/event-types", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(sampleEventTypes())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfigNotify(t, srv.URL, "config", "notify", "events", "-q")
	if err != nil {
		t.Fatalf("events -q: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "sync.completed" || lines[1] != "import.completed" {
		t.Errorf("quiet output = %q, want two bare type lines", out)
	}
}

func TestNotifySubSetUnknownTypeErrorSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/notifications/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unknown event type: bogus"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runConfigNotify(t, srv.URL, "config", "notify", "sub", "set", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
	if !strings.Contains(err.Error(), "unknown event type: bogus") {
		t.Errorf("error = %v, want it to mention 'unknown event type: bogus'", err)
	}
}
