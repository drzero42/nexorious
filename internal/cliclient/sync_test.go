package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSyncConfigs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configs": []map[string]any{
				{"id": "sc-1", "storefront": "steam", "frequency": "daily", "is_configured": true,
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
				{"id": "sc-2", "storefront": "gog", "frequency": "weekly", "is_configured": false,
					"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	configs, err := c.ListSyncConfigs("k")
	if err != nil {
		t.Fatalf("ListSyncConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("len(configs) = %d, want 2", len(configs))
	}
	if configs[0].ID != "sc-1" || configs[0].Storefront != "steam" {
		t.Errorf("configs[0] = %+v", configs[0])
	}
	if configs[1].Storefront != "gog" || configs[1].Frequency != "weekly" {
		t.Errorf("configs[1] = %+v", configs[1])
	}
}

func TestUpdateSyncConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config/steam", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["frequency"] != "daily" {
			t.Errorf("frequency = %v, want daily", body["frequency"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "sc-1", "storefront": "steam", "frequency": "daily", "is_configured": true,
			"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	cfg, err := c.UpdateSyncConfig("k", "steam", "daily")
	if err != nil {
		t.Fatalf("UpdateSyncConfig: %v", err)
	}
	if cfg.Frequency != "daily" || cfg.Storefront != "steam" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestGetSyncStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		jobID := "j-42"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"storefront":          "steam",
			"is_syncing":          true,
			"active_job_id":       jobID,
			"external_game_count": 150,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	status, err := c.GetSyncStatus("k", "steam")
	if err != nil {
		t.Fatalf("GetSyncStatus: %v", err)
	}
	if status.Storefront != "steam" || !status.IsSyncing {
		t.Errorf("status = %+v", status)
	}
	if status.ActiveJobID == nil || *status.ActiveJobID != "j-42" {
		t.Errorf("ActiveJobID = %v", status.ActiveJobID)
	}
	if status.ExternalGameCount != 150 {
		t.Errorf("ExternalGameCount = %d, want 150", status.ExternalGameCount)
	}
}

func TestTriggerSync(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":    "sync started",
			"job_id":     "j-99",
			"storefront": "steam",
			"status":     "queued",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	result, err := c.TriggerSync("k", "steam")
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if result.JobID != "j-99" || result.Storefront != "steam" {
		t.Errorf("result = %+v", result)
	}
	if result.Status != "queued" {
		t.Errorf("status = %s, want queued", result.Status)
	}
}

func TestConnectStorefront(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["steam_id"] != "76561198000000001" {
			t.Errorf("steam_id = %v", body["steam_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"steam_username": "testuser",
			"connected":      true,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	resp, err := c.ConnectStorefront("k", "steam", map[string]string{"steam_id": "76561198000000001"})
	if err != nil {
		t.Fatalf("ConnectStorefront: %v", err)
	}
	if resp["steam_username"] != "testuser" {
		t.Errorf("steam_username = %v", resp["steam_username"])
	}
}

func TestDisconnectStorefront(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.DisconnectStorefront("k", "steam"); err != nil {
		t.Fatalf("DisconnectStorefront: %v", err)
	}
}

func TestResetSyncData(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.ResetSyncData("k", "steam"); err != nil {
		t.Fatalf("ResetSyncData: %v", err)
	}
}
