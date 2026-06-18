package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["username"] != "alice" {
			t.Errorf("username = %v, want alice", body["username"])
		}
		if body["password"] != "s3cr3tpw" {
			t.Errorf("password = %v, want s3cr3tpw", body["password"])
		}
		if body["is_admin"] != true {
			t.Errorf("is_admin = %v, want true", body["is_admin"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "u-1", "username": "alice", "is_active": true, "is_admin": true,
			"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	user, err := c.CreateUser("k", "alice", "s3cr3tpw", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID != "u-1" || user.Username != "alice" {
		t.Errorf("user = %+v", user)
	}
	if !user.IsActive || !user.IsAdmin {
		t.Errorf("expected IsActive=true IsAdmin=true, got %+v", user)
	}
}

func TestListUsers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "u-2", "username": "bob", "is_active": true, "is_admin": false,
				"created_at": "2026-06-01T00:00:00Z", "updated_at": "2026-06-01T00:00:00Z"},
			{"id": "u-1", "username": "alice", "is_active": true, "is_admin": true,
				"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	users, err := c.ListUsers("k")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}
	if users[0].ID != "u-2" || users[0].Username != "bob" {
		t.Errorf("users[0] = %+v", users[0])
	}
	if users[1].ID != "u-1" || users[1].IsAdmin != true {
		t.Errorf("users[1] = %+v", users[1])
	}
}

func TestGetUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "u-42", "username": "carol", "is_active": true, "is_admin": false,
			"created_at": "2026-03-01T00:00:00Z", "updated_at": "2026-03-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	user, err := c.GetUser("k", "u-42")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.ID != "u-42" || user.Username != "carol" {
		t.Errorf("user = %+v", user)
	}
}

func TestUpdateUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// Only is_active should be present in the partial update.
		if _, ok := body["is_active"]; !ok {
			t.Error("is_active missing from body")
		}
		if body["is_active"] != false {
			t.Errorf("is_active = %v, want false", body["is_active"])
		}
		if _, ok := body["username"]; ok {
			t.Error("unexpected username key in partial update body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "u-7", "username": "dave", "is_active": false, "is_admin": false,
			"created_at": "2026-04-01T00:00:00Z", "updated_at": "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	user, err := c.UpdateUser("k", "u-7", map[string]any{"is_active": false})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if user.IsActive {
		t.Errorf("IsActive = true, want false")
	}
}

func TestUpdateUser_ErrorSurfaced(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/self", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Cannot deactivate your own account"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	_, err := c.UpdateUser("k", "self", map[string]any{"is_active": false})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResetUserPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-3/password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["new_password"] != "newpass99" {
			t.Errorf("new_password = %v, want newpass99", body["new_password"])
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Password reset successfully. User will need to log in again.",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.ResetUserPassword("k", "u-3", "newpass99"); err != nil {
		t.Fatalf("ResetUserPassword: %v", err)
	}
}

func TestGetDeletionImpact(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-5/deletion-impact", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "u-5", "username": "eve",
			"total_games": 12, "total_tags": 5,
			"total_import_jobs": 2, "total_export_jobs": 1,
			"total_sync_jobs": 3, "total_sync_configs": 2,
			"total_sessions": 4,
			"warning":        "This action cannot be undone. All data listed above will be permanently deleted.",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	impact, err := c.GetDeletionImpact("k", "u-5")
	if err != nil {
		t.Fatalf("GetDeletionImpact: %v", err)
	}
	if impact.UserID != "u-5" || impact.Username != "eve" {
		t.Errorf("impact identity = %+v", impact)
	}
	if impact.TotalGames != 12 || impact.TotalTags != 5 {
		t.Errorf("impact counts = %+v", impact)
	}
	if impact.TotalImportJobs != 2 || impact.TotalExportJobs != 1 {
		t.Errorf("impact job counts = %+v", impact)
	}
	if impact.TotalSyncJobs != 3 || impact.TotalSyncConfigs != 2 {
		t.Errorf("impact sync counts = %+v", impact)
	}
	if impact.TotalSessions != 4 {
		t.Errorf("TotalSessions = %d, want 4", impact.TotalSessions)
	}
	if impact.Warning == "" {
		t.Error("Warning should be non-empty")
	}
}

func TestDeleteUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/users/u-9", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "User and all associated data deleted successfully",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.DeleteUser("k", "u-9"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestAdminReset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": 42})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	deleted, err := c.AdminReset("k")
	if err != nil {
		t.Fatalf("AdminReset: %v", err)
	}
	if deleted != 42 {
		t.Errorf("deleted = %d, want 42", deleted)
	}
}
